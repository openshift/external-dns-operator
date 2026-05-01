/*
Copyright 2026.

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

// metricsService returns the desired metrics Service for the given ExternalDNS instance.
// It creates one port per zone container to match the kube-rbac-proxy sidecars.
func metricsService(namespace string, externalDNS *operatorv1beta1.ExternalDNS) *corev1.Service {
	metricsSecretName := controller.ExternalDNSMetricsSecretName(externalDNS)
	numZones := numMetricsPorts(externalDNS)

	ports := make([]corev1.ServicePort, numZones)
	for i := 0; i < numZones; i++ {
		portName := kubeRBACProxyPortNameForSeq(i)
		ports[i] = corev1.ServicePort{
			Name:       portName,
			Port:       int32(kubeRBACProxySecurePort + i),
			TargetPort: intstr.FromString(portName),
			Protocol:   corev1.ProtocolTCP,
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controller.ExternalDNSMetricsServiceName(externalDNS),
			Namespace: namespace,
			Labels: map[string]string{
				appNameLabel:     controller.ExternalDNSBaseName,
				appInstanceLabel: externalDNS.Name,
			},
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": metricsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				appNameLabel:     controller.ExternalDNSBaseName,
				appInstanceLabel: externalDNS.Name,
			},
			Ports: ports,
		},
	}
}

// ensureExternalDNSMetricsService ensures that the metrics service for the operand exists.
func (r *reconciler) ensureExternalDNSMetricsService(ctx context.Context, namespace string, externalDNS *operatorv1beta1.ExternalDNS) error {
	desired := metricsService(namespace, externalDNS)

	if err := controllerutil.SetControllerReference(externalDNS, desired, r.scheme); err != nil {
		return fmt.Errorf("failed to set the controller reference for metrics service: %w", err)
	}

	current := &corev1.Service{}
	nsName := types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}
	err := r.client.Get(ctx, nsName, current)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get metrics service %s: %w", nsName, err)
		}
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create metrics service %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		r.log.Info("created metrics service", "namespace", desired.Namespace, "name", desired.Name)
		return nil
	}

	if metricsServiceChanged(current, desired) {
		// Preserve ClusterIP to avoid immutable field errors.
		desired.Spec.ClusterIP = current.Spec.ClusterIP
		desired.ResourceVersion = current.ResourceVersion
		if err := r.client.Update(ctx, desired); err != nil {
			return fmt.Errorf("failed to update metrics service %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		r.log.Info("updated metrics service", "namespace", desired.Namespace, "name", desired.Name)
	}

	return nil
}

// deleteExternalDNSMetricsService deletes the metrics service if it exists.
func (r *reconciler) deleteExternalDNSMetricsService(ctx context.Context, namespace string, externalDNS *operatorv1beta1.ExternalDNS) error {
	svc := &corev1.Service{}
	nsName := types.NamespacedName{Namespace: namespace, Name: controller.ExternalDNSMetricsServiceName(externalDNS)}
	if err := r.client.Get(ctx, nsName, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get metrics service %s: %w", nsName, err)
	}
	if err := r.client.Delete(ctx, svc); err != nil {
		return fmt.Errorf("failed to delete metrics service %s: %w", nsName, err)
	}
	r.log.Info("deleted metrics service", "namespace", namespace, "name", nsName.Name)
	return nil
}

// metricsServiceChanged returns true if the current service needs to be updated to match the desired.
func metricsServiceChanged(current, desired *corev1.Service) bool {
	if !reflect.DeepEqual(current.Spec.Selector, desired.Spec.Selector) {
		return true
	}
	if len(current.Spec.Ports) != len(desired.Spec.Ports) {
		return true
	}
	for i := range desired.Spec.Ports {
		if current.Spec.Ports[i].Name != desired.Spec.Ports[i].Name ||
			current.Spec.Ports[i].Port != desired.Spec.Ports[i].Port ||
			current.Spec.Ports[i].TargetPort != desired.Spec.Ports[i].TargetPort {
			return true
		}
	}
	return false
}
