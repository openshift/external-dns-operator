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

	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	"github.com/openshift/external-dns-operator/pkg/utils"
)

// ensureExternalCredentialsRequest ensures that the externalDNS credential request exists.
// Returns a boolean if the credential request exists, its current state if it exists
// and an error if it cannot be created or updated.
func (r *reconciler) ensureExternalCredentialsRequest(ctx context.Context, externalDNS *operatorv1beta1.ExternalDNS) (bool, *cco.CredentialsRequest, error) {
	name := controller.ExternalDNSCredentialsRequestName(externalDNS)

	exists, current, err := r.currentExternalDNSCredentialsRequest(ctx, name)

	if err != nil {
		return false, nil, err
	}

	secretName := types.NamespacedName{
		Name:      controller.SecretFromCloudCredentialsOperator,
		Namespace: r.config.OperatorNamespace,
	}
	desired, err := desiredCredentialsRequest(name, secretName, externalDNS, r.config.PlatformStatus)
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

// updateExternalDNSClusterRole updates the cluster role with the desired state if the rules differ
func (r *reconciler) updateExternalDNSCredentialsRequest(ctx context.Context, current, desired *cco.CredentialsRequest, externalDNS *operatorv1beta1.ExternalDNS) (bool, error) {
	updated := current.DeepCopy()
	changed, err := externalDNSCredentialsRequestChanged(current, desired, updated, externalDNS)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	if err := r.client.Update(ctx, updated); err != nil {
		return false, err
	}
	r.log.Info("updated externalDNS credentials request", "name", updated.Name, "namespace", updated.Namespace)
	return true, nil
}

// desiredCredentialsRequestName returns the desired credentials request definition for externalDNS
func desiredCredentialsRequest(name, secretName types.NamespacedName, externalDNS *operatorv1beta1.ExternalDNS, platformStatus *configv1.PlatformStatus) (*cco.CredentialsRequest, error) {
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
			SecretRef: corev1.ObjectReference{
				Name:      secretName.Name,
				Namespace: secretName.Namespace,
			},
		},
	}

	codec, err := cco.NewCodec()
	if err != nil {
		return nil, err
	}

	providerSpec, err := createProviderConfig(externalDNS, platformStatus, codec)

	if err != nil {
		return nil, err
	}
	credentialsRequest.Spec.ProviderSpec = providerSpec
	return credentialsRequest, nil
}

func externalDNSCredentialsRequestChanged(current, desired, updated *cco.CredentialsRequest, externalDNS *operatorv1beta1.ExternalDNS) (bool, error) {
	changed := false

	if !equalStringSliceContent(desired.Spec.ServiceAccountNames, current.Spec.ServiceAccountNames) {
		updated.Spec.ServiceAccountNames = desired.Spec.ServiceAccountNames
		changed = true
	}

	if current.Spec.SecretRef.Name != desired.Spec.SecretRef.Name {
		updated.Spec.SecretRef.Name = desired.Spec.SecretRef.Name
		changed = true
	}

	if current.Spec.SecretRef.Namespace != desired.Spec.SecretRef.Namespace {
		updated.Spec.SecretRef.Namespace = desired.Spec.SecretRef.Namespace
		changed = true
	}

	switch externalDNS.Spec.Provider.Type {
	case operatorv1beta1.ProviderTypeAWS:
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
			updated.Spec.ProviderSpec = desired.Spec.ProviderSpec
			changed = true
		}
	case operatorv1beta1.ProviderTypeAzure:
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
			updated.Spec.ProviderSpec = desired.Spec.ProviderSpec
			changed = true
		}
	case operatorv1beta1.ProviderTypeGCP:
		codec, _ := cco.NewCodec()
		currentGCPSpec := cco.GCPProviderSpec{}
		err := codec.DecodeProviderSpec(current.Spec.ProviderSpec, &currentGCPSpec)
		if err != nil {
			return false, err
		}

		desiredGCPSpec := cco.GCPProviderSpec{}
		err = codec.DecodeProviderSpec(desired.Spec.ProviderSpec, &desiredGCPSpec)
		if err != nil {
			return false, err
		}

		if !(reflect.DeepEqual(currentGCPSpec, desiredGCPSpec)) {
			updated.Spec.ProviderSpec = desired.Spec.ProviderSpec
			changed = true
		}
	}

	return changed, nil
}

func createProviderConfig(externalDNS *operatorv1beta1.ExternalDNS, platformStatus *configv1.PlatformStatus, codec *cco.ProviderCodec) (*runtime.RawExtension, error) {
	switch externalDNS.Spec.Provider.Type {
	case operatorv1beta1.ProviderTypeAWS:
		region := ""
		if platformStatus != nil && platformStatus.Type == configv1.AWSPlatformType && platformStatus.AWS != nil {
			region = platformStatus.AWS.Region
		}
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
						Resource: arnPrefix(region) + ":route53:::hostedzone/*",
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
	case operatorv1beta1.ProviderTypeGCP:
		return codec.EncodeProviderSpec(
			&cco.GCPProviderSpec{
				TypeMeta: metav1.TypeMeta{
					Kind: "GCPProviderSpec",
				},
				PredefinedRoles: []string{
					"roles/dns.admin",
				},
			})

	case operatorv1beta1.ProviderTypeAzure:
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

func arnPrefix(region string) string {
	if utils.IsUSGovAWSRegion(region) {
		return "arn:aws-us-gov"
	}
	return "arn:aws"
}
