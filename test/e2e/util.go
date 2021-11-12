//+build e2e

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/service/route53"
)

var defaultExtDNSTemplate = operatorv1alpha1.ExternalDNS{
	ObjectMeta: metav1.ObjectMeta{
		Name: "test-extdns",
	},
	Spec: operatorv1alpha1.ExternalDNSSpec{
		Source: operatorv1alpha1.ExternalDNSSource{
			ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
				Type: operatorv1alpha1.SourceTypeService,
				Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
					ServiceType: []corev1.ServiceType{
						corev1.ServiceTypeLoadBalancer,
						corev1.ServiceTypeClusterIP,
					},
				},
				Namespace: StringRef("publish-external-dns"),
				AnnotationFilter: map[string]string{
					"external-dns.mydomain.org/publish": "yes",
				},
			},
			HostnameAnnotationPolicy: "Ignore",
		},
	},
}

func randomURLString(n int) string {
	var chars = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	str := make([]rune, n)
	for i := range str {
		str[i] = chars[rand.Intn(len(chars))]
	}
	return string(str)
}

func generateDomainName() string {
	//id := randomURLString(16)
	return fmt.Sprintf("external-dns-test.%s", defaultParentDomainName)
}

/* creates a hosted zone on the dns provider. returns the zone id */
func createHostedZone(t *testing.T, domainName, parentDomain, parentZoneID string) (string, error) {
	t.Helper()
	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		t.Fatalf("platform status is missing for infrastructure %s", infraConfig.Name)
	}
	switch platform.Type {
	case configv1.AWSPlatformType:
		zoneInput := route53.CreateHostedZoneInput{
			CallerReference: StringRef(fmt.Sprintf("%v", time.Now().UTC())),
			Name:            &domainName,
			HostedZoneConfig: &route53.HostedZoneConfig{
				Comment:     StringRef("Hosted zone for external-dns testing. If this still exists after tests have completed, please remove it"),
				PrivateZone: BoolRef(false),
			},
		}
		var zoneOutput *route53.CreateHostedZoneOutput
		zoneOutput, err := dnsClient.aws.CreateHostedZone(&zoneInput)
		if err != nil {
			t.Fatalf("Failed to create hosted zone: %v", err)
		}
		err = addSubdomainZoneToParent(t, domainName, *zoneOutput.HostedZone.Id, parentDomain, parentZoneID)
		return *zoneOutput.HostedZone.Id, err
	default:
		t.Fatalf("Unsupported Provider")
	}
	return "", nil
}

func deleteHostedZone(t *testing.T, zoneID, domainName, parentZoneID string) {
	t.Helper()
	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		t.Fatalf("platform status is missing for infrastructure %s", infraConfig.Name)
	}
	switch platform.Type {
	case configv1.AWSPlatformType:
		removeSubdomainZoneFromParent(t, domainName, parentZoneID)
		input := route53.ListResourceRecordSetsInput{
			HostedZoneId: &zoneID,
			MaxItems:     StringRef("10"),
		}
		output, err := dnsClient.aws.ListResourceRecordSets(&input)
		if err != nil {
			t.Errorf("Failed to list resource record sets: %v", err)
		}
		var recordChanges []*route53.Change
		for len(output.ResourceRecordSets) != 0 {
			for _, recordset := range output.ResourceRecordSets {
				if *recordset.Type == "SOA" || *recordset.Type == "NS" {
					continue
				} else {
					recordDelete := &route53.Change{
						Action:            StringRef("DELETE"),
						ResourceRecordSet: recordset,
					}
					recordChanges = append(recordChanges, recordDelete)
				}
			}
			if !(*output.IsTruncated) {
				break
			} else {
				input.StartRecordName = output.NextRecordName
				output, err = dnsClient.aws.ListResourceRecordSets(&input)
				if err != nil {
					t.Errorf("Failed to list resource record sets: %v", err)
					break
				}
			}
		}
		if len(recordChanges) != 0 {
			changeRecordsInput := route53.ChangeResourceRecordSetsInput{
				HostedZoneId: &zoneID,
				ChangeBatch: &route53.ChangeBatch{
					Changes: recordChanges,
				},
			}
			if _, err := dnsClient.aws.ChangeResourceRecordSets(&changeRecordsInput); err != nil {
				t.Errorf("Failed to delete records in hosted zone: %v", err)
			}
		}
		zoneInput := route53.DeleteHostedZoneInput{
			Id: &zoneID,
		}
		if _, err := dnsClient.aws.DeleteHostedZone(&zoneInput); err != nil {
			t.Errorf("Failed to delete hosted zone: %v", err)
		}
	default:
		t.Fatalf("Unsupported Provider")
	}
}

