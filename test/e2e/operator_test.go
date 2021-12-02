//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/version"
)

const (
	baseZoneDomain     = "example-naga.info"
	testNamespace      = "external-dns-test"
	testServiceName    = "test-service"
	testCredSecretName = "external-dns-operator"
	testExtDNSName     = "test-extdns"
	dnsPollingInterval = 15 * time.Second
	dnsPollingTimeout  = 15 * time.Minute
	dialTimeout        = 10 * time.Second
)

var (
	kubeClient       client.Client
	scheme           *runtime.Scheme
	nameServers      []string
	hostedZoneID     string
	providerOptions  []string
	helper           providerTestHelper
	hostedZoneDomain = baseZoneDomain
)

func init() {
	scheme = kscheme.Scheme
	if err := configv1.Install(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1.Install(scheme); err != nil {
		panic(err)
	}
	if err := routev1.Install(scheme); err != nil {
		panic(err)
	}
}

func initKubeClient() error {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kube config: %w", err)
	}

	kubeClient, err = client.New(kubeConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kube client: %w", err)
	}
	return nil
}

func initProviderHelper(openshiftCI bool, platformType string) (providerTestHelper, error) {
	switch platformType {
	case string(configv1.AWSPlatformType):
		return newAWSHelper(openshiftCI, kubeClient)
	case string(configv1.AzurePlatformType):
		return newAzureHelper(kubeClient)
	case string(configv1.GCPPlatformType):
		return newGCPHelper(openshiftCI, kubeClient)
	default:
		return nil, fmt.Errorf("unsupported provider: %q", platformType)
	}
}

