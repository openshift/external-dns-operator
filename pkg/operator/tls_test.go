package operator

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
)

func customTLSProfile(groups ...configv1.TLSGroup) *configv1.TLSSecurityProfile {
	return &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{TLSProfileSpec: configv1.TLSProfileSpec{
			MinTLSVersion: configv1.VersionTLS12,
			Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			Groups:        groups,
		}},
	}
}

func TestTLSConfigFromProfile(test *testing.T) {
	test.Parallel()
	defaults, err := tlsConfigFromProfile(nil)
	if err != nil {
		test.Fatal(err)
	}
	if defaults.MinVersion != tls.VersionTLS13 {
		test.Fatalf("default minimum version=%d, want TLS 1.3", defaults.MinVersion)
	}
	for _, profileType := range []configv1.TLSProfileType{configv1.TLSProfileModernType, "unknown", configv1.TLSProfileCustomType, ""} {
		test.Run(string(profileType), func(test *testing.T) {
			test.Parallel()
			config, err := tlsConfigFromProfile(&configv1.TLSSecurityProfile{Type: profileType})
			if err != nil || config.MinVersion != defaults.MinVersion || !reflect.DeepEqual(config.CipherSuites, defaults.CipherSuites) || !reflect.DeepEqual(config.CurvePreferences, defaults.CurvePreferences) {
				test.Fatalf("fallback differs: config=%+v err=%v", config, err)
			}
		})
	}
	for _, testCase := range []struct {
		name    string
		profile *configv1.TLSSecurityProfile
		version uint16
		curves  []tls.CurveID
	}{
		{name: "old", profile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType}, version: tls.VersionTLS10, curves: defaults.CurvePreferences},
		{name: "intermediate", profile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}, version: tls.VersionTLS12, curves: defaults.CurvePreferences},
		{name: "modern", profile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}, version: tls.VersionTLS13, curves: defaults.CurvePreferences},
		{name: "classical", profile: customTLSProfile(configv1.TLSGroupX25519, configv1.TLSGroupSecP256r1, configv1.TLSGroupSecP384r1, configv1.TLSGroupSecP521r1), version: tls.VersionTLS12, curves: []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521}},
		{name: "hybrid", profile: customTLSProfile(configv1.TLSGroupX25519MLKEM768), version: tls.VersionTLS12, curves: []tls.CurveID{tls.X25519MLKEM768}},
		{name: "mixed", profile: customTLSProfile("unknown", configv1.TLSGroupSecP256r1), version: tls.VersionTLS12, curves: []tls.CurveID{tls.CurveP256}},
		{name: "unmapped groups use defaults", profile: customTLSProfile("unknown"), version: tls.VersionTLS12},
		{name: "empty groups use defaults", profile: customTLSProfile(), version: tls.VersionTLS12},
		{name: "duplicate mapped groups", profile: customTLSProfile(configv1.TLSGroupSecP256r1, configv1.TLSGroupSecP256r1), version: tls.VersionTLS12, curves: []tls.CurveID{tls.CurveP256, tls.CurveP256}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			config, err := tlsConfigFromProfile(testCase.profile)
			if err != nil {
				test.Fatal(err)
			}
			if config.MinVersion != testCase.version || !reflect.DeepEqual(config.CurvePreferences, testCase.curves) {
				test.Fatalf("version=%d curves=%v", config.MinVersion, config.CurvePreferences)
			}
			if spec, named := configv1.TLSProfiles[testCase.profile.Type]; named {
				var expectedCiphers []uint16
				for _, name := range libgocrypto.OpenSSLToIANACipherSuites(spec.Ciphers) {
					if cipher, err := libgocrypto.CipherSuite(name); err == nil {
						expectedCiphers = append(expectedCiphers, cipher)
					}
				}
				expected := libgocrypto.SecureTLSConfig(&tls.Config{CipherSuites: expectedCiphers})
				if !reflect.DeepEqual(config.CipherSuites, expected.CipherSuites) {
					test.Fatalf("explicit named profile ciphers=%v want=%v", config.CipherSuites, expected.CipherSuites)
				}
			}
		})
	}
	test.Run("invalid version", func(test *testing.T) {
		profile := customTLSProfile()
		profile.Custom.MinTLSVersion = "invalid"
		if _, err := tlsConfigFromProfile(profile); err == nil {
			test.Fatal("invalid version accepted")
		}
	})
	for _, ciphers := range [][]string{nil, {"unsupported"}, {"unsupported", "ECDHE-RSA-AES128-GCM-SHA256"}} {
		test.Run(fmt.Sprintf("cipher fallback %d", len(ciphers)), func(test *testing.T) {
			profile := customTLSProfile()
			profile.Custom.Ciphers = ciphers
			config, err := tlsConfigFromProfile(profile)
			if err != nil || len(config.CipherSuites) == 0 {
				test.Fatalf("expected mapped or default ciphers: %v %v", config, err)
			}
			if len(ciphers) == 2 && !reflect.DeepEqual(config.CipherSuites, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}) {
				test.Fatalf("mapped ciphers=%v", config.CipherSuites)
			}
			if len(ciphers) < 2 && !reflect.DeepEqual(config.CipherSuites, libgocrypto.DefaultCiphers()) {
				test.Fatal("empty conversion did not use exact library-go defaults")
			}
		})
	}
	test.Run("omitted minimum version", func(test *testing.T) {
		profile := customTLSProfile()
		profile.Custom.MinTLSVersion = ""
		config, err := tlsConfigFromProfile(profile)
		if err != nil || config.MinVersion != tls.VersionTLS12 {
			test.Fatalf("minimum version fallback failed: %v", err)
		}
	})
}

