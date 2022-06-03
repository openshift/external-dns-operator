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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	configv1 "github.com/openshift/api/config/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TODO (alebedev): CPaaS onboarding is ongoing to replace this with Red Hat built image
	DefaultExternalDNSImage        = "quay.io/alebedev/openshift-external-dns:0.12.0-rc2"
	DefaultMetricsAddr             = "127.0.0.1:8080"
	DefaultOperatorNamespace       = "external-dns-operator"
	DefaultOperandNamespace        = "external-dns"
	DefaultEnableWebhook           = true
	DefaultEnablePlatformDetection = true
	DefaultTrustedCAConfigMapName  = ""
	DefaultHealthProbeAddr         = ":9440"

	openshiftKind              = "OpenShiftAPIServer"
	openshiftResourceGroup     = "operator.openshift.io"
	openshiftResourceVersion   = "v1"
	openshiftClusterConfigName = "cluster"
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

	// EnablePlatformDetection is the flag indicating if the operator needs to detect on which platform it runs.
	EnablePlatformDetection bool

	// IsOpenShift is the flag indicating that the operator runs in OpenShift cluster.
	IsOpenShift bool

	// PlatformStatus is the details about the underlying platform.
	PlatformStatus *configv1.PlatformStatus

	// TrustedCAConfigMapName is the name of the configmap containing CA bundle to be trusted by ExternalDNS containers.
	TrustedCAConfigMapName string

	// HealthProbeBindAddress is the TCP address that the operator should bind to for
	// serving health probes (readiness and liveness).
	HealthProbeBindAddress string
}

// DetectPlatform detects the underlying platform and fills corresponding config fields
func (c *Config) DetectPlatform(kubeConfig *rest.Config) error {
	if c.EnablePlatformDetection {
		kubeClient, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}

		c.IsOpenShift = isOCP(kubeClient)
	}
	return nil
}

// FillPlatformDetails fills the config with the platform details
func (c *Config) FillPlatformDetails(ctx context.Context, ctrlClient ctrlclient.Client) error {
	if c.IsOpenShift {
		infraConfig := &configv1.Infrastructure{}
		if err := ctrlClient.Get(ctx, types.NamespacedName{Name: openshiftClusterConfigName}, infraConfig); err != nil {
			return fmt.Errorf("failed to get infrastructure config: %w", err)
		}
		c.PlatformStatus = infraConfig.Status.PlatformStatus
	}
	return nil
}

// InjectTrustedCA returns true if the trusted CA needs to be injected into ExternalDNS containers.
func (c *Config) InjectTrustedCA() bool {
	return len(strings.TrimSpace(c.TrustedCAConfigMapName)) != 0
}

// isOCP returns true if the platform is OCP
func isOCP(kubeClient discovery.DiscoveryInterface) bool {
	// Since, CRD for OpenShift API Server was introduced in OCP v4.x we can verify if the current cluster is on OCP v4.x by
	// ensuring that resource exists against Group(operator.openshift.io), Version(v1) and Kind(OpenShiftAPIServer)
	// In case it doesn't exist we assume that external dns is running on non OCP 4.x environment
	resources, err := kubeClient.ServerResourcesForGroupVersion(openshiftResourceGroup + "/" + openshiftResourceVersion)
	if err != nil {
		return false
	}

	for _, apiResource := range resources.APIResources {
		if apiResource.Kind == openshiftKind {
			return true
		}
	}
	return false
}
