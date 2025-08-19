//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openshift/external-dns-operator/test/common"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/openshift/external-dns-operator/pkg/version"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	baseZoneDomain  = "example-test.info"
	testNamespace   = "external-dns-test"
	testServiceName = "test-service"
	testRouteName   = "test-route"
	testExtDNSName  = "test-extdns"
)

var (
	nameServers      []string
	hostedZoneID     string
	helper           providerTestHelper
	hostedZoneDomain = baseZoneDomain
)

func initProviderHelper(openshiftCI bool, platformType string) (providerTestHelper, error) {
	switch platformType {
	case string(configv1.AWSPlatformType):
		return newAWSHelper(openshiftCI)
	case string(configv1.AzurePlatformType):
		return newAzureHelper()
	case string(configv1.GCPPlatformType):
		return newGCPHelper(openshiftCI)
	case infobloxDNSProvider:
		return newInfobloxHelper()
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

	openshiftCI = common.IsOpenShift()
	platformType, err = common.GetPlatformType(openshiftCI)
	if err != nil {
		fmt.Printf("Failed to determine platform type: %v\n", err)
		os.Exit(1)
	}

	if common.SkipProvider(platformType) {
		fmt.Printf("Skipping e2e test for the provider %q!\n", platformType)
		os.Exit(0)
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

	if err = common.EnsureOperandResources(context.TODO()); err != nil {
		fmt.Printf("Failed to ensure operand resources: %v\n", err)
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

// func TestOperatorAvailable(t *testing.T) {
// 	expected := []appsv1.DeploymentCondition{
// 		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
// 	}
// 	if err := waitForOperatorDeploymentStatusCondition(context.TODO(), t, common.KubeClient, expected...); err != nil {
// 		t.Errorf("Did not get expected available condition: %v", err)
// 	}
// }

func TestExternalDNSWithRoute(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := common.KubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	// secret is needed only for DNS providers which cannot get their credentials from CCO
	// namely Infobox, BlueCat
	t.Log("Creating credentials secret")
	credSecret := helper.makeCredentialsSecret(common.OperatorNamespace)
	err = common.KubeClient.Create(context.TODO(), credSecret)
	if err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}

	t.Log("Creating external dns instance with source type route")
	extDNS := helper.buildOpenShiftExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain, "", credSecret)
	if err := common.KubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), &extDNS)
	}()

	// create a route with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source route")
	testRouteHost := "myroute." + hostedZoneDomain
	route := testRoute(testRouteName, testNamespace, testRouteHost, testServiceName)
	if err := common.KubeClient.Create(context.TODO(), route); err != nil {
		t.Fatalf("Failed to create test route %s/%s: %v", testNamespace, testRouteName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), route)
	}()
	t.Logf("Created Route Host is %v", testRouteHost)

	// get the router canonical name
	var targetRoute routev1.Route
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		t.Log("Waiting for the route to be acknowledged by the router")
		err = common.KubeClient.Get(ctx, types.NamespacedName{
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
	t.Logf("Target router's CNAME is %v", targetRouterCName)

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)

		// verify dns records has been created for the route host.
		if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
			cNameHost, err := lookupCNAME(testRouteHost, nameSrv)
			if err != nil {
				t.Logf("Waiting for DNS record: %s, error: %v", testRouteHost, err)
				return false, nil
			}
			if equalFQDN(cNameHost, targetRouterCName) {
				t.Log("DNS record found")
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
	err := common.KubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	t.Log("Creating credentials secret")
	credSecret := helper.makeCredentialsSecret(common.OperatorNamespace)
	err = common.KubeClient.Create(context.TODO(), credSecret)
	if err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}

	t.Log("Creating external dns instance")
	extDNS := helper.buildExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain, credSecret)
	if err := common.KubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), &extDNS)
	}()

	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source service")
	service := common.DefaultService(testServiceName, testNamespace)
	if err := common.KubeClient.Create(context.TODO(), service); err != nil {
		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testServiceName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), service)
	}()

	// Get the resolved service IPs of the load balancer
	_, serviceIPs, err := common.GetServiceIPs(context.TODO(), t, common.DnsPollingTimeout, types.NamespacedName{Name: testServiceName, Namespace: testNamespace})
	if err != nil {
		t.Fatalf("failed to get service IPs %s/%s: %v", testNamespace, testServiceName, err)
	}

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)

		// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
		if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
			expectedHost := fmt.Sprintf("%s.%s", testServiceName, hostedZoneDomain)
			ips, err := common.LookupARecord(expectedHost, nameSrv)
			if err != nil {
				t.Logf("Waiting for dns record: %s", expectedHost)
				return false, nil
			}
			gotIPs := make(map[string]struct{})
			for _, ip := range ips {
				gotIPs[ip] = struct{}{}
			}
			t.Logf("Got IPs: %v", gotIPs)

			// If all IPs of the loadbalancer are not present query again.
			if len(gotIPs) < len(serviceIPs) {
				return false, nil
			}
			// all expected IPs should be in the received IPs
			// but these 2 sets are not necessary equal
			for ip := range serviceIPs {
				if _, found := gotIPs[ip]; !found {
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

// Test to verify the ExternalDNS should create the CNAME record for the OpenshiftRoute
// with multiple ingress controller deployed in Openshift.
// Route's host should resolve to the canonical name of the specified ingress controller.
func TestExternalDNSCustomIngress(t *testing.T) {
	testIngressNamespace := "test-extdns-openshift-route"
	t.Logf("Ensuring test namespace %s", testIngressNamespace)
	err := common.KubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testIngressNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testIngressNamespace, err)
	}

	openshiftRouterName := "external-dns"
	// ingress controllers are supposed to be created in the ingress operator namespace
	name := types.NamespacedName{Namespace: "openshift-ingress-operator", Name: openshiftRouterName}
	ingDomain := fmt.Sprintf("%s.%s", name.Name, hostedZoneDomain)
	t.Log("Create custom ingress controller")
	ing := newHostNetworkController(name, ingDomain)
	if err = common.KubeClient.Create(context.TODO(), ing); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create ingresscontroller %s/%s: %v", name.Namespace, name.Name, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), ing)
	}()

	// secret is needed only for DNS providers which cannot get their credentials from CCO
	// namely Infobox, BlueCat
	t.Log("Creating credentials secret")
	credSecret := helper.makeCredentialsSecret(common.OperatorNamespace)
	err = common.KubeClient.Create(context.TODO(), credSecret)
	if err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}

	externalDnsServiceName := fmt.Sprintf("%s-source-as-openshift-route", testExtDNSName)
	t.Log("Creating external dns instance")
	extDNS := helper.buildOpenShiftExternalDNS(externalDnsServiceName, hostedZoneID, hostedZoneDomain, openshiftRouterName, credSecret)
	if err = common.KubeClient.Create(context.TODO(), &extDNS); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), &extDNS)
	}()

	routeName := types.NamespacedName{Namespace: testIngressNamespace, Name: "external-dns-route"}
	host := fmt.Sprintf("app.%s", ingDomain)
	route := testRoute(routeName.Name, routeName.Namespace, host, testServiceName)
	t.Log("Creating test route")
	if err = common.KubeClient.Create(context.TODO(), route); err != nil {
		t.Fatalf("Failed to create route %s/%s: %v", routeName.Namespace, routeName.Name, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), route)
	}()

	canonicalName, err := fetchRouterCanonicalHostname(context.TODO(), t, routeName, ingDomain)
	if err != nil {
		t.Fatalf("Failed to get RouterCanonicalHostname for route %s/%s: %v", routeName.Namespace, routeName.Name, err)
	}
	t.Logf("CanonicalName: %s for the route: %s", canonicalName, routeName.Name)

	verifyCNAMERecordForOpenshiftRoute(context.TODO(), t, canonicalName, host)
}

