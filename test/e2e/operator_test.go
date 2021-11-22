//+build e2e

package e2e

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"net"
	"os"
	"testing"
	"time"

	kscheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	hostedZoneDomain   = "example-test.info"
	testNamespace      = "external-dns-test"
	dnsPollingInterval = 15 * time.Second
	dnsPollingTimeout  = 15 * time.Minute
)

var (
	kubeClient   client.Client
	scheme       *runtime.Scheme
	nameServers  []string
	hostedZoneID string
	helper       providerTestHelper
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
	case string(configv1.AzurePlatformType):
		return newAzureHelper(kubeClient)

	default:
		return nil, fmt.Errorf("unsupported Provider: '%s'", platformType)
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

	extDNS := defaultExternalDNS(t, "test-extdns", testNamespace, hostedZoneID, hostedZoneDomain, resourceSecret, helper.platform())
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS: %v", err)
	}
	defer kubeClient.Delete(context.TODO(), &extDNS)

	// create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource
	service := defaultService("test-service", testNamespace)
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
			Name:      "test-service",
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
		ips, err := net.LookupIP(lbHostname)
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
		t.Fatalf("failed to get loadbalancers IPs for service %s/%s: %v", testNamespace, "test-service", err)
	}

	// create a DNS resolver which uses the nameservers of the test hosted zone
	customResolver := net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Second * 10,
			}
			return d.DialContext(ctx, network, fmt.Sprintf("%s:53", nameServers[0]))
		},
	}

	// verify that the IPs of the record created by ExternalDNS match the IPs of loadbalancer obtained in the previous step.
	if err := wait.PollImmediate(dnsPollingInterval, dnsPollingTimeout, func() (done bool, err error) {
		ips, err := customResolver.LookupHost(context.TODO(), fmt.Sprintf("%s.%s", "test-service", hostedZoneDomain))
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
		t.Fatalf("failed to verify that DNS has been correctly set.")
	}
}
