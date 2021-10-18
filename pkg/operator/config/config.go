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

package config

import (
	"os"
	"path/filepath"
)

const (
	// TODO (alebedev): CPaaS onboarding is ongoing to replace this with Red Hat built image
	DefaultExternalDNSImage  = "k8s.gcr.io/external-dns/external-dns:v0.8.0"
	DefaultMetricsAddr       = "127.0.0.1:8080"
	DefaultOperatorNamespace = "external-dns-operator"
	DefaultOperandNamespace  = "external-dns"
	DefaultEnableWebhook     = true
)

var (
	DefaultCertDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")
)

// Config is configuration of the operator.
type Config struct {
	// ExternalDNSImage is the ExternalDNS image for the ExternalDNS container(s) managed
	// by the operator.
	ExternalDNSImage string

	// MetricsBindAddress is the TCP address that the operator should bind to for
	// serving prometheus metrics. It can be set to "0" to disable the metrics serving.
	MetricsBindAddress string

	// OperatorNamespace is the namespace that the operator is deployed in.
	OperatorNamespace string

	// OperandNamespace is the namespace that the operator should deploy ExternalDNS container(s) in.
	OperandNamespace string

	// CertDir is the directory from where the operator loads keys and certificates.
	CertDir string

	// EnableWebhook is the flag indicating if the webhook server should be started.
	EnableWebhook bool
}

// New returns an operator config using default values.
func New() *Config {
	return &Config{
		ExternalDNSImage:   DefaultExternalDNSImage,
		MetricsBindAddress: DefaultMetricsAddr,
		OperatorNamespace:  DefaultOperatorNamespace,
		OperandNamespace:   DefaultOperandNamespace,
		CertDir:            DefaultCertDir,
		EnableWebhook:      DefaultEnableWebhook,
	}
}
