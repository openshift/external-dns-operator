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

package credentials_secret

import (
	"context"

	"encoding/json"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

// ensureCredentialsSecret ensures that the source secret has been copied to the operand namespace.
// Returns the destination secret, a boolean if the destination secret exists, and an error when relevant.
func (r *reconciler) ensureCredentialsSecret(ctx context.Context, sourceName types.NamespacedName, extDNS *operatorv1alpha1.ExternalDNS, fromCR bool) (bool, *corev1.Secret, error) {
	// get the source secret
	sourceExists, source, err := r.currentCredentialsSecret(ctx, sourceName)
	if err != nil {
		return false, nil, err
	} else if !sourceExists {
		return false, nil, nil
	}

	destName := controller.ExternalDNSDestCredentialsSecretName(r.config.TargetNamespace, extDNS.Name)
	// desired is created from source
	desired := desiredCredentialsSecret(source, destName, extDNS, r.config.IsOpenShift, fromCR)

	if err := controllerutil.SetControllerReference(extDNS, desired, r.scheme); err != nil {
		return false, nil, fmt.Errorf("failed to set the controller reference for credentials secret: %w", err)
	}

	// check if the destination secret exists
	destExists, dest, err := r.currentCredentialsSecret(ctx, destName)
	if err != nil {
		return false, nil, err
	}
	if !destExists {
		// destination secret doesn't exist, create it
		if err := r.createCredentialsSecret(ctx, desired); err != nil {
			return false, nil, err
		}
		return r.currentCredentialsSecret(ctx, destName)
	}

	// destination secret exists, try to update it with source data
	if updated, err := r.updateCredentialsSecret(ctx, dest, desired); err != nil {
		return true, dest, err
	} else if updated {
		return r.currentCredentialsSecret(ctx, destName)
	}

	return true, dest, nil
}

// currentCredentialsSecret returns the definition of the secret object with the given name.
func (r *reconciler) currentCredentialsSecret(ctx context.Context, name types.NamespacedName) (bool, *corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := r.client.Get(ctx, name, secret); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, secret, nil
}

// createCredentialsSecret creates the given secret using the reconciler's client.
func (r *reconciler) createCredentialsSecret(ctx context.Context, secret *corev1.Secret) error {
	if err := r.client.Create(ctx, secret); err != nil {
		return err
	}
	r.log.Info("created secret", "namespace", secret.Namespace, "name", secret.Name)
	return nil
}

// desiredCredentialsSecret returns the desired destination secret.
func desiredCredentialsSecret(sourceSecret *corev1.Secret, destName types.NamespacedName, extDNS *operatorv1alpha1.ExternalDNS, isOpenShift, fromCR bool) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      destName.Name,
			Namespace: destName.Namespace,
		},
		Data: map[string][]byte{},
	}

	if isOpenShift && !fromCR {
		// secret came from CCO: use CCO fields
		switch extDNS.Spec.Provider.Type {
		case operatorv1alpha1.ProviderTypeGCP:
			secret.Data["gcp-credentials.json"] = sourceSecret.Data["service_account.json"]
		case operatorv1alpha1.ProviderTypeAzure:
			azure_map := map[string]string{
				"aadClientId":     string(sourceSecret.Data["azure_client_id"]),
				"aadClientSecret": string(sourceSecret.Data["azure_client_secret"]),
				"resourceGroup":   string(sourceSecret.Data["azure_resourcegroup"]),
				"subscriptionId":  string(sourceSecret.Data["azure_subscription_id"]),
				"tenantId":        string(sourceSecret.Data["azure_tenant_id"]),
			}
			azureMarshalledJson, _ := json.Marshal(azure_map)
			secret.Data["azure.json"] = azureMarshalledJson
		case operatorv1alpha1.ProviderTypeAWS:
			secret.Data = sourceSecret.Data
		}
	} else {
		secret.Data = sourceSecret.Data
	}

	return secret
}

// updateCredentialsSecret updates the destination secret with the desired content if update is needed.
// Returns a Boolean indicating whether the secret was updated, and an error value.
func (r *reconciler) updateCredentialsSecret(ctx context.Context, current, desired *corev1.Secret) (bool, error) {
	if secretsEqual(current, desired) {
		return false, nil
	}
	updated := current.DeepCopy()
	updated.Data = desired.Data
	if err := r.client.Update(ctx, updated); err != nil {
		return false, err
	}
	r.log.Info("updated secret", "namespace", updated.Namespace, "name", updated.Name)
	return true, nil
}

// secretsEqual compares two secrets. Returns true if
// the secrets should be considered equal for the purpose of determining
// whether an update is necessary, false otherwise.
func secretsEqual(a, b *corev1.Secret) bool {
	return reflect.DeepEqual(a.Data, b.Data)
}
