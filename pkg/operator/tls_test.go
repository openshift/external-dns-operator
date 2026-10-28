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
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	for _, profileType := range []configv1.TLSProfileType{configv1.TLSProfileIntermediateType, "unknown", configv1.TLSProfileCustomType} {
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

type stubDiscovery struct {
	discovery.ServerResourcesInterface
	resources []metav1.APIResource
	err       error
}

func (stub stubDiscovery) ServerResourcesForGroupVersion(string) (*metav1.APIResourceList, error) {
	return &metav1.APIResourceList{APIResources: stub.resources}, stub.err
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
		name      string
		discovery stubDiscovery
		readError error
		adherence configv1.TLSAdherencePolicy
		wantError bool
		watch     bool
		selected  bool
	}{
		{name: "API missing", discovery: stubDiscovery{err: apierrors.NewNotFound(resource, "")}},
		{name: "API no match", discovery: stubDiscovery{err: &meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{Group: configv1.GroupName}}}},
		{name: "resource absent"},
		{name: "discovery forbidden", discovery: stubDiscovery{err: apierrors.NewForbidden(resource, "", errors.New("denied"))}, wantError: true},
		{name: "discovery timeout", discovery: stubDiscovery{err: context.DeadlineExceeded}, wantError: true},
		{name: "GET missing", watch: true, readError: apierrors.NewNotFound(resource, "cluster")},
		{name: "GET forbidden", watch: true, readError: apierrors.NewForbidden(resource, "cluster", errors.New("denied"))},
		{name: "GET timeout", watch: true, readError: context.DeadlineExceeded},
		{name: "unset", watch: true},
		{name: "legacy", watch: true, adherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly},
		{name: "strict", watch: true, adherence: configv1.TLSAdherencePolicyStrictAllComponents, selected: true},
		{name: "unknown honors", watch: true, adherence: "future", selected: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			if testCase.watch {
				testCase.discovery.resources = []metav1.APIResource{{Name: "apiservers", Kind: "APIServer"}}
			}
			apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
			apiServer.Spec.TLSAdherence = testCase.adherence
			apiServer.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP256r1)
			var reader client.Reader = fake.NewClientBuilder().WithScheme(GetOperatorScheme()).WithObjects(apiServer).Build()
			if testCase.readError != nil {
				reader = errorReader{err: testCase.readError}
			}
			profile, err := resolveTLSProfile(context.Background(), testCase.discovery, reader)
			if (err != nil) != testCase.wantError {
				test.Fatalf("err=%v", err)
			}
			if err != nil {
				return
			}
			if profile.watch != testCase.watch || (profile.spec != nil) != testCase.selected {
				test.Fatalf("profile=%+v", profile)
			}
			if testCase.readError != nil && profile.adherence != configv1.TLSAdherencePolicyStrictAllComponents {
				test.Fatal("failed read did not retain Strict fallback sentinel")
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

func TestAbsentAPIRegistersNoObserver(test *testing.T) {
	test.Parallel()
	if err := setupTLSProfileWatch(nil, nil, &tlsProfile{}); err != nil {
		test.Fatal(err)
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
	profile, err := resolveTLSProfile(ctx, stubDiscovery{resources: []metav1.APIResource{{Kind: "APIServer"}}}, reader)
	if err != nil || !profile.watch || reader.deadline.IsZero() || time.Until(reader.deadline) > apiServerGetTimeout {
		test.Fatalf("bounded fallback failed: %v %v", profile, err)
	}
}

func TestLoadTLSProfileHTTP(test *testing.T) {
	test.Parallel()
	for _, testCase := range []struct {
		name            string
		discoveryStatus int
		getStatus       int
		wantError       bool
		watch           bool
	}{
		{name: "served", discoveryStatus: http.StatusOK, getStatus: http.StatusOK, watch: true},
		{name: "initial GET error", discoveryStatus: http.StatusOK, getStatus: http.StatusForbidden, watch: true},
		{name: "absent", discoveryStatus: http.StatusNotFound},
		{name: "uncertain", discoveryStatus: http.StatusForbidden, wantError: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				status := testCase.getStatus
				var body any = &configv1.APIServer{TypeMeta: metav1.TypeMeta{APIVersion: configv1.GroupVersion.String(), Kind: "APIServer"}, ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
				if request.URL.Path == "/apis/config.openshift.io/v1" {
					status = testCase.discoveryStatus
					body = &metav1.APIResourceList{GroupVersion: configv1.GroupVersion.String(), APIResources: []metav1.APIResource{{Name: "apiservers", Kind: "APIServer", Verbs: metav1.Verbs{"get", "list", "watch"}}}}
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
			profile, err := loadTLSProfile(context.Background(), &rest.Config{Host: server.URL})
			if (err != nil) != testCase.wantError {
				test.Fatalf("profile=%+v err=%v", profile, err)
			}
			if err == nil && profile.watch != testCase.watch {
				test.Fatalf("watch=%v", profile.watch)
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
