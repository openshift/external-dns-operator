//+build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kscheme "k8s.io/client-go/kubernetes/scheme"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	awsSession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

const (
	recordExistsTimeout  = 5 * time.Minute
	podReadyTimeout      = 1 * time.Minute
	httpGetTimeout       = 10 * time.Minute
	recordDeletedTimeout = 5 * time.Minute
	dnsLookupTimeout     = 5 * time.Second
	dnsServer            = "8.8.8.8:53"
)

var kclient client.Client
var infraConfig configv1.Infrastructure
var dnsClient struct {
	aws *route53.Route53
	//TODO: add other provider clients here
}
var domainName string

var scheme *runtime.Scheme

var dialer *net.Dialer
var httpClient *http.Client

func init() {
	scheme = kscheme.Scheme
	if err := configv1.Install(scheme); err != nil {
		panic(err)
	}
	if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

var defaultParentDomainName string
var defaultParentZoneID string

var (
	defaultNamespace = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "publish-external-dns",
		},
	}
	defaultBackend = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-openshift",
			Namespace: "publish-external-dns",
			Labels: map[string]string{
				"name": "hello-openshift",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "hello-openshift",
					Image: "openshift/hello-openshift",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8080,
							Protocol:      corev1.ProtocolTCP,
						},
					},
				},
			},
		},
	}
	defaultService = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hello-openshift-service",
			Namespace: "publish-external-dns",
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
)

func TestMain(m *testing.M) {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		fmt.Printf("failed to get kube config: %s\n", err)
		os.Exit(1)
	}

	kubeClient, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		fmt.Printf("failed to create kube client: %s\n", err)
		os.Exit(1)
	}
	kclient = kubeClient

	if err := kclient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &infraConfig); err != nil {
		fmt.Printf("failed to get infrastructure config: %v\n", err)
		os.Exit(1)
	}

	defaultParentDomainName = os.Getenv("EXTDNS_PARENT_DOMAIN")
	if defaultParentDomainName == "" {
		fmt.Printf("No parent domain specified. Please set the environment variable EXTDNS_PARENT_DOMAIN\n")
		os.Exit(1)
	}
	defaultParentZoneID = os.Getenv("EXTDNS_PARENT_ZONEID")
	if defaultParentZoneID == "" {
		fmt.Printf("No parent zone ID specified. Please set the environment variable EXTDNS_PARENT_ZONEID\n")
		os.Exit(1)
	}

	domainName = generateDomainName()

	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		fmt.Printf("platform status is missing for infrastructure %s", infraConfig.Name)
		os.Exit(1)
	}
	switch platform.Type {
	case configv1.AWSPlatformType:
		mySession := awsSession.Must(awsSession.NewSession())
		dnsClient.aws = route53.New(mySession)
	default:
		fmt.Printf("Unsupported Provider")
		os.Exit(1)
	}

	dialer = &net.Dialer{
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: dnsLookupTimeout,
				}
				return d.DialContext(ctx, "udp", dnsServer)
			},
		},
	}

	httpClient = &http.Client{
		Transport: &http.Transport{
			Dial:        dialer.Dial,
			DialContext: dialer.DialContext,
		},
	}

	os.Exit(m.Run())
}

func TestOperatorAvailable(t *testing.T) {
	expected := []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
	}
	if err := waitForOperatorDeploymentStatusCondition(t, kclient, expected...); err != nil {
		t.Errorf("did not get expected available condition: %v", err)
	}
}

func TestExternalDNSRecordLifecycle(t *testing.T) {
	if err := kclient.Create(context.TODO(), &defaultNamespace); err != nil {
		t.Fatalf("Failed to create default namespace: %v", err)
	}
	defer kclient.Delete(context.TODO(), &defaultNamespace)
	credsSecret, err := createCredsSecret(t, kclient)
	if err != nil {
		t.Fatalf("Failed to create credentials secret: %v", err)
	}
	defer kclient.Delete(context.TODO(), credsSecret)
	hostedZoneID, err := createHostedZone(t, domainName, defaultParentDomainName, defaultParentZoneID)
	if err != nil {
		t.Fatalf("Failed to create hosted zone for tests: %v", err)
	}
	defer deleteHostedZone(t, hostedZoneID, domainName, defaultParentZoneID)
	extDNS := defaultExternalDNS(t, hostedZoneID, credsSecret)
	if err := kclient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS: %v", err)
	}
	defer kclient.Delete(context.TODO(), &extDNS)
	if err := kclient.Create(context.TODO(), &defaultBackend); err != nil {
		t.Fatalf("Failed to create default backend: %v", err)
	}
	defer kclient.Delete(context.TODO(), &defaultBackend)
	if err := kclient.Create(context.TODO(), &defaultService); err != nil {
		t.Fatalf("Failed to create default service: %v", err)
	}
	defer kclient.Delete(context.TODO(), &defaultService)
	expectedDomainName := fmt.Sprintf("%s.%s", defaultService.ObjectMeta.Name, domainName)
	err = wait.PollImmediate(1*time.Second, recordExistsTimeout, func() (bool, error) {
		exists, err := recordExistsInHostedZone(t, hostedZoneID, expectedDomainName)
		if err != nil {
			t.Errorf("Failed to query cloud service for dns records: %v", err)
			return false, err
		}
		if !exists {
			return false, nil
		} else {
			return true, nil
		}
	})
	if err != nil {
		t.Fatalf("Domain name %s could not be found in hosted zone: %v", expectedDomainName, err)
	}
	// wait for backend pod to become ready
	err = wait.PollImmediate(1*time.Second, podReadyTimeout, func() (bool, error) {
		pod := &corev1.Pod{}
		podNamespacedName := types.NamespacedName{
			Namespace: defaultBackend.Namespace,
			Name:      defaultBackend.Name,
		}
		expectedConditions := []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		}
		if err := kclient.Get(context.TODO(), podNamespacedName, pod); err != nil {
			t.Logf("failed to get pod %s: %v", podNamespacedName.Name, err)
			return false, nil
		}
		expected := podConditionMap(expectedConditions...)
		current := podConditionMap(pod.Status.Conditions...)
		return conditionsMatchExpected(expected, current), nil
	})
	err = wait.PollImmediate(5*time.Second, httpGetTimeout, func() (bool, error) {
		var resp *http.Response
		resp, err := httpClient.Get(fmt.Sprintf("http://%s", expectedDomainName))
		if err != nil {
			return false, nil
		}
		if resp.StatusCode != 200 {
			t.Errorf("Got response code %v", resp.StatusCode)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Failed to query http://%s: %v", expectedDomainName, err)
	}
	kclient.Delete(context.TODO(), &defaultService)
	err = wait.PollImmediate(5*time.Second, recordDeletedTimeout, func() (bool, error) {
		exists, err := recordExistsInHostedZone(t, hostedZoneID, expectedDomainName)
		if err != nil {
			t.Errorf("Failed to query cloud service for dns records: %v", err)
			return false, err
		}
		if exists {
			return false, nil
		} else {
			return true, nil
		}
	})
	if err != nil {
		t.Fatalf("A record for %q was not properly deleted: %v", expectedDomainName, err)
	}
}
