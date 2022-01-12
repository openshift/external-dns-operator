package externaldnscontroller

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	k8sv1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

func TestEnsureCredentialsRequest(t *testing.T) {

	testCases := []struct {
		name                      string
		existingObjects           []runtime.Object
		inputConfig               Config
		inputRequest              ctrl.Request
		expectedResult            reconcile.Result
		expectedEvents            []test.Event
		errExpected               bool
		ocpPlatform               bool
		inputExtDNS               *operatorv1alpha1.ExternalDNS
		expectedCredentialRequest *cco.CredentialsRequest
	}{
		{
			name:                      "Ensure Credential request when no credential requests are there in AWS",
			existingObjects:           []runtime.Object{},
			inputExtDNS:               testAWSExtDNSInstanceWhenOCPRouteSourceWhenAWSCredentialsNotProvided(),
			expectedCredentialRequest: getDesiredCredentialRequest(testAWSExtDNSInstanceWhenOCPRouteSourceWhenAWSCredentialsNotProvided()),
		},
		{
			name:                      "Ensure Credential request when invalid credential requests are there in AWS",
			existingObjects:           []runtime.Object{getUnDesiredCredentialRequest(testAWSExtDNSInstanceWhenOCPRouteSourceWhenAWSCredentialsNotProvided())},
			inputExtDNS:               testAWSExtDNSInstanceWhenOCPRouteSourceWhenAWSCredentialsNotProvided(),
			expectedCredentialRequest: getDesiredCredentialRequest(testAWSExtDNSInstanceWhenOCPRouteSourceWhenAWSCredentialsNotProvided()),
		},
		{
			name:                      "Ensure Credential request when no credential requests are there in Azure",
			existingObjects:           []runtime.Object{},
			inputExtDNS:               testAzureExtDNSInstanceWhenSourceIsOCPRoute(),
			expectedCredentialRequest: getDesiredCredentialRequest(testAzureExtDNSInstanceWhenSourceIsOCPRoute()),
		},

		{
			name:                      "Ensure Credential request when invalid credential requests are there in Azure",
			existingObjects:           []runtime.Object{getUnDesiredCredentialRequest(testAzureExtDNSInstanceWhenSourceIsOCPRoute())},
			inputExtDNS:               testAzureExtDNSInstanceWhenSourceIsOCPRoute(),
			expectedCredentialRequest: getDesiredCredentialRequest(testAzureExtDNSInstanceWhenSourceIsOCPRoute()),
		},
		{
			name:                      "Ensure Credential request when no credential requests are there in GCP",
			existingObjects:           []runtime.Object{},
			inputExtDNS:               testGCPExtDNSInstanceWhenSourceIsOCPRoute(),
			expectedCredentialRequest: getDesiredCredentialRequest(testGCPExtDNSInstanceWhenSourceIsOCPRoute()),
		},
		{
			name:                      "Ensure Credential request when invalid credential requests are there in GCP",
			existingObjects:           []runtime.Object{getUnDesiredCredentialRequest(testGCPExtDNSInstanceWhenSourceIsOCPRoute())},
			inputExtDNS:               testGCPExtDNSInstanceWhenSourceIsOCPRoute(),
			expectedCredentialRequest: getDesiredCredentialRequest(testGCPExtDNSInstanceWhenSourceIsOCPRoute()),
		},
	}
	for _, tc := range testCases {
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()

		r := &reconciler{
			client: cl,
			scheme: test.Scheme,
			config: testConfig(),
			log:    zap.New(zap.UseDevMode(true)),
		}

		t.Run(tc.name, func(t *testing.T) {

			exists, got, err := r.ensureExternalCredentialsRequest(context.TODO(), tc.inputExtDNS)

			if err != nil {
				t.Log("Error while ensuring Credentials request")
			}

			if !exists {
				t.Errorf("Credential request does not exist")
			}

			if got.Name != tc.expectedCredentialRequest.Name {
				t.Errorf("Got name %v  but expected name is %v", got.Name, tc.expectedCredentialRequest.Name)
			}

			if got.Namespace != tc.expectedCredentialRequest.Namespace {
				t.Errorf("Got namespace %v  but expected namespace is %v", got.Namespace, tc.expectedCredentialRequest.Namespace)
			}

			if tc.inputExtDNS.Spec.Provider.Type == operatorv1alpha1.ProviderTypeAWS {
				gotDecodedAWSSpec, expectedAWSSpec, err := decodeAWSProviderSpec(*got, *tc.expectedCredentialRequest)
				if err != nil {
					t.Errorf("Not able to decode AWS Provider Spec because of %v", err)
				}
				if !reflect.DeepEqual(gotDecodedAWSSpec, expectedAWSSpec) {
					t.Errorf("Got CredentialRequest Spec %v\n  but expected CredentialRequest Spec is %v", gotDecodedAWSSpec, expectedAWSSpec)
				}

			}

			if tc.inputExtDNS.Spec.Provider.Type == operatorv1alpha1.ProviderTypeGCP {
				gotDecodedGCPSpec, expectedGCPSpec, err := decodeGCPProviderSpec(*got, *tc.expectedCredentialRequest)
				if err != nil {
					t.Errorf("Not able to decode GCP Provider Spec because of %v", err)
				}
				if !reflect.DeepEqual(gotDecodedGCPSpec, expectedGCPSpec) {
					t.Errorf("Got CredentialRequest Spec %v\n  but expected CredentialRequest Spec is %v", gotDecodedGCPSpec, expectedGCPSpec)
				}

			}

			if tc.inputExtDNS.Spec.Provider.Type == operatorv1alpha1.ProviderTypeAzure {
				gotDecodedAzureSpec, expectedAzureSpec, err := decodeAzureProviderSpec(*got, *tc.expectedCredentialRequest)
				if err != nil {
					t.Errorf("Not able to decode Azure Provider Spec because of %v", err)
				}
				if !reflect.DeepEqual(gotDecodedAzureSpec, expectedAzureSpec) {
					t.Errorf("Got CredentialRequest Spec %v\n  but expected CredentialRequest Spec is %v", gotDecodedAzureSpec, expectedAzureSpec)
				}

			}

		})

	}

}

