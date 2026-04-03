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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

var serviceMonitorGVK = schema.GroupVersionKind{
	Group:   "monitoring.coreos.com",
	Version: "v1",
	Kind:    "ServiceMonitor",
}

// ensureExternalDNSServiceMonitor ensures that the service monitor for the operand exists.
func (r *reconciler) ensureExternalDNSServiceMonitor(ctx context.Context, namespace string, externalDNS *operatorv1beta1.ExternalDNS) error {
	desired := desiredServiceMonitor(namespace, externalDNS)

	// Set the controller reference so the ServiceMonitor is owned by the ExternalDNS CR.
	ownerRef := metav1.NewControllerRef(externalDNS, operatorv1beta1.GroupVersion.WithKind("ExternalDNS"))
	desired.SetOwnerReferences([]metav1.OwnerReference{*ownerRef})

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(serviceMonitorGVK)
	nsName := types.NamespacedName{Namespace: namespace, Name: controller.ExternalDNSServiceMonitorName(externalDNS)}

	err := r.client.Get(ctx, nsName, current)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get service monitor %s: %w", nsName, err)
		}
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create service monitor %s/%s: %w", namespace, desired.GetName(), err)
		}
		r.log.Info("created service monitor", "namespace", namespace, "name", desired.GetName())
		return nil
	}

	// Update the spec if the fields we manage have drifted.
	// We compare only the fields we set (endpoints, selector, namespaceSelector)
	// rather than the full spec, because the API server may add defaulted fields
	// that would cause reflect.DeepEqual to always detect drift.
	if serviceMonitorChanged(current, desired) {
		desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")
		current.Object["spec"] = desiredSpec
		if err := r.client.Update(ctx, current); err != nil {
			return fmt.Errorf("failed to update service monitor %s/%s: %w", namespace, current.GetName(), err)
		}
		r.log.Info("updated service monitor", "namespace", namespace, "name", current.GetName())
	}

	return nil
}

// deleteExternalDNSServiceMonitor deletes the service monitor if it exists.
func (r *reconciler) deleteExternalDNSServiceMonitor(ctx context.Context, namespace string, externalDNS *operatorv1beta1.ExternalDNS) error {
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(serviceMonitorGVK)
	nsName := types.NamespacedName{Namespace: namespace, Name: controller.ExternalDNSServiceMonitorName(externalDNS)}
	if err := r.client.Get(ctx, nsName, sm); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get service monitor %s: %w", nsName, err)
	}
	if err := r.client.Delete(ctx, sm); err != nil {
		return fmt.Errorf("failed to delete service monitor %s: %w", nsName, err)
	}
	r.log.Info("deleted service monitor", "namespace", namespace, "name", nsName.Name)
	return nil
}

// desiredServiceMonitor returns the desired ServiceMonitor as an unstructured object.
// It creates one endpoint per zone container to match the kube-rbac-proxy sidecars,
// and configures TLS so that Prometheus can scrape the HTTPS endpoints.
func desiredServiceMonitor(namespace string, externalDNS *operatorv1beta1.ExternalDNS) *unstructured.Unstructured {
	smName := controller.ExternalDNSServiceMonitorName(externalDNS)
	serviceName := controller.ExternalDNSMetricsServiceName(externalDNS)
	serverName := fmt.Sprintf("%s.%s.svc", serviceName, namespace)

	numZones := numMetricsPorts(externalDNS)
	endpoints := make([]interface{}, numZones)
	for i := 0; i < numZones; i++ {
		endpoints[i] = map[string]interface{}{
			"interval":        "30s",
			"path":            "/metrics",
			"port":            kubeRBACProxyPortNameForSeq(i),
			"scheme":          "https",
			"bearerTokenFile": "/var/run/secrets/kubernetes.io/serviceaccount/token",
			"tlsConfig": map[string]interface{}{
				"caFile":     "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
				"serverName": serverName,
			},
		}
	}

	sm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "ServiceMonitor",
			"metadata": map[string]interface{}{
				"name":      smName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					appNameLabel:     controller.ExternalDNSBaseName,
					appInstanceLabel: externalDNS.Name,
				},
			},
			"spec": map[string]interface{}{
				"endpoints": endpoints,
				"namespaceSelector": map[string]interface{}{
					"matchNames": []interface{}{
						namespace,
					},
				},
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						appNameLabel:     controller.ExternalDNSBaseName,
						appInstanceLabel: externalDNS.Name,
					},
				},
			},
		},
	}
	sm.SetGroupVersionKind(serviceMonitorGVK)
	return sm
}

// serviceMonitorChanged returns true if the fields we manage have drifted
// between the current and desired ServiceMonitor objects.
func serviceMonitorChanged(current, desired *unstructured.Unstructured) bool {
	for _, field := range []string{"endpoints", "selector", "namespaceSelector"} {
		currentVal, _, _ := unstructured.NestedFieldNoCopy(current.Object, "spec", field)
		desiredVal, _, _ := unstructured.NestedFieldNoCopy(desired.Object, "spec", field)
		if !reflect.DeepEqual(currentVal, desiredVal) {
			return true
		}
	}
	return false
}
