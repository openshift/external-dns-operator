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

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	"reflect"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/externaldns/test"
)

const (
	testSecretName = "testsecret"
)

func TestReconcile(t *testing.T) {
	managedTypesList := []client.ObjectList{
		&cco.CredentialsRequestList{},
		&corev1.NamespaceList{},
		&appsv1.DeploymentList{},
		&corev1.ServiceAccountList{},
		&rbacv1.ClusterRoleList{},
		&rbacv1.ClusterRoleBindingList{},
		&operatorv1alpha1.ExternalDNSList{},
	}
	eventWaitTimeout := time.Duration(1 * time.Second)

	testCases := []struct {
		name            string
		existingObjects []runtime.Object
		inputConfig     Config
		inputRequest    ctrl.Request
		expectedResult  reconcile.Result
		expectedEvents  []testEvent
		errExpected     bool
	}{
		{
			name:            "Bootstrap",
			existingObjects: []runtime.Object{testExtDNSInstance(), testSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []testEvent{
				{
					eventType: watch.Added,
					objType:   "clusterrole",
					NamespacedName: types.NamespacedName{
						Name: "external-dns",
					},
				},
				{
					eventType: watch.Added,
					objType:   "deployment",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      "external-dns-test",
					},
				},
				{
					eventType: watch.Added,
					objType:   "serviceaccount",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      "external-dns-test",
					},
				},
				{
					eventType: watch.Modified,
					objType:   "externaldns",
					NamespacedName: types.NamespacedName{
						Name: test.Name,
					},
				},
				{
					eventType: watch.Added,
					objType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: "external-dns-test",
					},
				},
			},
		},
		{
			name:            "Bootstrap when OCP",
			existingObjects: []runtime.Object{testExtDNSInstance(), testSecret()},
			inputConfig:     testConfigOpenShift(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []testEvent{
				{
					eventType: watch.Added,
					objType:   "credentialsrequest",
					NamespacedName: types.NamespacedName{
						Name: "externaldns-credentials-request-" + strings.ToLower(string(testExtDNSInstance().Spec.Provider.Type)),
					},
				},
				{
					eventType: watch.Added,
					objType:   "clusterrole",
					NamespacedName: types.NamespacedName{
						Name: "external-dns",
					},
				},
				{
					eventType: watch.Added,
					objType:   "deployment",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      "external-dns-test",
					},
				},
				{
					eventType: watch.Added,
					objType:   "serviceaccount",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      "external-dns-test",
					},
				},
				{
					eventType: watch.Modified,
					objType:   "externaldns",
					NamespacedName: types.NamespacedName{
						Name: test.Name,
					},
				},
				{
					eventType: watch.Added,
					objType:   "clusterrolebinding",
					NamespacedName: types.NamespacedName{
						Name: "external-dns-test",
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
			cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()

			r := &reconciler{
				client: cl,
				scheme: test.Scheme,
				config: tc.inputConfig,
				log:    zap.New(zap.UseDevMode(true)),
			}

			// get watch interfaces from all the type managed by the operator
			watches := []watch.Interface{}
			for _, managedType := range managedTypesList {
				w, err := cl.Watch(context.TODO(), managedType)
				if err != nil {
					t.Fatalf("failed to start the watch for %T: %v", managedType, err)
				}
				watches = append(watches, w)
			}

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
			// events check
			if len(tc.expectedEvents) == 0 {
				return
			}
			// fan in the events
			allEventsCh := make(chan watch.Event, len(watches))
			for _, w := range watches {
				go func(c <-chan watch.Event) {
					for e := range c {
						t.Logf("Got event: %v", e)
						allEventsCh <- e
					}
				}(w.ResultChan())
			}
			defer func() {
				for _, w := range watches {
					w.Stop()
				}
			}()
			idxExpectedEvents := indexTestEvents(tc.expectedEvents)
			for {
				select {
				case e := <-allEventsCh:
					key := watch2test(e).key()
					if _, exists := idxExpectedEvents[key]; !exists {
						t.Fatalf("unexpected event received: %v", e)
					}
					delete(idxExpectedEvents, key)
					if len(idxExpectedEvents) == 0 {
						return
					}
				case <-time.After(eventWaitTimeout):
					t.Fatalf("timed out waiting for all expected events")
				}
			}
		})
	}
}

type testEvent struct {
	eventType watch.EventType
	objType   string
	types.NamespacedName
}

func (e testEvent) key() string {
	return string(e.eventType) + "/" + e.objType + "/" + e.Namespace + "/" + e.Name
}

func indexTestEvents(events []testEvent) map[string]testEvent {
	m := map[string]testEvent{}
	for _, e := range events {
		m[e.key()] = e
	}
	return m
}

func watch2test(we watch.Event) testEvent {
	te := testEvent{
		eventType: we.Type,
	}

	switch obj := we.Object.(type) {
	case *appsv1.Deployment:
		te.objType = "deployment"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	case *corev1.ServiceAccount:
		te.objType = "serviceaccount"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	case *rbacv1.ClusterRole:
		te.objType = "clusterrole"
		te.Name = obj.Name
	case *rbacv1.ClusterRoleBinding:
		te.objType = "clusterrolebinding"
		te.Name = obj.Name
	case *corev1.Namespace:
		te.objType = "namespace"
		te.Name = obj.Name
	case *operatorv1alpha1.ExternalDNS:
		te.objType = "externaldns"
		te.Name = obj.Name
	case *cco.CredentialsRequest:
		te.objType = "credentialsrequest"
		te.Name = obj.Name
	}
	return te
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

func testExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	// No need in other providers for the test externalDNS instances
	// as we are testing the events generated by Reconcile function.
	// Provider specific logic should be tested in the places where it's implemented (desired deployment, etc.).
	return &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Provider: operatorv1alpha1.ExternalDNSProvider{
				Type: operatorv1alpha1.ProviderTypeAWS,
				AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
					Credentials: operatorv1alpha1.SecretReference{
						Name: testSecretName,
					},
				},
			},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeService,
					Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
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

func testSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: test.OperandNamespace,
		},
	}
}
