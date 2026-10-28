package operator

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
)

func TestServingOptionsPreserveEndpointConfiguration(test *testing.T) {
	test.Parallel()
	policy, err := tlsConfigFromProfile(customTLSProfile(configv1.TLSGroupSecP256r1))
	if err != nil {
		test.Fatal(err)
	}
	for _, disableHTTP2 := range []bool{false, true} {
		config := &operatorconfig.Config{CertDir: "webhook-certs", MetricsTLSCertDir: "metrics-certs", MetricsBindAddress: "0", WebhookDisableHTTP2: disableHTTP2}
		webhookOpts, metricsOpts := servingOptions(config, policy)
		if webhookOpts.CertDir != config.CertDir || metricsOpts.CertDir != config.MetricsTLSCertDir || metricsOpts.BindAddress != "0" || !metricsOpts.SecureServing {
			test.Fatal("endpoint configuration changed")
		}
		if metricsOpts.FilterProvider == nil || reflect.ValueOf(metricsOpts.FilterProvider).Pointer() != reflect.ValueOf(filters.WithAuthenticationAndAuthorization).Pointer() {
			test.Fatal("metrics authentication/authorization filter changed")
		}
		for endpoint, options := range map[string][]func(*tls.Config){"metrics": metricsOpts.TLSOpts, "webhook": webhookOpts.TLSOpts} {
			certificates := []tls.Certificate{{Certificate: [][]byte{{1, 2, 3}}}}
			roots := x509.NewCertPool()
			config := &tls.Config{NextProtos: []string{"h2", "http/1.1"}, Certificates: certificates, ClientAuth: tls.RequireAnyClientCert, RootCAs: roots, ClientCAs: roots}
			for _, option := range options {
				option(config)
			}
			if config.MinVersion != policy.MinVersion || !reflect.DeepEqual(config.CipherSuites, policy.CipherSuites) || !reflect.DeepEqual(config.CurvePreferences, policy.CurvePreferences) || !reflect.DeepEqual(config.Certificates, certificates) || config.ClientAuth != tls.RequireAnyClientCert || config.RootCAs != roots || config.ClientCAs != roots {
				test.Fatalf("%s lost policy or certificate settings", endpoint)
			}
			wantProtos := []string{"h2", "http/1.1"}
			if endpoint == "metrics" || disableHTTP2 {
				wantProtos = []string{"http/1.1"}
			}
			if !reflect.DeepEqual(config.NextProtos, wantProtos) {
				test.Fatalf("%s protocols=%v", endpoint, config.NextProtos)
			}
		}
	}
}

