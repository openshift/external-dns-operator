//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
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

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/version"
)

const (
	baseZoneDomain     = "example-test.info"
	testNamespace      = "external-dns-test"
	testServiceName    = "test-service"
	testRouteName      = "test-route"
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
	helper           providerTestHelper
	hostedZoneDomain = baseZoneDomain
	operandVersion   = "v0.10.1"
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
			if strings.EqualFold(provider, platformType) {
				fmt.Printf("Skipping e2e test for the provider %q!\n", provider)
				os.Exit(0)
			}
		}
	}

	if version.SHORTCOMMIT != "" {
		hostedZoneDomain = strconv.FormatInt(time.Now().Unix(), 10) + "." + version.SHORTCOMMIT + "." + baseZoneDomain
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

func TestOperatorAvailable(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	if err := waitForOperatorDeploymentStatusCondition(t, kubeClient, expected...); err != nil {
		t.Errorf("Did not get expected available condition: %v", err)
	}
}

func TestExternalDNSWithRoute(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	t.Log("Creating external dns instance with source type route")
	extDNS := helper.buildOpenShiftExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain, "")
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), &extDNS)
	}()

	// create a route with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source route")
	testRouteHost := "myroute." + hostedZoneDomain
	route := testRoute(testRouteName, testNamespace, testRouteHost, testServiceName)
	if err := kubeClient.Create(context.Background(), route); err != nil {
		t.Fatalf("Failed to create test route %s/%s: %v", testNamespace, testRouteName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), route)
	}()
	t.Logf("Created Route Host is %v", testRouteHost)

	// get the router canonical name
	var targetRoute routev1.Route
	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
		t.Log("Waiting for the route to be acknowledged by the router")
		err = kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: testNamespace,
			Name:      testRouteName,
		}, &targetRoute)
		if err != nil {
			return false, err
		}

		// if the status ingress slice is not populated by the ingress controller, try later
		if len(targetRoute.Status.Ingress) < 1 {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Failed to retrieve the created route %s/%s: %v", testNamespace, testRouteName, err)
	}

	t.Logf("Target route ingress is %v", targetRoute.Status.Ingress)

	targetRouterCName := targetRoute.Status.Ingress[0].RouterCanonicalHostname
	if targetRouterCName == "" {
		t.Fatalf("Router's canonical name is empty %v", err)
	}
	t.Logf("Target router's CName is %v", targetRouterCName)

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)

		// verify dns records has been created for the route host.
		if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
			cname, err := lookupCNAME(testRouteHost, nameSrv)
			if err != nil {
				t.Logf("Waiting for DNS record: %s, error: %v", testRouteHost, err)
				return false, nil
			}
			if equalFQDN(cname, targetRouterCName) {
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Logf("Failed to verify that DNS has been correctly set.")
		} else {
			return
		}
	}
	t.Fatalf("All nameservers failed to verify that DNS has been correctly set.")
}

func TestExternalDNSRecordLifecycle(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	t.Log("Creating credentials secret")
	resourceSecret := helper.makeCredentialsSecret(testCredSecretName)
	err = kubeClient.Create(context.TODO(), resourceSecret)
	if err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s for resource: %v", resourceSecret.Namespace, resourceSecret.Name, err)
	}

	t.Log("Creating external dns instance")
	extDNS := helper.buildExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain, resourceSecret)
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), &extDNS)
	}()

	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source service")
	service := defaultService(testServiceName, testNamespace)
	if err := kubeClient.Create(context.Background(), service); err != nil {
		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testServiceName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), service)
	}()

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
	t.Fatalf("All nameservers failed to verify that DNS has been correctly set.")
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

// Test to verify the ExternalDNS should create the CNAME record for the OpenshiftRoute
// with multiple ingress controller deployed in Openshift.
// Route's host should resolve to the canonical name of the specified ingress controller.
func TestExternalDNSCustomIngress(t *testing.T) {
	if operandVersion == "v0.10.1" {
		t.Skip("The test needs to be enabled once latest external-dns image available (>v0.10.1).")
	}
	testIngressNamespace := "test-extdns-openshift-route"
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testIngressNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testIngressNamespace, err)
	}
	openshiftRouterName := "external-dns"
	name := types.NamespacedName{Namespace: testIngressNamespace, Name: openshiftRouterName}
	t.Logf("Create custom ingress controller %s/%s", name.Namespace, name.Name)
	ing := newHostNetworkController(name, name.Name+"."+hostedZoneDomain)
	if err = kubeClient.Create(context.TODO(), ing); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create ingresscontroller: %v", err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), ing)
	}()

	externalDnsServiceName := fmt.Sprintf("%s-source-as-openshift-route", testExtDNSName)
	t.Logf("Creating external dns instance: %s", externalDnsServiceName)
	extDNS := helper.buildOpenShiftExternalDNS(externalDnsServiceName, hostedZoneID, hostedZoneDomain, openshiftRouterName)
	if err = kubeClient.Create(context.TODO(), &extDNS); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), &extDNS)
	}()

	routeName := types.NamespacedName{Namespace: testIngressNamespace, Name: "external-dns-route"}
	host := fmt.Sprintf("app.%s", hostedZoneDomain)
	route := testRoute(routeName.Name, routeName.Namespace, host, "testServiceName")

	// The first route should be admitted
	if err = kubeClient.Create(context.TODO(), route); err != nil {
		t.Fatalf("Failed to create route: %v", err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), route)
	}()
	canonicalName, err := fetchRouterCanonicalHostname(t, routeName)
	if err != nil {
		t.Fatalf("Failed to get RouterCanonicalHostname for route %s/%s: %v", routeName.Namespace, routeName.Name, err)
	}
	t.Logf("CanonicalName: %s for the route: %s ", routeName.Name, canonicalName)
	verifyCNAMERecordForOpenshiftRoute(t, canonicalName, host)
}

func verifyCNAMERecordForOpenshiftRoute(t *testing.T, canonicalName, host string) {
	// try all nameservers and fail only if all failed
	recordExist := false
	for _, nameSrv := range nameServers {
		t.Logf("Looking for cname record in nameserver: %s", nameSrv)
		if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
			cname, err := lookupCNAME(host, nameSrv)
			if err != nil {
				t.Logf("Cname lookup failed for nameserver: %s , error: %v", nameSrv, err)
				return false, nil
			}
			if strings.Contains(cname, canonicalName) {
				recordExist = true
				return true, nil
			}
			return false, nil
		}); err != nil {
			t.Logf("Failed to verify host record with CNAME Record")
		}
	}

	if !recordExist {
		t.Fatalf("CNAME record not found in any name server")
	}
}

func fetchRouterCanonicalHostname(t *testing.T, routeName types.NamespacedName) (string, error) {
	route := routev1.Route{}
	canonicalName := ""
	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
		err = kubeClient.Get(context.TODO(), types.NamespacedName{
			Namespace: routeName.Namespace,
			Name:      routeName.Name,
		}, &route)
		if err != nil {
			return false, err
		}
		if len(route.Status.Ingress) < 1 {
			t.Logf("No ingress found in route, retrying..")
			return false, nil
		}

		for _, ingress := range route.Status.Ingress {
			if strings.Contains(ingress.RouterCanonicalHostname, hostedZoneDomain) {
				canonicalName = ingress.RouterCanonicalHostname
				return true, nil
			}
		}
		t.Logf("Unable to fetch the canonicalHostname, retrying..")
		return false, nil
	}); err != nil {
		return "", err
	}
	return canonicalName, nil
}