type errorReader struct {
	client.Reader
	err error
}

func (reader errorReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return reader.err
}

func TestResolveTLSProfile(test *testing.T) {
	test.Parallel()
	resource := schema.GroupResource{Group: configv1.GroupName, Resource: "apiservers"}
	for _, testCase := range []struct {
		name       string
		readError  error
		adherence  configv1.TLSAdherencePolicy
		selected   bool
		nilProfile bool
	}{
		{name: "GET missing", readError: apierrors.NewNotFound(resource, "cluster")},
		{name: "GET forbidden", readError: apierrors.NewForbidden(resource, "cluster", errors.New("denied"))},
		{name: "GET timeout", readError: context.DeadlineExceeded},
		{name: "unset"},
		{name: "legacy", adherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly},
		{name: "strict", adherence: configv1.TLSAdherencePolicyStrictAllComponents, selected: true},
		{name: "strict nil profile", adherence: configv1.TLSAdherencePolicyStrictAllComponents, nilProfile: true},
		{name: "unknown honors", adherence: "future", selected: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
			apiServer.Spec.TLSAdherence = testCase.adherence
			if !testCase.nilProfile {
				apiServer.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP256r1)
			}
			var reader client.Reader = fake.NewClientBuilder().WithScheme(GetOperatorScheme()).WithObjects(apiServer).Build()
			if testCase.readError != nil {
				reader = errorReader{err: testCase.readError}
			}
			profile := resolveTLSProfile(context.Background(), reader)
			if !profile.watch || (profile.spec != nil) != testCase.selected {
				test.Fatalf("profile=%+v", profile)
			}
			if testCase.readError != nil && profile.adherence != configv1.TLSAdherencePolicyStrictAllComponents {
				test.Fatal("failed read did not retain Strict fallback sentinel")
			}
			policy, err := tlsConfigFromProfile(profile.spec)
			if err != nil {
				test.Fatal(err)
			}
			wantVersion := uint16(tls.VersionTLS13)
			if testCase.selected {
				wantVersion = tls.VersionTLS12
			}
			if policy.MinVersion != wantVersion {
				test.Fatalf("selected minimum=%d want=%d", policy.MinVersion, wantVersion)
			}
		})
	}
}

