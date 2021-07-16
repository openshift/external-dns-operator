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

package externaldnscontroller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "externaldns_controller"
)

// Config holds all the things necessary for the controller to run.
type Config struct {
	// Namespace is the namespace that ExternalDNS should be deployed in.
	Namespace string
	// Image is the ExternalDNS image to use.
	Image string
}

// reconciler reconciles an ExternalDNS object.
type reconciler struct {
	config Config
	client client.Client
	log    logr.Logger
}

// New creates the externaldns controller from mgr and cfg. The controller will be pre-configured
// to watch for ExternalDNS objects across all namespaces.
func New(mgr manager.Manager, cfg Config) (controller.Controller, error) {
	r := &reconciler{
		config: cfg,
		client: mgr.GetClient(),
		log:    ctrl.Log.WithName(controllerName),
	}

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &operatorv1alpha1.ExternalDNS{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile reconciles watched objects and attempts to make the current state of
// the object match the desired state.
func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result := reconcile.Result{}

	externalDNS := &operatorv1alpha1.ExternalDNS{}
	if err := r.client.Get(ctx, req.NamespacedName, externalDNS); err != nil {
		if errors.IsNotFound(err) {
			r.log.Info("externalDNS not found; reconciliation will be skipped", "request", req)
			return result, nil
		}
		return result, fmt.Errorf("failed to get externalDNS %s: %w", req, err)
	}

	if _, _, err := r.ensureExternalDNSNamespace(ctx, r.config.Namespace); err != nil {
		// Return if the externalDNS namespace cannot be created since
		// resource creation in a namespace that does not exist will fail.
		return result, fmt.Errorf("failed to ensure externalDNS namespace: %w", err)
	}

	haveServiceAccount, sa, err := r.ensureExternalDNSServiceAccount(ctx, r.config.Namespace, externalDNS)
	if err != nil {
		return result, fmt.Errorf("failed to ensure externalDNS service account: %w", err)
	} else if !haveServiceAccount {
		return result, fmt.Errorf("failed to get externalDNS service account: %w", err)
	}

	if _, _, err := r.ensureExternalDNSDeployment(ctx, r.config.Namespace, r.config.Image, sa, externalDNS); err != nil {
		return result, fmt.Errorf("failed to ensure externalDNS deployment: %w", err)
	}

	return result, nil
}
