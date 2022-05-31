package externaldnscontroller

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

func TestEnsureCredentialsRequest(t *testing.T) {

	testCases := []struct {
		name                      string
		existingObjects           []runtime.Object
		inputExtDNS               *operatorv1beta1.ExternalDNS
		inputPlatformStatus       *configv1.PlatformStatus
		expectedCredentialRequest *cco.CredentialsRequest
	}{
		{
			name:                      "Create credentials request from scratch in AWS",
			existingObjects:           []runtime.Object{},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAWS().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build(),
		},
		{
			name:                      "Update drifted credentials request in AWS. Provider spec",
			existingObjects:           []runtime.Object{newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(undesiredAWSProviderSpec).build()},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAWS().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build(),
		},
		{
			name:                      "Update drifted credentials request in AWS. Secret name",
			existingObjects:           []runtime.Object{newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build()},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAWS().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build(),
		},
		{
			name:                      "Update drifted credentials request in AWS. Secret namespace",
			existingObjects:           []runtime.Object{newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "wrong-ns").withProviderSpec(desiredAWSProviderSpec).build()},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAWS().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build(),
		},
		{
			name:                      "Update drifted credentials request in AWS. Service accounts",
			existingObjects:           []runtime.Object{newCredentialsRequest("externaldns-credentials-request-aws").withSAs("wrong-sa").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build()},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAWS().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpec).build(),
		},
		{
			name:            "Create credentials request from scratch in AWS Gov",
			existingObjects: []runtime.Object{},
			inputExtDNS:     test.NewExternalDNS(test.Name).WithAWS().WithRouteSource().WithZones("public-zone").Build(),
			inputPlatformStatus: &configv1.PlatformStatus{
				Type: configv1.AWSPlatformType,
				AWS: &configv1.AWSPlatformStatus{
					Region: "us-gov-west-1",
				},
			},
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-aws").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAWSProviderSpecGovARN).build(),
		},
		{
			name:                      "Create credentials request from scratch in Azure",
			existingObjects:           []runtime.Object{},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAzure().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-azure").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAzureProviderSpec).build(),
		},
		{
			name:                      "Update drifted credentials request in Azure. Provider spec",
			existingObjects:           []runtime.Object{newCredentialsRequest("externaldns-credentials-request-azure").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(undesiredAzureProviderSpec).build()},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithAzure().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-azure").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredAzureProviderSpec).build(),
		},
		{
			name:                      "Create credentials request from scratch in GCP",
			existingObjects:           []runtime.Object{},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithGCP().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-gcp").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredGCPProviderSpec).build(),
		},
		{
			name:                      "Update drifted credentials request in GCP. Provider spec",
			existingObjects:           []runtime.Object{newCredentialsRequest("externaldns-credentials-request-gcp").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(undesiredGCPProviderSpec).build()},
			inputExtDNS:               test.NewExternalDNS(test.Name).WithGCP().WithRouteSource().WithZones("public-zone").Build(),
			expectedCredentialRequest: newCredentialsRequest("externaldns-credentials-request-gcp").withSAs("external-dns-operator").withSecret("externaldns-cloud-credentials", "external-dns-operator").withProviderSpec(desiredGCPProviderSpec).build(),
		},
	}
	for _, tc := range testCases {
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()

		r := &reconciler{
			client: cl,
			scheme: test.Scheme,
			config: Config{
				Namespace:         test.OperandNamespace,
				Image:             test.OperandImage,
				OperatorNamespace: test.OperatorNamespace,
				PlatformStatus:    tc.inputPlatformStatus,
			},
			log: zap.New(zap.UseDevMode(true)),
		}

		t.Run(tc.name, func(t *testing.T) {
			exists, got, err := r.ensureExternalCredentialsRequest(context.TODO(), tc.inputExtDNS)
			if err != nil {
				t.Log("Error while ensuring credentials request")
			}

			if !exists {
				t.Errorf("Credentials request does not exist")
			}

			// check all but the provider spec

			ignoreCROpts := cmpopts.IgnoreFields(cco.CredentialsRequest{}, "ResourceVersion")
			ignoreSpecOpts := cmpopts.IgnoreFields(cco.CredentialsRequestSpec{}, "ProviderSpec")
			if diff := cmp.Diff(*tc.expectedCredentialRequest, *got, ignoreCROpts, ignoreSpecOpts); diff != "" {
				t.Errorf("Got unexpected credentials request (-want +got):\n%s", diff)
			}

			// check the provider spec

			if tc.inputExtDNS.Spec.Provider.Type == operatorv1beta1.ProviderTypeAWS {
				gotDecodedAWSSpec, expectedAWSSpec, err := decodeAWSProviderSpec(*got, *tc.expectedCredentialRequest)
				if err != nil {
					t.Errorf("Not able to decode AWS Provider Spec because of %v", err)
				}
				if diff := cmp.Diff(expectedAWSSpec, gotDecodedAWSSpec); diff != "" {
					t.Errorf("Got unexpected provider spec (-want +got):\n%s", diff)
				}
			}

			if tc.inputExtDNS.Spec.Provider.Type == operatorv1beta1.ProviderTypeGCP {
				gotDecodedGCPSpec, expectedGCPSpec, err := decodeGCPProviderSpec(*got, *tc.expectedCredentialRequest)
				if err != nil {
					t.Errorf("Not able to decode GCP Provider Spec because of %v", err)
				}
				if diff := cmp.Diff(expectedGCPSpec, gotDecodedGCPSpec); diff != "" {
					t.Errorf("Got unexpected provider spec (-want +got):\n%s", diff)
				}
			}

			if tc.inputExtDNS.Spec.Provider.Type == operatorv1beta1.ProviderTypeAzure {
				gotDecodedAzureSpec, expectedAzureSpec, err := decodeAzureProviderSpec(*got, *tc.expectedCredentialRequest)
				if err != nil {
					t.Errorf("Not able to decode Azure Provider Spec because of %v", err)
				}
				if diff := cmp.Diff(expectedAzureSpec, gotDecodedAzureSpec); diff != "" {
					t.Errorf("Got unexpected provider spec (-want +got):\n%s", diff)
				}
			}
		})
	}
}

