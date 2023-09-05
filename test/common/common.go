//go:build e2e
// +build e2e

package common

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"k8s.io/client-go/kubernetes"

	"github.com/miekg/dns"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/utils/pointer"
)

const (
	googleDNSServer            = "8.8.8.8"
	e2eSeparateOperandNsEnvVar = "E2E_SEPARATE_OPERAND_NAMESPACE"
	operandNamespace           = "external-dns"
	rbacRsrcName               = "external-dns-operator"
	operatorServiceAccount     = "external-dns-operator"
	OperatorNamespace          = "external-dns-operator"
	dnsProviderEnvVar          = "DNS_PROVIDER"
	e2eSkipDNSProvidersEnvVar  = "E2E_SKIP_DNS_PROVIDERS"
	DnsPollingInterval         = 15 * time.Second
	DnsPollingTimeout          = 3 * time.Minute
)

var (
	KubeClient    client.Client
	KubeClientSet *kubernetes.Clientset
	scheme        *runtime.Scheme
)

func init() {
	initScheme()

	initKubeClient()
}

func DefaultService(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
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

func MustGetEnv(name string) string {
	val := os.Getenv(name)
	if val == "" {
		panic(fmt.Sprintf("environment variable %s must be set", name))
	}
	return val
}

func RootCredentials(name string) (map[string][]byte, error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      name,
		Namespace: "kube-system",
	}
	if err := KubeClient.Get(context.TODO(), secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return secret.Data, nil
}

func LookupARecord(host, server string) ([]string, error) {
	dnsClient := &dns.Client{}
	message := &dns.Msg{}
	message.SetQuestion(dns.Fqdn(host), dns.TypeA)
	response, _, err := dnsClient.Exchange(message, fmt.Sprintf("%s:53", server))
	if err != nil {
		return nil, err
	}
	if len(response.Answer) == 0 {
		return nil, fmt.Errorf("not found")
	}
	var ips []string
	for _, ans := range response.Answer {
		if aRec, ok := ans.(*dns.A); ok {
			ips = append(ips, aRec.A.String())
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("not found")
	}
	return ips, nil
}

// LookupARecordInternal queries for a DNS hostname inside the VPC of a cluster using a dig pod. It returns a
// map structure of the resolved IPs for easy lookup.
func LookupARecordInternal(ctx context.Context, t *testing.T, namespace, host string) (map[string]struct{}, error) {
	t.Helper()

	// Create dig pod for querying DNS inside the cluster with random name
	// to prevent naming collisions in case one is still being deleted.
	digPodName := names.SimpleNameGenerator.GenerateName("digpod-")
	clientPod := buildDigPod(digPodName, namespace, host)
	if err := KubeClient.Create(ctx, clientPod); err != nil {
		return nil, fmt.Errorf("failed to create pod %s/%s: %v", clientPod.Namespace, clientPod.Name, err)
	}
	defer func() {
		_ = KubeClient.Delete(ctx, clientPod)
	}()

	// Loop until dig pod starts, then parse logs for query results.
	var responseCode string
	var gotIPs map[string]struct{}
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		if err := KubeClient.Get(ctx, types.NamespacedName{Name: clientPod.Name, Namespace: clientPod.Namespace}, clientPod); err != nil {
			t.Logf("Failed to get pod %s/%s: %v", clientPod.Namespace, clientPod.Name, err)
			return false, nil
		}
		switch clientPod.Status.Phase {
		case corev1.PodRunning:
			t.Log("Waiting for dig pod to finish")
			return false, nil
		case corev1.PodPending:
			t.Log("Waiting for dig pod to start")
			return false, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			// Failed or Succeeded, let's continue on to check the logs.
			break
		default:
			return true, fmt.Errorf("unhandled pod status type")
		}

		// Get logs of the dig pod.
		readCloser, err := KubeClientSet.CoreV1().Pods(clientPod.Namespace).GetLogs(clientPod.Name, &corev1.PodLogOptions{
			Container: clientPod.Spec.Containers[0].Name,
			Follow:    false,
		}).Stream(ctx)
		if err != nil {
			t.Logf("Failed to read output from pod %s: %v (retrying)", clientPod.Name, err)
			return false, nil
		}
		scanner := bufio.NewScanner(readCloser)
		defer func() {
			if err := readCloser.Close(); err != nil {
				t.Fatalf("Failed to close reader for pod %s: %v", clientPod.Name, err)
			}
		}()

		gotIPs = make(map[string]struct{})
		for scanner.Scan() {
			line := scanner.Text()

			// Skip blank lines.
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Parse status out (helpful for future debugging)
			if strings.HasPrefix(line, ";;") && strings.Contains(line, "status:") {
				responseCodeSection := strings.TrimSpace(strings.Split(line, ",")[1])
				responseCode = strings.Split(responseCodeSection, " ")[1]
				t.Logf("DNS Response Code: %s", responseCode)
			}
			// If it doesn't begin with ";", then we have an answer.
			if !strings.HasPrefix(line, ";") {
				splitAnswer := strings.Fields(line)
				if len(splitAnswer) < 5 {
					t.Logf("Expected dig answer to have 5 fields: %q", line)
					return true, nil
				}
				gotIP := strings.Fields(line)[4]
				gotIPs[gotIP] = struct{}{}
			}
		}
		t.Logf("Got IPs: %v", gotIPs)
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to observe the expected dig results: %v", err)
	}

	return gotIPs, nil
}

// buildDigPod returns a pod definition for a pod with the given name and image
// and in the given namespace that digs the specified address.
func buildDigPod(name, namespace, address string, extraArgs ...string) *corev1.Pod {
	digArgs := []string{
		address,
		"A",
		"+noall",
		"+answer",
		"+comments",
	}
	digArgs = append(digArgs, extraArgs...)
	digArgs = append(digArgs, address)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "dig",
					Image:   "image-registry.openshift-image-registry.svc:5000/openshift/tools:latest",
					Command: []string{"/bin/dig"},
					Args:    digArgs,
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						Privileged:               pointer.Bool(false),
						RunAsNonRoot:             pointer.Bool(true),
						AllowPrivilegeEscalation: pointer.Bool(false),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

// GetServiceIPs retrieves the provided service's IP or hostname and resolves the hostname to IPs (if applicable).
// Returns values are the serviceAddress (LoadBalancer IP or Hostname), the service resolved IPs, and an error
// (if applicable).
func GetServiceIPs(ctx context.Context, t *testing.T, timeout time.Duration, svcName types.NamespacedName) (string, map[string]struct{}, error) {
	t.Helper()

	// Get the IPs of the loadbalancer which is created for the service
	var serviceAddress string
	serviceResolvedIPs := make(map[string]struct{})
	if err := wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (done bool, err error) {
		t.Log("Getting IPs of service's load balancer")
		var service corev1.Service
		err = KubeClient.Get(ctx, svcName, &service)
		if err != nil {
			return false, err
		}

		// If there is no associated loadbalancer then retry later
		if len(service.Status.LoadBalancer.Ingress) < 1 {
			return false, nil
		}

		// Get the IPs of the loadbalancer
		if service.Status.LoadBalancer.Ingress[0].IP != "" {
			serviceAddress = service.Status.LoadBalancer.Ingress[0].IP
			serviceResolvedIPs[service.Status.LoadBalancer.Ingress[0].IP] = struct{}{}
		} else if service.Status.LoadBalancer.Ingress[0].Hostname != "" {
			lbHostname := service.Status.LoadBalancer.Ingress[0].Hostname
			serviceAddress = lbHostname
			ips, err := LookupARecord(lbHostname, googleDNSServer)
			if err != nil {
				t.Logf("Waiting for IP of loadbalancer %s", lbHostname)
				// If the hostname cannot be resolved currently then retry later
				return false, nil
			}
			for _, ip := range ips {
				serviceResolvedIPs[ip] = struct{}{}
			}
		} else {
			t.Logf("Waiting for loadbalancer details for service %s", svcName.Name)
			return false, nil
		}
		t.Logf("Loadbalancer's IP(s): %v", serviceResolvedIPs)
		return true, nil
	}); err != nil {
		return "", nil, fmt.Errorf("failed to get loadbalancer IPs for service %s/%s: %v", svcName.Name, svcName.Namespace, err)
	}

	return serviceAddress, serviceResolvedIPs, nil
}

func EnsureOperandResources(ctx context.Context) error {
	if os.Getenv(e2eSeparateOperandNsEnvVar) != "true" {
		return nil
	}

	if err := ensureOperandNamespace(ctx); err != nil {
		return fmt.Errorf("failed to create %s namespace: %v", operandNamespace, err)
	}

	if err := ensureOperandRole(ctx); err != nil {
		return fmt.Errorf("failed to create role external-dns-operator in ns %s: %v", operandNamespace, err)
	}

	if err := ensureOperandRoleBinding(ctx); err != nil {
		return fmt.Errorf("failed to create rolebinding external-dns-operator in ns %s: %v", operandNamespace, err)
	}

	return nil
}

func ensureOperandNamespace(ctx context.Context) error {
	return KubeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: operandNamespace}})
}

