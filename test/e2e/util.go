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

type providerTestHelper interface {
	ensureHostedZone(string) (string, []string, error)
	deleteHostedZone(string, string) error
	platform() string
	makeCredentialsSecret(namespace string) *corev1.Secret
	buildExternalDNS(name, zoneID, zoneDomain string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS
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