func decodeGCPProviderSpec(gotCrededentialrequest, expectedCredentialRequest cco.CredentialsRequest) (gotDecodedGCPSpec, expectedDecodedGCPSpec cco.GCPProviderSpec, err error) {

	codec, _ := cco.NewCodec()
	gotDecodedGCPSpec = cco.GCPProviderSpec{}
	err = codec.DecodeProviderSpec(gotCrededentialrequest.Spec.ProviderSpec, &gotDecodedGCPSpec)
	if err != nil {
		return gotDecodedGCPSpec, cco.GCPProviderSpec{}, err
	}

	expectedDecodedGCPSpec = cco.GCPProviderSpec{}
	err = codec.DecodeProviderSpec(expectedCredentialRequest.Spec.ProviderSpec, &expectedDecodedGCPSpec)
	if err != nil {
		return gotDecodedGCPSpec, expectedDecodedGCPSpec, err
	}
	return gotDecodedGCPSpec, expectedDecodedGCPSpec, err
}

func decodeAWSProviderSpec(gotCrededentialrequest, expectedCredentialRequest cco.CredentialsRequest) (gotDecodedAWSSpec, expectedDecodedAWSSpec cco.AWSProviderSpec, err error) {

	codec, _ := cco.NewCodec()
	gotDecodedAWSSpec = cco.AWSProviderSpec{}
	err = codec.DecodeProviderSpec(gotCrededentialrequest.Spec.ProviderSpec, &gotDecodedAWSSpec)
	if err != nil {
		return gotDecodedAWSSpec, cco.AWSProviderSpec{}, err
	}

	expectedDecodedAWSSpec = cco.AWSProviderSpec{}
	err = codec.DecodeProviderSpec(expectedCredentialRequest.Spec.ProviderSpec, &expectedDecodedAWSSpec)
	if err != nil {
		return gotDecodedAWSSpec, expectedDecodedAWSSpec, err
	}
	return gotDecodedAWSSpec, expectedDecodedAWSSpec, err
}

func decodeAzureProviderSpec(gotCrededentialrequest, expectedCredentialRequest cco.CredentialsRequest) (gotDecodedAzureSpec, expectedDecodedAzureSpec cco.AzureProviderSpec, err error) {

	codec, _ := cco.NewCodec()
	gotDecodedAzureSpec = cco.AzureProviderSpec{}
	err = codec.DecodeProviderSpec(gotCrededentialrequest.Spec.ProviderSpec, &gotDecodedAzureSpec)
	if err != nil {
		return gotDecodedAzureSpec, cco.AzureProviderSpec{}, err
	}

	expectedDecodedAzureSpec = cco.AzureProviderSpec{}
	err = codec.DecodeProviderSpec(expectedCredentialRequest.Spec.ProviderSpec, &expectedDecodedAzureSpec)
	if err != nil {
		return gotDecodedAzureSpec, expectedDecodedAzureSpec, err
	}
	return gotDecodedAzureSpec, expectedDecodedAzureSpec, err
}

func testGCPExtDNSInstanceWhenSourceIsOCPRoute() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeGCP,
		GCP:  &operatorv1alpha1.ExternalDNSGCPProviderOptions{},
	}
	return extDNS
}

func testAzureExtDNSInstanceWhenSourceIsOCPRoute() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type:  operatorv1alpha1.ProviderTypeAzure,
		Azure: &operatorv1alpha1.ExternalDNSAzureProviderOptions{},
	}
	return extDNS
}

