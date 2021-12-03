//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

type awsTestHelper struct {
	r53Client *route53.Route53
	keyID     string
	secretKey string
}

var _ providerTestHelper = &awsTestHelper{}

func newAWSHelper(isOpenShiftCI bool, kubeClient client.Client) (providerTestHelper, error) {
	provider := &awsTestHelper{}
	if err := provider.prepareConfigurations(isOpenShiftCI, kubeClient); err != nil {
		return nil, err
	}

	awsSession := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(provider.keyID, provider.secretKey, ""),
	}))

	provider.r53Client = route53.New(awsSession)
	return provider, nil
}

func (a *awsTestHelper) makeCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("aws-access-key-%s", randomString(16)),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"aws_access_key_id":     []byte(a.keyID),
			"aws_secret_access_key": []byte(a.secretKey),
		},
	}
}

func (a *awsTestHelper) buildExternalDNS(name, zoneID, zoneDomain, sourceType, routerName string,
	credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
	resource := defaultExternalDNS(name, zoneID, zoneDomain, sourceType, routerName)
	resource.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
		AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: credsSecret.Name,
			},
		},
	}
	return resource
}
func (a *awsTestHelper) platform() string {
	return string(configv1.AWSPlatformType)
}

func (a *awsTestHelper) ensureHostedZone(zoneDomain string) (string, []string, error) {
	zones, err := a.r53Client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	// if hosted zone exists then return its id and nameservers
	for _, zone := range zones.HostedZones {
		if aws.StringValue(zone.Name) == zoneDomain {
			hostedZone, err := a.r53Client.GetHostedZone(&route53.GetHostedZoneInput{Id: zone.Id})
			if err != nil {
				return "", nil, fmt.Errorf("failed to get hosted zone %s: %v", aws.StringValue(zone.Id), err)
			}
			return aws.StringValue(zone.Id), aws.StringValueSlice(hostedZone.DelegationSet.NameServers), nil
		}
	}

	// create hosted zone and return its id and nameservers
	zone, err := a.r53Client.CreateHostedZone(&route53.CreateHostedZoneInput{
		Name:            aws.String(zoneDomain),
		CallerReference: aws.String(time.Now().Format(time.RFC3339)),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get create hosted zone: %w", err)
	}
	return aws.StringValue(zone.HostedZone.Id), aws.StringValueSlice(zone.DelegationSet.NameServers), nil
}

// AWS sdk expect to zone ID to delete the xone, where as Azure expect Domain Name
func (a *awsTestHelper) deleteHostedZone(zoneID, zoneDomain string) error {
	listInput := &route53.ListHostedZonesInput{}
	outputList, err := a.r53Client.ListHostedZones(listInput)
	if err != nil {
		return err
	}
	zoneIDs := []string{}
	for _, zone := range outputList.HostedZones {
		if strings.Contains(*zone.Name, zoneDomain) {
			zoneIDs = append(zoneIDs, *zone.Id)
		}
	}
	for _, zoneID = range zoneIDs {
		input := route53.ListResourceRecordSetsInput{
			HostedZoneId: &zoneID,
		}
		output, err := a.r53Client.ListResourceRecordSets(&input)
		if err != nil {
			return err
		}
		var recordChanges []*route53.Change

		// create change set deleting all DNS records which are not of NS and SOA types in hosted zone
		for len(output.ResourceRecordSets) != 0 {
			for _, recordset := range output.ResourceRecordSets {
				if *recordset.Type == "SOA" || *recordset.Type == "NS" {
					continue
				} else {
					recordDelete := &route53.Change{
						Action:            aws.String("DELETE"),
						ResourceRecordSet: recordset,
					}
					recordChanges = append(recordChanges, recordDelete)
				}
			}
			if !(*output.IsTruncated) {
				break
			} else {
				input.StartRecordName = output.NextRecordName
				output, err = a.r53Client.ListResourceRecordSets(&input)
				if err != nil {
					return err
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
			if _, err := a.r53Client.ChangeResourceRecordSets(&changeRecordsInput); err != nil {
				return err
			}
		}

		zoneInput := route53.DeleteHostedZoneInput{
			Id: &zoneID,
		}
		if _, err := a.r53Client.DeleteHostedZone(&zoneInput); err != nil {
			return err
		}
	}
	return nil
}

func (a *awsTestHelper) prepareConfigurations(isOpenShiftCI bool, kubeClient client.Client) error {
	//if isOpenShiftCI {
	data, err := rootCredentials(kubeClient, "aws-creds")
	if err != nil {
		return fmt.Errorf("failed to get AWS credentials: %w", err)
	}
	a.keyID = string(data["aws_access_key_id"])
	a.secretKey = string(data["aws_secret_access_key"])
	//} else {
	//	a.keyID = mustGetEnv("AWS_ACCESS_KEY_ID")
	//	a.secretKey = mustGetEnv("AWS_SECRET_ACCESS_KEY")
	//}
	return nil
}