func addSubdomainZoneToParent(t *testing.T, subdomainName, subdomainZoneID, parentName, parentZoneID string) error {
	t.Helper()
	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		t.Fatalf("platform status is missing for infrastructure %s", infraConfig.Name)
	}
	switch platform.Type {
	case configv1.AWSPlatformType:
		var nameservers []*route53.ResourceRecord
		input := route53.ListResourceRecordSetsInput{
			HostedZoneId:    &subdomainZoneID,
			MaxItems:        StringRef("10"),
			StartRecordName: &subdomainName,
			StartRecordType: StringRef("NS"),
		}
		output, err := dnsClient.aws.ListResourceRecordSets(&input)
		if err != nil {
			return err
		}
		for len(output.ResourceRecordSets) != 0 {
			for _, recordset := range output.ResourceRecordSets {
				if *recordset.Type == "NS" {
					if *recordset.Name == subdomainName {
						for _, record := range recordset.ResourceRecords {
							nameservers = append(nameservers, record)
						}
					} else {
						t.Errorf("Found NS record for domain %q, but wanted %q", *recordset.Name, subdomainName)
					}
				}
			}
			if !(*output.IsTruncated) {
				break
			} else {
				input.StartRecordName = output.NextRecordName
				output, err = dnsClient.aws.ListResourceRecordSets(&input)
				if err != nil {
					return err
				}
			}
		}
		if len(nameservers) != 0 {
			changeRecordsInput := route53.ChangeResourceRecordSetsInput{
				HostedZoneId: &parentZoneID,
				ChangeBatch: &route53.ChangeBatch{
					Changes: []*route53.Change{
						{
							Action: StringRef("CREATE"),
							ResourceRecordSet: &route53.ResourceRecordSet{
								Name:            &subdomainName,
								ResourceRecords: nameservers,
								Type:            StringRef("NS"),
								TTL:             Int64Ref(180),
							},
						},
					},
				},
			}
			_, err = dnsClient.aws.ChangeResourceRecordSets(&changeRecordsInput)
			if err != nil {
				t.Errorf("Failed to add subdomain NS record to parent zone: %v", err)
			}
			return err
		} else {
			t.Errorf("No NS Records found")
		}
	default:
		t.Fatalf("Unsupported Provider")
	}
	return nil
}

func removeSubdomainZoneFromParent(t *testing.T, subdomainName, parentZoneID string) error {
	t.Helper()
	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		t.Fatalf("platform status is missing for infrastructure %s", infraConfig.Name)
	}
	switch platform.Type {
	case configv1.AWSPlatformType:
		var nsRecordSet *route53.ResourceRecordSet = nil
		input := route53.ListResourceRecordSetsInput{
			HostedZoneId:    &parentZoneID,
			MaxItems:        StringRef("10"),
			StartRecordName: &subdomainName,
			StartRecordType: StringRef("NS"),
		}
		output, err := dnsClient.aws.ListResourceRecordSets(&input)
		if err != nil {
			return err
		}
		for len(output.ResourceRecordSets) != 0 {
			for _, recordset := range output.ResourceRecordSets {
				if *recordset.Name == subdomainName && *recordset.Type == "NS" {
					nsRecordSet = recordset
					break
				}
			}
			if nsRecordSet != nil {
				break
			}
			if !(*output.IsTruncated) {
				break
			} else {
				input.StartRecordName = output.NextRecordName
				output, err = dnsClient.aws.ListResourceRecordSets(&input)
				if err != nil {
					return err
				}
			}
		}
		if nsRecordSet != nil {
			changeRecordsInput := route53.ChangeResourceRecordSetsInput{
				HostedZoneId: &parentZoneID,
				ChangeBatch: &route53.ChangeBatch{
					Changes: []*route53.Change{
						{
							Action:            StringRef("DELETE"),
							ResourceRecordSet: nsRecordSet,
						},
					},
				},
			}
			_, err = dnsClient.aws.ChangeResourceRecordSets(&changeRecordsInput)
			if err != nil {
				t.Errorf("Failed to remove subdomain NS record from parent zone: %v", err)
			}
			return err
		}
	default:
		t.Fatalf("Unsupported Provider")
	}
	return nil
}

