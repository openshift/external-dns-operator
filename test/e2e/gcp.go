// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	dns "google.golang.org/api/dns/v1"
)

type gcpTestHelper struct {
	dnsService      *dns.Service
	gcpCredentials  string
	gcpProjectId    string
	providerOptions []string
}

func newGCPHelper(isOpenShiftCI bool, kubeClient client.Client) (providerTestHelper, error) {
	gcpCredentials, gcpProjectId, err := prepareGCPConfigurations(isOpenShiftCI, kubeClient)
	if err != nil {
		return nil, err
	}
	dnsService, err := dns.NewService(context.Background(), option.WithCredentialsJSON([]byte(gcpCredentials)))
	if err != nil {
		return nil, fmt.Errorf("could not authenticate with the given credentials: %w", err)
	}

	return &gcpTestHelper{
		dnsService:     dnsService,
		gcpCredentials: gcpCredentials,
		gcpProjectId:   gcpProjectId,
	}, nil
}

func (a *gcpTestHelper) externalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
	resource := defaultExternalDNS(testExtDNSName, hostedZoneID, hostedZoneDomain)
	resource.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeGCP,
		GCP: &operatorv1alpha1.ExternalDNSGCPProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: credsSecret.Name,
			},
			Project: &a.gcpProjectId,
		},
	}
	return resource
}
func (g *gcpTestHelper) makeCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("gcp-credentials-%s", randomString(16)),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"gcp-credentials.json": []byte(g.gcpCredentials),
		},
	}
}

func (g *gcpTestHelper) platform() string {
	return string(configv1.GCPPlatformType)
}

func (g *gcpTestHelper) ensureHostedZone(hostedZoneDomain string) (string, []string, error) {
	gcpRootDomain := hostedZoneDomain + "."

	resp, err := g.dnsService.ManagedZones.List(g.gcpProjectId).Do()
	if err != nil {
		return "", nil, fmt.Errorf("failed to list managed zones: %w", err)
	}
	zones := resp.ManagedZones
	// if managed zone exists then return its id and nameservers
	for _, zone := range zones {
		if zone.DnsName == gcpRootDomain {
			return strconv.FormatUint(zone.Id, 10), zone.NameServers, nil
		}
	}

	// if zone does not exist, create managed zone and return its id and nameservers
	zone, err := g.dnsService.ManagedZones.Create(g.gcpProjectId, &dns.ManagedZone{
		// must be 1-63 characters long, must begin with a letter,
		// end with a letter or digit, and only contain lowercase letters, digits or dashes
		Name:        "a" + strings.ToLower(strings.ReplaceAll(hostedZoneDomain, ".", "-")),
		DnsName:     gcpRootDomain,
		Description: "ExternalDNS Operator test managed zone.",
	}).Do()
	if err != nil {
		return "", nil, fmt.Errorf("error creating managed zone: %w", err)
	}
	return strconv.FormatUint(zone.Id, 10), zone.NameServers, nil
}

func (g *gcpTestHelper) deleteHostedZone(hostedZoneID, hostedZoneDomain string) error {
	resp, err := g.dnsService.ResourceRecordSets.List(g.gcpProjectId, hostedZoneID).Do()
	if err != nil {
		return fmt.Errorf("failed to retrieve dns records for zoneID %v: %w", hostedZoneID, err)
	}

	recordChanges := &dns.Change{}

	// create change set deleting all DNS records which are not of NS and SOA types in managed zone
	for len(resp.Rrsets) != 0 {
		for _, recordset := range resp.Rrsets {
			if recordset.Type == "SOA" || recordset.Type == "NS" {
				continue
			} else {
				recordChanges.Deletions = append(recordChanges.Deletions, recordset)
			}
		}
		if resp.NextPageToken == "" {
			break
		} else {
			token := resp.NextPageToken
			resp, err = g.dnsService.ResourceRecordSets.List(g.gcpProjectId, hostedZoneID).PageToken(token).Do()
			if err != nil {
				return fmt.Errorf("failed to retrieve dns records for zoneID %v and pageToken %v: %w", hostedZoneID, token, err)
			}
		}
	}

	// atomically delete the records from the managed zone
	if len(recordChanges.Deletions) != 0 {
		if _, err := g.dnsService.Changes.Create(g.gcpProjectId, hostedZoneID, recordChanges).Do(); err != nil {
			return fmt.Errorf("failed to execute the change operation for records deletion: %w", err)
		}
	}

	// delete the managed zone itself
	err = g.dnsService.ManagedZones.Delete(g.gcpProjectId, hostedZoneID).Do()
	if err != nil {
		return fmt.Errorf("failed to delete zone with id %v: %w", hostedZoneID, err)
	}

	return nil
}

func prepareGCPConfigurations(openshiftCI bool, kubeClient client.Client) (gcpCredentials, gcpProjectId string, err error) {
	if openshiftCI {
		data, err := rootCredentials(kubeClient, "gcp-credentials")
		if err != nil {
			return "", "", fmt.Errorf("failed to get GCP credentials: %w", err)
		}
		gcpCredentials = string(data["service_account.json"])
		gcpProjectId, err = getGCPProjectId(kubeClient)
		if err != nil {
			return "", "", fmt.Errorf("failed to get GCP project id: %w", err)
		}
	} else {
		gcpCredentials = mustGetEnv("GCP_CREDENTIALS")
		gcpProjectId = mustGetEnv("GCP_PROJECT_ID")
	}
	return gcpCredentials, gcpProjectId, nil
}

func getGCPProjectId(kubeClient client.Client) (string, error) {
	infraConfig := &configv1.Infrastructure{}
	err := kubeClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, infraConfig)
	if err != nil {
		return "", err
	}
	return infraConfig.Status.PlatformStatus.GCP.ProjectID, nil
}
