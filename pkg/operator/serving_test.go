package operator

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"

	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
)

func TestServingOptionsPreserveEndpointConfiguration(test *testing.T) {
	policy, err := tlsConfigFromProfile(customTLSProfile(configv1.TLSGroupSecP256r1))
	if err != nil {
		test.Fatal(err)
	}
	for _, disableHTTP2 := range []bool{false, true} {
		config := &operatorconfig.Config{CertDir: "webhook-certs", MetricsTLSCertDir: "metrics-certs", MetricsBindAddress: "0", WebhookDisableHTTP2: disableHTTP2}
		webhookOpts, metricsOpts := servingOptions(config, policy)
		if webhookOpts.CertDir != config.CertDir {
			test.Fatalf("webhook certificate directory=%q", webhookOpts.CertDir)
		}
		if metricsOpts.CertDir != config.MetricsTLSCertDir {
			test.Fatalf("metrics certificate directory=%q", metricsOpts.CertDir)
		}
		if metricsOpts.BindAddress != config.MetricsBindAddress {
			test.Fatalf("metrics bind address=%q", metricsOpts.BindAddress)
		}
		if !metricsOpts.SecureServing {
			test.Fatal("metrics secure serving disabled")
		}
		if metricsOpts.FilterProvider == nil || reflect.ValueOf(metricsOpts.FilterProvider).Pointer() != reflect.ValueOf(filters.WithAuthenticationAndAuthorization).Pointer() {
			test.Fatal("metrics authentication/authorization filter changed")
		}
		for endpoint, options := range map[string][]func(*tls.Config){"metrics": metricsOpts.TLSOpts, "webhook": webhookOpts.TLSOpts} {
			test.Run(fmt.Sprintf("%s/http2-disabled=%v", endpoint, disableHTTP2), func(test *testing.T) {
				certificates := []tls.Certificate{{Certificate: [][]byte{{1, 2, 3}}}}
				roots := x509.NewCertPool()
				callbackConfig := &tls.Config{}
				tlsConfig := &tls.Config{NextProtos: []string{"h2", "http/1.1"}, Certificates: certificates, ClientAuth: tls.RequireAnyClientCert, RootCAs: roots, ClientCAs: roots,
					GetCertificate:     func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &certificates[0], nil },
					GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) { return callbackConfig, nil }}
				for _, option := range options {
					option(tlsConfig)
				}
				if tlsConfig.MinVersion != policy.MinVersion {
					test.Fatalf("minimum TLS version=%d", tlsConfig.MinVersion)
				}
				if !reflect.DeepEqual(tlsConfig.CipherSuites, policy.CipherSuites) {
					test.Fatalf("ciphers=%v", tlsConfig.CipherSuites)
				}
				if !reflect.DeepEqual(tlsConfig.CurvePreferences, policy.CurvePreferences) {
					test.Fatalf("groups=%v", tlsConfig.CurvePreferences)
				}
				if !reflect.DeepEqual(tlsConfig.Certificates, certificates) {
					test.Fatal("certificates changed")
				}
				if tlsConfig.ClientAuth != tls.RequireAnyClientCert {
					test.Fatal("client authentication changed")
				}
				if tlsConfig.RootCAs != roots {
					test.Fatal("root CAs changed")
				}
				if tlsConfig.ClientCAs != roots {
					test.Fatal("client CAs changed")
				}
				if tlsConfig.GetCertificate == nil || tlsConfig.GetConfigForClient == nil {
					test.Fatal("certificate callbacks removed")
				}
				if certificate, err := tlsConfig.GetCertificate(nil); err != nil || certificate != &certificates[0] {
					test.Fatalf("certificate callback changed: %v", err)
				}
				if selected, err := tlsConfig.GetConfigForClient(nil); err != nil || selected != callbackConfig {
					test.Fatalf("config callback changed: %v", err)
				}
				wantProtos := []string{"h2", "http/1.1"}
				if endpoint == "metrics" || disableHTTP2 {
					wantProtos = []string{"http/1.1"}
				}
				if !reflect.DeepEqual(tlsConfig.NextProtos, wantProtos) {
					test.Fatalf("protocols=%v want=%v", tlsConfig.NextProtos, wantProtos)
				}
			})
		}
		config.WebhookDisableHTTP2 = !disableHTTP2
		changed := &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
		for _, option := range webhookOpts.TLSOpts {
			option(changed)
		}
		if (len(changed.NextProtos) == 1) != config.WebhookDisableHTTP2 {
			test.Fatal("webhook HTTP2 callback lost live config pointer")
		}
	}
}

