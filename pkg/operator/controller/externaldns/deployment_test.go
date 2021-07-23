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
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	namespace = "externaldns"
	name      = "test"
	image     = "bitname/external-dns:latest"
)

func TestDesiredExternalDNSDeployment(t *testing.T) {
	sourceNamespace := "my-namespace"
	externalDNS := &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Provider: operatorv1alpha1.ExternalDNSProvider{
				Type: operatorv1alpha1.ProviderTypeGCP,
			},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type:      operatorv1alpha1.SourceTypeService,
					Namespace: &sourceNamespace,
					Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeNodePort,
							corev1.ServiceTypeLoadBalancer,
							corev1.ServiceTypeClusterIP,
						},
					},
				},
				HostnameAnnotationPolicy: operatorv1alpha1.HostnameAnnotationPolicyIgnore,
			},
			Zones: []string{
				"my-dns-public-zone",
				"my-dns-private-zone",
			},
		},
	}
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	depl, err := desiredExternalDNSDeployment(namespace, image, serviceAccount, externalDNS)

	if err != nil {
		t.Errorf("expected no error from calling desiredExternalDNSDeployment, but received %v", err)
	}

	if len(depl.Spec.Template.Spec.Containers) != len(externalDNS.Spec.Zones) {
		t.Errorf("expected externalDNS deployment to have %d containers, but found %d", len(externalDNS.Spec.Zones), len(depl.Spec.Template.Spec.Containers))
	}

	expectedArgs := []string{
		"--provider=google",
		"--source=service",
		"--service-type-filter=NodePort,LoadBalancer,ClusterIP",
		"--publish-internal-services",
		"--ignore-hostname-annotation",
		fmt.Sprintf("--namespace=%s", sourceNamespace),
	}

	for i, container := range depl.Spec.Template.Spec.Containers {
		expectedCustomArgs := append(expectedArgs, fmt.Sprintf("--metrics-address=127.0.0.1:%d", metricsStartPort+i))
		expectedCustomArgs = append(expectedCustomArgs, fmt.Sprintf("--zone-id-filter=%s", externalDNS.Spec.Zones[i]))
		argSliceString := strings.Join(container.Args, " ")
		for _, arg := range expectedCustomArgs {
			if !strings.Contains(argSliceString, arg) {
				t.Errorf("expected externalDNS container %s to contain the following argument %q, but it did not. Found args: %v",
					container.Name, arg, container.Args)
			}
		}
	}
}

func TestExternalDNSDeploymentChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(*appsv1.Deployment)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *appsv1.Deployment) {},
			expect:      false,
		},
		{
			description: "if externalDNS image changes",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Image = "foo.io/test:latest"
			},
			expect: true,
		},
		{
			description: "if externalDNS container args",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Args = []string{"Nada"}
			},
			expect: true,
		},
		{
			description: "if externalDNS container args order changes",
			mutate: func(depl *appsv1.Deployment) {
				// swap the last and the first elements
				last := len(depl.Spec.Template.Spec.Containers[0].Args) - 1
				tmp := depl.Spec.Template.Spec.Containers[0].Args[0]
				depl.Spec.Template.Spec.Containers[0].Args[0] = depl.Spec.Template.Spec.Containers[0].Args[last]
				depl.Spec.Template.Spec.Containers[0].Args[last] = tmp
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		externalDNS := &operatorv1alpha1.ExternalDNS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: operatorv1alpha1.ExternalDNSSpec{
				Provider: operatorv1alpha1.ExternalDNSProvider{
					Type: operatorv1alpha1.ProviderTypeAWS,
				},
				Source: operatorv1alpha1.ExternalDNSSource{
					ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
						Type: operatorv1alpha1.SourceTypeRoute,
					},
				},
				Zones: []string{"my-dns-zone"},
			},
		}

		serviceAccount := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		original, err := desiredExternalDNSDeployment(namespace, image, serviceAccount, externalDNS)
		if err != nil {
			t.Errorf("expected no error from calling desiredExternalDNSDeployment, but received %v", err)
		}

		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := externalDNSDeploymentChanged(original, mutated); changed != tc.expect {
			t.Errorf("%s, expect externalDNSDeploymentChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := externalDNSDeploymentChanged(mutated, updated); changedAgain {
				t.Errorf("%s, externalDNSDeploymentChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
