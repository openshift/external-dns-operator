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

	"github.com/google/go-cmp/cmp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
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
		expectedEvents  []test.Event
		errExpected     bool
	}{
		{
			name:            "Bootstrap",
			existingObjects: []runtime.Object{testAWSExtDNSInstance(), testSrcSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "secret",
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
			expectedEvents: []test.Event{
				{
					EventType: watch.Modified,
					ObjType:   "secret",
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
			name:            "Target secret doesn't have credentials key",
			existingObjects: []runtime.Object{testAWSExtDNSInstance(), testSrcSecret(), testTargetSecretWithoutCredentialsKey()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Modified,
					ObjType:   "secret",
					NamespacedName: types.NamespacedName{
						Namespace: testOperandNamespace,
						Name:      testTargetSecretName,
					},
				},
			},
		},
		{
			name:            "Target secret didn't change. Credentials key only",
			existingObjects: []runtime.Object{testAWSExtDNSInstance(), testSrcSecretWithCredentialsKey(), testTargetSecretWithCredentialsKey()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name:            "Target secret has expected keys for Infoblox provider",
			existingObjects: []runtime.Object{testInfobloxExtDNSInstance(), testInfoBloxSrcSecret(), testInfoBloxTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name:            "Target secret doesn't have expected keys for Infoblox provider",
			existingObjects: []runtime.Object{testInfobloxExtDNSInstance(), testInfobloxWrongSrcSecret(), testInfoBloxTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			errExpected:     true,
		},
		{
			name:            "Target secret has expected keys for Azure provider",
			existingObjects: []runtime.Object{testAzureExtDNSInstance(), testAzureSrcSecret(), testAzureTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name:            "Target secret doesn't have expected keys for Azure provider",
			existingObjects: []runtime.Object{testAzureExtDNSInstance(), testAzureWrongSrcSecret(), testAzureTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			errExpected:     true,
		},
		{
			name:            "Target secret has expected keys for GCP provider",
			existingObjects: []runtime.Object{testGCPExtDNSInstance(), testGCPSrcSecret(), testGCPTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name:            "Target secret doesn't have expected keys for GCP provider",
			existingObjects: []runtime.Object{testGCPExtDNSInstance(), testGCPWrongSecret(), testGCPTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			errExpected:     true,
		},
		{
			name:            "Target secret has expected keys for Bluecat provider",
			existingObjects: []runtime.Object{testBlueCatExtDNSInstance(), testBlueCatSrcSecret(), testBlueCatTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
		},
		{
			name:            "Target secret doesn't have expected keys for Bluecat provider",
			existingObjects: []runtime.Object{testBlueCatExtDNSInstance(), testBlueCatWrongSecret(), testBlueCatTargetSecret()},
			inputConfig:     testConfig(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			errExpected:     true,
		},
		{
			name: "Bootstrap when platform is OCP and it provided the credentials secret",
			// externaldns without credentials specified + secret provided by OCP
			existingObjects: []runtime.Object{testAWSExtDNSInstanceRouteSource(), testSrcSecretWhenPlatformOCP()},
			inputConfig:     testConfigOpenShift(),
			inputRequest:    testRequest(),
			expectedResult:  reconcile.Result{},
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "secret",
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
			expectedEvents: []test.Event{
				{
					EventType: watch.Added,
					ObjType:   "secret",
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

			c := test.NewEventCollector(t, cl, managedTypesList, len(tc.expectedEvents))

			// get watch interfaces from all the type managed by the operator
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

func TestGetExternalDNSCredentialsSecretName(t *testing.T) {
	testCases := []struct {
		name             string
		inputExtDNS      *operatorv1beta1.ExternalDNS
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

func testExtDNSInstance() *operatorv1beta1.ExternalDNS {
	return &operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testExtDNSName,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
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

func testExtDNSInstanceforOCPRouteSource() *operatorv1beta1.ExternalDNS {
	return &operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testExtDNSName,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Source: operatorv1beta1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
					Type: operatorv1beta1.SourceTypeRoute,
				},
			},
			Zones: []string{"public-zone"},
		},
	}
}

// AWS
func testAWSExtDNSInstance() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAWS,
		AWS: &operatorv1beta1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1beta1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testAWSExtDNSInstanceRouteSource() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAWS,
	}
	return extDNS
}

func testAWSExtDNSInstanceRouteSourceWithSecret() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstanceforOCPRouteSource()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAWS,
		AWS: &operatorv1beta1.ExternalDNSAWSProviderOptions{
			Credentials: operatorv1beta1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testAWSExtDNSInstanceNoSecret() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAWS,
	}
	return extDNS
}

// Azure
func testAzureExtDNSInstance() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAzure,
		Azure: &operatorv1beta1.ExternalDNSAzureProviderOptions{
			ConfigFile: operatorv1beta1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testAzureExtDNSInstanceNoSecret() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeAzure,
	}
	return extDNS
}

func testAzureSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"azure.json": []byte("val1"),
		},
	}
}

func testAzureWrongSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"wrong-azure-config.json": []byte("val1"),
		},
	}
}

func testAzureTargetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"azure.json": []byte("val1"),
		},
	}
}

// BlueCat
func testBlueCatExtDNSInstance() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeBlueCat,
		BlueCat: &operatorv1beta1.ExternalDNSBlueCatProviderOptions{
			ConfigFile: operatorv1beta1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testBlueCatSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"bluecat.json": []byte("val1"),
		},
	}
}

func testBlueCatWrongSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"wrong-bluecat-config.json": []byte("val1"),
		},
	}
}

func testBlueCatTargetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"bluecat.json": []byte("val1"),
		},
	}
}

// InfoBlox
func testInfobloxExtDNSInstance() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeInfoblox,
		Infoblox: &operatorv1beta1.ExternalDNSInfobloxProviderOptions{
			Credentials: operatorv1beta1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testInfoBloxSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME": []byte("val1"),
			"EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD": []byte("val2"),
		},
	}
}

func testInfobloxWrongSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"WRONG_USERNAME": []byte("val1"),
			"WRONG_PASSWORD": []byte("val2"),
		},
	}
}

func testInfoBloxTargetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME": []byte("val1"),
			"EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD": []byte("val2"),
		},
	}
}

// GCP

func testGCPExtDNSInstance() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeGCP,
		GCP: &operatorv1beta1.ExternalDNSGCPProviderOptions{
			Credentials: operatorv1beta1.SecretReference{
				Name: testSrcSecretName,
			},
		},
	}
	return extDNS
}

func testGCPExtDNSInstanceNoSecret() *operatorv1beta1.ExternalDNS {
	extDNS := testExtDNSInstance()
	extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{
		Type: operatorv1beta1.ProviderTypeGCP,
	}
	return extDNS
}

func testGCPSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"gcp-credentials.json": []byte("val1"),
		},
	}
}

func testGCPWrongSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"wrong-gcp-config.json": []byte("val1"),
		},
	}
}

func testGCPTargetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"gcp-credentials.json": []byte("val1"),
		},
	}
}

func testSrcSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"aws_access_key_id":     []byte("val1"),
			"aws_secret_access_key": []byte("val2"),
		},
	}
}

func testSrcSecretWithCredentialsKey() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSrcSecretName,
			Namespace: testOperatorNamespace,
		},
		Data: map[string][]byte{
			"credentials": []byte("[default]\naws_access_key_id = val1\naws_secret_access_key = val2"),
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
			"aws_access_key_id":     []byte("val1"),
			"aws_secret_access_key": []byte("val2"),
			"credentials":           []byte("[default]\naws_access_key_id = val1\naws_secret_access_key = val2"),
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
			"aws_access_key_id":     []byte("otherval1"),
			"aws_secret_access_key": []byte("otherval2"),
			"credentials":           []byte("[default]\naws_access_key_id = otherval1\naws_secret_access_key = otherval2"),
		},
	}
}

func testTargetSecretWithCredentialsKey() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"credentials": []byte("[default]\naws_access_key_id = val1\naws_secret_access_key = val2"),
		},
	}
}

func testTargetSecretWithoutCredentialsKey() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testTargetSecretName,
			Namespace: testOperandNamespace,
		},
		Data: map[string][]byte{
			"aws_access_key_id":     []byte("val1"),
			"aws_secret_access_key": []byte("val2"),
		},
	}
}