func getUnDesiredCredentialRequest(externalDNS *operatorv1alpha1.ExternalDNS) *cco.CredentialsRequest {
	var t *testing.T
	name := controller.ExternalDNSCredentialsRequestName(externalDNS)
	credentialsRequest := &cco.CredentialsRequest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CredentialsRequest",
			APIVersion: "cloudcredential.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: cco.CredentialsRequestSpec{
			ServiceAccountNames: []string{"external-dns-operator"},
			SecretRef: k8sv1.ObjectReference{
				Name:      "invalid-externaldns-cloud-credentials",
				Namespace: "external-dns-operator",
			},
		},
	}

	codec, err := cco.NewCodec()
	if err != nil {
		t.Log("error encoding:", err)
		return nil
	}
	providerSpec, err := createUnDesiredProviderConfig(externalDNS, codec)
	if err != nil {
		t.Log("error encoding:", err)
		return nil
	}

	credentialsRequest.Spec.ProviderSpec = providerSpec

	return credentialsRequest

}

func createUnDesiredProviderConfig(externalDNS *operatorv1alpha1.ExternalDNS, codec *cco.ProviderCodec) (*runtime.RawExtension, error) {
	switch externalDNS.Spec.Provider.Type {
	case operatorv1alpha1.ProviderTypeAWS:
		return codec.EncodeProviderSpec(
			&cco.AWSProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "AWSProviderSpec",
				},
				StatementEntries: []cco.StatementEntry{
					{
						Effect: "Allow",
						Action: []string{
							"route53:ChangeResourceRecordSets",
						},
						Resource: "arn:aws:route53:::hostedzone/*",
					},
					{
						Effect:   "Allow",
						Action:   []string{},
						Resource: "*",
					},
				},
			})
	case operatorv1alpha1.ProviderTypeGCP:
		return codec.EncodeProviderSpec(
			&cco.GCPProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "GCPProviderSpec",
				},
				PredefinedRoles: []string{
					"Invalid roles/dns.admin",
				},
			})

	case operatorv1alpha1.ProviderTypeAzure:
		return codec.EncodeProviderSpec(
			&cco.AzureProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "AzureProviderSpec",
				},
				RoleBindings: []cco.RoleBinding{
					{Role: "Invalid Contributor"},
				},
			})
	}
	return nil, nil
}

func getDesiredCredentialRequest(externalDNS *operatorv1alpha1.ExternalDNS) *cco.CredentialsRequest {
	name := controller.ExternalDNSCredentialsRequestName(externalDNS)
	credentialsRequest := &cco.CredentialsRequest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CredentialsRequest",
			APIVersion: "cloudcredential.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: cco.CredentialsRequestSpec{
			ServiceAccountNames: []string{"external-dns-operator", controller.ExternalDNSResourceName(externalDNS)},
			SecretRef: k8sv1.ObjectReference{
				Name:      "externaldns-cloud-credentials",
				Namespace: "external-dns-operator",
			},
		},
	}

	codec, err := cco.NewCodec()
	if err != nil {
		fmt.Printf("error encoding: %v", err)
		return nil
	}
	providerSpec, err := createDesiredProviderConfig(externalDNS, codec)
	if err != nil {
		fmt.Printf("error encoding: %v", err)
		return nil
	}

	credentialsRequest.Spec.ProviderSpec = providerSpec

	return credentialsRequest

}

func createDesiredProviderConfig(externalDNS *operatorv1alpha1.ExternalDNS, codec *cco.ProviderCodec) (*runtime.RawExtension, error) {
	switch externalDNS.Spec.Provider.Type {
	case operatorv1alpha1.ProviderTypeAWS:
		return codec.EncodeProviderSpec(
			&cco.AWSProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "AWSProviderSpec",
				},
				StatementEntries: []cco.StatementEntry{
					{
						Effect: "Allow",
						Action: []string{
							"route53:ChangeResourceRecordSets",
						},
						Resource: "arn:aws:route53:::hostedzone/*",
					},
					{
						Effect: "Allow",
						Action: []string{
							"route53:ListHostedZones",
							"route53:ListResourceRecordSets",
							"tag:GetResources",
						},
						Resource: "*",
					},
				},
			})
	case operatorv1alpha1.ProviderTypeGCP:
		return codec.EncodeProviderSpec(
			&cco.GCPProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "GCPProviderSpec",
				},
				PredefinedRoles: []string{
					"roles/dns.admin",
				},
			})

	case operatorv1alpha1.ProviderTypeAzure:
		return codec.EncodeProviderSpec(
			&cco.AzureProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "AzureProviderSpec",
				},
				RoleBindings: []cco.RoleBinding{
					{Role: "Contributor"},
				},
			})
	}
	return nil, nil
}

func testAWSExtDNSInstanceWhenOCPRouteSourceWhenAWSCredentialsNotProvided() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
		AWS:  &operatorv1alpha1.ExternalDNSAWSProviderOptions{},
	}
	return extDNS
}

func testExtDNSInstanceforOCPRouteSource() *operatorv1alpha1.ExternalDNS {
	return &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeRoute,
				},
			},
			Zones: []string{"public-zone"},
		},
	}
}
