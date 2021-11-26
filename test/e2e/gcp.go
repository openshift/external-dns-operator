// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	dns "google.golang.org/api/dns/v1"
)

type gcpTestHelper struct {
	dnsService     *dns.Service
	gcpCredentials string
	gcpProjectId   string
}

func newGCPHelper(gcpCredentials, gcpProjectId string) (providerTestHelper, error) {
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

func (g *gcpTestHelper) ensureHostedZone(rootDomain string) (string, []string, error) {
	gcpRootDomain := rootDomain + "."

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
		Name:        "a" + strings.ToLower(strings.ReplaceAll(rootDomain, ".", "-")),
		DnsName:     gcpRootDomain,
		Description: "ExternalDNS Operator test managed zone.",
	}).Do()
	if err != nil {
		return "", nil, fmt.Errorf("error creating managed zone: %w", err)
	}
	return strconv.FormatUint(zone.Id, 10), zone.NameServers, nil
}

func (g *gcpTestHelper) deleteHostedZone(zoneID string) error {
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
