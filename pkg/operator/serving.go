package operator

import (
	"context"
	"crypto/tls"
	"fmt"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
)

func loadServingOptions(ctx context.Context, cliCfg *rest.Config, opCfg *operatorconfig.Config) (*tlsProfile, webhook.Options, metrics.Options, error) {
	profile, err := loadTLSProfile(ctx, cliCfg, opCfg.IsOpenShift)
	if err != nil {
		return nil, webhook.Options{}, metrics.Options{}, fmt.Errorf("resolve TLS profile: %w", err)
	}
	policy, err := tlsConfigFromProfile(profile.spec)
	if err != nil {
		return nil, webhook.Options{}, metrics.Options{}, fmt.Errorf("convert TLS profile: %w", err)
	}
	webhookOpts, metricsOpts := servingOptions(opCfg, policy)
	return profile, webhookOpts, metricsOpts, nil
}

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
