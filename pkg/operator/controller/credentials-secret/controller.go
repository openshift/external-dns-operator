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

package credentials_secret

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	extdnscontroller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	ctrlutils "github.com/openshift/external-dns-operator/pkg/operator/controller/utils"
	operatorutils "github.com/openshift/external-dns-operator/pkg/utils"
)

const (
	controllerName                           = "credentials_secret_controller"
	credentialsSecretIndexFieldName          = "credentialsSecretName"
	credentialsSecretIndexFieldNameInOperand = "credentialsSecretNameofOperand"
)

// Config holds all the things necessary for the controller to run.
type Config struct {
	SourceNamespace string
	TargetNamespace string
	IsOpenShift     bool
}

type reconciler struct {
	cache  cache.Cache
	scheme *runtime.Scheme
	client client.Client
	config Config
	log    logr.Logger
}

// New creates a new controller that syncs ExternalDNS' providers credentials secrets
// between the operator and operand namespaces.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	log := ctrl.Log.WithName(controllerName)
	operatorCache := mgr.GetCache()

	reconciler := &reconciler{
		cache:  mgr.GetCache(),
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: config,
		log:    log,
	}
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: reconciler,
	})
	if err != nil {
		return nil, err
	}

	// Enqueue if ExternalDNS references a secret or if the secret name changes
	if err := c.Watch(
		source.Kind(operatorCache, &operatorv1beta1.ExternalDNS{}),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return hasSecret(e.Object, config.IsOpenShift)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return hasSecret(e.Object, config.IsOpenShift)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldED := e.ObjectOld.(*operatorv1beta1.ExternalDNS)
				newED := e.ObjectNew.(*operatorv1beta1.ExternalDNS)
				oldName := getExternalDNSCredentialsSecretName(oldED, config.IsOpenShift)
				newName := getExternalDNSCredentialsSecretName(newED, config.IsOpenShift)
				return oldName != newName || oldED.DeletionTimestamp != newED.DeletionTimestamp
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return hasSecret(e.Object, config.IsOpenShift)
			},
		},
	); err != nil {
		return nil, err
	}

	// Index ExternalDNS instances by Spec.Provider.*.Credentials
	// so that we can look up ExternalDNS when the secret is changed.
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&operatorv1beta1.ExternalDNS{},
		credentialsSecretIndexFieldName,
		client.IndexerFunc(func(o client.Object) []string {
			ed := o.(*operatorv1beta1.ExternalDNS)
			name := getExternalDNSCredentialsSecretName(ed, config.IsOpenShift)
			if len(name) == 0 {
				return []string{}
			}
			return []string{name}
		}),
	); err != nil {
		return nil, fmt.Errorf("failed to create index for credentials secret: %w", err)
	}

	// function to get all ExternalDNS resources which match the secret's index key
	credSecretToExtDNS := func(ctx context.Context, o client.Object) []reconcile.Request {
		externalDNSList := &operatorv1beta1.ExternalDNSList{}
		listOpts := client.MatchingFields{credentialsSecretIndexFieldName: o.GetName()}
		requests := []reconcile.Request{}
		if err := reconciler.cache.List(ctx, externalDNSList, listOpts); err != nil {
			log.Error(err, "failed to list externalDNS for secret")
			return requests
		}
		for _, ed := range externalDNSList.Items {
			log.Info("queueing externalDNS", "name", ed.Name)
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: ed.Name,
				},
			}
			requests = append(requests, request)
		}
		return requests
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&operatorv1beta1.ExternalDNS{},
		credentialsSecretIndexFieldNameInOperand,
		client.IndexerFunc(func(o client.Object) []string {
			ed := o.(*operatorv1beta1.ExternalDNS)
			name := extdnscontroller.ExternalDNSDestCredentialsSecretName("", ed.Name).Name
			if len(name) == 0 {
				return []string{}
			}
			return []string{name}
		}),
	); err != nil {
		return nil, fmt.Errorf("failed to create index for credentials secret: %w", err)
	}

	credSecretToExtDNSTargetNS := func(ctx context.Context, o client.Object) []reconcile.Request {
		externalDNSList := &operatorv1beta1.ExternalDNSList{}
		listOpts := client.MatchingFields{credentialsSecretIndexFieldNameInOperand: o.GetName()}
		requests := []reconcile.Request{}
		if err := reconciler.cache.List(ctx, externalDNSList, listOpts); err != nil {
			log.Error(err, "failed to list externalDNS for secret")
			return requests
		}
		for _, ed := range externalDNSList.Items {
			log.Info("queueing externalDNS", "name", ed.Name)
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: ed.Name,
				},
			}
			requests = append(requests, request)
		}
		return requests
	}

	// Watch secrets from the source namespace
	// and if a secret was indexed as belonging to ExternalDNS
	// we send the reconcile requests with all the ExternalDNS resources
	// which referenced it
	if err := c.Watch(
		source.Kind(operatorCache, &corev1.Secret{}),
		handler.EnqueueRequestsFromMapFunc(credSecretToExtDNS),
		predicate.NewPredicateFuncs(ctrlutils.InNamespace(config.SourceNamespace)),
	); err != nil {
		return nil, err
	}

	// Watch secrets from the target namespace
	// and if a secret was indexed as belonging to ExternalDNS
	// we send the reconcile requests with all the ExternalDNS resources
	// which referenced it
	if err := c.Watch(
		source.Kind(operatorCache, &corev1.Secret{}),
		handler.EnqueueRequestsFromMapFunc(credSecretToExtDNSTargetNS),
		predicate.NewPredicateFuncs(ctrlutils.InNamespace(config.TargetNamespace)),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile reconciles an ExternalDNS and its associated credentials secret
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("externaldns", request.NamespacedName)
	reqLogger.Info("reconciling credentials secret for externalDNS instance")

	extDNS := &operatorv1beta1.ExternalDNS{}
	if err := r.client.Get(ctx, request.NamespacedName, extDNS); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("externalDNS not found; reconciliation will be skipped")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get externalDNS %q: %w", request.NamespacedName, err)
	}

	// get the source secret name and whether it came from ExternalDNS CR or not
	srcSecretNameOnly, fromCR := getExternalDNSCredentialsSecretNameWithTrace(extDNS, r.config.IsOpenShift)
	srcSecretName := types.NamespacedName{
		Namespace: r.config.SourceNamespace,
		Name:      srcSecretNameOnly,
	}

	if _, _, err := r.ensureCredentialsSecret(ctx, srcSecretName, extDNS, fromCR); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure credentials secret for externalDNS %q: %w", extDNS.Name, err)
	}

	reqLogger.Info("credentials secret is reconciled for externalDNS instance")

	return reconcile.Result{}, nil
}