func TestExternalDNSWithRouteV1Alpha1(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := common.KubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	// secret is needed only for DNS providers which cannot get their credentials from CCO
	// namely Infobox, BlueCat
	t.Log("Creating credentials secret")
	credSecret := helper.makeCredentialsSecret(common.OperatorNamespace)
	err = common.KubeClient.Create(context.TODO(), credSecret)
	if err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}

	t.Log("Creating external dns instance with source type route")
	extDNS := helper.buildOpenShiftExternalDNSV1Alpha1(testExtDNSName, hostedZoneID, hostedZoneDomain, "", credSecret)
	if err := common.KubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), &extDNS)
	}()

	// create a route with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source route")
	testRouteHost := "myroute." + hostedZoneDomain
	route := testRoute(testRouteName, testNamespace, testRouteHost, testServiceName)
	if err := common.KubeClient.Create(context.TODO(), route); err != nil {
		t.Fatalf("Failed to create test route %s/%s: %v", testNamespace, testRouteName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), route)
	}()
	t.Logf("Created Route Host is %v", testRouteHost)

	// get the router canonical name
	var targetRoute routev1.Route
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		t.Log("Waiting for the route to be acknowledged by the router")
		err = common.KubeClient.Get(ctx, types.NamespacedName{
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
	t.Logf("Target router's CNAME is %v", targetRouterCName)

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("Looking for DNS record in nameserver: %s", nameSrv)

		// verify dns records has been created for the route host.
		if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
			cNameHost, err := lookupCNAME(testRouteHost, nameSrv)
			if err != nil {
				t.Logf("Waiting for DNS record: %s, error: %v", testRouteHost, err)
				return false, nil
			}
			if equalFQDN(cNameHost, targetRouterCName) {
				t.Log("DNS record found")
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

// TestExternalDNSSecretCredentialUpdate verifies that at first DNS records are not created when wrong secret is supplied.
// When the wrong secret is updated with the right values, DNS records are created.
func TestExternalDNSSecretCredentialUpdate(t *testing.T) {
	t.Log("Ensuring test namespace")
	testService := fmt.Sprintf("%s-credential-update", testServiceName)
	err := common.KubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	t.Log("Creating wrong credentials secret")
	credSecret := makeWrongCredentialsSecret(common.OperatorNamespace)
	err = common.KubeClient.Create(context.TODO(), credSecret)
	if err != nil {
		t.Fatalf("Failed to create credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}

	t.Log("Creating external dns instance")
	extDNS := helper.buildExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain, credSecret)
	if err := common.KubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), &extDNS)
	}()

	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
	t.Log("Creating source service")
	service := common.DefaultService(testService, testNamespace)
	if err := common.KubeClient.Create(context.TODO(), service); err != nil {
		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testService, err)
	}
	defer func() {
		_ = common.KubeClient.Delete(context.TODO(), service)
	}()

	// Get the resolved service IPs of the load balancer
	_, serviceIPs, err := common.GetServiceIPs(context.TODO(), t, common.DnsPollingTimeout, types.NamespacedName{Name: testService, Namespace: testNamespace})
	if err != nil {
		t.Fatalf("failed to get service IPs %s/%s: %v", testNamespace, testServiceName, err)
	}

	dnsCheck := make(chan bool)
	go func() {
		// try all nameservers and fail only if all failed
		for _, nameSrv := range nameServers {
			t.Logf("Looking for DNS record in nameserver: %s", nameSrv)
			// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
			if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
				expectedHost := fmt.Sprintf("%s.%s", testService, hostedZoneDomain)
				ips, err := common.LookupARecord(expectedHost, nameSrv)
				if err != nil {
					t.Logf("Waiting for dns record: %s", expectedHost)
					return false, nil
				}
				gotIPs := make(map[string]struct{})
				for _, ip := range ips {
					gotIPs[ip] = struct{}{}
				}
				t.Logf("Got IPs: %v", gotIPs)

				// If all IPs of the loadbalancer are not present query again.
				if len(gotIPs) < len(serviceIPs) {
					t.Logf("Expected %d IPs, but got %d, retrying...", len(serviceIPs), len(gotIPs))
					return false, nil
				}
				// all expected IPs should be in the received IPs
				// but these 2 sets are not necessary equal
				for ip := range serviceIPs {
					if _, found := gotIPs[ip]; !found {
						return false, nil
					}
				}
				return true, nil
			}); err != nil {
				t.Logf("Failed to verify that DNS has been correctly set.")
			} else {
				dnsCheck <- true
				return
			}
		}
		dnsCheck <- false
	}()

	t.Logf("Updating credentials secret")
	credSecret.Data = helper.makeCredentialsSecret(common.OperatorNamespace).Data
	err = common.KubeClient.Update(context.TODO(), credSecret)
	if err != nil {
		t.Fatalf("Failed to update credentials secret %s/%s: %v", credSecret.Namespace, credSecret.Name, err)
	}
	t.Logf("Credentials secret updated successfully")

	if resolved := <-dnsCheck; !resolved {
		t.Fatal("All nameservers failed to verify that DNS has been correctly set.")
	}
}

