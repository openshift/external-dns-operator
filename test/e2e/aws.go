// +build e2e

package e2e

import (
	"fmt"
	"time"

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

func newAWSHelper(awsAccessKeyID, awsSecretAccessKey string) (providerTestHelper, error) {
	awsSession := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(awsAccessKeyID, awsSecretAccessKey, ""),
	}))
	r53Client := route53.New(awsSession)
	return &awsTestHelper{
		keyID:     awsAccessKeyID,
		secretKey: awsSecretAccessKey,
		r53Client: r53Client,
	}, nil
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

func (a *awsTestHelper) platform() string {
	return string(configv1.AWSPlatformType)
}

func (a *awsTestHelper) ensureHostedZone(rootDomain string) (string, []string, error) {
	zones, err := a.r53Client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	var (
		nameServers  []string
		hostedZoneID string
	)
	// if hosted zone exists then return its id and nameservers
	for _, zone := range zones.HostedZones {
		if aws.StringValue(zone.Name) == rootDomain {
			hostedZone, err := a.r53Client.GetHostedZone(&route53.GetHostedZoneInput{Id: zone.Id})
			if err != nil {
				return "", nil, fmt.Errorf("failed to get hosted zone %s: %v", aws.StringValue(zone.Id), err)
			}
			nameServers = aws.StringValueSlice(hostedZone.DelegationSet.NameServers)
			hostedZoneID = aws.StringValue(zone.Id)
			return hostedZoneID, nameServers, nil
		}
	}

	// create hosted zone and return its id and nameservers
	zone, err := a.r53Client.CreateHostedZone(&route53.CreateHostedZoneInput{
		Name:            aws.String(rootDomain),
		CallerReference: aws.String(time.Now().Format(time.RFC3339)),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get create hosted zone: %w", err)
	}
	nameServers = aws.StringValueSlice(zone.DelegationSet.NameServers)
	hostedZoneID = aws.StringValue(zone.HostedZone.Id)
	return hostedZoneID, nameServers, nil
}

func (a *awsTestHelper) deleteHostedZone(zoneID string) error {
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
	return nil
}
