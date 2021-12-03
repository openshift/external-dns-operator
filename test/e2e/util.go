//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	miekg "github.com/miekg/dns"
	operatorv1 "github.com/openshift/api/operator/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const dialTimeout = 10 * time.Second

type providerTestHelper interface {
	ensureHostedZone(string) (string, []string, error)
	deleteHostedZone(string, string) error
	platform() string
	makeCredentialsSecret(namespace string) *corev1.Secret
	buildExternalDNS(name, zoneID, zoneDomain, sourceType, routerName string,
		credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS
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

func defaultExternalDNS(name, zoneID, zoneDomain, sourceType, routerName string) operatorv1alpha1.ExternalDNS {
	resource := operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Zones: []string{zoneID},
		},
	}

	switch sourceType {
	case "Service":
		resource.Spec.Source = getServiceTypeSource(zoneDomain)
	case "OpenShiftRoute":
		resource.Spec.Source = getOCRouteTypeSource(routerName)
	default:
		//TODO : we should return error and fail the test on Source type
		fmt.Errorf("source type not support : %s", sourceType)
	}

	return resource
}

func getServiceTypeSource(zoneDomain string) operatorv1alpha1.ExternalDNSSource {
	return operatorv1alpha1.ExternalDNSSource{
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
	}
}

func getOCRouteTypeSource(routerName string) operatorv1alpha1.ExternalDNSSource {
	return operatorv1alpha1.ExternalDNSSource{
		ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
			Type: operatorv1alpha1.SourceTypeRoute,
			OpenShiftRoute: &operatorv1alpha1.ExternalDNSOpenShiftRouteOptions{
				RouterName: routerName,
			},
		},
		HostnameAnnotationPolicy: "Allow",
	}
}
func defaultExternalDNSOpenShiftRoute(name, routerName, zoneID, zoneDomain string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
	resource := operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeRoute,
					OpenShiftRoute: &operatorv1alpha1.ExternalDNSOpenShiftRouteOptions{
						RouterName: routerName,
					},
				},
				HostnameAnnotationPolicy: "Allow",
			},
		},
	}

	resource.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
		AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: credsSecret.Name,
			},
		},
	}
	return resource
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

func customResolver(nameserver string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: dialTimeout,
			}
			if len(nameserver) > 0 {
				return d.DialContext(ctx, network, fmt.Sprintf("%s:53", nameserver))
			}
			return d.DialContext(ctx, network, address)
		},
	}
}

func assertIngressControllerDeleted(t *testing.T, cl client.Client, ing *operatorv1.IngressController) {
	t.Helper()
	if err := deleteIngressController(t, cl, ing, 2*time.Minute); err != nil {
		t.Fatalf("WARNING: cloud resources may have been leaked! failed to delete ingresscontroller %s: %v", ing.Name, err)
	} else {
		t.Logf("deleted ingresscontroller %s", ing.Name)
	}
}

func operatorConditionMap(conditions ...operatorv1.OperatorCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[cond.Type] = string(cond.Status)
	}
	return conds
}

func waitForIngressControllerCondition(t *testing.T, cl client.Client, timeout time.Duration, name types.NamespacedName) error {
	conditions := []operatorv1.OperatorCondition{
		{Type: "Admitted", Status: operatorv1.ConditionTrue},
		{Type: operatorv1.IngressControllerAvailableConditionType, Status: operatorv1.ConditionTrue},
		{Type: operatorv1.LoadBalancerManagedIngressConditionType, Status: operatorv1.ConditionFalse},
		{Type: operatorv1.DNSManagedIngressConditionType, Status: operatorv1.ConditionFalse},
	}
	return wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		ic := &operatorv1.IngressController{}
		if err := cl.Get(context.TODO(), name, ic); err != nil {
			t.Logf("failed to get ingresscontroller %s: %v", name.Name, err)
			return false, nil
		}
		expected := operatorConditionMap(conditions...)
		current := operatorConditionMap(ic.Status.Conditions...)
		return conditionsMatchExpected(expected, current), nil
	})
}

func deleteIngressController(t *testing.T, cl client.Client, ic *operatorv1.IngressController, timeout time.Duration) error {
	t.Helper()
	name := types.NamespacedName{Namespace: ic.Namespace, Name: ic.Name}
	if err := cl.Delete(context.TODO(), ic); err != nil {
		return fmt.Errorf("failed to delete ingresscontroller: %v", err)
	}

	err := wait.PollImmediate(1*time.Second, timeout, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, ic); err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			t.Logf("failed to delete ingress controller %s/%s: %v", ic.Namespace, ic.Name, err)
			return false, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for ingresscontroller to be deleted: %v", err)
	}
	return nil
}

func newHostNetworkController(name types.NamespacedName, domain string) *operatorv1.IngressController {
	repl := int32(1)
	return &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
		},
		Spec: operatorv1.IngressControllerSpec{
			Domain:   domain,
			Replicas: &repl,
			EndpointPublishingStrategy: &operatorv1.EndpointPublishingStrategy{
				Type: operatorv1.HostNetworkStrategyType,
			},
		},
	}
}

func lookupCNAME(host, server string) ([]string, error) {
	c := miekg.Client{}
	m := miekg.Msg{}
	m.SetQuestion(host+".", miekg.TypeCNAME)
	r, _, err := c.Exchange(&m, server+":53")
	if err != nil {
		return nil, err
	}
	if len(r.Answer) == 0 {
		return nil, fmt.Errorf("No results for the host :%s in nameServer : %s ", host, server)
	}
	var cname []string
	for _, ans := range r.Answer {
		rec := ans.(*miekg.CNAME)
		cname = append(cname, rec.Target)
	}
	return cname, nil
}
