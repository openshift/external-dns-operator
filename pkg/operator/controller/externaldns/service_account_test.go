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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

func TestEnsureExternalDNSServiceAccount(t *testing.T) {
	testCases := []struct {
		name            string
		existingObjects []runtime.Object
		expectedExist   bool
		expectedSA      corev1.ServiceAccount
		errExpected     bool
	}{
		{
			name:            "Does not exist",
			existingObjects: []runtime.Object{},
			expectedExist:   true,
			expectedSA: corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ExternalDNSResourceName(test.ExternalDNS),
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               "ExternalDNS",
							Name:               test.ExternalDNS.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
			},
		},
		{
			name: "Exists",
			existingObjects: []runtime.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controller.ExternalDNSResourceName(test.ExternalDNS),
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               "ExternalDNS",
								Name:               test.ExternalDNS.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
				},
			},
			expectedExist: true,
			expectedSA: corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      controller.ExternalDNSResourceName(test.ExternalDNS),
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               "ExternalDNS",
							Name:               test.ExternalDNS.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithRuntimeObjects(tc.existingObjects...).Build()
			r := &reconciler{
				client: cl,
				scheme: test.Scheme,
				log:    zap.New(zap.UseDevMode(true)),
			}
			gotExist, gotSA, err := r.ensureExternalDNSServiceAccount(context.TODO(), test.OperandNamespace, test.ExternalDNS)
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("unexpected error received: %v", err)
				}
				return
			}
			if tc.errExpected {
				t.Fatalf("Error expected but wasn't received")
			}
			if gotExist != tc.expectedExist {
				t.Errorf("expected service account's exist to be %t, got %t", tc.expectedExist, gotExist)
			}
			diffOpts := cmpopts.IgnoreFields(corev1.ServiceAccount{}, "ResourceVersion", "Kind", "APIVersion")
			if diff := cmp.Diff(tc.expectedSA, *gotSA, diffOpts); diff != "" {
				t.Errorf("unexpected service account (-want +got):\n%s", diff)
			}
		})
	}
}
