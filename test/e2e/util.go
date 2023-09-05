//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"testing"
	"time"

	ibclient "github.com/infobloxopen/infoblox-go-client"
	"github.com/miekg/dns"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	"github.com/openshift/external-dns-operator/pkg/utils"
)

const (
	infobloxDNSProvider = "INFOBLOX"
)

type providerTestHelper interface {
	ensureHostedZone(string) (string, []string, error)
	deleteHostedZone(string, string) error
	platform() string
	makeCredentialsSecret(namespace string) *corev1.Secret
	buildExternalDNS(name, zoneID, zoneDomain string, credsSecret *corev1.Secret) operatorv1beta1.ExternalDNS
	buildOpenShiftExternalDNS(name, zoneID, zoneDomain, routeName string, credsSecret *corev1.Secret) operatorv1beta1.ExternalDNS
	buildOpenShiftExternalDNSV1Alpha1(name, zoneID, zoneDomain, routeName string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS
}

func randomString(n int) string {
	var chars = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	str := make([]rune, n)
	for i := range str {
		str[i] = chars[rand.Intn(len(chars))]
	}
	return string(str)
}

func testRoute(name, namespace, host, svcName string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				"external-dns.mydomain.org/publish": "yes",
			},
		},
		Spec: routev1.RouteSpec{
			Host: host,
			To: routev1.RouteTargetReference{
				Name: svcName,
			},
		},
	}
}

func deploymentConditionMap(conditions ...appsv1.DeploymentCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[string(cond.Type)] = string(cond.Status)
	}
	return conds
}

func waitForOperatorDeploymentStatusCondition(ctx context.Context, t *testing.T, cl client.Client, conditions ...appsv1.DeploymentCondition) error {
	t.Helper()
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 1*time.Minute, false, func(ctx context.Context) (bool, error) {
		dep := &appsv1.Deployment{}
		depNamespacedName := types.NamespacedName{
			Name:      "external-dns-operator",
			Namespace: "external-dns-operator",
		}
		if err := cl.Get(ctx, depNamespacedName, dep); err != nil {
			t.Logf("failed to get deployment %s: %v", depNamespacedName.Name, err)
			return false, nil
		}

		expected := deploymentConditionMap(conditions...)
		current := deploymentConditionMap(dep.Status.Conditions...)
		return conditionsMatchExpected(expected, current), nil
	})
}

func conditionsMatchExpected(expected, actual map[string]string) bool {
	filtered := map[string]string{}
	for k := range actual {
		if _, comparable := expected[k]; comparable {
			filtered[k] = actual[k]
		}
	}
	return reflect.DeepEqual(expected, filtered)
}

func defaultExternalDNS(name, zoneID, zoneDomain string) operatorv1beta1.ExternalDNS {
	return operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1beta1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
					Type: operatorv1beta1.SourceTypeService,
					Service: &operatorv1beta1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeLoadBalancer,
							corev1.ServiceTypeClusterIP,
						},
					},
					LabelFilter: utils.MustParseLabelSelector("external-dns.mydomain.org/publish=yes"),
				},
				HostnameAnnotationPolicy: "Ignore",
				FQDNTemplate:             []string{fmt.Sprintf("{{.Name}}.%s", zoneDomain)},
			},
		},
	}
}

func routeExternalDNS(name, zoneID, zoneDomain, routerName string) operatorv1beta1.ExternalDNS {
	extDns := operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1beta1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
					Type:        operatorv1beta1.SourceTypeRoute,
					LabelFilter: utils.MustParseLabelSelector("external-dns.mydomain.org/publish=yes"),
				},
				HostnameAnnotationPolicy: operatorv1beta1.HostnameAnnotationPolicyIgnore,
			},
		},
	}
	// this additional check can be removed with latest external-dns image (>v0.10.1)
	// instantiate the route additional information at ExternalDNS initiation level.
	if routerName != "" {
		extDns.Spec.Source.ExternalDNSSourceUnion.OpenShiftRoute = &operatorv1beta1.ExternalDNSOpenShiftRouteOptions{
			RouterName: routerName,
		}
	}
	return extDns
}

func routeExternalDNSV1Alpha1(name, zoneID, zoneDomain, routerName string) operatorv1alpha1.ExternalDNS {
	extDns := operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type:        operatorv1alpha1.SourceTypeRoute,
					LabelFilter: utils.MustParseLabelSelector("external-dns.mydomain.org/publish=yes"),
				},
				HostnameAnnotationPolicy: operatorv1alpha1.HostnameAnnotationPolicyIgnore,
			},
		},
	}
	// this additional check can be removed with latest external-dns image (>v0.10.1)
	// instantiate the route additional information at ExternalDNS initiation level.
	if routerName != "" {
		extDns.Spec.Source.ExternalDNSSourceUnion.OpenShiftRoute = &operatorv1alpha1.ExternalDNSOpenShiftRouteOptions{
			RouterName: routerName,
		}
	}
	return extDns
}

