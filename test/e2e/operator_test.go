// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"

	corev1 "k8s.io/api/core/v1"
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
	baseZoneDomain     = "example-test.info"
	testNamespace      = "external-dns-test"
	testServiceName    = "test-service"
	testCredSecretName = "external-dns-operator"
	testExtDNSName     = "test-extdns"
	dnsPollingInterval = 15 * time.Second
	dnsPollingTimeout  = 15 * time.Minute
)

var (
	kubeClient       client.Client
	scheme           *runtime.Scheme
	nameServers      []string
	hostedZoneID     string
	providerOptions  []string
	helper           providerTestHelper
	resourceSecret   *corev1.Secret
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

	fmt.Printf("create the provider credentails ready for external-dns-operator")
	resourceSecret = helper.makeCredentialsSecret("external-dns-operator")
	err = kubeClient.Create(context.TODO(), resourceSecret)
	if err != nil && !errors.IsAlreadyExists(err) {
		fmt.Printf("Failed to create credentials secret %s/%s for resource: %v", resourceSecret.Namespace, resourceSecret.Name, err)
		os.Exit(1)
	}
	defer func() {
		if err := kubeClient.Delete(context.TODO(), resourceSecret); err != nil {
			fmt.Printf("failed while deleting the secret :%s", resourceSecret.Name)
		}
	}()

	exitStatus := m.Run()

	fmt.Printf("Deleting hosted zone: %s\n", hostedZoneDomain)
	err = helper.deleteHostedZone(hostedZoneID, hostedZoneDomain)
	if err != nil {
		fmt.Printf("Failed to delete hosted zone %s: %v\n", hostedZoneID, err)
		os.Exit(1)
	}
	os.Exit(exitStatus)
}

func TestOperatorAvailable(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	if err := waitForOperatorDeploymentStatusCondition(t, kubeClient, expected...); err != nil {
		t.Errorf("Did not get expected available condition: %v", err)
	}
}

func TestExternalDNSRecordLifecycleWithSourceAs_Service(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	externalDnsServiceName := fmt.Sprintf("%s-source-as-service", testExtDNSName)
	t.Logf("Creating external dns instance :%s", externalDnsServiceName)
	extDNS := helper.buildExternalDNS(externalDnsServiceName, hostedZoneID, hostedZoneDomain,
		"Service", "", resourceSecret)
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer kubeClient.Delete(context.TODO(), &extDNS)

	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source service")
	service := defaultService(testServiceName, testNamespace)
	if err := kubeClient.Create(context.Background(), service); err != nil {
		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testServiceName, err)
	}

	defer kubeClient.Delete(context.TODO(), service)
	verifySourceServiceDNSRecords(t, testNamespace, testServiceName)
}

func TestExternalDNSRecordLifecycleWithSourceAs_OpenShiftRoute(t *testing.T) {
	testIngressNamespace := "test-extdns-openshift-route"
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testIngressNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Logf("Failed to ensure namespace %s: %v", testIngressNamespace, err)
		t.Fail()
		return
	}

	name := types.NamespacedName{Namespace: testIngressNamespace, Name: "external-dns"}
	t.Logf("Create custome ingress controller %s/%s", name.Namespace, name.Name)
	ing := newHostNetworkController(name, name.Name+"."+hostedZoneDomain)
	if err = kubeClient.Create(context.TODO(), ing); err != nil {
		t.Logf("failed to create ingresscontroller: %v", err)
		t.Fail()
		return
	}
	defer assertIngressControllerDeleted(t, kubeClient, ing)

	if err = waitForIngressControllerCondition(t, kubeClient, 5*time.Minute, name); err != nil {
		t.Errorf("failed to observe expected conditions: %v", err)
		t.Fail()
		return
	}

	externalDnsServiceName := fmt.Sprintf("%s-source-as-openshift-route", testExtDNSName)
	t.Logf("Creating external dns instance : %s", externalDnsServiceName)
	extDNS := defaultExternalDNSOpenShiftRoute(externalDnsServiceName, "external-dns", hostedZoneID, hostedZoneDomain, resourceSecret)
	if err = kubeClient.Create(context.TODO(), &extDNS); err != nil && !errors.IsAlreadyExists(err) {
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
					Name: "testServiceName",
				},
			},
		}
	}
	// use unique names for each test route to simplify debugging.
	route1Name := types.NamespacedName{Namespace: testIngressNamespace, Name: "external-dns-route"}
	route1 := makeRoute(route1Name, "app."+hostedZoneDomain, "/apis")

	// The first route should be admitted
	if err = kubeClient.Create(context.TODO(), route1); err != nil && !errors.IsAlreadyExists(err) {
		t.Logf("failed to create route: %v", err)
		t.Fail()
		return
	}
	defer kubeClient.Delete(context.TODO(), route1)
	canonicalName := ""
	t.Logf("Getting canonicalName for the route :%s ", route1Name.Name)
	if canonicalName, err = fetchRouterCanonicalHostname(route1Name); err != nil {
		t.Logf("Failed to get RouterCanonicalHostname for route %s/%s: %v", route1Name.Namespace, route1Name.Name, err)
		t.Fail()
		return
	}
	t.Logf("canonicalName  : %s for the route :%s ", route1Name.Name, canonicalName)
	rec := fmt.Sprintf("app.%s", hostedZoneDomain)
	verifyOpenShiftRouteSource(t, canonicalName, rec)
}