func TestTLSProfileWatcher(test *testing.T) {
	test.Parallel()
	strict := configv1.TLSAdherencePolicyStrictAllComponents
	legacy := configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly
	profile := customTLSProfile(configv1.TLSGroupSecP256r1)
	versionProfile := profile.DeepCopy()
	versionProfile.Custom.MinTLSVersion = configv1.VersionTLS13
	cipherProfile := profile.DeepCopy()
	cipherProfile.Custom.Ciphers = []string{"ECDHE-RSA-AES256-GCM-SHA384"}
	invalidProfile := profile.DeepCopy()
	invalidProfile.Custom.MinTLSVersion = "invalid"
	for _, testCase := range []struct {
		name             string
		initial          *tlsProfile
		currentAdherence configv1.TLSAdherencePolicy
		currentProfile   *configv1.TLSSecurityProfile
		cancel           bool
	}{
		{name: "identical", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: strict, currentProfile: profile},
		{name: "group only", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: strict, currentProfile: customTLSProfile(configv1.TLSGroupSecP384r1), cancel: true},
		{name: "minimum version", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: strict, currentProfile: versionProfile, cancel: true},
		{name: "cipher only", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: strict, currentProfile: cipherProfile, cancel: true},
		{name: "invalid desired policy still cancels", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: strict, currentProfile: invalidProfile, cancel: true},
		{name: "raw group order", initial: &tlsProfile{spec: customTLSProfile(configv1.TLSGroupSecP256r1, configv1.TLSGroupSecP384r1), adherence: strict}, currentAdherence: strict, currentProfile: customTLSProfile(configv1.TLSGroupSecP384r1, configv1.TLSGroupSecP256r1), cancel: true},
		{name: "legacy ignores profile", initial: &tlsProfile{adherence: legacy}, currentAdherence: legacy, currentProfile: profile},
		{name: "unset to legacy", initial: &tlsProfile{}, currentAdherence: legacy, currentProfile: profile},
		{name: "legacy to strict", initial: &tlsProfile{adherence: legacy}, currentAdherence: strict, currentProfile: profile, cancel: true},
		{name: "strict to legacy", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: legacy, currentProfile: profile, cancel: true},
		{name: "unknown honors", initial: &tlsProfile{spec: profile, adherence: strict}, currentAdherence: "future", currentProfile: profile},
		{name: "fallback sentinel to legacy", initial: &tlsProfile{adherence: strict}, currentAdherence: legacy, cancel: true},
		{name: "fallback sentinel to desired", initial: &tlsProfile{adherence: strict}, currentAdherence: strict, currentProfile: profile, cancel: true},
		{name: "fallback sentinel to nil", initial: &tlsProfile{adherence: strict}, currentAdherence: strict},
		{name: "nil to explicit intermediate", initial: &tlsProfile{adherence: strict}, currentAdherence: strict, currentProfile: &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType}, cancel: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster", ResourceVersion: "2"}}
			apiServer.Spec.TLSAdherence = testCase.currentAdherence
			apiServer.Spec.TLSSecurityProfile = testCase.currentProfile
			reader := fake.NewClientBuilder().WithScheme(GetOperatorScheme()).WithObjects(apiServer).Build()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			watcher := &tlsProfileWatcher{reader: reader, initial: testCase.initial, cancel: cancel}
			for range 2 {
				if _, err := watcher.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "cluster"}}); err != nil {
					test.Fatal(err)
				}
			}
			if (ctx.Err() != nil) != testCase.cancel {
				test.Fatalf("cancelled=%v, want=%v", ctx.Err(), testCase.cancel)
			}
		})
	}
}

func TestNonOpenShiftSkipsTLSProfile(test *testing.T) {
	test.Parallel()
	for _, detectionEnabled := range []bool{false, true} {
		test.Run(fmt.Sprintf("platform-detection=%v", detectionEnabled), func(test *testing.T) {
			test.Parallel()
			var detectionRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				detectionRequests.Add(1)
				if request.URL.Path != "/apis/operator.openshift.io/v1" {
					test.Errorf("unexpected platform request: %s", request.URL.Path)
				}
				response.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(response).Encode(&metav1.APIResourceList{GroupVersion: "operator.openshift.io/v1"}); err != nil {
					test.Errorf("write fixture: %v", err)
				}
			}))
			defer server.Close()
			config := &operatorconfig.Config{EnablePlatformDetection: detectionEnabled}
			if err := config.DetectPlatform(&rest.Config{Host: server.URL}); err != nil {
				test.Fatal(err)
			}
			wantRequests := int32(0)
			if detectionEnabled {
				wantRequests = 1
			}
			if config.IsOpenShift || detectionRequests.Load() != wantRequests {
				test.Fatalf("platform result=%v requests=%d want=%d", config.IsOpenShift, detectionRequests.Load(), wantRequests)
			}
			profile, err := loadTLSProfile(context.Background(), nil, config.IsOpenShift)
			if err != nil || profile == nil || profile.watch || profile.spec != nil {
				test.Fatalf("non-OpenShift loaded centralized profile: %+v %v", profile, err)
			}
			if err := setupTLSProfileWatch(nil, nil, profile); err != nil {
				test.Fatal(err)
			}
			policy, err := tlsConfigFromProfile(profile.spec)
			if err != nil || policy.MinVersion != tls.VersionTLS13 {
				test.Fatalf("non-OpenShift default differs: %+v %v", policy, err)
			}
		})
	}
}

func TestTLSWatcherReadErrors(test *testing.T) {
	test.Parallel()
	for _, readError := range []error{apierrors.NewNotFound(schema.GroupResource{Resource: "apiservers"}, "cluster"), context.DeadlineExceeded} {
		ctx, cancel := context.WithCancel(context.Background())
		watcher := &tlsProfileWatcher{reader: errorReader{err: readError}, initial: &tlsProfile{}, cancel: cancel}
		result, err := watcher.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "cluster"}})
		if apierrors.IsNotFound(readError) && (err != nil || result.RequeueAfter != 0) {
			test.Fatalf("NotFound unexpectedly requeued: %v %v", result, err)
		}
		if !apierrors.IsNotFound(readError) && !errors.Is(err, readError) {
			test.Fatalf("read error lost: %v", err)
		}
		if ctx.Err() != nil {
			test.Fatal("read failure cancelled manager")
		}
		apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
		apiServer.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP256r1)
		watcher.reader = fake.NewClientBuilder().WithScheme(GetOperatorScheme()).WithObjects(apiServer).Build()
		if _, err := watcher.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "other"}}); err != nil || ctx.Err() != nil {
			test.Fatal("unrelated singleton event triggered cancellation")
		}
		if _, err := watcher.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "cluster"}}); err != nil || ctx.Err() == nil {
			test.Fatalf("successful retry did not cancel: %v", err)
		}
		cancel()
	}
}