// lookupCNAME retrieves the first canonical name of the given host.
// This function is different from net.LookupCNAME.
// net.LookupCNAME assumes the nameserver used is the recursive resolver (https://github.com/golang/go/blob/master/src/net/dnsclient_unix.go#L637).
// Therefore CNAME is tried to be resolved to its last canonical name, the quote from doc:
// "A canonical name is the final name after following zero or more CNAME records."
// This may be a problem if the default nameserver (from host /etc/resolv.conf, default lookup order is files,dns)
// is replaced (custom net.Resolver with overridden Dial function) with not recursive resolver
// and the other CNAMEs down to the last one are not known to this replaced nameserver.
// This may result in "no such host" error.
func lookupCNAME(host, server string) (string, error) {
	dnsClient := dns.Client{}
	message := &dns.Msg{}
	message.SetQuestion(dns.Fqdn(host), dns.TypeCNAME)
	r, _, err := dnsClient.Exchange(message, fmt.Sprintf("%s:53", server))
	if err != nil {
		return "", err
	}
	if len(r.Answer) == 0 {
		return "", fmt.Errorf("not found")
	}
	cname, ok := r.Answer[0].(*dns.CNAME)
	if !ok {
		return "", fmt.Errorf("not a CNAME record")
	}
	return cname.Target, nil
}

func equalFQDN(name1, name2 string) bool {
	return dns.Fqdn(name1) == dns.Fqdn(name2)
}

func newHostNetworkController(name types.NamespacedName, domain string) *operatorv1.IngressController {
	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: operatorv1.IngressControllerSpec{
			Domain:   domain,
			Replicas: pointer.Int32(1),
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.HostNetworkStrategyType,
			},
		},
	}
}

// enhancedIBClient provides enhancements not implemented in Infoblox golang client.
// https://pkg.go.dev/github.com/infobloxopen/infoblox-go-client
type enhancedIBClient struct {
	*ibclient.Connector
	httpClient *http.Client
}

func newEnhancedIBClient(hostConfig ibclient.HostConfig, transportConfig ibclient.TransportConfig) (*enhancedIBClient, error) {
	ibcli, err := ibclient.NewConnector(hostConfig, transportConfig, &ibclient.WapiRequestBuilder{}, &ibclient.WapiHttpRequestor{})
	if err != nil {
		return nil, err
	}

	return &enhancedIBClient{
		Connector: ibcli,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: !transportConfig.SslVerify,
				},
			},
		},
	}, nil
}

// addNameServer uses NIOS REST API to add the Grid host as the nameserver for the given DNS zone.
// Infoblox golang client has the interface for creating DNS zones.
// However these zones don't have NS/SOA records added by default.
func (c *enhancedIBClient) addNameServer(zoneRef, nameserver string) error {
	payload := fmt.Sprintf(`{"grid_primary":[{"name":"%s"}]}`, nameserver)
	qparams := map[string]string{
		"_return_fields+":   "fqdn,grid_primary",
		"_return_as_object": "1",
	}
	_, err := c.doHTTPRequest(context.TODO(), "PUT", "https://"+c.HostConfig.Host+"/wapi/v"+c.HostConfig.Version+"/"+zoneRef, qparams, []byte(payload))
	return err
}

// restartServices uses NIOS REST API to restart all Grid services.
// Some configurations don't take effect until the corresponding service is not restarted,
// see the doc: https://docs.infoblox.com/display/nios85/Configurations+Requiring+Service+Restart
func (c *enhancedIBClient) restartServices() error {
	respJSON, err := c.doHTTPRequest(context.TODO(), "GET", "https://"+c.HostConfig.Host+"/wapi/v"+c.HostConfig.Version+"/grid", nil, nil)
	if err != nil {
		return err
	}
	type Ref struct {
		Ref string `json:"_ref"`
	}
	resp := &[]Ref{}
	if err = json.Unmarshal(respJSON, resp); err != nil {
		return err
	}

	payload := `{"member_order" : "SIMULTANEOUSLY","service_option": "ALL"}`
	qparams := map[string]string{
		"_function": "restartservices",
	}
	_, err = c.doHTTPRequest(context.TODO(), "POST", "https://"+c.HostConfig.Host+"/wapi/v"+c.HostConfig.Version+"/"+(*resp)[0].Ref, qparams, []byte(payload))
	return err
}

func (c *enhancedIBClient) doHTTPRequest(ctx context.Context, method, url string, queryParams map[string]string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %s", err)
	}

	// set query parameters
	query := req.URL.Query()
	for k, v := range queryParams {
		query.Add(k, v)
	}
	req.URL.RawQuery = query.Encode()

	// set headers
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(c.HostConfig.Username, c.HostConfig.Password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %s", err)
	}
	defer resp.Body.Close()

	// 2xx
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("failure http status code received: %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read http response body: %s", err)
	}

	return respBody, nil
}

// readServerTLSCert returns PEM encoded TLS certificate of the given server.
func readServerTLSCert(addr string, selfSigned bool) ([]byte, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: selfSigned,
	}

	conn, err := tls.Dial("tcp", addr, tlsConf)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	derCertsRaw := []byte{}
	certs := conn.ConnectionState().PeerCertificates
	for _, cert := range certs {
		derCertsRaw = append(derCertsRaw, cert.Raw...)
	}

	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derCertsRaw,
	}
	return pem.EncodeToMemory(block), nil
}

// ensureEnvVar ensures the environment variable is present in the given list.
func ensureEnvVar(vars []corev1.EnvVar, v corev1.EnvVar) []corev1.EnvVar {
	if vars == nil {
		return []corev1.EnvVar{v}
	}
	for i := range vars {
		if vars[i].Name == v.Name {
			vars[i].Value = v.Value
			return vars
		}
	}
	return append(vars, v)
}