func TestMain(m *testing.M) {
	var (
		err          error
		platformType string
		openshiftCI  bool
	)
	if err = initKubeClient(); err != nil {
		fmt.Printf("Failed to init kube client: %v\n", err)
		os.Exit(1)
	}

	if os.Getenv("OPENSHIFT_CI") != "" {
		openshiftCI = true
		platformType, err = getPlatformType(kubeClient)
		if err != nil {
			fmt.Printf("Failed to determine platform type: %v\n", err)
			os.Exit(1)
		}
	} else {
		platformType = mustGetEnv("CLOUD_PROVIDER")
	}

	if providersToSkip := os.Getenv("E2E_SKIP_CLOUD_PROVIDERS"); len(providersToSkip) > 0 {
		for _, provider := range strings.Split(providersToSkip, ",") {
			if strings.ToLower(provider) == strings.ToLower(platformType) {
				fmt.Printf("Skipping e2e test for the provider %q!\n", provider)
				os.Exit(0)
			}
		}
	}

	if version.SHORTCOMMIT != "" {
		hostedZoneDomain = version.SHORTCOMMIT + "." + baseZoneDomain
	}

	if helper, err = initProviderHelper(openshiftCI, platformType); err != nil {
		fmt.Printf("Failed to init provider helper: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Ensuring hosted zone: %s\n", hostedZoneDomain)
	hostedZoneID, nameServers, err = helper.ensureHostedZone(hostedZoneDomain)
	if err != nil {
		fmt.Printf("Failed to created hosted zone for domain %s: %v\n", hostedZoneDomain, err)
		os.Exit(1)
	}

	exitStatus := m.Run()

	fmt.Printf("Deleting hosted zone: %s\n", hostedZoneDomain)
	err = helper.deleteHostedZone(hostedZoneID, hostedZoneDomain)
	if err != nil {
		fmt.Printf("Failed to delete hosted zone %s: %v\n", hostedZoneID, err)
		os.Exit(1)
	}
	os.Exit(exitStatus)
}

//func TestOperatorAvailable(t *testing.T) {
//	expected := []appsv1.DeploymentCondition{
//		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
//	}
//	if err := waitForOperatorDeploymentStatusCondition(t, kubeClient, expected...); err != nil {
//		t.Errorf("Did not get expected available condition: %v", err)
//	}
//}
//
//func TestExternalDNSRecordLifecycle(t *testing.T) {
//	t.Log("Ensuring test namespace")
//	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
//	if err != nil && !errors.IsAlreadyExists(err) {
//		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
//	}
//
//	t.Log("Creating credentials secret")
//	resourceSecret := helper.makeCredentialsSecret(testCredSecretName)
//	err = kubeClient.Create(context.TODO(), resourceSecret)
//	if err != nil {
//		t.Fatalf("Failed to create credentials secret %s/%s for resource: %v", resourceSecret.Namespace, resourceSecret.Name, err)
//	}
//
//	t.Log("Creating external dns instance")
//	extDNS := helper.buildExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain, resourceSecret)
//	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
//		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
//	}
//	defer kubeClient.Delete(context.TODO(), &extDNS)
//
//	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
//	t.Log("Creating source service")
//	service := defaultService(testServiceName, testNamespace)
//	if err := kubeClient.Create(context.Background(), service); err != nil {
//		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testServiceName, err)
//	}
//	defer kubeClient.Delete(context.TODO(), service)
//
//	serviceIPs := make(map[string]struct{})
//	// get the IPs of the loadbalancer which is created for the service
//	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
//		t.Log("Getting IPs of service's load balancer")
//		var service corev1.Service
//		err = kubeClient.Get(context.TODO(), types.NamespacedName{
//			Namespace: testNamespace,
//			Name:      testServiceName,
//		}, &service)
//		if err != nil {
//			return false, err
//		}
//
//		// if there is no associated loadbalancer then retry later
//		if len(service.Status.LoadBalancer.Ingress) < 1 {
//			return false, nil
//		}
//
//		// get the IPs of the loadbalancer
//		if service.Status.LoadBalancer.Ingress[0].IP != "" {
//			serviceIPs[service.Status.LoadBalancer.Ingress[0].IP] = struct{}{}
//		} else if service.Status.LoadBalancer.Ingress[0].Hostname != "" {
//			lbHostname := service.Status.LoadBalancer.Ingress[0].Hostname
//			// use built in Go resolver instead of the platform's one
//			ips, err := customResolver("").LookupIP(context.TODO(), "ip", lbHostname)
//			if err != nil {
//				t.Logf("Waiting for IP of loadbalancer %s", lbHostname)
//				// if the hostname cannot be resolved currently then retry later
//				return false, nil
//			}
//			for _, ip := range ips {
//				serviceIPs[ip.String()] = struct{}{}
//			}
//		} else {
//			t.Logf("Waiting for loadbalancer details for service %s", testServiceName)
//			return false, nil
//		}
//		t.Logf("Loadbalancer's IP(s): %v", serviceIPs)
//		return true, nil
//	}); err != nil {
//		t.Fatalf("Failed to get loadbalancer IPs for service %s/%s: %v", testNamespace, testServiceName, err)
//	}
//
//	// try all nameservers and fail only if all failed
//	for _, nameSrv := range nameServers {
//		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)
//		// create a DNS resolver which uses the nameservers of the test hosted zone
//		customResolver := customResolver(nameSrv)
//
//		// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
//		if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
//			rec := fmt.Sprintf("%s.%s", testServiceName, hostedZoneDomain)
//			ips, err := customResolver.LookupHost(context.TODO(), rec)
//			if err != nil {
//				t.Logf("Waiting for dns record: %s", rec)
//				return false, nil
//			}
//			for _, ip := range ips {
//				if _, ok := serviceIPs[ip]; !ok {
//					return false, nil
//				}
//			}
//			return true, nil
//		}); err != nil {
//			t.Logf("Failed to verify that DNS has been correctly set.")
//		} else {
//			return
//		}
//	}
//	t.Fatalf("All nameservers failed to verify that DNS has been correctly set.")
//}


func TestExternalDNSRecordLifecycleWithIngress(t *testing.T) {
	testIngressNamespace := "test-extdns-ingress"
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testIngressNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testIngressNamespace, err)
	}

	t.Log("Creating credentials secret")
	resourceSecret := helper.makeCredentialsSecret("external-dns-operator")
	err = kubeClient.Create(context.TODO(), resourceSecret)
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create credentials secret %s/%s for resource: %v", resourceSecret.Namespace, resourceSecret.Name, err)
	}

	t.Log("Create ingress controller ")

	name := types.NamespacedName{Namespace: testIngressNamespace, Name: "external-dns"}
	ing := newHostNetworkController(name, name.Name+"."+baseZoneDomain)
	if err := kubeClient.Create(context.TODO(), ing); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create ingresscontroller: %v", err)
	}
	defer assertIngressControllerDeleted(t, kubeClient, ing)

	conditions := []operatorv1.OperatorCondition{
		{Type: "Admitted", Status: operatorv1.ConditionTrue},
		{Type: operatorv1.IngressControllerAvailableConditionType, Status: operatorv1.ConditionTrue},
		{Type: operatorv1.LoadBalancerManagedIngressConditionType, Status: operatorv1.ConditionFalse},
		{Type: operatorv1.DNSManagedIngressConditionType, Status: operatorv1.ConditionFalse},
	}
	err = waitForIngressControllerCondition(t, kubeClient, 5*time.Minute, name, conditions...)
	if err != nil {
		t.Errorf("failed to observe expected conditions: %v", err)
	}

	t.Log("Creating external dns instance")
	extDNS := defaultExternalDNSOpenShiftRoute(testExtDNSName, "external-dns", hostedZoneID, hostedZoneDomain, resourceSecret)
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer kubeClient.Delete(context.TODO(), &extDNS)

	// Create conflicting routes in the namespaces
	makeRoute := func(name types.NamespacedName, host, path string) *routev1.Route {
		return &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: name.Namespace,
				Name:      name.Name,
				Annotations: map[string]string{
					"external-dns.mydomain.org/publish": "yes",
				},
				Labels: map[string]string{
					"external-dns": "",
				},
			},
			Spec: routev1.RouteSpec{
				Host: host,
				Path: path,
				To: routev1.RouteTargetReference{
					Kind: "Service",
					Name: testServiceName,  // create service foo and assign the name
				},
			},
		}
	}
	// use unique names for each test route to simplify debugging.
	route1Name := types.NamespacedName{Namespace: testIngressNamespace, Name: "external-dns-route"}
	route1 := makeRoute(route1Name, "app."+baseZoneDomain, "/foo")


	// The first route should be admitted
	if err := kubeClient.Create(context.TODO(), route1); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create route: %v", err)
	}
	defer kubeClient.Delete(context.TODO(), route1)

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)
		// create a DNS resolver which uses the nameservers of the test hosted zone
		address, err :=net.LookupHost(nameSrv)
		if err != nil{
			continue
		}
		t.Logf("NameServer %s: %s ", nameSrv , address)
		customResolver := customResolver(nameSrv)

		// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
		if err := wait.PollImmediate(dnsPollingInterval, 15, func() (done bool, err error) {
			rec := fmt.Sprintf("app.%s",baseZoneDomain)
			hostname, err := customResolver.LookupCNAME(context.TODO(), rec)
			if err != nil {
				t.Logf("Waiting for dns record: %s, err %v", rec, err)
				return false, nil
			}
			if hostname == "router-external-dns.external-dns.example-andrey.info"{
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Logf("Failed to verify that DNS has been correctly set.")
		} else {
			return
		}
	}
	//t.Fatalf("All nameservers failed to verify that DNS has been correctly set.")
	t.Fail()

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

func waitForIngressControllerCondition(t *testing.T, cl client.Client, timeout time.Duration, name types.NamespacedName, conditions ...operatorv1.OperatorCondition) error {
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