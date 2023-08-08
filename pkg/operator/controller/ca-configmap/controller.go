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

package ca_configmap

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	extdnscontroller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	ctrlutils "github.com/openshift/external-dns-operator/pkg/operator/controller/utils"
)

const (
	controllerName = "ca_configmap_controller"
)

// Config holds all the things necessary for the controller to run.
type Config struct {
	SourceNamespace string
	TargetNamespace string
	CAConfigMapName string
}

type reconciler struct {
	client client.Client
	config Config
	log    logr.Logger
}

// New creates a new controller that syncs the configmap containing trusted CA(s)
// between the operator and operand namespaces.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	log := ctrl.Log.WithName(controllerName)
	operatorCache := mgr.GetCache()

	reconciler := &reconciler{
		client: mgr.GetClient(),
		config: config,
		log:    log,
	}
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}

	targetToSource := func(ctx context.Context, o client.Object) []reconcile.Request {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: config.SourceNamespace,
					Name:      config.CAConfigMapName,
				},
			},
		}
	}

	// Watch the configmap from the source namespace
	if err := c.Watch(
		source.Kind(operatorCache, &corev1.ConfigMap{}),
		&handler.EnqueueRequestForObject{},
		predicate.And(predicate.NewPredicateFuncs(ctrlutils.InNamespace(config.SourceNamespace)), predicate.NewPredicateFuncs(ctrlutils.HasName(config.CAConfigMapName))),
	); err != nil {
		return nil, err
	}

	// Watch the configmap from the target namespace
	// and enqueue the one from the source namespace
	if err := c.Watch(
		source.Kind(operatorCache, &corev1.ConfigMap{}),
		handler.EnqueueRequestsFromMapFunc(targetToSource),
		predicate.And(predicate.NewPredicateFuncs(ctrlutils.InNamespace(config.TargetNamespace)), predicate.NewPredicateFuncs(ctrlutils.HasName(extdnscontroller.ExternalDNSDestTrustedCAConfigMapName("").Name))),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile reconciles the configmap from the operand namespace
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("configmap", request.NamespacedName)
	reqLogger.Info("reconciling trusted CA configmap")

	cm := &corev1.ConfigMap{}
	if err := r.client.Get(ctx, request.NamespacedName, cm); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("configmap not found; reconciliation will be skipped")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get configmap %q: %w", request.NamespacedName, err)
	}

	if _, _, err := r.ensureTrustedCAConfigMap(ctx); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure the trusted CA configmap: %w", err)
	}

	reqLogger.Info("trusted CA configmap is reconciled")

	return reconcile.Result{}, nil
}
