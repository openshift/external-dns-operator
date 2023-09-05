//go:build e2e
// +build e2e

package e2e_awssharedvpc

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/utils"
	"github.com/openshift/external-dns-operator/test/common"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
)

const (
	testNamespace   = "external-dns-test"
	testServiceName = "test-service"
	testExtDNSName  = "test-extdns"
)

var (
	kubeClient          client.Client
	kubeClientSet       *kubernetes.Clientset
	r53ClientAssumeRole *route53.Route53
	roleARN             string
	scheme              *runtime.Scheme
	hostedZoneID        string
	hostedZoneDomain    string
)

func init() {
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

func TestMain(m *testing.M) {
	var (
		err          error
		platformType string
		openshiftCI  bool
	)

	if err, kubeClient, kubeClientSet = common.InitKubeClient(); err != nil {
		fmt.Printf("Failed to init kube client: %v\n", err)
		os.Exit(1)
	}

	openshiftCI = common.IsOpenShift()
	err, platformType = common.GetPlatformType(openshiftCI, kubeClient)
	if err != nil {
		fmt.Printf("Failed to determine platform type: %v\n", err)
		os.Exit(1)
	}

	if common.SkipProvider(platformType) {
		fmt.Printf("Skipping e2e test for the provider %q!\n", platformType)
		os.Exit(0)
	}

	// Only run this test if the DNS config contains the privateZoneIAMRole which indicates it's a "Shared VPC" cluster.
	// Note: Only AWS supports privateZoneIAMRole.
	dnsConfig := configv1.DNS{}
	err = kubeClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &dnsConfig)
	if err != nil {
		fmt.Printf("Failed to get dns 'cluster': %v\n", err)
		os.Exit(1)
	}
	if dnsConfig.Spec.Platform.AWS == nil || dnsConfig.Spec.Platform.AWS.PrivateZoneIAMRole == "" {
		fmt.Printf("Test skipped on non-shared-VPC cluster\n")
		os.Exit(0)
	}
	roleARN = dnsConfig.Spec.Platform.AWS.PrivateZoneIAMRole
	hostedZoneID = dnsConfig.Spec.PrivateZone.ID
	hostedZoneDomain = dnsConfig.Spec.BaseDomain

	err, r53ClientAssumeRole = initRoute53ClientAssumeRole(openshiftCI, kubeClient, roleARN)

	if err := common.EnsureOperandResources(kubeClient); err != nil {
		fmt.Printf("Failed to ensure operand resources: %v\n", err)
	}

	exitStatus := m.Run()
	os.Exit(exitStatus)
}

// TestExternalDNSAssumeRole tests the assumeRole functionality in which you can specify a Role ARN to use another
// account's hosted zone for creating DNS records. Only AWS is supported.
func TestExternalDNSAssumeRole(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := kubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	// Create an External object that uses the role ARN in the dns config to create DNS records in the private DNS
	// zone in another AWS account route 53.
	t.Log("Creating ExternalDNS object that assumes role our of private zone in another account's route 53")
	extDNS := buildExternalDNSAssumeRole(testExtDNSName, hostedZoneID, hostedZoneDomain, roleARN)
	if err := kubeClient.Create(context.TODO(), &extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), &extDNS)
	}()

	// Create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource.
	t.Log("Creating source service")
	service := common.DefaultService(testServiceName, testNamespace)
	if err := kubeClient.Create(context.TODO(), service); err != nil {
		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testServiceName, err)
	}
	defer func() {
		_ = kubeClient.Delete(context.TODO(), service)
	}()

	// Get the service address (hostname or IP) and the resolved service IPs of the load balancer.
	serviceAddress, serviceIPs, err := common.GetServiceIPs(context.TODO(), t, kubeClient, common.DnsPollingTimeout, types.NamespacedName{Name: testServiceName, Namespace: testNamespace})
	if err != nil {
		t.Fatalf("failed to get service IPs %s/%s: %v", testNamespace, testServiceName, err)
	}

	// Query Route 53 API with assume role ARN from the dns config, then compare the results to ensure it matches the
	// service hostname.
	t.Logf("Querying Route 53 API to confirm DNS record exists in a different AWS account")
	expectedHost := fmt.Sprintf("%s.%s", testServiceName, hostedZoneDomain)
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		recordValues, err := getDNSRecordValues(hostedZoneID, expectedHost, "A")
		if err != nil {
			t.Logf("Failed to get DNS record for shared VPC zone: %v", err)
			return false, nil
		} else if len(recordValues) == 0 {
			t.Logf("No DNS records with name %q", expectedHost)
			return false, nil
		}

		if _, found := recordValues[serviceAddress]; !found {
			if _, foundWithDot := recordValues[serviceAddress+"."]; !foundWithDot {
				t.Logf("DNS record with name %q didn't contain expected service IP %q", expectedHost, serviceAddress)
				return false, nil
			}
		}
		t.Logf("DNS record with name %q found in shared Route 53 private zone %q and matched service IPs", expectedHost, hostedZoneID)
		return true, nil
	}); err != nil {
		t.Fatalf("Failed to verify that DNS has been correctly set.")
	}

	t.Logf("Querying for DNS record inside cluster VPC using a dig pod")

	// Verify that the IPs of the record created by ExternalDNS match the IPs of load balancer obtained in the previous
	// step. We will start a pod that runs a dig command for the expected hostname and parse the dig output.
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		gotIPs, err := common.DnsQueryClusterInternal(ctx, t, kubeClient, kubeClientSet, testNamespace, expectedHost)
		if err != nil {
			t.Errorf("Failed to query hostname inside cluster: %v", err)
			return false, nil
		}

		// If all IPs of the loadbalancer are not present, query again.
		if len(gotIPs) == 0 {
			return false, nil
		}
		if len(gotIPs) < len(serviceIPs) {
			return false, nil
		}
		// All expected IPs should be in the received IPs,
		// but these 2 sets are not necessary equal.
		for ip := range serviceIPs {
			if _, found := gotIPs[ip]; !found {
				return false, nil
			}
		}
		t.Log("Expected IPs are equal to IPs resolved.")
		return true, nil
	}); err != nil {
		t.Logf("Failed to verify that DNS has been correctly set.")
	} else {
		return
	}
	t.Fatalf("All nameservers failed to verify that DNS has been correctly set.")
}