func TestApplyTLSPolicyPreservesServerSettings(test *testing.T) {
	test.Parallel()
	policy, err := tlsConfigFromProfile(customTLSProfile(configv1.TLSGroupSecP256r1))
	if err != nil {
		test.Fatal(err)
	}
	config := &tls.Config{NextProtos: []string{"http/1.1"}, ClientAuth: tls.RequireAnyClientCert,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, errors.New("certificate watcher") }}
	applyTLSPolicy(policy)(config)
	if !reflect.DeepEqual(config.NextProtos, []string{"http/1.1"}) || config.ClientAuth != tls.RequireAnyClientCert || config.GetCertificate == nil {
		test.Fatal("policy overwrote independent server settings")
	}
	config.CurvePreferences[0] = tls.CurveP384
	config.CipherSuites[0] = 0
	if policy.CurvePreferences[0] != tls.CurveP256 || policy.CipherSuites[0] == 0 {
		test.Fatal("TLSOpts shared mutable slices")
	}
}

func TestInitialReadIsBounded(test *testing.T) {
	test.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	reader := &deadlineReader{}
	profile := resolveTLSProfile(ctx, reader)
	if !profile.watch || reader.deadline.IsZero() || time.Until(reader.deadline) > apiServerGetTimeout {
		test.Fatalf("bounded fallback failed: %v", profile)
	}
}

func TestLoadTLSProfileHTTP(test *testing.T) {
	test.Parallel()
	for _, testCase := range []struct {
		name          string
		getStatus     int
		mappingStatus int
	}{
		{name: "served", getStatus: http.StatusOK},
		{name: "initial GET forbidden", getStatus: http.StatusForbidden},
		{name: "initial GET not found", getStatus: http.StatusNotFound},
		{name: "REST mapping forbidden", mappingStatus: http.StatusForbidden},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			var singletonReads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				status := testCase.getStatus
				var body any = &configv1.APIServer{TypeMeta: metav1.TypeMeta{APIVersion: configv1.GroupVersion.String(), Kind: "APIServer"}, ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
				switch request.URL.Path {
				case "/apis/config.openshift.io/v1":
					status = http.StatusOK
					if testCase.mappingStatus != 0 {
						status = testCase.mappingStatus
					}
					body = &metav1.APIResourceList{GroupVersion: configv1.GroupVersion.String(), APIResources: []metav1.APIResource{{Name: "apiservers", Kind: "APIServer", Verbs: metav1.Verbs{"get", "list", "watch"}}}}
				case "/apis/config.openshift.io/v1/apiservers/cluster":
					singletonReads.Add(1)
				default:
					test.Errorf("unexpected TLS request: %s", request.URL.Path)
				}
				if status != http.StatusOK {
					body = &metav1.Status{Status: metav1.StatusFailure, Code: int32(status)}
				}
				response.WriteHeader(status)
				if err := json.NewEncoder(response).Encode(body); err != nil {
					test.Errorf("write fixture: %v", err)
				}
			}))
			defer server.Close()
			profile, err := loadTLSProfile(context.Background(), &rest.Config{Host: server.URL}, true)
			if err != nil {
				test.Fatalf("profile=%+v err=%v", profile, err)
			}
			if !profile.watch {
				test.Fatalf("watch=%v", profile.watch)
			}
			wantReads := int32(1)
			if testCase.mappingStatus != 0 {
				wantReads = 0
			}
			if singletonReads.Load() != wantReads {
				test.Fatalf("singleton reads=%d want=%d", singletonReads.Load(), wantReads)
			}
			policy, err := tlsConfigFromProfile(profile.spec)
			if err != nil || policy.MinVersion != tls.VersionTLS13 {
				test.Fatalf("read/default path did not select Modern: %+v %v", policy, err)
			}
		})
	}
}

type deadlineReader struct {
	client.Reader
	deadline time.Time
}

func (reader *deadlineReader) Get(ctx context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	reader.deadline, _ = ctx.Deadline()
	<-ctx.Done()
	return ctx.Err()
}
