//+build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
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

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/version"
)

const (
	baseZoneDomain     = "example-test.info"
	testNamespace      = "external-dns-test"
	testServiceName    = "test-service"
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
	hostedZoneDomain = version.SHORTCOMMIT + "." + baseZoneDomain
)

func init() {
	scheme = kscheme.Scheme
	if err := configv1.Install(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

func initKubeClient() error {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kube config: %w\n", err)
	}

	kubeClient, err = client.New(kubeConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kube client: %w\n", err)
	}
	return nil
}

func initProviderHelper() (providerTestHelper, error) {
	var (
		openshiftCI  bool
		platformType string
		err          error
	)

	if os.Getenv("OPENSHIFT_CI") != "" {
		openshiftCI = true
		platformType, err = getPlatformType(kubeClient)
		if err != nil {
			return nil, fmt.Errorf("failed to determine platform type: %w", err)
		}
	} else {
		platformType = mustGetEnv("CLOUD_PROVIDER")
	}

	switch platformType {
	case string(configv1.AWSPlatformType):
		var (
			awsAccessKeyID     string
			awsSecretAccessKey string
		)
		if openshiftCI {
			awsAccessKeyID, awsSecretAccessKey, err = rootAWSCredentials(kubeClient)
			if err != nil {
				return nil, fmt.Errorf("failed to get AWS credentials from CCO: %w", err)
			}
		} else {
			awsAccessKeyID = mustGetEnv("AWS_ACCESS_KEY_ID")
			awsSecretAccessKey = mustGetEnv("AWS_SECRET_ACCESS_KEY")
		}
		return newAWSHelper(awsAccessKeyID, awsSecretAccessKey)
	case string(configv1.GCPPlatformType):
		var gcpCredentials string
		var gcpProjectId string
		if openshiftCI {
			gcpCredentials, err = rootGCPCredentials(kubeClient)
			if err != nil {
				return nil, fmt.Errorf("failed to get GCP credentials from CCO: %w", err)
			}
			gcpProjectId, err = getGCPProjectId(kubeClient)
			if err != nil {
				return nil, fmt.Errorf("failed to get GCP project id: %w", err)
			}
		} else {
			gcpCredentials = mustGetEnv("GCP_CREDENTIALS")
			gcpProjectId = mustGetEnv("GCP_PROJECT_ID")
		}
		return newGCPHelper(gcpCredentials, gcpProjectId)
	default:
		return nil, fmt.Errorf("unsupported Provider: '%s'", platformType)
	}
}

func TestMain(m *testing.M) {
	var (
		err error
	)
	if err = initKubeClient(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	if helper, err = initProviderHelper(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}

	fmt.Printf("ensuring hosted zone: %s\n", hostedZoneDomain)
	hostedZoneID, nameServers, err = helper.ensureHostedZone(hostedZoneDomain)
	if err != nil {
		fmt.Printf("Failed to created hosted zone for domain %s: %v", hostedZoneDomain, err)
		os.Exit(1)
	}

	exitStatus := m.Run()

	err = helper.deleteHostedZone(hostedZoneID)
	if err != nil {
		fmt.Printf("failed to delete hosted zone %s: %v", hostedZoneID, err)
		os.Exit(1)
	}
	os.Exit(exitStatus)
}

func TestOperatorAvailable(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	if err := waitForOperatorDeploymentStatusCondition(t, kubeClient, expected...); err != nil {
		t.Errorf("did not get expected available condition: %v", err)
	}
}

func TestExternalDNSRecordLifecycle(t *testing.T) {
	// ensure test namespace
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to ensure namespace %s: %v", testNamespace, err)
	}

	resourceSecret := helper.makeCredentialsSecret("external-dns-operator")
	err = kubeClient.Create(context.TODO(), resourceSecret)
	if err != nil {
		t.Fatalf("failed to create credentials secret %s/%s for resource: %v", resourceSecret.Namespace, resourceSecret.Name, err)
	}

	extDNS := defaultExternalDNS(t, testExtDNSName, testNamespace, hostedZoneID, hostedZoneDomain, resourceSecret, helper.platform())
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS: %v", err)
	}
	defer kubeClient.Delete(context.TODO(), &extDNS)

	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
	service := defaultService(testServiceName, testNamespace)
	if err := kubeClient.Create(context.Background(), service); err != nil {
		t.Fatalf("Failed to create test service: %v", err)
	}
	defer kubeClient.Delete(context.TODO(), service)

	serviceIPs := make(map[string]struct{})
	// get the IPs of the loadbalancer which is created for the service
	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
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
		lbHostname := service.Status.LoadBalancer.Ingress[0].Hostname
		// use built in Go resolver instead of the platform's one
		ips, err := customResolver("").LookupIP(context.TODO(), "ip", lbHostname)
		if err != nil {
			t.Logf("waiting for loadbalancer IP for %s", lbHostname)
			// if the hostname cannot be resolved currently then retry later
			return false, nil
		}
		for _, ip := range ips {
			serviceIPs[ip.String()] = struct{}{}
		}
		return true, nil
	}); err != nil {
		t.Fatalf("failed to get loadbalancers IPs for service %s/%s: %v", testNamespace, testServiceName, err)
	}

	// try all nameservers and fail only if all failed
	for _, nameSrv := range nameServers {
		t.Logf("looking for DNS record in nameserver: %s", nameSrv)
		// create a DNS resolver which uses the nameservers of the test hosted zone
		customResolver := customResolver(nameSrv)

		// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
		if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
			ips, err := customResolver.LookupHost(context.TODO(), fmt.Sprintf("%s.%s", testServiceName, hostedZoneDomain))
			if err != nil {
				t.Log("waiting for dns record")
				return false, nil
			}
			for _, ip := range ips {
				if _, ok := serviceIPs[ip]; !ok {
					return false, nil
				}
			}
			return true, nil
		}); err != nil {
			t.Logf("failed to verify that DNS has been correctly set.")
		} else {
			return
		}
	}
	t.Fatalf("all nameservers failed to verify that DNS has been correctly set.")
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
