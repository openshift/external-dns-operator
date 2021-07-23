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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

// ensureExternalDNSServiceAccount ensures that the externalDNS service account exists.
func (r *reconciler) ensureExternalDNSServiceAccount(ctx context.Context, namespace string, externalDNS *operatorv1alpha1.ExternalDNS) (bool, *corev1.ServiceAccount, error) {
	nsName := types.NamespacedName{Namespace: namespace, Name: controller.ExternalDNSResourceName(externalDNS)}

	desired := desiredExternalDNSServiceAccount(namespace, externalDNS)

	exist, current, err := r.currentExternalDNSServiceAccount(ctx, nsName)
	if err != nil {
		return false, nil, err
	}

	if !exist {
		if err := r.createExternalDNSServiceAccount(ctx, desired); err != nil {
			return false, nil, err
		}
		return r.currentExternalDNSServiceAccount(ctx, nsName)
	}

	return true, current, nil
}

// currentExternalDNSServiceAccount gets the current externalDNS service account resource.
func (r *reconciler) currentExternalDNSServiceAccount(ctx context.Context, nsName types.NamespacedName) (bool, *corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{}
	if err := r.client.Get(ctx, nsName, sa); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, sa, nil
}

// desiredExternalDNSServiceAccount returns the desired serivce account resource.
func desiredExternalDNSServiceAccount(namespace string, externalDNS *operatorv1alpha1.ExternalDNS) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      controller.ExternalDNSResourceName(externalDNS),
		},
	}
}

// createExternalDNSServiceAccount creates the given service account using the reconciler's client.
func (r *reconciler) createExternalDNSServiceAccount(ctx context.Context, sa *corev1.ServiceAccount) error {
	if err := r.client.Create(ctx, sa); err != nil {
		return fmt.Errorf("failed to create externalDNS service account %s/%s: %w", sa.Namespace, sa.Name, err)
	}

	r.log.Info("created externalDNS service account", "namespace", sa.Namespace, "name", sa.Name)
	return nil
}
