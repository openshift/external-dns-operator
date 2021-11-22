// +build e2e

package e2e

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2017-10-01/dns"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strconv"

	configv1 "github.com/openshift/api/config/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	_ "github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2017-10-01/dns"

	"github.com/Azure/azure-sdk-for-go/services/msi/mgmt/2018-11-30/msi"
)

const(
	RESOURCE_GROUP="azure_resourcegroup"
    SUBSCIPTION_ID = "azure_subscription_id"
	TANENT_ID = "azure_tenant_id"
)
type azureTestHelper struct {
	tenantId string
	subscriptionId string
	resourceGroup string
	useManagedIdentityExtension bool

}

func newAzureHelper(kubeClient client.Client) (providerTestHelper, error) {
	tenantId,subscriptionId,resourceGroup , err := fetchCredentials(kubeClient)
	if err != nil{
		return nil, err
	}
	return &azureTestHelper{
		tenantId:                    tenantId,
		subscriptionId:              subscriptionId,
		resourceGroup:               resourceGroup,
		useManagedIdentityExtension: true,
	}, nil
}


func  fetchCredentials(kubeClient client.Client) (tenantId,subscriptionId,resourceGroup string, err error ){
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      "azure-credentials",
		Namespace: "kube-system",
	}
	if err = kubeClient.Get(context.Background(), secretName, secret); err != nil {
		return "", "","", fmt.Errorf("failed to get credentials secret %s: %w", secretName.Name, err)
	}
	return string(secret.Data[TANENT_ID]), string(secret.Data[SUBSCIPTION_ID]),string(secret.Data[RESOURCE_GROUP]), nil
}

func (a *azureTestHelper) makeCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("azure-config-file-%s", randomString(16)),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tenantId":     []byte(a.tenantId),
			"subscriptionId": []byte(a.subscriptionId),
			"resourceGroup": []byte(a.resourceGroup),
			"useManagedIdentityExtension": []byte(strconv.FormatBool(a.useManagedIdentityExtension)),
		},
	}
}

func (a *azureTestHelper) platform() string {
	return string(configv1.AzurePlatformType)
}

func (a *azureTestHelper) ensureHostedZone(rootDomain string) (string, []string, error) {

	msiClient := msi.NewUserAssignedIdentitiesClient(a.subscriptionId)
	fmt.Printf(msiClient.BaseURI)
	zone := dns.NewZonesClient(a.subscriptionId)

	z, err := zone.CreateOrUpdate(context.TODO(),a.resourceGroup,"example-test.info",dns.Zone{},"","")
	if err != nil {
		return "", []string{}, err
	}
	var zoneID string
	zoneID = *z.ID
	nameservers := append(*z.ZoneProperties.NameServers)
	fmt.Printf("ZoneID : %s, Name Servers : %v",zoneID, nameservers)
	return zoneID,nameservers, nil
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