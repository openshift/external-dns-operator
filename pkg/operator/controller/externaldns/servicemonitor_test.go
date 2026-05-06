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
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

func TestDesiredServiceMonitor(t *testing.T) {
	testCases := []struct {
		name              string
		externalDNS       *operatorv1beta1.ExternalDNS
		namespace         string
		expectedEndpoints int
	}{
		{
			name:              "single zone AWS",
			externalDNS:       testAWSExternalDNS(operatorv1beta1.SourceTypeService),
			namespace:         test.OperandNamespace,
			expectedEndpoints: 1,
		},
		{
			name:              "multiple zones AWS",
			externalDNS:       testAWSExternalDNSZones([]string{test.PublicZone, test.PrivateZone}, operatorv1beta1.SourceTypeService),
			namespace:         test.OperandNamespace,
			expectedEndpoints: 2,
		},
		{
			name:              "Azure no zones creates 2 endpoints",
			externalDNS:       testAzureExternalDNSNoZones(operatorv1beta1.SourceTypeService),
			namespace:         test.OperandNamespace,
			expectedEndpoints: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sm := desiredServiceMonitor(tc.namespace, tc.externalDNS)

			// Verify GVK.
			if sm.GetKind() != "ServiceMonitor" {
				t.Errorf("expected kind ServiceMonitor, got %s", sm.GetKind())
			}
			if sm.GetAPIVersion() != "monitoring.coreos.com/v1" {
				t.Errorf("expected apiVersion monitoring.coreos.com/v1, got %s", sm.GetAPIVersion())
			}

			// Verify name and namespace.
			expectedName := controller.ExternalDNSServiceMonitorName(tc.externalDNS)
			if sm.GetName() != expectedName {
				t.Errorf("expected name %q, got %q", expectedName, sm.GetName())
			}
			if sm.GetNamespace() != tc.namespace {
				t.Errorf("expected namespace %q, got %q", tc.namespace, sm.GetNamespace())
			}

			// Verify endpoints count.
			endpoints, found, err := unstructured.NestedSlice(sm.Object, "spec", "endpoints")
			if err != nil || !found {
				t.Fatalf("failed to get endpoints from service monitor: found=%v, err=%v", found, err)
			}
			if len(endpoints) != tc.expectedEndpoints {
				t.Errorf("expected %d endpoints, got %d", tc.expectedEndpoints, len(endpoints))
			}

			// Verify each endpoint has correct fields.
			serviceName := controller.ExternalDNSMetricsServiceName(tc.externalDNS)
			expectedServerName := fmt.Sprintf("%s.%s.svc", serviceName, tc.namespace)
			for i, ep := range endpoints {
				epMap, ok := ep.(map[string]interface{})
				if !ok {
					t.Fatalf("endpoint %d is not a map", i)
				}
				if epMap["scheme"] != "https" {
					t.Errorf("endpoint %d: expected scheme https, got %v", i, epMap["scheme"])
				}
				if epMap["path"] != "/metrics" {
					t.Errorf("endpoint %d: expected path /metrics, got %v", i, epMap["path"])
				}
				expectedPort := kubeRBACProxyPortNameForSeq(i)
				if epMap["port"] != expectedPort {
					t.Errorf("endpoint %d: expected port %q, got %v", i, expectedPort, epMap["port"])
				}
				tlsConfig, ok := epMap["tlsConfig"].(map[string]interface{})
				if !ok {
					t.Fatalf("endpoint %d: tlsConfig missing or wrong type", i)
				}
				if tlsConfig["serverName"] != expectedServerName {
					t.Errorf("endpoint %d: expected serverName %q, got %v", i, expectedServerName, tlsConfig["serverName"])
				}
			}

			// Verify selector labels.
			matchLabels, found, err := unstructured.NestedStringMap(sm.Object, "spec", "selector", "matchLabels")
			if err != nil || !found {
				t.Fatalf("failed to get selector matchLabels: found=%v, err=%v", found, err)
			}
			if matchLabels[appNameLabel] != controller.ExternalDNSBaseName {
				t.Errorf("expected selector label %s=%s, got %s", appNameLabel, controller.ExternalDNSBaseName, matchLabels[appNameLabel])
			}
			if matchLabels[appInstanceLabel] != tc.externalDNS.Name {
				t.Errorf("expected selector label %s=%s, got %s", appInstanceLabel, tc.externalDNS.Name, matchLabels[appInstanceLabel])
			}
		})
	}
}

func TestServiceMonitorChanged(t *testing.T) {
	extDNS := &operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{Name: test.Name},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Provider: operatorv1beta1.ExternalDNSProvider{Type: operatorv1beta1.ProviderTypeAWS},
			Zones:    []string{test.PublicZone},
		},
	}
	base := desiredServiceMonitor(test.OperandNamespace, extDNS)

	testCases := []struct {
		name    string
		mutate  func(*unstructured.Unstructured)
		changed bool
	}{
		{
			name:    "no change",
			mutate:  func(sm *unstructured.Unstructured) {},
			changed: false,
		},
		{
			name: "label changed",
			mutate: func(sm *unstructured.Unstructured) {
				labels := sm.GetLabels()
				labels[appInstanceLabel] = "different"
				sm.SetLabels(labels)
			},
			changed: true,
		},
		{
			name: "extra defaulted field in spec does not trigger change",
			mutate: func(sm *unstructured.Unstructured) {
				// Simulate API server adding a defaulted field we don't manage.
				_ = unstructured.SetNestedField(sm.Object, "None", "spec", "targetLabels")
			},
			changed: false,
		},
		{
			name: "endpoints changed",
			mutate: func(sm *unstructured.Unstructured) {
				endpoints, _, _ := unstructured.NestedSlice(sm.Object, "spec", "endpoints")
				if len(endpoints) > 0 {
					ep := endpoints[0].(map[string]interface{})
					ep["interval"] = "60s"
					_ = unstructured.SetNestedSlice(sm.Object, endpoints, "spec", "endpoints")
				}
			},
			changed: true,
		},
		{
			name: "selector changed",
			mutate: func(sm *unstructured.Unstructured) {
				_ = unstructured.SetNestedField(sm.Object, "different", "spec", "selector", "matchLabels", appInstanceLabel)
			},
			changed: true,
		},
		{
			name: "namespaceSelector changed",
			mutate: func(sm *unstructured.Unstructured) {
				_ = unstructured.SetNestedStringSlice(sm.Object, []string{"other-ns"}, "spec", "namespaceSelector", "matchNames")
			},
			changed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := base.DeepCopy()
			tc.mutate(current)

			got := serviceMonitorChanged(current, desiredServiceMonitor(test.OperandNamespace, extDNS))
			if got != tc.changed {
				t.Errorf("expected changed=%v, got %v", tc.changed, got)
			}
		})
	}
}
