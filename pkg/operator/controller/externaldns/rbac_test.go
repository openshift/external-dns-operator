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
	"reflect"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

const (
	testNamespace = "testns"
)

var (
	testExternalDNS = &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testexternaldns",
		},
	}
)

func TestEnsureExternalDNSClusterRole(t *testing.T) {
	testCases := []struct {
		name            string
		existingObjects []runtime.Object
		expectedExist   bool
		expectedRole    rbacv1.ClusterRole
	}{
		{
			name:            "Does not exist",
			existingObjects: []runtime.Object{},
			expectedExist:   true,
			expectedRole: rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSBaseName,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"networking.k8s.io"},
						Resources: []string{"ingresses"},
						Verbs:     []string{"get", "list", "watch"},
					},
					{
						APIGroups: []string{""},
						Resources: []string{"endpoints", "services", "pods", "nodes"},
						Verbs:     []string{"get", "list", "watch"},
					},
				},
			},
		},
		{
			name: "Exists and as expected",
			existingObjects: []runtime.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: controller.ExternalDNSBaseName,
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"networking.k8s.io"},
							Resources: []string{"ingresses"},
							Verbs:     []string{"get", "list", "watch"},
						},
						{
							APIGroups: []string{""},
							Resources: []string{"endpoints", "services", "pods", "nodes"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
			},
			expectedExist: true,
			expectedRole: rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSBaseName,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"networking.k8s.io"},
						Resources: []string{"ingresses"},
						Verbs:     []string{"get", "list", "watch"},
					},
					{
						APIGroups: []string{""},
						Resources: []string{"endpoints", "services", "pods", "nodes"},
						Verbs:     []string{"get", "list", "watch"},
					},
				},
			},
		},
		{
			name: "Exists and needs to be updated",
			existingObjects: []runtime.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: controller.ExternalDNSBaseName,
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"networking.k8s.io"},
							Resources: []string{"ingresses"},
							Verbs:     []string{"get", "list", "watch"},
						},
						{
							APIGroups: []string{""},
							Resources: []string{"endpoints", "services", "pods"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				},
			},
			expectedExist: true,
			expectedRole: rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSBaseName,
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"networking.k8s.io"},
						Resources: []string{"ingresses"},
						Verbs:     []string{"get", "list", "watch"},
					},
					{
						APIGroups: []string{""},
						Resources: []string{"endpoints", "services", "pods", "nodes"},
						Verbs:     []string{"get", "list", "watch"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewFakeClient(tc.existingObjects...)
			r := &reconciler{
				client: cl,
				log:    zap.New(zap.UseDevMode(true)),
			}
			gotExist, gotRole, _ := r.ensureExternalDNSClusterRole(context.TODO())
			if gotExist != tc.expectedExist {
				t.Errorf("expected cluster roles's exist to be %t, got %t", tc.expectedExist, gotExist)
			}
			if gotExist {
				if reflect.DeepEqual(*gotRole, tc.expectedRole) {
					t.Errorf("expected cluster role %v, got %v", tc.expectedRole, gotRole)
				}
			}
		})
	}
}

func TestExternalDNSRoleRulesChanged(t *testing.T) {
	testCases := []struct {
		name          string
		inputCurrent  []rbacv1.PolicyRule
		inputExpected []rbacv1.PolicyRule
		expectChanged bool
	}{
		{
			name: "Same",
			inputCurrent: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			inputExpected: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			expectChanged: false,
		},
		{
			name: "All reordered but the same",
			inputCurrent: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "nodes", "services", "endpoints"},
					Verbs:     []string{"list", "get", "watch"},
				},
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "watch", "list"},
				},
			},
			inputExpected: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			expectChanged: false,
		},
		{
			name: "Changed. Verb added",
			inputCurrent: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"list", "get", "watch", "create"},
				},
			},
			inputExpected: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			expectChanged: true,
		},
		{
			name: "Changed. Verb removed",
			inputCurrent: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"list", "get"},
				},
			},
			inputExpected: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"networking.k8s.io"},
					Resources: []string{"ingresses"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"endpoints", "services", "pods", "nodes"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			expectChanged: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotChanged, _ := externalDNSRoleRulesChanged(tc.inputCurrent, tc.inputExpected)
			if gotChanged != tc.expectChanged {
				t.Errorf("expected that the role rules changed %t, got %t", tc.expectChanged, gotChanged)
			}
		})
	}
}

