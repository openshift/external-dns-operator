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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

const (
	testSecretName             = "testsecret"
	deploymentResource         = "deployment"
	externalDNSResource        = "externaldns"
	serviceAccountResource     = "serviceaccount"
	credentialsrequestResource = "credentialsrequest"
)

func TestReconcile(t *testing.T) {
	managedTypesList := []client.ObjectList{
		&cco.CredentialsRequestList{},
		&corev1.NamespaceList{},
		&appsv1.DeploymentList{},
		&corev1.ServiceAccountList{},
		&operatorv1beta1.ExternalDNSList{},
	}
	eventWaitTimeout := time.Duration(1 * time.Second)

	testCases := []struct {
		name            string
		existingObjects []runtime.Object
		inputConfig     Config
		inputRequest    ctrl.Request
		expectedResult  reconcile.Result
		expectedEvents  []test.Event
		errExpected     bool
	}{
		{
			name:            "Bootstrap",
			existingObjects: []runtime.Object{testExtDNSInstance(), testSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   deploymentResource,
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      test.OperandName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   serviceAccountResource,
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      test.OperandName,
					},
				},
				{
					EventType: watch.Modified,
					ObjType:   externalDNSResource,
					NamespacedName: types.NamespacedName{
						Name: test.Name,
					},
				},
			},
		},
		{
			name:            "Bootstrap when OCP",
			existingObjects: []runtime.Object{testExtDNSInstanceNoSecret(), testSecret()},
			inputConfig:     testConfigOpenShift(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   credentialsrequestResource,
					NamespacedName: types.NamespacedName{
						Name: "externaldns-credentials-request-" + strings.ToLower(string(testExtDNSInstance().Spec.Provider.Type)),
					},
				},
				{
					EventType: watch.Added,
					ObjType:   deploymentResource,
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      test.OperandName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   serviceAccountResource,
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      test.OperandName,
					},
				},
				{
					EventType: watch.Modified,
					ObjType:   externalDNSResource,
					NamespacedName: types.NamespacedName{
						Name: test.Name,
					},
				},
			},
		},
		{
			name:            "Bootstrap when OCP and secret is given",
			existingObjects: []runtime.Object{testExtDNSInstance(), testSecret()},
			inputConfig:     testConfigOpenShift(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   deploymentResource,
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      test.OperandName,
					},
				},
				{
					EventType: watch.Added,
					ObjType:   serviceAccountResource,
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      test.OperandName,
					},
				},
				{
					EventType: watch.Modified,
					ObjType:   externalDNSResource,
					NamespacedName: types.NamespacedName{
						Name: test.Name,
					},
				},
			},
		},
		{
			name:            "Deleted ExternalDNS",
			existingObjects: []runtime.Object{},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithStatusSubresource(&operatorv1beta1.ExternalDNS{}).WithRuntimeObjects(tc.existingObjects...).Build()

			r := &reconciler{
				client: cl,
				scheme: test.Scheme,
				config: tc.inputConfig,
				log:    zap.New(zap.UseDevMode(true)),
			}

			c := test.NewEventCollector(t, cl, managedTypesList, len(tc.expectedEvents))

			// get watch interfaces from all the types managed by the operator
			c.Start(context.TODO())
			defer c.Stop()

			// TEST FUNCTION
			gotResult, err := r.Reconcile(context.TODO(), tc.inputRequest)

			// error check
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("got unexpected error: %v", err)
				}
			} else if tc.errExpected {
				t.Fatalf("error expected but not received")
			}

			// result check
			if !reflect.DeepEqual(gotResult, tc.expectedResult) {
				t.Fatalf("expected result %v, got %v", tc.expectedResult, gotResult)
			}

			// collect the events received from Reconcile()
			collectedEvents := c.Collect(len(tc.expectedEvents), eventWaitTimeout)

			// compare collected and expected events
			idxExpectedEvents := test.IndexEvents(tc.expectedEvents)
			idxCollectedEvents := test.IndexEvents(collectedEvents)
			if diff := cmp.Diff(idxExpectedEvents, idxCollectedEvents); diff != "" {
				t.Fatalf("found diff between expected and collected events: %s", diff)
			}
		})
	}
}

func testConfig() Config {
	return Config{
		Namespace: test.OperandNamespace,
		Image:     test.OperandImage,
	}
}

func testConfigOpenShift() Config {
	return Config{
		Namespace:   test.OperandNamespace,
		Image:       test.OperandImage,
		IsOpenShift: true,
	}
}

func testRequest() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "",
			Name:      test.Name,
		},
	}
}

func testExtDNSInstanceNoSecret() *operatorv1beta1.ExternalDNS {
	// No need in other providers for the test externalDNS instances
	// as we are testing the events generated by Reconcile function.
	// Provider specific logic should be tested in the places where it's implemented (desired deployment, etc.).
	return &operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Provider: operatorv1beta1.ExternalDNSProvider{
				Type: operatorv1beta1.ProviderTypeAWS,
			},
			Source: operatorv1beta1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
					Type: operatorv1beta1.SourceTypeService,
					Service: &operatorv1beta1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			Zones: []string{"public-zone"},
		},
	}
}

func testExtDNSInstance() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstanceNoSecret()
	extDNS.Spec.Provider.AWS = &operatorv1beta1.ExternalDNSAWSProviderOptions{
		Credentials: operatorv1beta1.SecretReference{
			Name: testSecretName,
		},
	}
	return extDNS
}

func testSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      test.OperandSecretName,
			Namespace: test.OperandNamespace,
		},
	}
}
