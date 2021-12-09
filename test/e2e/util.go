// +build e2e

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/miekg/dns"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"

	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	routev1 "github.com/openshift/api/route/v1"
)

type providerTestHelper interface {
	ensureHostedZone(string) (string, []string, error)
	deleteHostedZone(string, string) error
	platform() string
	makeCredentialsSecret(namespace string) *corev1.Secret
	buildExternalDNS(name, zoneID, zoneDomain string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS
	buildOpenShiftExternalDNS(name, zoneID, zoneDomain, routeName string) operatorv1alpha1.ExternalDNS
}

func randomString(n int) string {
	var chars = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	str := make([]rune, n)
	for i := range str {
		str[i] = chars[rand.Intn(len(chars))]
	}
	return string(str)
}

func getPlatformType(kubeClient client.Client) (string, error) {
	var infraConfig configv1.Infrastructure
	err := kubeClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, &infraConfig)
	if err != nil {
		return "", err
	}
	return string(infraConfig.Status.PlatformStatus.Type), nil
}

func defaultService(name, namespace string) *corev1.Service {
	return testService(name, namespace, corev1.ServiceTypeLoadBalancer)
}

func clusterIPService(name, namespace string) *corev1.Service {
	return testService(name, namespace, corev1.ServiceTypeClusterIP)
}

func testRoute(name, namespace, host, svcName string) *routev1.Route {
	return &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Annotations: map[string]string{
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

func testService(name, namespace string, svcType corev1.ServiceType) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"external-dns.mydomain.org/publish": "yes",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": "hello-openshift",
			},
			Type: svcType,
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 8080,
					},
				},
			},
		},
	}
}

func mustGetEnv(name string) string {
	val := os.Getenv(name)
	if val == "" {
		panic(fmt.Sprintf("environment variable %s must be set", name))
	}
	return val
}

func deploymentConditionMap(conditions ...appsv1.DeploymentCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[string(cond.Type)] = string(cond.Status)
	}
	return conds
}

func waitForOperatorDeploymentStatusCondition(t *testing.T, cl client.Client, conditions ...appsv1.DeploymentCondition) error {
	t.Helper()
	return wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
		dep := &appsv1.Deployment{}
		depNamespacedName := types.NamespacedName{
			Name:      "external-dns-operator",
			Namespace: "external-dns-operator",
		}
		if err := cl.Get(context.TODO(), depNamespacedName, dep); err != nil {
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

func defaultExternalDNS(name, zoneID, zoneDomain string) operatorv1alpha1.ExternalDNS {
	return operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeService,
					Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeLoadBalancer,
							corev1.ServiceTypeClusterIP,
						},
					},
					AnnotationFilter: map[string]string{
						"external-dns.mydomain.org/publish": "yes",
					},
				},
				HostnameAnnotationPolicy: "Ignore",
				FQDNTemplate:             []string{fmt.Sprintf("{{.Name}}.%s", zoneDomain)},
			},
		},
	}
}

func routeExternalDNS(name, zoneID, zoneDomain, routerName string) operatorv1alpha1.ExternalDNS {
	return operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeRoute,
					AnnotationFilter: map[string]string{
						"external-dns.mydomain.org/publish": "yes",
					},
					OpenShiftRoute: &operatorv1alpha1.ExternalDNSOpenShiftRouteOptions{
						RouterName: routerName,
					},
				},
				HostnameAnnotationPolicy: "Ignore",
				FQDNTemplate:             []string{fmt.Sprintf("{{.Name}}.%s", zoneDomain)},
			},
		},
	}
}

func rootCredentials(kubeClient client.Client, name string) (map[string][]byte, error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      name,
		Namespace: "kube-system",
	}
	if err := kubeClient.Get(context.TODO(), secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return secret.Data, nil
}

// lookupCNAMEMiekg retrieves the first canonical name of the given host.
// This function is different from net.LookupCNAME.
// net.LookupCNAME assumes the nameserver used is the recursive resolver (https://github.com/golang/go/blob/master/src/net/dnsclient_unix.go#L637).
// Therefore CNAME is tried to be resolved to its last canonical name, the quote from doc:
// "A canonical name is the final name after following zero or more CNAME records."
// This may be a problem if the default nameserver (from host /etc/resolv.conf, default lookup order is files,dns)
// is replaced (custom net.Resolver with overridden Dial function) with not recursive resolver
// and the other CNAMEs down to the last one are not known to this replaced nameserver.
// This may result in "no such host" error.
func lookupCNAME(host, server string) (string, error) {
	c := dns.Client{}
	m := dns.Msg{}
	if host[len(host)-1] != '.' {
		host += "."
	}
	m.SetQuestion(host, dns.TypeCNAME)
	r, _, err := c.Exchange(&m, server+":53")
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
