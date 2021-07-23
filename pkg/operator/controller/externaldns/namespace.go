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
)

// ensureExternalDNSNamespace ensures that the externaldns namespace exists.
func (r *reconciler) ensureExternalDNSNamespace(ctx context.Context, namespace string) (bool, *corev1.Namespace, error) {
	nsName := types.NamespacedName{Name: namespace}

	desired := desiredExternalDNSNamespace(namespace)

	exist, current, err := r.currentExternalDNSNamespace(ctx, nsName)
	if err != nil {
		return false, nil, err
	}

	if !exist {
		if err := r.createExternalDNSNamespace(ctx, desired); err != nil {
			return false, nil, err
		}
		return r.currentExternalDNSNamespace(ctx, nsName)
	}

	return true, current, nil
}

// currentExternalDNSNamespace  gets the current externalDNS namespace resource.
func (r *reconciler) currentExternalDNSNamespace(ctx context.Context, nsName types.NamespacedName) (bool, *corev1.Namespace, error) {
	ns := &corev1.Namespace{}
	if err := r.client.Get(ctx, nsName, ns); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, ns, nil

}

// desiredExternalDNSNamespace returns the desired namespace resource.
func desiredExternalDNSNamespace(namespace string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			//TODO: Add more fields here? Labels?
		},
	}
}

// createExternalDNSNamespace creates the given namespace using the reconciler's client.
func (r *reconciler) createExternalDNSNamespace(ctx context.Context, ns *corev1.Namespace) error {
	if err := r.client.Create(ctx, ns); err != nil {
		return fmt.Errorf("failed to create externalDNS namespace %s: %v", ns.Name, err)
	}
	r.log.Info("created externalDNS namespace", "namespace", ns.Name)
	return nil
}
