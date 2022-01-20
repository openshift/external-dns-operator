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

package ca_configmap

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

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

	test "github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
)

const (
	testSrcConfigMapName    = "test-trusted-ca"
	testTargetConfigMapName = "external-dns-trusted-ca"
)

func TestReconcile(t *testing.T) {
	managedTypesList := []client.ObjectList{
		&corev1.ConfigMapList{},
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
			existingObjects: []runtime.Object{testSrcConfigMap()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "configmap",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      testTargetConfigMapName,
					},
				},
			},
		},
		{
			name:            "Target configmap drifted",
			existingObjects: []runtime.Object{testSrcConfigMap(), testDriftedConfigMap()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Modified,
					ObjType:   "configmap",
					NamespacedName: types.NamespacedName{
						Namespace: test.OperandNamespace,
						Name:      testTargetConfigMapName,
					},
				},
			},
		},
		{
			name:            "Target configmap didn't change",
			existingObjects: []runtime.Object{testSrcConfigMap(), testTargetConfigMap()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name:            "Deleted source configmap",
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
				config: tc.inputConfig,
				log:    zap.New(zap.UseDevMode(true)),
			}

			c := test.NewEventCollector(t, cl, managedTypesList, len(tc.expectedEvents))

			// start watching for events from the types managed by the operator
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
		SourceNamespace: test.OperatorNamespace,
		TargetNamespace: test.OperandNamespace,
		CAConfigMapName: testSrcConfigMapName,
	}
}

func testRequest() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      testSrcConfigMapName,
			Namespace: test.OperatorNamespace,
		},
	}
}

func testSrcConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcConfigMapName,
			Namespace: test.OperatorNamespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": "---pem encoded certificate---",
		},
	}
}

func testTargetConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetConfigMapName,
			Namespace: test.OperandNamespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": "---pem encoded certificate---",
		},
	}
}

func testDriftedConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetConfigMapName,
			Namespace: test.OperandNamespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": "---pem encoded certificate number 2---",
		},
	}
}
