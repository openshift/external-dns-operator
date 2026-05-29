//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/external-dns-operator/test/common"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
)

type awsTestHelper struct {
	r53Client *route53.Client
	keyID     string
	secretKey string
}

func newAWSHelper(isOpenShiftCI bool) (providerTestHelper, error) {
	provider := &awsTestHelper{}
	if err := provider.prepareConfigurations(isOpenShiftCI); err != nil {
		return nil, err
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(provider.keyID, provider.secretKey, "")),
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	provider.r53Client = route53.NewFromConfig(cfg)
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

func (a *awsTestHelper) buildExternalDNS(name, zoneID, zoneDomain string, credsSecret *corev1.Secret) operatorv1beta1.ExternalDNS {
	resource := defaultExternalDNS(name, zoneID, zoneDomain)
	resource.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAWS,
		AWS: &operatorv1beta1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1beta1.SecretReference{
				Name: credsSecret.Name,
			},
		},
	}
	return resource
}

func (a *awsTestHelper) buildOpenShiftExternalDNS(name, zoneID, zoneDomain, routerName string, _ *corev1.Secret) operatorv1beta1.ExternalDNS {
	resource := routeExternalDNS(name, zoneID, zoneDomain, routerName)
	resource.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAWS,
	}
	return resource
}

func (a *awsTestHelper) buildOpenShiftExternalDNSV1Alpha1(name, zoneID, zoneDomain, routerName string, _ *corev1.Secret) operatorv1alpha1.ExternalDNS {
	resource := routeExternalDNSV1Alpha1(name, zoneID, zoneDomain, routerName)
	resource.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
	}
	return resource
}

func (a *awsTestHelper) platform() string {
	return string(configv1.AWSPlatformType)
}

func (a *awsTestHelper) ensureHostedZone(zoneDomain string) (string, []string, error) {
	zones, err := a.r53Client.ListHostedZones(context.TODO(), &route53.ListHostedZonesInput{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	// if hosted zone exists then return its id and nameservers
	for _, zone := range zones.HostedZones {
		if aws.ToString(zone.Name) == zoneDomain {
			hostedZone, err := a.r53Client.GetHostedZone(context.TODO(), &route53.GetHostedZoneInput{Id: zone.Id})
			if err != nil {
				return "", nil, fmt.Errorf("failed to get hosted zone %s: %v", aws.ToString(zone.Id), err)
			}
			if hostedZone.DelegationSet == nil {
				return aws.ToString(zone.Id), nil, nil
			}
			return aws.ToString(zone.Id), hostedZone.DelegationSet.NameServers, nil
		}
	}

	// create hosted zone and return its id and nameservers
	zone, err := a.r53Client.CreateHostedZone(context.TODO(), &route53.CreateHostedZoneInput{
		Name:            aws.String(zoneDomain),
		CallerReference: aws.String(time.Now().Format(time.RFC3339)),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get create hosted zone: %w", err)
	}
	if zone.DelegationSet == nil {
		return aws.ToString(zone.HostedZone.Id), nil, nil
	}
	return aws.ToString(zone.HostedZone.Id), zone.DelegationSet.NameServers, nil
}

// AWS sdk expect to zone ID to delete the xone, where as Azure expect Domain Name
func (a *awsTestHelper) deleteHostedZone(zoneID, zoneDomain string) error {
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: &zoneID,
	}
	output, err := a.r53Client.ListResourceRecordSets(context.TODO(), input)
	if err != nil {
		return err
	}
	var recordChanges []types.Change

	// create change set deleting all DNS records which are not of NS and SOA types in hosted zone
	for len(output.ResourceRecordSets) != 0 {
		for _, recordset := range output.ResourceRecordSets {
			if recordset.Type == types.RRTypeSoa || recordset.Type == types.RRTypeNs {
				continue
			}
			recordChanges = append(recordChanges, types.Change{
				Action:            types.ChangeActionDelete,
				ResourceRecordSet: &recordset,
			})
		}
		if !output.IsTruncated {
			break
		}
		input.StartRecordName = output.NextRecordName
		input.StartRecordType = output.NextRecordType
		input.StartRecordIdentifier = output.NextRecordIdentifier
		output, err = a.r53Client.ListResourceRecordSets(context.TODO(), input)
		if err != nil {
			return err
		}
	}

	if len(recordChanges) != 0 {
		changeRecordsInput := &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: &zoneID,
			ChangeBatch: &types.ChangeBatch{
				Changes: recordChanges,
			},
		}
		if _, err := a.r53Client.ChangeResourceRecordSets(context.TODO(), changeRecordsInput); err != nil {
			return err
		}
	}

	zoneInput := &route53.DeleteHostedZoneInput{
		Id: &zoneID,
	}
	if _, err := a.r53Client.DeleteHostedZone(context.TODO(), zoneInput); err != nil {
		return err
	}
	return nil
}

func (a *awsTestHelper) prepareConfigurations(isOpenShiftCI bool) error {
	if isOpenShiftCI {
		data, err := common.RootCredentials("aws-creds")
		if err != nil {
			return fmt.Errorf("failed to get AWS credentials: %w", err)
		}
		a.keyID = string(data["aws_access_key_id"])
		a.secretKey = string(data["aws_secret_access_key"])
	} else {
		a.keyID = common.MustGetEnv("AWS_ACCESS_KEY_ID")
		a.secretKey = common.MustGetEnv("AWS_SECRET_ACCESS_KEY")
	}
	return nil
}
