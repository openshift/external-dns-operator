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

var _ providerTestHelper = &gcpTestHelper{}

func newGCPHelper(isOpenShiftCI bool, kubeClient client.Client) (providerTestHelper, error) {
	provider := &gcpTestHelper{}
	err := provider.prepareConfigurations(isOpenShiftCI, kubeClient)
	if err != nil {
		return nil, err
	}

	provider.dnsService, err = dns.NewService(context.Background(), option.WithCredentialsJSON([]byte(provider.gcpCredentials)))
	if err != nil {
		return nil, fmt.Errorf("could not authenticate with the given credentials: %w", err)
	}

	return provider, nil
}

func (g *gcpTestHelper) buildExternalDNS(name, zoneID, zoneDomain string, credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
	resource := defaultExternalDNS(name, zoneID, zoneDomain)
	resource.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeGCP,
		GCP: &operatorv1alpha1.ExternalDNSGCPProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: credsSecret.Name,
			},
			Project: &g.gcpProjectId,
		},
	}
	return resource
}

func (g *gcpTestHelper) buildOpenShiftExternalDNS(name, zoneID, zoneDomain string) operatorv1alpha1.ExternalDNS {
	resource := routeExternalDNS(name, zoneID, zoneDomain)
	resource.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeGCP,
		GCP: &operatorv1alpha1.ExternalDNSGCPProviderOptions{
			Project: &g.gcpProjectId,
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

func (g *gcpTestHelper) ensureHostedZone(zoneDomain string) (string, []string, error) {
	gcpRootDomain := zoneDomain + "."

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
		Name:        "a" + strings.ToLower(strings.ReplaceAll(zoneDomain, ".", "-")),
		DnsName:     gcpRootDomain,
		Description: "ExternalDNS Operator test managed zone.",
	}).Do()
	if err != nil {
		return "", nil, fmt.Errorf("error creating managed zone: %w", err)
	}
	return strconv.FormatUint(zone.Id, 10), zone.NameServers, nil
}

func (g *gcpTestHelper) deleteHostedZone(zoneID, zoneDomain string) error {
	resp, err := g.dnsService.ResourceRecordSets.List(g.gcpProjectId, zoneID).Do()
	if err != nil {
		return fmt.Errorf("failed to retrieve dns records for zoneID %v: %w", zoneID, err)
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
			resp, err = g.dnsService.ResourceRecordSets.List(g.gcpProjectId, zoneID).PageToken(token).Do()
			if err != nil {
				return fmt.Errorf("failed to retrieve dns records for zoneID %v and pageToken %v: %w", zoneID, token, err)
			}
		}
	}

	// atomically delete the records from the managed zone
	if len(recordChanges.Deletions) != 0 {
		if _, err := g.dnsService.Changes.Create(g.gcpProjectId, zoneID, recordChanges).Do(); err != nil {
			return fmt.Errorf("failed to execute the change operation for records deletion: %w", err)
		}
	}

	// delete the managed zone itself
	err = g.dnsService.ManagedZones.Delete(g.gcpProjectId, zoneID).Do()
	if err != nil {
		return fmt.Errorf("failed to delete zone with id %v: %w", zoneID, err)
	}

	return nil
}

func (a *gcpTestHelper) prepareConfigurations(openshiftCI bool, kubeClient client.Client) error {
	if openshiftCI {
		data, err := rootCredentials(kubeClient, "gcp-credentials")
		if err != nil {
			return fmt.Errorf("failed to get GCP credentials: %w", err)
		}
		a.gcpCredentials = string(data["service_account.json"])
		a.gcpProjectId, err = getGCPProjectId(kubeClient)
		if err != nil {
			return fmt.Errorf("failed to get GCP project id: %w", err)
		}
	} else {
		a.gcpCredentials = mustGetEnv("GCP_CREDENTIALS")
		a.gcpProjectId = mustGetEnv("GCP_PROJECT_ID")
	}
	return nil
}

func getGCPProjectId(kubeClient client.Client) (string, error) {
	infraConfig := &configv1.Infrastructure{}
	err := kubeClient.Get(context.Background(), types.NamespacedName{Name: "cluster"}, infraConfig)
	if err != nil {
		return "", err
	}
	return infraConfig.Status.PlatformStatus.GCP.ProjectID, nil
}