// hasSecret returns true if ExternalDNS references a secret
func hasSecret(o client.Object, isOpenShift bool) bool {
	ed := o.(*operatorv1beta1.ExternalDNS)
	return len(getExternalDNSCredentialsSecretName(ed, isOpenShift)) != 0
}

// getExternalDNSCredentialsSecretName returns the name of the credentials secret which should be used as source
func getExternalDNSCredentialsSecretName(externalDNS *operatorv1beta1.ExternalDNS, isOpenShift bool) string {
	name, _ := getExternalDNSCredentialsSecretNameWithTrace(externalDNS, isOpenShift)
	return name
}

// getExternalDNSCredentialsSecretNameWithTrace returns the name of the credentials secret which should be used as source
// second value is true if the secret came from the ExternalDNS' provider, false otherwise
func getExternalDNSCredentialsSecretNameWithTrace(externalDNS *operatorv1beta1.ExternalDNS, isOpenShift bool) (string, bool) {
	if name := extdnscontroller.ExternalDNSCredentialsSecretNameFromProvider(externalDNS); name != "" {
		return name, true
	}

	if isOpenShift && operatorutils.ManagedCredentialsProvider(externalDNS) {
		return extdnscontroller.SecretFromCloudCredentialsOperator, false
	}

	return "", false
}
