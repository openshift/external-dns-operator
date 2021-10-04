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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	controllerName = "external_dns_controller"
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
	scheme *runtime.Scheme
	log    logr.Logger
}

// New creates the externaldns controller from mgr and cfg. The controller will be pre-configured
// to watch for ExternalDNS objects across all namespaces.
func New(mgr manager.Manager, cfg Config) (controller.Controller, error) {
	r := &reconciler{
		config: cfg,
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
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
	reqLogger := r.log.WithValues("externaldns", req.NamespacedName)
	reqLogger.Info("reconciling externalDNS")

	externalDNS := &operatorv1alpha1.ExternalDNS{}
	if err := r.client.Get(ctx, req.NamespacedName, externalDNS); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("externalDNS not found; reconciliation will be skipped")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get externalDNS %s: %w", req, err)
	}

	if _, _, err := r.ensureExternalDNSClusterRole(ctx); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS cluster role: %w", err)
	}

	if _, _, err := r.ensureExternalDNSNamespace(ctx, r.config.Namespace); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS namespace: %w", err)
	}

	haveServiceAccount, sa, err := r.ensureExternalDNSServiceAccount(ctx, r.config.Namespace, externalDNS)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS service account: %w", err)
	} else if !haveServiceAccount {
		return reconcile.Result{}, fmt.Errorf("failed to get externalDNS service account: %w", err)
	}

	if _, _, err := r.ensureExternalDNSClusterRoleBinding(ctx, r.config.Namespace, externalDNS); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS cluster role binding: %w", err)
	}

	deploymentExists, currentDeployment, err := r.ensureExternalDNSDeployment(ctx, r.config.Namespace, r.config.Image, sa, externalDNS)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS deployment: %w", err)
	}
	if err := updateExternalDNSStatus(r.client, ctx, externalDNS, deploymentExists, currentDeployment); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to update externalDNS custom resource %s/%s: %w", externalDNS.Namespace, externalDNS.Name, err)
	}

	return reconcile.Result{}, nil
}