func TestProductionServingEndpoints(test *testing.T) {
	for _, endpoint := range []string{"metrics", "webhook"} {
		for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
			test.Run(fmt.Sprintf("%s/TLS%d", endpoint, version), func(test *testing.T) {
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					test.Fatal(err)
				}
				firstDER := servingCertificate(test, key, 1)
				nextDER := servingCertificate(test, key, 2)
				certDir := test.TempDir()
				writeServingCertificate(test, certDir, firstDER)
				if err := os.WriteFile(filepath.Join(certDir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
					test.Fatal(err)
				}
				profile := customTLSProfile(configv1.TLSGroupSecP256r1)
				if version == tls.VersionTLS13 {
					profile.Custom.MinTLSVersion = configv1.VersionTLS13
				}
				policy, err := tlsConfigFromProfile(profile)
				if err != nil {
					test.Fatal(err)
				}
				config := &operatorconfig.Config{CertDir: certDir, MetricsTLSCertDir: certDir, MetricsBindAddress: "127.0.0.1:0", WebhookDisableHTTP2: true}
				webhookOpts, metricsOpts := servingOptions(config, policy)
				ctx, cancel := context.WithCancel(context.Background())
				finished := make(chan error, 1)
				var address func() string
				if endpoint == "metrics" {
					authAPI := httptest.NewServer(http.HandlerFunc(serveMetricsAuthorization))
					test.Cleanup(authAPI.Close)
					server, err := metrics.NewServer(metricsOpts, &rest.Config{Host: authAPI.URL}, authAPI.Client())
					if err != nil {
						cancel()
						test.Fatal(err)
					}
					address = server.(interface{ GetBindAddr() string }).GetBindAddr
					go func() { finished <- server.Start(ctx) }()
				} else {
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					if err != nil {
						cancel()
						test.Fatal(err)
					}
					webhookOpts.Host = "127.0.0.1"
					webhookOpts.Port = listener.Addr().(*net.TCPAddr).Port
					if err := listener.Close(); err != nil {
						cancel()
						test.Fatal(err)
					}
					server := webhook.NewServer(webhookOpts)
					server.Register("/", http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusOK) }))
					address = func() string { return fmt.Sprintf("127.0.0.1:%d", webhookOpts.Port) }
					go func() { finished <- server.Start(ctx) }()
				}
				test.Cleanup(func() {
					cancel()
					select {
					case err := <-finished:
						if err != nil {
							test.Errorf("server shutdown: %v", err)
						}
					case <-time.After(5 * time.Second):
						test.Error("server did not shut down")
					}
				})
				roots := x509.NewCertPool()
				for _, der := range [][]byte{firstDER, nextDER} {
					certificate, err := x509.ParseCertificate(der)
					if err != nil {
						test.Fatal(err)
					}
					roots.AddCert(certificate)
				}
				clientConfig := &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: version, MaxVersion: version, CurvePreferences: []tls.CurveID{tls.CurveP256}, CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, NextProtos: []string{"h2", "http/1.1"}}
				waitForServingCertificate(test, address, clientConfig, 1)
				if endpoint == "metrics" {
					for token, wantStatus := range map[string]int{"": http.StatusUnauthorized, "allowed-fixture": http.StatusOK, "denied-fixture": http.StatusForbidden} {
						transport := &http.Transport{TLSClientConfig: clientConfig.Clone()}
						client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
						request, err := http.NewRequest(http.MethodGet, "https://"+address()+"/metrics", nil)
						if err != nil {
							test.Fatal(err)
						}
						if token != "" {
							request.Header.Set("Authorization", "Bearer "+token)
						}
						response, err := client.Do(request)
						if err != nil {
							test.Fatal(err)
						}
						response.Body.Close()
						transport.CloseIdleConnections()
						if response.StatusCode != wantStatus {
							test.Fatalf("metrics status=%d want=%d", response.StatusCode, wantStatus)
						}
					}
				}
				for _, denied := range []string{"version", "group", "cipher"} {
					if denied == "cipher" && version == tls.VersionTLS13 {
						continue
					}
					config := clientConfig.Clone()
					switch denied {
					case "version":
						config.MinVersion, config.MaxVersion = version-1, version-1
					case "group":
						config.CurvePreferences = []tls.CurveID{tls.CurveP384}
					case "cipher":
						config.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384}
					}
					connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", address(), config)
					if err == nil {
						connection.Close()
						test.Fatalf("%s accepted prohibited %s", endpoint, denied)
					}
				}
				writeServingCertificate(test, certDir, nextDER)
				waitForServingCertificate(test, address, clientConfig, 2)
			})
		}
	}
}

func servingCertificate(test *testing.T, key *rsa.PrivateKey, serial int64) []byte {
	test.Helper()
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		test.Fatal(err)
	}
	return der
}

func writeServingCertificate(test *testing.T, directory string, der []byte) {
	test.Helper()
	temporary := filepath.Join(directory, "next.crt")
	if err := os.WriteFile(temporary, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		test.Fatal(err)
	}
	if err := os.Rename(temporary, filepath.Join(directory, "tls.crt")); err != nil {
		test.Fatal(err)
	}
}

func waitForServingCertificate(test *testing.T, address func() string, config *tls.Config, serial int64) {
	test.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if address() != "" {
			connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address(), config)
			if err == nil {
				state := connection.ConnectionState()
				connection.Close()
				if state.NegotiatedProtocol != "http/1.1" || state.Version != config.MinVersion {
					test.Fatal("endpoint lost ALPN/version policy")
				}
				if state.PeerCertificates[0].SerialNumber.Int64() == serial {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	test.Fatal("endpoint certificate/readiness did not converge")
}

func serveMetricsAuthorization(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	var object map[string]any
	if err := json.NewDecoder(request.Body).Decode(&object); err != nil {
		response.WriteHeader(http.StatusBadRequest)
		return
	}
	spec, _ := object["spec"].(map[string]any)
	if object["kind"] == "TokenReview" {
		token, _ := spec["token"].(string)
		object["status"] = map[string]any{"authenticated": true, "user": map[string]string{"username": token}}
	} else {
		object["status"] = map[string]any{"allowed": spec["user"] == "allowed-fixture"}
	}
	if err := json.NewEncoder(response).Encode(object); err != nil {
		return
	}
}
