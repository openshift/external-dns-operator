// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// clusterConfig represents common config items for Azure DNS and Azure Private DNS
type clusterConfig struct {
	Cloud          string
	Environment    azure.Environment
	TenantID       string
	SubscriptionID string
	ResourceGroup  string
	Location       string
	ClientID       string
	ClientSecret   string
}

type azureTestHelper struct {
	config     *clusterConfig
	zoneClient dns.ZonesClient
}

// Build the necessary object for the provider test
// for Azure Need the credentials ref clusterConfig
func newAzureHelper() (providerTestHelper, error) {
	azureProvider := &azureTestHelper{}

	if err := azureProvider.prepareCredentials(); err != nil {
		return nil, err
	}

	if err := azureProvider.prepareZoneClient(); err != nil {
		return nil, err
	}
	return azureProvider, nil
}

func (a *azureTestHelper) prepareCredentials() (err error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      "azure-credentials",
		Namespace: "kube-system",
	}
	if err = kubeClient.Get(context.Background(), secretName, secret); err != nil {
		return fmt.Errorf("failed to get credentials secret %s, error : %v", secretName.Name, err)
	}

	a.config = &clusterConfig{
		TenantID:       string(secret.Data["azure_tenant_id"]),
		SubscriptionID: string(secret.Data["azure_subscription_id"]),
		ResourceGroup:  string(secret.Data["azure_resourcegroup"]),
		Location:       string(secret.Data["azure_region"]),
		ClientID:       string(secret.Data["azure_client_id"]),
		ClientSecret:   string(secret.Data["azure_client_secret"]),
		Environment:    azure.PublicCloud,
	}
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

// Credentials should in as json string for azure provider.
func (a *azureTestHelper) makeCredentialsSecret() *corev1.Secret {
	credData := struct {
		TenantID       string `json:"tenantId"`
		SubscriptionID string `json:"subscriptionId"`
		ResourceGroup  string `json:"resourceGroup"`
		ClientID       string `json:"aadClientId"`
		ClientSecret   string `json:"aadClientSecret"`
	}{
		TenantID:       a.config.TenantID,
		SubscriptionID: a.config.SubscriptionID,
		ResourceGroup:  a.config.ResourceGroup,
		ClientID:       a.config.ClientID,
		ClientSecret:   a.config.ClientSecret,
	}
	azureCreds, _ := json.Marshal(credData)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("azure-config-file-%s", randomString(16)),
			Namespace: testCredSecretName,
		},
		Data: map[string][]byte{
			"azure.json": azureCreds,
		},
	}
}

func (a *azureTestHelper) platform() string {
	return string(configv1.AzurePlatformType)
}

func (a *azureTestHelper) ensureHostedZone() (string, []string, error) {
	location := "global"
	z, err := a.zoneClient.CreateOrUpdate(context.TODO(), a.config.ResourceGroup, hostedZoneDomain,
		dns.Zone{Location: &location}, "", "")
	if err != nil {
		return "", []string{}, err
	}
	return *z.ID, *z.ZoneProperties.NameServers, nil
}

func (a *azureTestHelper) deleteHostedZone() error {
	if _, err := a.zoneClient.Delete(context.TODO(), a.config.ResourceGroup, hostedZoneDomain, ""); err != nil {
		return fmt.Errorf("unable to delete zone :%s, failed error: %v", hostedZoneDomain, err)
	}
	return nil
}

func (a *azureTestHelper) defaultExternalDNS(credsSecret *corev1.Secret) operatorv1alpha1.ExternalDNS {
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
				Type: operatorv1alpha1.ProviderTypeAzure,
				Azure: &operatorv1alpha1.ExternalDNSAzureProviderOptions{
					ConfigFile: operatorv1alpha1.SecretReference{
						Name: credsSecret.Name,
					},
				},
			},
		},
	}
}

// ref: https://github.com/kubernetes-sigs/external-dns/blob/master/provider/azure/azure.go
// getAccessToken retrieves Azure API access token.
func getAccessToken(cfg *clusterConfig) (*adal.ServicePrincipalToken, error) {
	// Try to retrieve token with service principal credentials.
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