func buildExternalDNSAssumeRole(name, zoneID, zoneDomain, roleArn string) operatorv1beta1.ExternalDNS {
	extDns := operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Zones: []string{zoneID},
			Source: operatorv1beta1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
					Type: operatorv1beta1.SourceTypeService,
					Service: &operatorv1beta1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeLoadBalancer,
							corev1.ServiceTypeClusterIP,
						},
					},
					LabelFilter: utils.MustParseLabelSelector("external-dns.mydomain.org/publish=yes"),
				},
				HostnameAnnotationPolicy: operatorv1beta1.HostnameAnnotationPolicyIgnore,
				FQDNTemplate:             []string{fmt.Sprintf("{{.Name}}.%s", zoneDomain)},
			},
			Provider: operatorv1beta1.ExternalDNSProvider{
				Type: operatorv1beta1.ProviderTypeAWS,
				AWS: &operatorv1beta1.ExternalDNSAWSProviderOptions{
					AssumeRole: &operatorv1beta1.ExternalDNSAWSAssumeRoleOptions{
						ARN: roleArn,
					},
					// Use an empty secret in our ExternalDNS object because:
					// 1. This E2E runs only on OpenShift and AWS, we will have a CredentialsRequest to provide credentials.
					// 2. To also validate the v1beta1 "workaround" of providing an empty credentials name with assumeRole due to
					// credentials being required by the CRD. Empty credentials should cause ExternalDNS Operator to use
					// CredentialsRequest.
					Credentials: operatorv1beta1.SecretReference{
						Name: "",
					},
				},
			},
		},
	}

	return extDns
}

func initRoute53ClientAssumeRole(isOpenShiftCI bool, kubeClient client.Client, roleARN string) (error, *route53.Route53) {
	var keyID, secretKey string
	if isOpenShiftCI {
		data, err := common.RootCredentials(kubeClient, "aws-creds")
		if err != nil {
			return fmt.Errorf("failed to get AWS credentials: %w", err), nil
		}
		keyID = string(data["aws_access_key_id"])
		secretKey = string(data["aws_secret_access_key"])
	} else {
		keyID = common.MustGetEnv("AWS_ACCESS_KEY_ID")
		secretKey = common.MustGetEnv("AWS_SECRET_ACCESS_KEY")
	}

	awsSession := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(keyID, secretKey, ""),
	}))

	r53AssumeRoleClient := route53.New(awsSession)
	r53AssumeRoleClient.Config.WithCredentials(stscreds.NewCredentials(awsSession, roleARN))

	return nil, r53AssumeRoleClient
}

func getDNSRecordValues(zoneId, recordName, recordType string) (map[string]struct{}, error) {
	records, err := r53ClientAssumeRole.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:    &zoneId,
		StartRecordName: &recordName,
		StartRecordType: &recordType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resource record sets: %w", err)
	}

	if len(records.ResourceRecordSets) == 0 {
		return nil, nil
	}

	recordList := make(map[string]struct{})
	if records.ResourceRecordSets[0].AliasTarget != nil {
		recordList[*records.ResourceRecordSets[0].AliasTarget.DNSName] = struct{}{}
	} else {
		for _, record := range records.ResourceRecordSets[0].ResourceRecords {
			recordList[*record.Value] = struct{}{}
		}
	}

	return recordList, nil
}