func TestEnsureExternalDNSClusterRoleBinding(t *testing.T) {
	testCases := []struct {
		name                string
		existingObjects     []runtime.Object
		expectedExist       bool
		expectedRoleBinding rbacv1.ClusterRoleBinding
	}{
		{
			name:            "Does not exist",
			existingObjects: []runtime.Object{},
			expectedExist:   true,
			expectedRoleBinding: rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
		},
		{
			name: "Exists and as expected",
			existingObjects: []runtime.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: controller.ExternalDNSResourceName(testExternalDNS),
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     controller.ExternalDNSBaseName,
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      controller.ExternalDNSResourceName(testExternalDNS),
							Namespace: testNamespace,
						},
					},
				},
			},
			expectedExist: true,
			expectedRoleBinding: rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
		},
		{
			name: "Exists and needs to be updated",
			existingObjects: []runtime.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: controller.ExternalDNSResourceName(testExternalDNS),
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "otherrole",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "othersa",
							Namespace: "otherns",
						},
					},
				},
			},
			expectedExist: true,
			expectedRoleBinding: rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewFakeClient(tc.existingObjects...)
			r := &reconciler{
				client: cl,
				log:    zap.New(zap.UseDevMode(true)),
			}
			gotExist, gotRoleBinding, _ := r.ensureExternalDNSClusterRoleBinding(context.TODO(), testNamespace, testExternalDNS)
			if gotExist != tc.expectedExist {
				t.Errorf("expected cluster roles binding's exist to be %t, got %t", tc.expectedExist, gotExist)
			}
			if reflect.DeepEqual(*gotRoleBinding, tc.expectedRoleBinding) {
				t.Errorf("expected cluster role binding %v, got %v", tc.expectedRoleBinding, gotRoleBinding)
			}
		})
	}
}

func TestExternalDNSRoleBindingChanged(t *testing.T) {
	testCases := []struct {
		name          string
		inputCurrent  *rbacv1.ClusterRoleBinding
		inputExpected *rbacv1.ClusterRoleBinding
		expectChanged bool
	}{
		{
			name: "Same",
			inputCurrent: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			inputExpected: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			expectChanged: false,
		},
		{
			name: "Role changed",
			inputCurrent: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "otherrole",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			inputExpected: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			expectChanged: true,
		},
		{
			name: "Subject name changed",
			inputCurrent: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "othersa",
						Namespace: testNamespace,
					},
				},
			},
			inputExpected: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			expectChanged: true,
		},
		{
			name: "Subject namespace changed",
			inputCurrent: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: "otherns",
					},
				},
			},
			inputExpected: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			expectChanged: true,
		},
		{
			name: "All fields changed",
			inputCurrent: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "otherrole",
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "othersa",
						Namespace: "otherns",
					},
				},
			},
			inputExpected: &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: controller.ExternalDNSResourceName(testExternalDNS),
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     controller.ExternalDNSBaseName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      controller.ExternalDNSResourceName(testExternalDNS),
						Namespace: testNamespace,
					},
				},
			},
			expectChanged: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updated := tc.inputCurrent.DeepCopy()
			gotChanged, _ := externalDNSRoleBindingChanged(tc.inputCurrent, tc.inputExpected, updated)
			if gotChanged != tc.expectChanged {
				t.Errorf("expected that the role binding changed %t, got %t", tc.expectChanged, gotChanged)
			}
		})
	}
}
