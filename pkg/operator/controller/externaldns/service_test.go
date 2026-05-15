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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

func TestMetricsService(t *testing.T) {
	testCases := []struct {
		name          string
		externalDNS   *operatorv1beta1.ExternalDNS
		namespace     string
		expectedPorts int
		expectedName  string
	}{
		{
			name:          "single zone AWS",
			externalDNS:   testAWSExternalDNS(operatorv1beta1.SourceTypeService),
			namespace:     test.OperandNamespace,
			expectedPorts: 1,
			expectedName:  controller.ExternalDNSMetricsServiceName(testAWSExternalDNS(operatorv1beta1.SourceTypeService)),
		},
		{
			name:          "multiple zones AWS",
			externalDNS:   testAWSExternalDNSZones([]string{test.PublicZone, test.PrivateZone}, operatorv1beta1.SourceTypeService),
			namespace:     test.OperandNamespace,
			expectedPorts: 2,
			expectedName:  controller.ExternalDNSMetricsServiceName(testAWSExternalDNSZones([]string{test.PublicZone, test.PrivateZone}, operatorv1beta1.SourceTypeService)),
		},
		{
			name:          "Azure no zones creates 2 ports",
			externalDNS:   testAzureExternalDNSNoZones(operatorv1beta1.SourceTypeService),
			namespace:     test.OperandNamespace,
			expectedPorts: 2,
			expectedName:  controller.ExternalDNSMetricsServiceName(testAzureExternalDNSNoZones(operatorv1beta1.SourceTypeService)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := desiredMetricsService(tc.namespace, tc.externalDNS)

			if svc.Name != tc.expectedName {
				t.Errorf("expected service name %q, got %q", tc.expectedName, svc.Name)
			}
			if svc.Namespace != tc.namespace {
				t.Errorf("expected namespace %q, got %q", tc.namespace, svc.Namespace)
			}
			if len(svc.Spec.Ports) != tc.expectedPorts {
				t.Errorf("expected %d ports, got %d", tc.expectedPorts, len(svc.Spec.Ports))
			}

			// Verify serving cert annotation.
			expectedSecretName := controller.ExternalDNSMetricsSecretName(tc.externalDNS)
			if svc.Annotations["service.beta.openshift.io/serving-cert-secret-name"] != expectedSecretName {
				t.Errorf("expected serving cert annotation %q, got %q", expectedSecretName, svc.Annotations["service.beta.openshift.io/serving-cert-secret-name"])
			}

			// Verify labels match selector.
			if svc.Labels[appNameLabel] != controller.ExternalDNSBaseName {
				t.Errorf("expected label %s=%s, got %s", appNameLabel, controller.ExternalDNSBaseName, svc.Labels[appNameLabel])
			}

			// Verify port names and numbering.
			for i, port := range svc.Spec.Ports {
				expectedPortName := kubeRBACProxyPortNameForSeq(i)
				if port.Name != expectedPortName {
					t.Errorf("port %d: expected name %q, got %q", i, expectedPortName, port.Name)
				}
				expectedPort := int32(kubeRBACProxySecurePort + i)
				if port.Port != expectedPort {
					t.Errorf("port %d: expected port %d, got %d", i, expectedPort, port.Port)
				}
				if port.TargetPort != intstr.FromString(expectedPortName) {
					t.Errorf("port %d: expected target port %q, got %v", i, expectedPortName, port.TargetPort)
				}
			}
		})
	}
}

func TestMetricsServiceChanged(t *testing.T) {
	extDNS := testAWSExternalDNS(operatorv1beta1.SourceTypeService)
	base := desiredMetricsService(test.OperandNamespace, extDNS)

	testCases := []struct {
		name    string
		mutate  func(*corev1.Service)
		changed bool
	}{
		{
			name:    "no change",
			mutate:  func(s *corev1.Service) {},
			changed: false,
		},
		{
			name: "label changed",
			mutate: func(s *corev1.Service) {
				s.Labels[appInstanceLabel] = "different"
			},
			changed: true,
		},
		{
			name: "annotation changed",
			mutate: func(s *corev1.Service) {
				s.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = "wrong-secret"
			},
			changed: true,
		},
		{
			name: "selector changed",
			mutate: func(s *corev1.Service) {
				s.Spec.Selector[appInstanceLabel] = "different"
			},
			changed: true,
		},
		{
			name: "port count changed",
			mutate: func(s *corev1.Service) {
				s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{
					Name: "extra",
					Port: 9999,
				})
			},
			changed: true,
		},
		{
			name: "port name changed",
			mutate: func(s *corev1.Service) {
				s.Spec.Ports[0].Name = "changed"
			},
			changed: true,
		},
		{
			name: "port number changed",
			mutate: func(s *corev1.Service) {
				s.Spec.Ports[0].Port = 9999
			},
			changed: true,
		},
		{
			name: "target port changed",
			mutate: func(s *corev1.Service) {
				s.Spec.Ports[0].TargetPort = intstr.FromInt(1234)
			},
			changed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := base.DeepCopy()
			tc.mutate(current)
			desired := desiredMetricsService(test.OperandNamespace, extDNS)

			got := metricsServiceChanged(current, desired)
			if got != tc.changed {
				t.Errorf("expected changed=%v, got %v", tc.changed, got)
			}
		})
	}
}
