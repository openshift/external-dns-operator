//+build e2e

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

type providerTestHelper interface {
	ensureHostedZone(string) (string, []string, error)
	deleteHostedZone(string) error
	platform() string
	makeCredentialsSecret(namespace string) *corev1.Secret
}

func rootAWSCredentials(kubeClient client.Client) (string, string, error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      "aws-creds",
		Namespace: "kube-system",
	}
	if err := kubeClient.Get(context.TODO(), secretName, secret); err != nil {
		return "", "", fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return string(secret.Data["aws_access_key_id"]), string(secret.Data["aws_secret_access_key"]), nil
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

func defaultExternalDNS(t *testing.T, name string, namespace string, zoneID string, rootDomain string, credsSecret *corev1.Secret, platformType string) operatorv1alpha1.ExternalDNS {
	resource := operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
				FQDNTemplate:             []string{fmt.Sprintf("{{.Name}}.%s", rootDomain)},
			},
		},
	}

	var provider operatorv1alpha1.ExternalDNSProvider
	switch platformType {
	case string(configv1.AWSPlatformType):
		provider = operatorv1alpha1.ExternalDNSProvider{
			Type: operatorv1alpha1.ProviderTypeAWS,
			AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
				Credentials: operatorv1alpha1.SecretReference{
					Name: credsSecret.Name,
				},
			},
		}
	default:
		t.Fatalf("Unsupported Provider")
	}

	resource.Spec.Provider = provider
	return resource
}

func defaultService(name, namespace string) *corev1.Service {
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
			Type: corev1.ServiceTypeLoadBalancer,
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