func recordExistsInHostedZone(t *testing.T, zoneID string, hostRecord string) (bool, error) {
	t.Helper()
	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		t.Fatalf("platform status is missing for infrastructure %s", infraConfig.Name)
	}
	switch platform.Type {
	case configv1.AWSPlatformType:
		input := route53.ListResourceRecordSetsInput{
			HostedZoneId:    &zoneID,
			MaxItems:        StringRef("10"),
			StartRecordName: &hostRecord,
		}
		output, err := dnsClient.aws.ListResourceRecordSets(&input)
		if err != nil {
			return false, err
		}
		for len(output.ResourceRecordSets) != 0 {
			for _, recordset := range output.ResourceRecordSets {
				if *recordset.Name == hostRecord {
					return true, nil
				}
			}
			if !(*output.IsTruncated) {
				return false, nil
			} else {
				input.StartRecordName = output.NextRecordName
				output, err = dnsClient.aws.ListResourceRecordSets(&input)
				if err != nil {
					return false, err
				}
			}
		}
	default:
		t.Fatalf("Unsupported Provider")
	}
	return false, nil
}

func StringRef(in string) *string {
	return &in
}

func BoolRef(in bool) *bool {
	return &in
}

func Int64Ref(in int64) *int64 {
	return &in
}

func defaultExternalDNS(t *testing.T, zoneID string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
	t.Helper()
	extdns := defaultExtDNSTemplate

	platform := infraConfig.Status.PlatformStatus
	if platform == nil {
		t.Fatalf("platform status is missing for infrastructure %s", infraConfig.Name)
	}
	var provider operatorv1alpha1.ExternalDNSProvider
	switch platform.Type {
	case configv1.AWSPlatformType:
		provider = operatorv1alpha1.ExternalDNSProvider{
			Type: operatorv1alpha1.ProviderTypeAWS,
			AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
				Credentials: operatorv1alpha1.SecretReference{
					Name: credsSecret.Name,
				},
			},
		}
	default:
		t.Fatalf("Unsupported Provider")
	}

	extdns.Spec.Provider = provider
	extdns.Spec.Zones = []string{zoneID}

	extdns.Spec.Source.FQDNTemplate = []string{fmt.Sprintf("{{.Name}}.%s", domainName)}

	return extdns
}

func createCredsSecret(t *testing.T, cl client.Client) (*corev1.Secret, error) {
	t.Helper()
	originalCredsSecret := &corev1.Secret{}
	originalCredsSecretName := types.NamespacedName{
		Name:      "aws-creds",
		Namespace: "kube-system",
	}
	if err := cl.Get(context.TODO(), originalCredsSecretName, originalCredsSecret); err != nil {
		t.Logf("failed to get credentials secret %s: %v", originalCredsSecretName.Name, err)
		return nil, err
	}

	credsSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("aws-access-key-%s", randomURLString(16)),
			Namespace: "external-dns-operator",
		},
		Data: map[string][]byte{
			"aws_access_key_id":     originalCredsSecret.Data["aws_access_key_id"],
			"aws_secret_access_key": originalCredsSecret.Data["aws_secret_access_key"],
		},
	}
	if err := cl.Create(context.TODO(), &credsSecret); err != nil {
		return nil, err
	}
	return &credsSecret, nil
}

func podConditionMap(conditions ...corev1.PodCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[string(cond.Type)] = string(cond.Status)
	}
	return conds
}

func deploymentConditionMap(conditions ...appsv1.DeploymentCondition) map[string]string {
	conds := map[string]string{}
	for _, cond := range conditions {
		conds[string(cond.Type)] = string(cond.Status)
	}
	return conds
}

func waitForOperatorDeploymentStatusCondition(t *testing.T, cl client.Client, conditions ...appsv1.DeploymentCondition) error {
	t.Helper()
	return wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
		dep := &appsv1.Deployment{}
		depNamespacedName := types.NamespacedName{
			Name:      "external-dns-operator",
			Namespace: "external-dns-operator",
		}
		if err := cl.Get(context.TODO(), depNamespacedName, dep); err != nil {
			t.Logf("failed to get deployment %s: %v", depNamespacedName.Name, err)
			return false, nil
		}

		expected := deploymentConditionMap(conditions...)
		current := deploymentConditionMap(dep.Status.Conditions...)
		return conditionsMatchExpected(expected, current), nil
	})
}

func conditionsMatchExpected(expected, actual map[string]string) bool {
	filtered := map[string]string{}
	for k := range actual {
		if _, comparable := expected[k]; comparable {
			filtered[k] = actual[k]
		}
	}
	return reflect.DeepEqual(expected, filtered)
}
