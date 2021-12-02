/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package externaldnscontroller

import (
	"context"
	"fmt"

	"reflect"

	"k8s.io/apimachinery/pkg/runtime"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

// ensureExternalCredentialsRequest ensures that the externalDNS credential request exists.
// Returns a boolean if the credential request exists, its current state if it exists
// and an error if it cannot be created or updated.
func (r *reconciler) ensureExternalCredentialsRequest(ctx context.Context, externalDNS *operatorv1alpha1.ExternalDNS) (bool, *cco.CredentialsRequest, error) {
	name := controller.ExternalDNSCredentialsRequestName(externalDNS)

	exists, current, err := r.currentExternalDNSCredentialsRequest(ctx, name)

	if err != nil {
		return false, nil, err
	}

	secretName := types.NamespacedName{
		Name:      controller.SecretFromCloudCredentialsOperator,
		Namespace: r.config.OperatorNamespace,
	}
	desired, err := desiredCredentialsRequest(name, secretName, externalDNS)
	if err != nil {
		return false, nil, err
	}

	if !exists {
		if err := r.createExternalDNSCredentialsRequest(ctx, desired); err != nil {
			return false, nil, err
		}
		return r.currentExternalDNSCredentialsRequest(ctx, name)
	}

	if updated, err := r.updateExternalDNSCredentialsRequest(ctx, current, desired, externalDNS); err != nil {
		return true, current, err
	} else if updated {
		return r.currentExternalDNSCredentialsRequest(ctx, name)
	}

	return true, current, nil
}

// updateExternalDNSClusterRole updates the cluster role with the desired state if the rules differ
func (r *reconciler) updateExternalDNSCredentialsRequest(ctx context.Context, current, desired *cco.CredentialsRequest, externalDNS *operatorv1alpha1.ExternalDNS) (bool, error) {
	var updated *cco.CredentialsRequest
	changed, err := externalDNSCredentialsRequestChanged(current, desired, externalDNS)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	updated = current.DeepCopy()
	updated.Name = desired.Name
	updated.Namespace = desired.Namespace
	updated.Spec = desired.Spec
	if err := r.client.Update(ctx, updated); err != nil {
		return false, err
	}
	r.log.Info("updated externalDNS credential request", "name", updated.Name, "reason", err)
	return true, nil
}

func externalDNSCredentialsRequestChanged(current, desired *cco.CredentialsRequest, externalDNS *operatorv1alpha1.ExternalDNS) (bool, error) {

	if current.Name != desired.Name {
		return true, nil
	}

	if current.Namespace != desired.Namespace {
		return true, nil
	}

	if externalDNS.Spec.Provider.Type == operatorv1alpha1.ProviderTypeAWS {
		codec, _ := cco.NewCodec()
		currentAwsSpec := cco.AWSProviderSpec{}
		err := codec.DecodeProviderSpec(current.Spec.ProviderSpec, &currentAwsSpec)
		if err != nil {
			return false, err
		}

		desiredAwsSpec := cco.AWSProviderSpec{}
		err = codec.DecodeProviderSpec(desired.Spec.ProviderSpec, &desiredAwsSpec)
		if err != nil {
			return false, err
		}

		if !(reflect.DeepEqual(currentAwsSpec, desiredAwsSpec)) {
			return true, nil
		}
	}

	if externalDNS.Spec.Provider.Type == operatorv1alpha1.ProviderTypeAzure {
		codec, _ := cco.NewCodec()
		currentAzureSpec := cco.AzureProviderSpec{}
		err := codec.DecodeProviderSpec(desired.Spec.ProviderSpec, &currentAzureSpec)
		if err != nil {
			return false, err
		}

		desiredAzureSpec := cco.AzureProviderSpec{}
		err = codec.DecodeProviderSpec(current.Spec.ProviderSpec, &desiredAzureSpec)
		if err != nil {
			return false, err
		}

		if !(reflect.DeepEqual(currentAzureSpec, desiredAzureSpec)) {
			return true, nil
		}
	}

	if externalDNS.Spec.Provider.Type == operatorv1alpha1.ProviderTypeGCP {
		codec, _ := cco.NewCodec()
		currentGcpSpec := cco.GCPProviderSpec{}
		err := codec.DecodeProviderSpec(current.Spec.ProviderSpec, &currentGcpSpec)
		if err != nil {
			return false, err
		}

		desiredGCPSpec := cco.GCPProviderSpec{}
		err = codec.DecodeProviderSpec(desired.Spec.ProviderSpec, &desiredGCPSpec)
		if err != nil {
			return false, err
		}

		if !(reflect.DeepEqual(currentGcpSpec, desiredGCPSpec)) {
			return true, nil
		}
	}
	return false, nil
}

func createProviderConfig(externalDNS *operatorv1alpha1.ExternalDNS, codec *cco.ProviderCodec) (*runtime.RawExtension, error) {
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

// desiredCredentialsRequestName returns the desired credentials request definition for externalDNS
func desiredCredentialsRequest(name, secretName types.NamespacedName, externalDNS *operatorv1alpha1.ExternalDNS) (*cco.CredentialsRequest, error) {
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
			ServiceAccountNames: []string{controller.ServiceAccountName},
			SecretRef: k8sv1.ObjectReference{
				Name:      secretName.Name,
				Namespace: secretName.Namespace,
			},
		},
	}

	codec, err := cco.NewCodec()
	if err != nil {
		return nil, err
	}

	providerSpec, err := createProviderConfig(externalDNS, codec)

	if err != nil {
		return nil, err
	}
	credentialsRequest.Spec.ProviderSpec = providerSpec
	return credentialsRequest, nil
}

// currentExternalDNSCredentialsRequest returns true if credentials request exists.
func (r *reconciler) currentExternalDNSCredentialsRequest(ctx context.Context, name types.NamespacedName) (bool, *cco.CredentialsRequest, error) {
	cr := &cco.CredentialsRequest{}
	if err := r.client.Get(ctx, name, cr); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, cr, nil
}

// createExternalDNSCredentialsRequest creates the given credentials request.
func (r *reconciler) createExternalDNSCredentialsRequest(ctx context.Context, desired *cco.CredentialsRequest) error {
	if err := r.client.Create(ctx, desired); err != nil {
		return fmt.Errorf("failed to create externalDNS credentials request %s: %w", desired.Name, err)
	}
	r.log.Info("created externalDNS credentials request", "name", desired.Name)
	return nil
}
