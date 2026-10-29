package operator

import (
	"crypto/tls"

	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
)

func servingOptions(config *operatorconfig.Config, policy *tls.Config) (webhook.Options, metrics.Options) {
	webhookOptions := webhook.Options{
		CertDir: config.CertDir,
		TLSOpts: []func(*tls.Config){
			applyTLSPolicy(policy),
			func(tlsConfig *tls.Config) {
				if config.WebhookDisableHTTP2 {
					tlsConfig.NextProtos = []string{"http/1.1"}
				}
			},
		},
	}
	metricsOptions := metrics.Options{
		BindAddress:    config.MetricsBindAddress,
		SecureServing:  true,
		CertDir:        config.MetricsTLSCertDir,
		FilterProvider: filters.WithAuthenticationAndAuthorization,
		TLSOpts: []func(*tls.Config){
			applyTLSPolicy(policy),
			func(tlsConfig *tls.Config) {
				tlsConfig.NextProtos = []string{"http/1.1"}
			},
		},
	}
	return webhookOptions, metricsOptions
}
