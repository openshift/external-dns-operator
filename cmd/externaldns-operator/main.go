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
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")
	ctrl.Log.Info("using operator namespace", "namespace", opCfg.OperatorNamespace)
	ctrl.Log.Info("using operand namespace", "namespace", opCfg.OperandNamespace)
	ctrl.Log.Info("using ExternalDNS image", "image", opCfg.ExternalDNSImage)

	op, err := operator.New(ctrl.GetConfigOrDie(), &opCfg)
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
