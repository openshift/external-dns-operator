//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
)

const (
	RESOURCE_GROUP = "azure_resourcegroup"
	SUBSCIPTION_ID = "azure_subscription_id"
	TANENT_ID      = "azure_tenant_id"
	CLIENT_ID      = "azure_client_id"
	CLIENT_SECRET  = "azure_client_secret"
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
	tenantId                    string
	subscriptionId              string
	resourceGroup               string
	clientID                    string
	clientSecret                string
	useManagedIdentityExtension bool
	congi                       cluserConfig
}

func newAzureHelper(kubeClient client.Client) (providerTestHelper, error) {
	data, err := fetchCredentials(kubeClient)
	if err != nil {
		return nil, err
	}
	return &azureTestHelper{
		tenantId:                    string(data[TANENT_ID]),
		subscriptionId:              string(data[SUBSCIPTION_ID]),
		resourceGroup:               string(data[RESOURCE_GROUP]),
		useManagedIdentityExtension: true,
		clientID:                    string(data[RESOURCE_GROUP]),
		clientSecret:                string(data[RESOURCE_GROUP]),
	}, nil
}

func fetchCredentials(kubeClient client.Client) (data map[string][]byte, err error) {
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      "azure-credentials",
		Namespace: "kube-system",
	}
	if err = kubeClient.Get(context.Background(), secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return secret.Data, nil
}

func (a *azureTestHelper) makeCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("azure-config-file-%s", randomString(16)),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tenantId":                    []byte(a.tenantId),
			"subscriptionId":              []byte(a.subscriptionId),
			"resourceGroup":               []byte(a.resourceGroup),
			"useManagedIdentityExtension": []byte(strconv.FormatBool(a.useManagedIdentityExtension)),
		},
	}
}

func (a *azureTestHelper) platform() string {
	return string(configv1.AzurePlatformType)
}

func (a *azureTestHelper) ensureHostedZone(rootDomain string) (string, []string, error) {
	cfg := &cluserConfig{}
	cfg, err := a.getConfig()
	if err != nil {
		return "", []string{}, err
	}

	token, err := getAccessToken(*cfg)
	if err != nil {
		return "", []string{}, err
	}
	zone := dns.NewZonesClient(a.subscriptionId)
	zone.Authorizer = autorest.NewBearerAuthorizer(token)
	z, err := zone.CreateOrUpdate(context.TODO(), a.resourceGroup, "example-test.info", dns.Zone{}, "", "")
	if err != nil {
		return "", []string{}, err
	}
	var zoneID string
	zoneID = *z.ID
	nameservers := append(*z.ZoneProperties.NameServers)
	fmt.Printf("ZoneID : %s, Name Servers : %v", zoneID, nameservers)
	return zoneID, nameservers, nil
}

func (a *azureTestHelper) deleteHostedZone(zoneID string) error {

	return nil
}

//
//func getMSIUserAssignedIDClient() (*msi.UserAssignedIdentitiesClient, error) {
//	a, err := iam.GetResourceManagementAuthorizer()
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to get authorizer")
//	}
//	msiClient := msi.NewUserAssignedIdentitiesClient(config.SubscriptionID())
//	msiClient.Authorizer = a
//	msiClient.AddToUserAgent(config.UserAgent())
//	return &msiClient, nil
//}
//
//// CreateUserAssignedIdentity creates a user-assigned identity in the specified resource group.
//func (a *azureTestHelper) CreateUserAssignedIdentity(resourceGroup, identity string) (*msi.Identity, error) {
//	msiClient, err := getMSIUserAssignedIDClient()
//	if err != nil {
//		return nil, err
//	}
//	id, err := msiClient.CreateOrUpdate(context.Background(), a.resourceGroup, a.subscriptionId, msi.Identity{
//		Location: to.StringPtr(config.Location()),
//	})
//	return &id, err
//}

func (a *azureTestHelper) getConfig() (*cluserConfig, error) {
	cfg := &cluserConfig{
		Cloud:                       "",
		TenantID:                    "",
		SubscriptionID:              a.subscriptionId,
		ResourceGroup:               a.resourceGroup,
		Location:                    "",
		ClientID:                    a.clientID,
		ClientSecret:                a.clientSecret,
		UseManagedIdentityExtension: true,
		UserAssignedIdentityID:      "",
	}

	var environment azure.Environment
	if cfg.Cloud == "" {
		environment = azure.PublicCloud
	}
	cfg.Environment = environment

	return cfg, nil
}

// getAccessToken retrieves Azure API access token.
func getAccessToken(cfg cluserConfig) (*adal.ServicePrincipalToken, error) {
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

	// Try to retrieve token with MSI.
	if cfg.UseManagedIdentityExtension {
		os.Setenv("MSI_ENDPOINT", "http://dummy")
		defer func() {
			os.Unsetenv("MSI_ENDPOINT")
		}()

		if cfg.UserAssignedIdentityID != "" {
			token, err := adal.NewServicePrincipalTokenFromManagedIdentity(cfg.Environment.ServiceManagementEndpoint, &adal.ManagedIdentityOptions{
				ClientID: cfg.UserAssignedIdentityID,
			})

			if err != nil {
				return nil, fmt.Errorf("failed to create the managed service identity token: %v", err)
			}
			return token, nil
		}

		token, err := adal.NewServicePrincipalTokenFromManagedIdentity(cfg.Environment.ServiceManagementEndpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create the managed service identity token: %v", err)
		}
		return token, nil
	}

	return nil, fmt.Errorf("no credentials provided for Azure API")
}
