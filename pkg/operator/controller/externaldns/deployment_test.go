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
	"reflect"
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
		description        string
		originalDeployment *appsv1.Deployment
		mutate             func(*appsv1.Deployment)
		expect             bool
		expectedDeployment *appsv1.Deployment
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
			expect:             true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{testContainerWithImage("foo.io/test:latest")}),
		},
		{
			description: "if externalDNS container args",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Args = []string{"Nada"}
			},
			expect:             true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{testContainerWithArgs([]string{"Nada"})}),
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
		{
			description: "if externalDNS misses container",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers = append(depl.Spec.Template.Spec.Containers, testContainerWithName("second"))
			},
			expect: true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{
				testContainer(),
				testContainerWithName("second"),
			}),
		},
		{
			description: "if externalDNS has extra container",
			originalDeployment: testDeploymentWithContainers([]corev1.Container{
				testContainer(),
				testContainerWithName("second"),
			}),
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers = []corev1.Container{testContainer()}
			},
			expect: true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{
				testContainer(),
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			original := testDeployment()
			if tc.originalDeployment != nil {
				original = tc.originalDeployment
			}

			mutated := original.DeepCopy()
			tc.mutate(mutated)
			if changed, updated := externalDNSDeploymentChanged(original, mutated); changed != tc.expect {
				t.Errorf("Expect externalDNSDeploymentChanged to be %t, got %t", tc.expect, changed)
			} else if changed {
				if changedAgain, updatedAgain := externalDNSDeploymentChanged(mutated, updated); changedAgain {
					t.Errorf("ExternalDNSDeploymentChanged does not behave as a fixed point function")
				} else {
					if !reflect.DeepEqual(updatedAgain, tc.expectedDeployment) {
						t.Errorf("Expect updated deployment to be %v, got %v", tc.expectedDeployment, updatedAgain)
					}
				}
			}
		})
	}
}

func testDeployment() *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "testns",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"testlbl": "yes",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"testlbl": "yes",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "testsa",
					NodeSelector: map[string]string{
						"testlbl": "yes",
					},
					Containers: []corev1.Container{testContainer()},
				},
			},
		},
	}
}

func testDeploymentWithContainers(containers []corev1.Container) *appsv1.Deployment {
	depl := testDeployment()
	depl.Spec.Template.Spec.Containers = containers
	return depl
}

func testContainer() corev1.Container {
	return corev1.Container{
		Name:                     "first",
		Image:                    image,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		Args: []string{
			"--flag1=value1",
			"--flag2=value2",
		},
	}
}

func testContainerWithName(name string) corev1.Container {
	cont := testContainer()
	cont.Name = name
	return cont
}

func testContainerWithImage(image string) corev1.Container {
	cont := testContainer()
	cont.Image = image
	return cont
}

func testContainerWithArgs(args []string) corev1.Container {
	cont := testContainer()
	cont.Args = args
	return cont
}
