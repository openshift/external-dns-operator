//go:build e2e
// +build e2e

package e2e_sharedvpc

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

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
	r53ClientAssumeRole *route53.Route53
	roleARN             string
	hostedZoneID        string
	hostedZoneDomain    string
)

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

	// Only run this test if the DNS config contains the privateZoneIAMRole which indicates it's a "Shared VPC" cluster.
	// Note: Only AWS supports privateZoneIAMRole.
	dnsConfig := configv1.DNS{}
	err = common.KubeClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &dnsConfig)
	if err != nil {
		fmt.Printf("Failed to get dns 'cluster': %v\n", err)
		os.Exit(1)
	}
	if dnsConfig.Spec.Platform.AWS == nil || dnsConfig.Spec.Platform.AWS.PrivateZoneIAMRole == "" {
		fmt.Println("Test skipped on non-shared-VPC cluster")
		os.Exit(0)
	}
	roleARN = dnsConfig.Spec.Platform.AWS.PrivateZoneIAMRole
	hostedZoneID = dnsConfig.Spec.PrivateZone.ID
	hostedZoneDomain = dnsConfig.Spec.BaseDomain

	if r53ClientAssumeRole, err = initRoute53ClientAssumeRole(openshiftCI, roleARN); err != nil {
		fmt.Printf("Failed to initialize Route 53 Client: %v\n", err)
		os.Exit(1)
	}

	if err = common.EnsureOperandResources(context.TODO()); err != nil {
		fmt.Printf("Failed to ensure operand resources: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// TestExternalDNSAssumeRole tests the assumeRole functionality in which you can specify a Role ARN to use another
// account's hosted zone for creating DNS records. Only AWS is supported.
func TestExternalDNSAssumeRole(t *testing.T) {
	t.Log("Ensuring test namespace")
	err := common.KubeClient.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to ensure namespace %s: %v", testNamespace, err)
	}

	// Create an External object that uses the role ARN in the dns config to create DNS records in the private DNS
	// zone in another AWS account route 53.
	t.Log("Creating ExternalDNS object that assumes role of private zone in another account's route 53")
	extDNS := buildExternalDNSAssumeRole(testExtDNSName, hostedZoneID, hostedZoneDomain, roleARN)
	if err := common.KubeClient.Create(context.TODO(), extDNS); err != nil {
		t.Fatalf("Failed to create external DNS %q: %v", testExtDNSName, err)
	}
	defer func() {
		if !t.Failed() {
			_ = common.KubeClient.Delete(context.TODO(), extDNS)
		} else {
			t.Logf("Skipping deletion of ExternalDNS %q to gather logs", testExtDNSName)
		}
	}()

	// Create a service of type LoadBalancer with the annotation targeted by the ExternalDNS resource.
	t.Log("Creating source service")
	expectedHost := fmt.Sprintf("%s.%s", testServiceName, hostedZoneDomain)
	service := common.DefaultService(testServiceName, testNamespace)
	if err := common.KubeClient.Create(context.TODO(), service); err != nil {
		t.Fatalf("Failed to create test service %s/%s: %v", testNamespace, testServiceName, err)
	}
	// Delete the service and make sure ExternalDNS cleans up the DNS Records.
	defer func() {
		t.Log("Deleting service and verifying ExternalDNS deletes DNS records.")
		if err = common.KubeClient.Delete(context.TODO(), service); err != nil {
			t.Fatalf("Error deleting service %s/%s: %v", service.Namespace, service.Name, err)
		}
		verifyResourceRecordDeleted(t, hostedZoneID, expectedHost)
	}()

	// Get the service address (hostname or IP) and the resolved service IPs of the load balancer.
	serviceAddress, serviceIPs, err := common.GetServiceIPs(context.TODO(), t, common.DnsPollingTimeout, types.NamespacedName{Name: testServiceName, Namespace: testNamespace})
	if err != nil {
		t.Fatalf("failed to get service IPs %s/%s: %v", testNamespace, testServiceName, err)
	}

	// Query Route 53 API with assume role ARN from the dns config, then compare the results to ensure it matches the
	// service hostname.
	t.Logf("Querying Route 53 API to confirm DNS record exists in a different AWS account")
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		recordValues, err := getResourceRecordValues(hostedZoneID, expectedHost, "A")
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
		t.Fatal("Failed to verify that DNS has been correctly set in a different account.")
	}

	t.Log("Querying for DNS record inside cluster VPC using a dig pod")

	// Verify that the IPs of the record created by ExternalDNS match the IPs of load balancer obtained in the previous
	// step. We will start a pod that runs a dig command for the expected hostname and parse the dig output.
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		gotIPs, err := common.LookupARecordInternal(ctx, t, testNamespace, expectedHost)
		if err != nil {
			t.Logf("Failed to query hostname inside cluster: %v", err)
			return false, nil
		}

		// If all IPs of the loadbalancer are not present, query again.
		if len(gotIPs) == 0 {
			t.Log("Failed to resolve any IPs for the DNS record, retrying...")
			return false, nil
		}
		if len(gotIPs) < len(serviceIPs) {
			t.Logf("Expected %d IPs, but got %d, retrying...", len(serviceIPs), len(gotIPs))
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
		t.Fatalf("Failed to verify that DNS has been correctly set: %v", err)
	}
}

// verifyResourceRecordDeleted verifies that a resource record (DNS record) is deleted
func verifyResourceRecordDeleted(t *testing.T, hostedZone, host string) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(context.TODO(), common.DnsPollingInterval, common.DnsPollingTimeout, true, func(ctx context.Context) (done bool, err error) {
		recordValues, err := getResourceRecordValues(hostedZone, host, "A")
		if err != nil {
			t.Logf("Failed to get DNS record for shared VPC zone: %v", err)
			return false, nil
		} else if len(recordValues) != 0 {
			t.Logf("Waiting for DNS record %q to be deleted.", host)
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Failed to verify that ExternalDNS deleted DNS records: %v", err)
	}
}

// buildExternalDNSAssumeRole builds a ExternalDNS object for a shared VPC cluster test.
func buildExternalDNSAssumeRole(name, zoneID, zoneDomain, roleArn string) *operatorv1beta1.ExternalDNS {
	return &operatorv1beta1.ExternalDNS{
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
				},
			},
		},
	}
}

// initRoute53ClientAssumeRole initializes a Route 53 client with an assumed role.
func initRoute53ClientAssumeRole(isOpenShiftCI bool, roleARN string) (*route53.Route53, error) {
	var keyID, secretKey string
	if isOpenShiftCI {
		data, err := common.RootCredentials("aws-creds")
		if err != nil {
			return nil, fmt.Errorf("failed to get AWS credentials: %w", err)
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

	return r53AssumeRoleClient, nil
}

// getResourceRecordValues gets the values (target address/IPs) of the DNS resource record associated with the provided
// zoneId, recordName, and recordType. If the record is an alias resource record, it will return the target DNS name.
// Otherwise, it will return the target IP address(es). The return type, map[string]struct{}, provides a convenient type
// for existence checking.
func getResourceRecordValues(zoneId, recordName, recordType string) (map[string]struct{}, error) {
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
