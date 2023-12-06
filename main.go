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

package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/openshift/external-dns-operator/pkg/operator"
	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
	"github.com/openshift/external-dns-operator/pkg/version"
	//+kubebuilder:scaffold:imports
)

var (
	opCfg operatorconfig.Config
)

func main() {
	flag.StringVar(&opCfg.MetricsBindAddress, "metrics-bind-address", operatorconfig.DefaultMetricsAddr, "The address the metric endpoint binds to.")
	flag.StringVar(&opCfg.OperatorNamespace, "operator-namespace", operatorconfig.DefaultOperatorNamespace, "The namespace that the operator is running in.")
	flag.StringVar(&opCfg.OperandNamespace, "operand-namespace", operatorconfig.DefaultOperandNamespace, "The namespace that ExternalDNS containers should run in.")
	flag.StringVar(&opCfg.ExternalDNSImage, "externaldns-image", operatorconfig.DefaultExternalDNSImage, "The container image used for running ExternalDNS.")
	flag.StringVar(&opCfg.CertDir, "cert-dir", operatorconfig.DefaultCertDir, "The directory for keys and certificates for serving the webhook.")
	flag.StringVar(&opCfg.TrustedCAConfigMapName, "trusted-ca-configmap", operatorconfig.DefaultTrustedCAConfigMapName, "The name of the config map containing TLS CA(s) which should be trusted by ExternalDNS containers. PEM encoded file under \"ca-bundle.crt\" key is expected.")
	flag.BoolVar(&opCfg.EnableWebhook, "enable-webhook", operatorconfig.DefaultEnableWebhook, "Enable the validating webhook server. Defaults to true.")
	flag.BoolVar(&opCfg.EnablePlatformDetection, "enable-platform-detection", operatorconfig.DefaultEnablePlatformDetection, "Enable the detection of the underlying platform. Defaults to true.")
	flag.StringVar(&opCfg.HealthProbeBindAddress, "health-probe-bind-addr", operatorconfig.DefaultHealthProbeAddr, "The address the health endpoint binds to.")
	flag.IntVar(&opCfg.RequeuePeriodSeconds, "requeue-period", operatorconfig.DefaultRequeuePeriodSeconds, "Requeue period for a failed reconciliation (in seconds).")
	flag.BoolVar(&opCfg.EnableLeaderElection, "leader-elect", operatorconfig.DefaultEnableLeaderElection, "Enable leader election for controller manager to ensure there is only one active controller manager.")
	flag.BoolVar(&opCfg.WebhookDisableHTTP2, "webhook-disable-http2", false, "Disable HTTP/2 for the webhook server.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")
	ctrl.Log.Info("build info", "commit", version.COMMIT)
	ctrl.Log.Info("using operator namespace", "namespace", opCfg.OperatorNamespace)
	ctrl.Log.Info("using operand namespace", "namespace", opCfg.OperandNamespace)
	ctrl.Log.Info("using ExternalDNS image", "image", opCfg.ExternalDNSImage)

	kubeConfig := ctrl.GetConfigOrDie()
	if err := opCfg.DetectPlatform(kubeConfig); err != nil {
		setupLog.Error(err, "failed to detect the platform")
		os.Exit(1)
	}

	op, err := operator.New(kubeConfig, &opCfg)
	if err != nil {
		setupLog.Error(err, "failed to create externaldns operator")
		os.Exit(1)
	}

	setupLog.Info("starting externalDNS operator")
	if err := op.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "failed to start externaldns operator")
		os.Exit(1)
	}
}