//
// Helper functions
//

func decodeGCPProviderSpec(gotCredentialRequest, expectedCredentialRequest cco.CredentialsRequest) (gotDecodedGCPSpec, expectedDecodedGCPSpec cco.GCPProviderSpec, err error) {

	codec, _ := cco.NewCodec()
	gotDecodedGCPSpec = cco.GCPProviderSpec{}
	err = codec.DecodeProviderSpec(gotCredentialRequest.Spec.ProviderSpec, &gotDecodedGCPSpec)
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

func decodeAWSProviderSpec(gotCredentialRequest, expectedCredentialRequest cco.CredentialsRequest) (gotDecodedAWSSpec, expectedDecodedAWSSpec cco.AWSProviderSpec, err error) {

	codec, _ := cco.NewCodec()
	gotDecodedAWSSpec = cco.AWSProviderSpec{}
	err = codec.DecodeProviderSpec(gotCredentialRequest.Spec.ProviderSpec, &gotDecodedAWSSpec)
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

func decodeAzureProviderSpec(gotCredentialRequest, expectedCredentialRequest cco.CredentialsRequest) (gotDecodedAzureSpec, expectedDecodedAzureSpec cco.AzureProviderSpec, err error) {

	codec, _ := cco.NewCodec()
	gotDecodedAzureSpec = cco.AzureProviderSpec{}
	err = codec.DecodeProviderSpec(gotCredentialRequest.Spec.ProviderSpec, &gotDecodedAzureSpec)
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

//
// Test CredentialsRequest CRs
//

type credentialsRequestBuilder struct {
	req *cco.CredentialsRequest
}

func newCredentialsRequest(name string) *credentialsRequestBuilder {
	return &credentialsRequestBuilder{
		req: &cco.CredentialsRequest{
			TypeMeta: metav1.TypeMeta{
				Kind:       "CredentialsRequest",
				APIVersion: "cloudcredential.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "openshift-cloud-credential-operator",
			},
			Spec: cco.CredentialsRequestSpec{},
		},
	}
}

func (b *credentialsRequestBuilder) withSAs(sa ...string) *credentialsRequestBuilder {
	b.req.Spec.ServiceAccountNames = sa
	return b
}

func (b *credentialsRequestBuilder) withSecret(name, ns string) *credentialsRequestBuilder {
	b.req.Spec.SecretRef = corev1.ObjectReference{
		Name:      name,
		Namespace: ns,
	}
	return b
}

func (b *credentialsRequestBuilder) withProviderSpec(makeProviderSpecFn func() runtime.Object) *credentialsRequestBuilder {
	codec, _ := cco.NewCodec()
	providerSpec, _ := codec.EncodeProviderSpec(makeProviderSpecFn())
	b.req.Spec.ProviderSpec = providerSpec
	return b
}

func (b *credentialsRequestBuilder) build() *cco.CredentialsRequest {
	return b.req
}

func desiredAWSProviderSpec() runtime.Object {
	return &cco.AWSProviderSpec{
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
	}
}

func desiredAWSProviderSpecGovARN() runtime.Object {
	return &cco.AWSProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind: "AWSProviderSpec",
		},
		StatementEntries: []cco.StatementEntry{
			{
				Effect: "Allow",
				Action: []string{
					"route53:ChangeResourceRecordSets",
				},
				Resource: "arn:aws-us-gov:route53:::hostedzone/*",
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
	}
}

func undesiredAWSProviderSpec() runtime.Object {
	return &cco.AWSProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind: "AWSProviderSpec",
		},
		StatementEntries: []cco.StatementEntry{
			{
				Effect: "Allow",
				Action: []string{
					"route53:ChangeResourceRecordSets",
				},
				Resource: "arn:aws:route53:::hostedzone/wrongzone",
			},
			{
				Effect:   "Allow",
				Action:   []string{},
				Resource: "*",
			},
		},
	}
}

func desiredGCPProviderSpec() runtime.Object {
	return &cco.GCPProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind: "GCPProviderSpec",
		},
		PredefinedRoles: []string{
			"roles/dns.admin",
		},
	}
}

func undesiredGCPProviderSpec() runtime.Object {
	return &cco.GCPProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind: "GCPProviderSpec",
		},
		PredefinedRoles: []string{
			"roles/dns.reader",
		},
	}
}

func desiredAzureProviderSpec() runtime.Object {
	return &cco.AzureProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind: "AzureProviderSpec",
		},
		RoleBindings: []cco.RoleBinding{
			{Role: "Contributor"},
		},
	}
}

func undesiredAzureProviderSpec() runtime.Object {
	return &cco.AzureProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind: "AzureProviderSpec",
		},
		RoleBindings: []cco.RoleBinding{
			{Role: "Nobody"},
		},
	}
}