func ensureOperandRole(ctx context.Context) error {
	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "serviceaccounts", "configmaps"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "delete"},
		},
	}

	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacRsrcName,
			Namespace: operandNamespace,
		},
		Rules: rules,
	}
	return KubeClient.Create(ctx, &role)
}

func ensureOperandRoleBinding(ctx context.Context) error {
	rb := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacRsrcName,
			Namespace: operandNamespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     rbacRsrcName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      operatorServiceAccount,
				Namespace: OperatorNamespace,
			},
		},
	}
	return KubeClient.Create(ctx, &rb)
}

func initKubeClient() {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		panic(err)
	}

	KubeClient, err = client.New(kubeConfig, client.Options{})
	if err != nil {
		panic(err)
	}

	KubeClientSet, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		panic(err)
	}
}

func IsOpenShift() bool {
	return os.Getenv("OPENSHIFT_CI") != ""
}

func GetPlatformType(isOpenShift bool) (string, error) {
	var platformType string
	if isOpenShift {
		if dnsProvider := os.Getenv(dnsProviderEnvVar); dnsProvider != "" {
			platformType = dnsProvider
		} else {
			var infraConfig configv1.Infrastructure
			err := KubeClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, &infraConfig)
			if err != nil {
				return "", err
			}
			return string(infraConfig.Status.PlatformStatus.Type), nil
		}
	} else {
		platformType = MustGetEnv(dnsProviderEnvVar)
	}
	return platformType, nil
}

func SkipProvider(platformType string) bool {
	if providersToSkip := os.Getenv(e2eSkipDNSProvidersEnvVar); len(providersToSkip) > 0 {
		for _, provider := range strings.Split(providersToSkip, ",") {
			if strings.EqualFold(provider, platformType) {
				return true
			}
		}
	}
	return false
}

func initScheme() {
	scheme = kscheme.Scheme
	if err := configv1.Install(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1beta1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1.Install(scheme); err != nil {
		panic(err)
	}
	if err := routev1.Install(scheme); err != nil {
		panic(err)
	}
	if err := olmv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}
