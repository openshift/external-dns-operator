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

package credentials_secret

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/external-dns-operator/pkg/operator/controller/externaldns/test"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	extdnscontroller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

const (
	testOperatorNamespace    = "external-dns-operator"
	testOperandNamespace     = "external-dns"
	testExtDNSName           = "test"
	testSrcSecretName        = "testsecret"
	testTargetSecretName     = "external-dns-credentials-test"
	testSrcSecretNameWhenOCP = "externaldns-cloud-credentials"
)

func TestReconcile(t *testing.T) {
	managedTypesList := []client.ObjectList{
		&corev1.SecretList{},
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
			existingObjects: []runtime.Object{testAWSExtDNSInstance(), testSrcSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []testEvent{
				{
					eventType: watch.Added,
					objType:   "secret",
					NamespacedName: types.NamespacedName{
						Namespace: testOperandNamespace,
						Name:      testTargetSecretName,
					},
				},
			},
		},
		{
			name:            "Target secret drifted",
			existingObjects: []runtime.Object{testAWSExtDNSInstance(), testSrcSecret(), testDriftedTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []testEvent{
				{
					eventType: watch.Modified,
					objType:   "secret",
					NamespacedName: types.NamespacedName{
						Namespace: testOperandNamespace,
						Name:      testTargetSecretName,
					},
				},
			},
		},
		{
			name:            "Target secret didn't change",
			existingObjects: []runtime.Object{testAWSExtDNSInstance(), testSrcSecret(), testTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name: "Bootstrap when platform is OCP and it provided the credentials secret",
			// externaldns without credentials specified + secret provided by OCP
			existingObjects: []runtime.Object{testAWSExtDNSInstanceRouteSource(), testSrcSecretWhenPlatformOCP()},
			inputConfig:     testConfigOpenShift(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []testEvent{
				{
					eventType: watch.Added,
					objType:   "secret",
					NamespacedName: types.NamespacedName{
						Namespace: testOperandNamespace,
						Name:      testTargetSecretName,
					},
				},
			},
		},
		{
			name: "Bootstrap when platform is OCP and the credentials are provided explicitly",
			// externaldns with credentials, platform secret is not there but we don't care as there is one given in CR
			existingObjects: []runtime.Object{testAWSExtDNSInstanceRouteSourceWithSecret(), testSrcSecret()},
			inputConfig:     testConfigOpenShift(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []testEvent{
				{
					eventType: watch.Added,
					objType:   "secret",
					NamespacedName: types.NamespacedName{
						Namespace: testOperandNamespace,
						Name:      testTargetSecretName,
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

func TestGetExternalDNSCredentialsSecretName(t *testing.T) {
	testCases := []struct {
		name             string
		inputExtDNS      *operatorv1alpha1.ExternalDNS
		inputIsOpenShift bool
		expected         string
	}{
		{
			name:        "AWS",
			inputExtDNS: testAWSExtDNSInstance(),
			expected:    testSrcSecretName,
		},
		{
			name:        "Azure",
			inputExtDNS: testAzureExtDNSInstance(),
			expected:    testSrcSecretName,
		},
		{
			name:        "GCP",
			inputExtDNS: testGCPExtDNSInstance(),
			expected:    testSrcSecretName,
		},
		{
			name:        "BlueCat",
			inputExtDNS: testBlueCatExtDNSInstance(),
			expected:    testSrcSecretName,
		},
		{
			name:        "Infoblox",
			inputExtDNS: testInfobloxExtDNSInstance(),
			expected:    testSrcSecretName,
		},
		{
			name:             "AWS OpenShift",
			inputExtDNS:      testAWSExtDNSInstanceNoSecret(),
			inputIsOpenShift: true,
			expected:         extdnscontroller.SecretFromCloudCredentialsOperator,
		},
		{
			name:             "Azure OpenShift",
			inputExtDNS:      testAzureExtDNSInstanceNoSecret(),
			inputIsOpenShift: true,
			expected:         extdnscontroller.SecretFromCloudCredentialsOperator,
		},
		{
			name:             "GCP OpenShift",
			inputExtDNS:      testGCPExtDNSInstanceNoSecret(),
			inputIsOpenShift: true,
			expected:         extdnscontroller.SecretFromCloudCredentialsOperator,
		},
		{
			name:             "AWS OpenShift with explicit credentials",
			inputExtDNS:      testAWSExtDNSInstance(),
			inputIsOpenShift: true,
			expected:         testSrcSecretName,
		},
		{
			name:             "Azure OpenShift with explicit credentials",
			inputExtDNS:      testAzureExtDNSInstance(),
			inputIsOpenShift: true,
			expected:         testSrcSecretName,
		},
		{
			name:             "GCP OpenShift with explicit credentials",
			inputExtDNS:      testGCPExtDNSInstance(),
			inputIsOpenShift: true,
			expected:         testSrcSecretName,
		},
		{
			name:             "BlueCat OpenShift with explicit credentials",
			inputExtDNS:      testBlueCatExtDNSInstance(),
			inputIsOpenShift: true,
			expected:         testSrcSecretName,
		},
		{
			name:             "Infoblox OpenShift with explicit credentials",
			inputExtDNS:      testInfobloxExtDNSInstance(),
			inputIsOpenShift: true,
			expected:         testSrcSecretName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getExternalDNSCredentialsSecretName(tc.inputExtDNS, tc.inputIsOpenShift); got != tc.expected {
				t.Errorf("unexpected name received. expected %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestIsInNS(t *testing.T) {
	testCases := []struct {
		name     string
		ns       string
		secret   client.Object
		expected bool
	}{
		{
			name:     "Belongs",
			ns:       testOperatorNamespace,
			secret:   testSrcSecret(),
			expected: true,
		},
		{
			name:     "Does not belong",
			ns:       "otherns",
			secret:   testSrcSecret(),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInNS(tc.ns)(tc.secret); got != tc.expected {
				t.Errorf("unexpected return value received. expected %t, got %t", tc.expected, got)
			}
		})
	}
}

func TestHasSecret(t *testing.T) {
	testCases := []struct {
		name             string
		inputObject      client.Object
		inputIsOpenShift bool
		expected         bool
	}{
		{
			name:        "Has secret",
			inputObject: testAWSExtDNSInstance(),
			expected:    true,
		},
		{
			name:        "Doesn't have secret",
			inputObject: testAWSExtDNSInstanceNoSecret(),
			expected:    false,
		},
		{
			name:             "Default secret for OpenShift",
			inputObject:      testAWSExtDNSInstanceNoSecret(),
			inputIsOpenShift: true,
			expected:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasSecret(tc.inputObject, tc.inputIsOpenShift); got != tc.expected {
				t.Errorf("unexpected return value received. expected %t, got %t", tc.expected, got)
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
	case *corev1.Secret:
		te.objType = "secret"
		te.Namespace = obj.Namespace
		te.Name = obj.Name
	}
	return te
}

func testConfig() Config {
	return Config{
		SourceNamespace: testOperatorNamespace,
		TargetNamespace: testOperandNamespace,
	}
}

func testConfigOpenShift() Config {
	return Config{
		SourceNamespace: testOperatorNamespace,
		TargetNamespace: testOperandNamespace,
		IsOpenShift:     true,
	}
}

func testRequest() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: testExtDNSName,
		},
	}
}

func testExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	return &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testExtDNSName,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
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

func testExtDNSInstanceforOCPRouteSource() *operatorv1alpha1.ExternalDNS {
	return &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testExtDNSName,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeRoute,
				},
			},
			Zones: []string{"public-zone"},
		},
	}
}

// AWS
func testAWSExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
		AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testAWSExtDNSInstanceRouteSource() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
	}
	return extDNS
}

func testAWSExtDNSInstanceRouteSourceWithSecret() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
		AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testAWSExtDNSInstanceNoSecret() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAWS,
	}
	return extDNS
}

// Azure
func testAzureExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAzure,
		Azure: &operatorv1alpha1.ExternalDNSAzureProviderOptions{
			ConfigFile: operatorv1alpha1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testAzureExtDNSInstanceNoSecret() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeAzure,
	}
	return extDNS
}

// BlueCat
func testBlueCatExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeBlueCat,
		BlueCat: &operatorv1alpha1.ExternalDNSBlueCatProviderOptions{
			ConfigFile: operatorv1alpha1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

// InfoBlox
func testInfobloxExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeInfoblox,
		Infoblox: &operatorv1alpha1.ExternalDNSInfobloxProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

// GCP

func testGCPExtDNSInstance() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeGCP,
		GCP: &operatorv1alpha1.ExternalDNSGCPProviderOptions{
			Credentials: operatorv1alpha1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testGCPExtDNSInstanceNoSecret() *operatorv1alpha1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1alpha1.ExternalDNSProvider{
		Type: operatorv1alpha1.ProviderTypeGCP,
	}
	return extDNS
}

func testSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"key1": []byte("val1"),
			"key2": []byte("val2"),
		},
	}
}

func testSrcSecretWhenPlatformOCP() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretNameWhenOCP,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"key1": []byte("val1"),
			"key2": []byte("val2"),
		},
	}
}

func testTargetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"key1": []byte("val1"),
			"key2": []byte("val2"),
		},
	}
}

func testDriftedTargetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"key1": []byte("otherval1"),
			"key2": []byte("otherval2"),
		},
	}
}
