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
	"fmt"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/externaldns"
)

const (
	operatorName = "externaldns_operator"
)

// Clients holds the API clients required by Operator.
type Client struct {
	client.Client
	meta.RESTMapper
}

// Operator hold the client, manager, and logging resources
// for the ExternalDNS opreator.
type Operator struct {
	client  Client
	manager manager.Manager
	log     logr.Logger
}

// Aggregate kubebuilder RBAC tags in one location for simplicity.
// +kubebuilder:rbac:groups=externaldns.olm.openshift.io,resources=externaldnses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=externaldns.olm.openshift.io,resources=externaldnses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=externaldns.olm.openshift.io,resources=externaldnses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces;serviceaccounts,verbs=get;list;watch;delete;create;update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete;create;update

// New creates a new operator from cliCfg and opCfg.
func New(cliCfg *rest.Config, opCfg *operatorconfig.Config) (*Operator, error) {
	mgrOpts := manager.Options{
		Scheme:             GetOperatorScheme(),
		MetricsBindAddress: opCfg.MetricsBindAddress,
		Namespace:          opCfg.OperatorNamespace,
		NewCache: cache.MultiNamespacedCacheBuilder([]string{
			opCfg.OperatorNamespace,
			opCfg.OperandNamespace,
		}),
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
	}

	mgr, err := ctrl.NewManager(cliCfg, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	// Create and register the externaldns controller with the operator manager.
	if _, err := externaldnscontroller.New(mgr, externaldnscontroller.Config{
		Namespace: opCfg.OperandNamespace,
		Image:     opCfg.ExternalDNSImage,
	}); err != nil {
		return nil, fmt.Errorf("failed to create externaldns controller: %w", err)
	}

	restMapper, err := apiutil.NewDiscoveryRESTMapper(cliCfg)
	if err != nil {
		return nil, err
	}

	return &Operator{
		manager: mgr,
		client:  Client{mgr.GetClient(), restMapper},
		log:     ctrl.Log.WithName(operatorName),
	}, nil
}

// Start starts the operator synchronously until a message is received from ctx.
func (o *Operator) Start(ctx context.Context) error {
	return o.manager.Start(ctx)
}
