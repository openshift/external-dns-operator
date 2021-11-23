//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RESOURCE_GROUP = "azure_resourcegroup"
	SUBSCIPTION_ID = "azure_subscription_id"
	TANENT_ID      = "azure_tenant_id"
	CLIENT_ID      = "azure_client_id"
	CLIENT_SECRET  = "azure_client_secret"
	REGION         = "azure_region"

	KUBE_SYSTEM_SECRET_NAME = "azure-credentials"
)

// config represents common config items for Azure DNS and Azure Private DNS
type cluserConfig struct {
	Cloud                       string            `json:"cloud" yaml:"cloud"`
	Environment                 azure.Environment `json:"-" yaml:"-"`
	TenantID                    string            `json:"tenantId" yaml:"tenantId"`
	SubscriptionID              string            `json:"subscriptionId" yaml:"subscriptionId"`
	ResourceGroup               string            `json:"resourceGroup" yaml:"resourceGroup"`
	Location                    string            `json:"location" yaml:"location"`
	ClientID                    string            `json:"aadClientId" yaml:"aadClientId"`
	ClientSecret                string            `json:"aadClientSecret" yaml:"aadClientSecret"`
	UseManagedIdentityExtension bool              `json:"useManagedIdentityExtension" yaml:"useManagedIdentityExtension"`
	UserAssignedIdentityID      string            `json:"userAssignedIdentityID" yaml:"userAssignedIdentityID"`
}

type azureTestHelper struct {
	config     *cluserConfig
	kubeClient client.Client
	zoneClient dns.ZonesClient
	zoneName string
}

func newAzureHelper(kubeClient client.Client) (providerTestHelper, error) {
	azureProvider := &azureTestHelper{
		kubeClient: kubeClient,
	}

	if err := azureProvider.prepareCredentials(); err != nil {
		return nil, err
	}

	if err := azureProvider.prepareZoneClient(); err != nil{
		return nil, err
	}
	return azureProvider, nil
}

func (a *azureTestHelper) prepareCredentials() (err error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      KUBE_SYSTEM_SECRET_NAME,
		Namespace: "kube-system",
	}
	if err = a.kubeClient.Get(context.Background(), secretName, secret); err != nil {
		return fmt.Errorf("failed to get credentials secret %s, error : %v", secretName.Name, err)
	}

	a.config = &cluserConfig{
		TenantID:                    string(secret.Data[TANENT_ID]),
		SubscriptionID:              string(secret.Data[SUBSCIPTION_ID]),
		ResourceGroup:               string(secret.Data[RESOURCE_GROUP]),
		Location:                    string(secret.Data[REGION]),
		ClientID:                    string(secret.Data[CLIENT_ID]),
		ClientSecret:                string(secret.Data[CLIENT_SECRET]),
		UseManagedIdentityExtension: true,
	}

	var environment azure.Environment
	if a.config.Cloud == "" {
		environment = azure.PublicCloud
	}
	a.config.Environment = environment

	return nil
}

func (a *azureTestHelper) prepareZoneClient() error {
	token, err := getAccessToken(a.config)
	if err != nil {
		return err
	}
	a.zoneClient = dns.NewZonesClientWithBaseURI(a.config.Environment.ResourceManagerEndpoint, a.config.SubscriptionID)
	a.zoneClient.Authorizer = autorest.NewBearerAuthorizer(token)
	return nil
}

func (a *azureTestHelper) makeCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("azure-config-file-%s", randomString(16)),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tenantId":        []byte(a.config.TenantID),
			"subscriptionId":  []byte(a.config.SubscriptionID),
			"resourceGroup":   []byte(a.config.ResourceGroup),
			"aadClientId":     []byte(a.config.ClientID),
			"aadClientSecret": []byte(a.config.ClientSecret),
		},
	}
}

func (a *azureTestHelper) platform() string {
	return string(configv1.AzurePlatformType)
}

func (a *azureTestHelper) ensureHostedZone(rootDomain string) (string, []string, error) {
	location := "global"
	z, err := a.zoneClient.CreateOrUpdate(context.TODO(), a.config.ResourceGroup, rootDomain,
		dns.Zone{Location: &location}, "", "")
	if err != nil {
		return "", []string{}, err
	}
	var zoneID string
	zoneID = *z.ID
	nameservers := append(*z.ZoneProperties.NameServers)
	a.zoneName = rootDomain
	return zoneID, nameservers, nil
}

func (a *azureTestHelper) deleteHostedZone(rootDomain string) error {
	if a.zoneName == ""{
		fmt.Printf("ZoneName is empty, nothing to be deleted")
		return nil
	}
	if _, err := a.zoneClient.Delete(context.TODO(), a.config.ResourceGroup, a.zoneName, ""); err != nil {
		return fmt.Errorf("unable to delete zone :%s, failed error: %v", a.zoneName, err)
	}
	// verify the zone is present
	if _, err := a.isZoneNameExists(); err != nil {
		return fmt.Errorf("unable to verfy zone deletion,failed with error : %v",err)
	}

	return nil
}

func (a *azureTestHelper) isZoneNameExists() (bool, error) {
	ctx := context.TODO()
	zonesIterator, err := a.zoneClient.ListByResourceGroupComplete(ctx, a.config.ResourceGroup, nil)
	if err != nil {
		return false, err
	}

	for zonesIterator.NotDone() {
		zone := zonesIterator.Value()

		if zone.Name != nil  &&  a.zoneName == *zone.Name{
			return true, nil
		}
		err = zonesIterator.NextWithContext(ctx)
		if err != nil {
			return false, err
		}
	}
	return false, nil
}


// ref: https://github.com/kubernetes-sigs/external-dns/blob/master/provider/azure/azure.go
// getAccessToken retrieves Azure API access token.
func getAccessToken(cfg *cluserConfig) (*adal.ServicePrincipalToken, error) {
	// Try to retrieve token with service principal credentials.
	// Try to use service principal first, some AKS clusters are in an intermediate state that `UseManagedIdentityExtension` is `true`
	// and service principal exists. In this case, we still want to use service principal to authenticate.
	if len(cfg.ClientID) > 0 &&
		len(cfg.ClientSecret) > 0 &&
		// due to some historical reason, for pure MSI cluster,
		// they will use "msi" as placeholder in azure.json.
		// In this case, we shouldn't try to use SPN to authenticate.
		!strings.EqualFold(cfg.ClientID, "msi") &&
		!strings.EqualFold(cfg.ClientSecret, "msi") {
		oauthConfig, err := adal.NewOAuthConfig(cfg.Environment.ActiveDirectoryEndpoint, cfg.TenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve OAuth config: %v", err)
		}

		token, err := adal.NewServicePrincipalToken(*oauthConfig, cfg.ClientID, cfg.ClientSecret, cfg.Environment.ResourceManagerEndpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create service principal token: %v", err)
		}
		return token, nil
	}
	return nil, fmt.Errorf("no credentials provided for Azure API")
}