func verifySourceServiceDNSRecords(t *testing.T, testNamespace, testServiceName string) {
	serviceIPs := make(map[string]struct{})
	// get the IPs of the loadbalancer which is created for the service
	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
		t.Log("Getting IPs of service's load balancer")
		var service corev1.Service
		err = kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: testNamespace,
			Name:      testServiceName,
		}, &service)
		if err != nil {
			return false, err
		}

		// if there is no associated loadbalancer then retry later
		if len(service.Status.LoadBalancer.Ingress) < 1 {
			return false, nil
		}

		// get the IPs of the loadbalancer
		if service.Status.LoadBalancer.Ingress[0].IP != "" {
			serviceIPs[service.Status.LoadBalancer.Ingress[0].IP] = struct{}{}
		} else if service.Status.LoadBalancer.Ingress[0].Hostname != "" {
			lbHostname := service.Status.LoadBalancer.Ingress[0].Hostname
			// use built in Go resolver instead of the platform's one
			ips, err := customResolver("").LookupIP(context.TODO(), "ip", lbHostname)
			if err != nil {
				t.Logf("Waiting for IP of loadbalancer %s", lbHostname)
				// if the hostname cannot be resolved currently then retry later
				return false, nil
			}
			for _, ip := range ips {
				serviceIPs[ip.String()] = struct{}{}
			}
		} else {
			t.Logf("Waiting for loadbalancer details for service %s", testServiceName)
			return false, nil
		}
		t.Logf("Loadbalancer's IP(s): %v", serviceIPs)
		return true, nil
	}); err != nil {
		t.Fatalf("Failed to get loadbalancer IPs for service %s/%s: %v", testNamespace, testServiceName, err)
	}

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)
		// create a DNS resolver which uses the nameservers of the test hosted zone
		customResolver := customResolver(nameSrv)

		// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
		if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
			rec := fmt.Sprintf("%s.%s", testServiceName, hostedZoneDomain)
			ips, err := customResolver.LookupHost(context.TODO(), rec)
			if err != nil {
				t.Logf("Waiting for dns record: %s", rec)
				return false, nil
			}
			for _, ip := range ips {
				if _, ok := serviceIPs[ip]; !ok {
					return false, nil
				}
			}
			return true, nil
		}); err != nil {
			t.Logf("Failed to verify that DNS has been correctly set.")
		} else {
			return
		}
	}
	t.Logf("All nameservers failed to verify that DNS has been correctly set.")
	t.Fail()
}

func verifyOpenShiftRouteSource(t *testing.T, canonicalName, host string) {
	//var canonicalName = "router-external-dns.external-dns.example-naga.info"
	// try all nameservers and fail only if all failed
	recordExist := false
	for _, nameSrv := range nameServers {
		t.Logf("Looking for cname record in nameserver: %s", nameSrv)
		if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
			cnames, err := lookupCNAME(host, nameSrv)
			if err != nil {
				t.Logf("cname lookup failed for nameserver : %s , error : %v", nameSrv, err)
				return false, nil
			}
			for _, cname := range cnames {
				if strings.Contains(cname, canonicalName) {
					recordExist = true
					return true, nil
				}
			}
			return false, nil
		}); err != nil {
			t.Logf("Failed to verify host record with CNAME Record")
		}
	}

	if !recordExist {
		t.Logf("Cname record not found in any nameService, heance test failed")
		t.Fail()
		return
	}
}

func fetchRouterCanonicalHostname(route1Name types.NamespacedName) (string, error) {
	route1 := routev1.Route{}
	canonicalName := ""
	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
		err = kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: route1Name.Namespace,
			Name:      route1Name.Name,
		}, &route1)
		if err != nil {
			return false, err
		}

		if len(route1.Status.Ingress) < 1 {
			return false, nil
		}

		for _, ingress := range route1.Status.Ingress {
			if strings.Contains(ingress.RouterCanonicalHostname, hostedZoneDomain) {
				canonicalName = ingress.RouterCanonicalHostname
			}
		}
		if canonicalName == "" {
			return false, fmt.Errorf("No RouterCanonicalHostname found")
		}
		return true, nil
	}); err != nil {
		return "", err
	}
	return canonicalName, nil
}