// HELPER FUNCTIONS

func verifyCNAMERecordForOpenshiftRoute(ctx context.Context, t *testing.T, canonicalName, host string) {
	// try all nameservers and fail only if all failed
	recordExist := false
	for _, nameSrv := range nameServers {
		t.Logf("Looking for cname record in nameserver: %s", nameSrv)
		if err := wait.PollUntilContextTimeout(ctx, common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
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

func fetchRouterCanonicalHostname(ctx context.Context, t *testing.T, routeName types.NamespacedName, routerDomain string) (string, error) {
	route := routev1.Route{}
	canonicalName := ""
	if err := wait.PollUntilContextTimeout(ctx, common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		err = common.KubeClient.Get(ctx, types.NamespacedName{
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
			if strings.Contains(ingress.RouterCanonicalHostname, routerDomain) {
				if ingressConditionHasStatus(ingress, routev1.RouteAdmitted, corev1.ConditionTrue) {
					canonicalName = ingress.RouterCanonicalHostname
					return true, nil
				}
				t.Logf("Router didn't admit the route, retrying..")
				return false, nil
			}
		}
		t.Logf("Unable to fetch the canonicalHostname, retrying..")
		return false, nil
	}); err != nil {
		return "", err
	}
	return canonicalName, nil
}

func ingressConditionHasStatus(ingress routev1.RouteIngress, condition routev1.RouteIngressConditionType, status corev1.ConditionStatus) bool {
	for _, c := range ingress.Conditions {
		if condition == c.Type {
			return c.Status == status
		}
	}
	return false
}

func makeWrongCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("wrong-credentials-%s", randomString(16)),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"wrong_credentials": []byte("wrong_access"),
		},
	}
}
