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
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	controlleroperator "github.com/openshift/external-dns-operator/pkg/operator/controller"
	ctrlutils "github.com/openshift/external-dns-operator/pkg/operator/controller/utils"
	operatorutils "github.com/openshift/external-dns-operator/pkg/utils"
)

// Config holds all the things necessary for the controller to run.
type Config struct {
	// Namespace is the namespace that ExternalDNS should be deployed in.
	Namespace string
	// Image is the ExternalDNS image to use.
	Image string
	// OperatorNamespace is the namespace in which this operator is deployed.
	OperatorNamespace string
	// IsOpenShift is the flag which instructs the operator that it runs in OpenShift.
	IsOpenShift bool
	// PlatformStatus is the details about the underlying platform.
	PlatformStatus *configv1.PlatformStatus
	// InjectTrustedCA is the flag which instructs the operator to inject the trusted CA into ExternalDNS containers.
	InjectTrustedCA bool
	// RequeuePeriod is the period to wait after a failed reconciliation.
	RequeuePeriod time.Duration
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
	log := ctrl.Log.WithName(controlleroperator.ControllerName)

	r := &reconciler{
		config: cfg,
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		log:    log,
	}

	c, err := controller.New(controlleroperator.ControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &operatorv1beta1.ExternalDNS{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &operatorv1beta1.ExternalDNS{},
	}); err != nil {
		return nil, err
	}

	if err := c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &operatorv1beta1.ExternalDNS{},
	}); err != nil {
		return nil, err
	}

	// secret replicated by the credentials controller
	// needs to trigger the reconciliation of the corresponding ExternalDNS
	// because of the annotation with the secret's hash in the operand deployment
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &operatorv1beta1.ExternalDNS{},
	}); err != nil {
		return nil, err
	}

	// enqueue all ExternalDNS instances if the trusted CA config map changed
	// EnqueueRequestForOwner won't work here
	// because the trusted CA configmap doesn't belong to any particular ExternalDNS instance
	allExtDNSInstances := func(o client.Object) []reconcile.Request {
		externalDNSList := &operatorv1beta1.ExternalDNSList{}
		requests := []reconcile.Request{}
		if err := mgr.GetCache().List(context.Background(), externalDNSList); err != nil {
			log.Error(err, "failed to list externalDNS for trusted CA configmap")
			return requests
		}
		for _, ed := range externalDNSList.Items {
			log.Info("queueing externalDNS for trusted CA configmap", "name", ed.Name)
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: ed.Name,
				},
			}
			requests = append(requests, request)
		}
		return requests
	}
	if err := c.Watch(
		&source.Kind{Type: &corev1.ConfigMap{}},
		handler.EnqueueRequestsFromMapFunc(allExtDNSInstances),
		// only the target trusted CA configmap
		predicate.NewPredicateFuncs(ctrlutils.InNamespace(cfg.Namespace)),
		predicate.NewPredicateFuncs(ctrlutils.HasName(controlleroperator.ExternalDNSDestTrustedCAConfigMapName(cfg.Namespace).Name)),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile reconciles watched objects and attempts to make the current state of
// the object match the desired state.
func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := r.log.WithValues("externaldns", req.NamespacedName)
	reqLogger.Info("reconciling externalDNS")

	externalDNS := &operatorv1beta1.ExternalDNS{}
	if err := r.client.Get(ctx, req.NamespacedName, externalDNS); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("externalDNS not found; reconciliation will be skipped")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get externalDNS %s: %w", req, err)
	}

	// request credentials from CCO only if all of the following is true:
	//  - underlying platform is OpenShift
	//  - a credentials secret is required
	//  - DNS provider is supported by CCO
	//  - no credentials secret was provided
	credSecretRequired := operatorutils.NeedsCredentialSecret(externalDNS)
	if r.config.IsOpenShift && credSecretRequired &&
		operatorutils.ManagedCredentialsProvider(externalDNS) &&
		controlleroperator.ExternalDNSCredentialsSecretNameFromProvider(externalDNS) == "" {
		if _, _, err := r.ensureExternalCredentialsRequest(ctx, externalDNS); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to ensure credentials request for externalDNS: %w", err)
		}
	}

	haveServiceAccount, sa, err := r.ensureExternalDNSServiceAccount(ctx, r.config.Namespace, externalDNS)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS service account: %w", err)
	} else if !haveServiceAccount {
		return reconcile.Result{}, fmt.Errorf("failed to get externalDNS service account: %w", err)
	}

	var credSecret *corev1.Secret
	if credSecretRequired {
		credSecretNsName := controlleroperator.ExternalDNSDestCredentialsSecretName(r.config.Namespace, externalDNS.Name)
		credSecretExists, credSecretCurrent, err := r.currentExternalDNSSecret(ctx, credSecretNsName)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to get the target credentials secret: %w", err)
		}

		if !credSecretExists {
			// show that the secret is not there yet
			if err := r.updateExternalDNSStatus(ctx, externalDNS, nil, false); err != nil {
				reqLogger.Error(err, "failed to update externalDNS custom resource")
			}
			// credentials secret was not synced yet or doesn't exist at all,
			// either way: no need to requeue immediately polluting the logs.
			return reconcile.Result{RequeueAfter: r.config.RequeuePeriod}, fmt.Errorf("target credentials secret %s not found", credSecretNsName)
		}
		credSecret = credSecretCurrent
	}

	var trustCAConfigMap *corev1.ConfigMap
	if r.config.InjectTrustedCA {
		configMapNsName := controlleroperator.ExternalDNSDestTrustedCAConfigMapName(r.config.Namespace)
		configMapExists, configMap, err := r.currentExternalDNSTrustedCAConfigMap(ctx, configMapNsName)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to get the target CA configmap: %w", err)
		}
		if !configMapExists {
			// trusted CA configmap was not synced yet or doesn't exist at all,
			// either way: no need to requeue immediately polluting the logs.
			return reconcile.Result{RequeueAfter: r.config.RequeuePeriod}, fmt.Errorf("target CA configmap %s not found", configMapNsName)
		}
		trustCAConfigMap = configMap
	}

	_, currentDeployment, err := r.ensureExternalDNSDeployment(ctx, r.config.Namespace, r.config.Image, sa, credSecret, trustCAConfigMap, externalDNS)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure externalDNS deployment: %w", err)
	}

	if err := r.updateExternalDNSStatus(ctx, externalDNS, currentDeployment, true); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to update externalDNS custom resource %s: %w", externalDNS.Name, err)
	}

	return reconcile.Result{}, nil
}
