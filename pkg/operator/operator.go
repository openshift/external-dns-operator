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

package operator

import (
	"context"
	"crypto/tls"
	"fmt"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
	operatorctrl "github.com/openshift/external-dns-operator/pkg/operator/controller"
	caconfigmapctrl "github.com/openshift/external-dns-operator/pkg/operator/controller/ca-configmap"
	credsecretctrl "github.com/openshift/external-dns-operator/pkg/operator/controller/credentials-secret"
	externaldnsctrl "github.com/openshift/external-dns-operator/pkg/operator/controller/externaldns"
)

// Operator holds the manager for the ExternalDNS opreator.
type Operator struct {
	manager manager.Manager
}

// Aggregate kubebuilder RBAC tags in one location for simplicity.
// +kubebuilder:rbac:groups=externaldns.olm.openshift.io,resources=externaldnses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=externaldns.olm.openshift.io,resources=externaldnses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=externaldns.olm.openshift.io,resources=externaldnses/finalizers,verbs=update
// +kubebuilder:rbac:groups=cloudcredential.openshift.io,resources=credentialsrequests;credentialsrequests/status;credentialsrequests/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;watch;list
// +kubebuilder:rbac:groups=config.openshift.io,resources=infrastructures,verbs=get;list;watch
// local role
// +kubebuilder:rbac:groups="",namespace=external-dns-operator,resources=secrets;serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",namespace=external-dns-operator,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",namespace=external-dns-operator,resources=pods,verbs=get;list;watch

// New creates a new operator from cliCfg and opCfg.
func New(cliCfg *rest.Config, opCfg *operatorconfig.Config) (*Operator, error) {
	webhookSrv := &webhook.Server{
		TLSOpts: []func(config *tls.Config){
			func(config *tls.Config) {
				if opCfg.WebhookDisableHTTP2 {
					config.NextProtos = []string{"http/1.1"}
				}
			},
		},
	}

	mgrOpts := manager.Options{
		Scheme:                 GetOperatorScheme(),
		MetricsBindAddress:     opCfg.MetricsBindAddress,
		HealthProbeBindAddress: opCfg.HealthProbeBindAddress,
		Namespace:              opCfg.OperatorNamespace,
		NewCache: cache.MultiNamespacedCacheBuilder([]string{
			opCfg.OperatorNamespace,
			opCfg.OperandNamespace,
		}),
		CertDir: opCfg.CertDir,
		// Use a non-caching client everywhere. The default split client does not
		// promise to invalidate the cache during writes (nor does it promise
		// sequential create/get coherence), and we have code which (probably
		// incorrectly) assumes a get immediately following a create/update will
		// return the updated resource. All client consumers will need audited to
		// ensure they are tolerant of stale data (or we need a cache or client that
		// makes stronger coherence guarantees).
		NewClient: func(_ cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
			return client.New(config, options)
		},
		LeaderElection:   opCfg.EnableLeaderElection,
		LeaderElectionID: "leaderelection.externaldns.olm.openshift.io",
		WebhookServer:    webhookSrv,
	}

	mgr, err := ctrl.NewManager(cliCfg, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	if opCfg.EnableWebhook {
		if err = (&operatorv1beta1.ExternalDNS{}).SetupWebhookWithManager(mgr, opCfg.IsOpenShift); err != nil {
			return nil, fmt.Errorf("unable to setup webhook for ExternalDNS: %w", err)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		return nil, fmt.Errorf("unable to setup ready check: %w", err)
	}

	if err = opCfg.FillPlatformDetails(context.TODO(), mgr.GetClient()); err != nil {
		return nil, fmt.Errorf("failed to fill the platform details: %w", err)
	}

	// Create and register the externaldns controller with the operator manager.
	if _, err := externaldnsctrl.New(mgr, externaldnsctrl.Config{
		Namespace:         opCfg.OperandNamespace,
		Image:             opCfg.ExternalDNSImage,
		OperatorNamespace: opCfg.OperatorNamespace,
		IsOpenShift:       opCfg.IsOpenShift,
		PlatformStatus:    opCfg.PlatformStatus,
		InjectTrustedCA:   opCfg.InjectTrustedCA(),
		RequeuePeriod:     opCfg.RequeuePeriod(),
	}); err != nil {
		return nil, fmt.Errorf("failed to create externaldns controller: %w", err)
	}

	// Create and register the credentials secret controller with the operator manager.
	if _, err := credsecretctrl.New(mgr, credsecretctrl.Config{
		SourceNamespace: operatorctrl.ExternalDNSCredentialsSourceNamespace(opCfg),
		TargetNamespace: opCfg.OperandNamespace,
		IsOpenShift:     opCfg.IsOpenShift,
	}); err != nil {
		return nil, fmt.Errorf("failed to create credentials secret controller: %w", err)
	}

	if opCfg.InjectTrustedCA() {
		// Create and register the CA config map controller with the operator manager.
		if _, err := caconfigmapctrl.New(mgr, caconfigmapctrl.Config{
			SourceNamespace: operatorctrl.ExternalDNSCredentialsSourceNamespace(opCfg),
			TargetNamespace: opCfg.OperandNamespace,
			CAConfigMapName: opCfg.TrustedCAConfigMapName,
		}); err != nil {
			return nil, fmt.Errorf("failed to create CA configmap controller: %w", err)
		}
	}

	return &Operator{
		manager: mgr,
	}, nil
}

// Start starts the operator synchronously until a message is received from ctx.
func (o *Operator) Start(ctx context.Context) error {
	return o.manager.Start(ctx)
}