func tlsStartupFixture(test *testing.T, apiServer *configv1.APIServer, status int) *rest.Config {
	test.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		var object any = apiServer
		switch request.URL.Path {
		case "/apis/config.openshift.io/v1":
			object = &metav1.APIResourceList{GroupVersion: configv1.GroupVersion.String(), APIResources: []metav1.APIResource{{Name: "apiservers", Kind: "APIServer", Verbs: metav1.Verbs{"get", "list", "watch"}}}}
		case "/apis/config.openshift.io/v1/apiservers/cluster":
			response.WriteHeader(status)
			if status != http.StatusOK {
				object = &metav1.Status{Status: metav1.StatusFailure, Code: int32(status)}
			}
		default:
			test.Errorf("unexpected startup request: %s", request.URL.Path)
		}
		if err := json.NewEncoder(response).Encode(object); err != nil {
			test.Errorf("encode startup fixture: %v", err)
		}
	}))
	test.Cleanup(server.Close)
	return &rest.Config{Host: server.URL}
}

func TestNewRejectsTLSProfileErrors(test *testing.T) {
	invalid := &configv1.APIServer{TypeMeta: metav1.TypeMeta{APIVersion: configv1.GroupVersion.String(), Kind: "APIServer"}, ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	invalid.Spec.TLSAdherence = configv1.TLSAdherencePolicyStrictAllComponents
	invalid.Spec.TLSSecurityProfile = customTLSProfile()
	invalid.Spec.TLSSecurityProfile.Custom.MinTLSVersion = "invalid"
	for _, testCase := range []struct {
		name    string
		config  *rest.Config
		message string
	}{
		{name: "fatal client construction", config: &rest.Config{Host: "https://unused.invalid", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("not a certificate")}}, message: "resolve TLS profile: create TLS config client:"},
		{name: "invalid honored minimum", config: tlsStartupFixture(test, invalid, http.StatusOK), message: "convert TLS profile: invalid TLS version"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			operator, err := New(context.Background(), testCase.config, &operatorconfig.Config{IsOpenShift: true}, func() { test.Error("startup failure cancelled an unstarted manager") })
			if operator != nil || err == nil || !strings.HasPrefix(err.Error(), testCase.message) {
				test.Fatalf("operator=%v error=%v want=%q", operator, err, testCase.message)
			}
			if errors.Unwrap(err) == nil {
				test.Fatal("startup lost underlying error")
			}
		})
	}
}

func TestLoadServingOptionsDefaults(test *testing.T) {
	for _, testCase := range []struct {
		name      string
		openShift bool
		adherence configv1.TLSAdherencePolicy
		status    int
	}{
		{name: "non OpenShift"},
		{name: "unset adherence", openShift: true, status: http.StatusOK},
		{name: "legacy", openShift: true, adherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly, status: http.StatusOK},
		{name: "GET fallback", openShift: true, status: http.StatusForbidden},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			var cliCfg *rest.Config
			if testCase.openShift {
				apiServer := &configv1.APIServer{TypeMeta: metav1.TypeMeta{APIVersion: configv1.GroupVersion.String(), Kind: "APIServer"}, ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
				apiServer.Spec.TLSAdherence = testCase.adherence
				apiServer.Spec.TLSSecurityProfile = customTLSProfile(configv1.TLSGroupSecP256r1)
				cliCfg = tlsStartupFixture(test, apiServer, testCase.status)
			}
			profile, webhookOpts, metricsOpts, err := loadServingOptions(context.Background(), cliCfg, &operatorconfig.Config{IsOpenShift: testCase.openShift})
			if err != nil || profile == nil || profile.watch != testCase.openShift || profile.spec != nil {
				test.Fatalf("startup profile=%+v error=%v", profile, err)
			}
			for _, options := range [][]func(*tls.Config){webhookOpts.TLSOpts, metricsOpts.TLSOpts} {
				config := &tls.Config{}
				for _, option := range options {
					option(config)
				}
				if config.MinVersion != tls.VersionTLS13 {
					test.Fatalf("fallback minimum=%d, want Modern", config.MinVersion)
				}
			}
		})
	}
}
