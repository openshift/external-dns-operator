//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"

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

func newAWSHelper(isOpenShiftCI bool) (providerTestHelper, error) {
	awsAccessKeyID, awsSecretAccessKey, err := prepareConfigurations(isOpenShiftCI)
	if err != nil {
		return nil, err
	}

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

func (a *awsTestHelper) makeCredentialsSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("aws-access-key-%s", randomString(16)),
			Namespace: testCredSecretName,
		},
		Data: map[string][]byte{
			"aws_access_key_id":     []byte(a.keyID),
			"aws_secret_access_key": []byte(a.secretKey),
		},
	}
}

func (a *awsTestHelper) defaultExternalDNS(credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
	return operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testExtDNSName,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Zones: []string{hostedZoneID},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeService,
					Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeLoadBalancer,
							corev1.ServiceTypeClusterIP,
						},
					},
					AnnotationFilter: map[string]string{
						"external-dns.mydomain.org/publish": "yes",
					},
				},
				HostnameAnnotationPolicy: "Ignore",
				FQDNTemplate:             []string{fmt.Sprintf("{{.Name}}.%s", hostedZoneDomain)},
			},
			Provider: operatorv1alpha1.ExternalDNSProvider{
				Type: operatorv1alpha1.ProviderTypeAWS,
				AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
					Credentials: operatorv1alpha1.SecretReference{
						Name: credsSecret.Name,
					},
				},
			},
		},
	}
}
func (a *awsTestHelper) platform() string {
	return string(configv1.AWSPlatformType)
}

func (a *awsTestHelper) ensureHostedZone() (string, []string, error) {
	zones, err := a.r53Client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	// if hosted zone exists then return its id and nameservers
	for _, zone := range zones.HostedZones {
		if aws.StringValue(zone.Name) == hostedZoneDomain {
			hostedZone, err := a.r53Client.GetHostedZone(&route53.GetHostedZoneInput{Id: zone.Id})
			if err != nil {
				return "", nil, fmt.Errorf("failed to get hosted zone %s: %v", aws.StringValue(zone.Id), err)
			}
			return aws.StringValue(zone.Id), aws.StringValueSlice(hostedZone.DelegationSet.NameServers), nil
		}
	}

	// create hosted zone and return its id and nameservers
	zone, err := a.r53Client.CreateHostedZone(&route53.CreateHostedZoneInput{
		Name:            aws.String(hostedZoneDomain),
		CallerReference: aws.String(time.Now().Format(time.RFC3339)),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get create hosted zone: %w", err)
	}
	return aws.StringValue(zone.HostedZone.Id), aws.StringValueSlice(zone.DelegationSet.NameServers), nil
}

func (a *awsTestHelper) deleteHostedZone() error {
	input := route53.ListResourceRecordSetsInput{
		HostedZoneId: &hostedZoneID,
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
			HostedZoneId: &hostedZoneID,
			ChangeBatch: &route53.ChangeBatch{
				Changes: recordChanges,
			},
		}
		if _, err := a.r53Client.ChangeResourceRecordSets(&changeRecordsInput); err != nil {
			return err
		}
	}

	zoneInput := route53.DeleteHostedZoneInput{
		Id: &hostedZoneID,
	}
	if _, err := a.r53Client.DeleteHostedZone(&zoneInput); err != nil {
		return err
	}
	return nil
}

func prepareConfigurations(isOpenShiftCI bool) (awsAccessKeyID, awsSecretAccessKey string, err error) {
	if isOpenShiftCI {
		awsAccessKeyID, awsSecretAccessKey, err = rootCredentials()
		if err != nil {
			return "", "", fmt.Errorf("failed to get AWS credentials: %w", err)
		}
	} else {
		awsAccessKeyID = mustGetEnv("AWS_ACCESS_KEY_ID")
		awsSecretAccessKey = mustGetEnv("AWS_SECRET_ACCESS_KEY")
	}
	return
}

func rootCredentials() (string, string, error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      "aws-creds",
		Namespace: "kube-system",
	}
	if err := kubeClient.Get(context.TODO(), secretName, secret); err != nil {
		return "", "", fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return string(secret.Data["aws_access_key_id"]), string(secret.Data["aws_secret_access_key"]), nil
}
