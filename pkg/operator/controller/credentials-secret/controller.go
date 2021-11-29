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

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	extdnscontroller "github.com/openshift/external-dns-operator/pkg/operator/controller"
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
// between the config and operand namespaces.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	log := ctrl.Log.WithName(controllerName)

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
		&source.Kind{Type: &operatorv1alpha1.ExternalDNS{}},
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return hasSecret(e.Object, config.IsOpenShift)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return hasSecret(e.Object, config.IsOpenShift)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldED := e.ObjectOld.(*operatorv1alpha1.ExternalDNS)
				newED := e.ObjectNew.(*operatorv1alpha1.ExternalDNS)
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
		&operatorv1alpha1.ExternalDNS{},
		credentialsSecretIndexFieldName,
		client.IndexerFunc(func(o client.Object) []string {
			ed := o.(*operatorv1alpha1.ExternalDNS)
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
	credSecretToExtDNS := func(o client.Object) []reconcile.Request {
		externalDNSList := &operatorv1alpha1.ExternalDNSList{}
		listOpts := client.MatchingFields{credentialsSecretIndexFieldName: o.GetName()}
		requests := []reconcile.Request{}
		if err := reconciler.cache.List(context.Background(), externalDNSList, listOpts); err != nil {
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
		&operatorv1alpha1.ExternalDNS{},
		credentialsSecretIndexFieldNameInOperand,
		client.IndexerFunc(func(o client.Object) []string {
			ed := o.(*operatorv1alpha1.ExternalDNS)
			name := extdnscontroller.ExternalDNSDestCredentialsSecretName("", ed.Name).Name
			if len(name) == 0 {
				return []string{}
			}
			return []string{name}
		}),
	); err != nil {
		return nil, fmt.Errorf("failed to create index for credentials secret: %w", err)
	}

	credSecretToExtDNSTargetNS := func(o client.Object) []reconcile.Request {
		externalDNSList := &operatorv1alpha1.ExternalDNSList{}
		listOpts := client.MatchingFields{credentialsSecretIndexFieldNameInOperand: o.GetName()}
		requests := []reconcile.Request{}
		if err := reconciler.cache.List(context.Background(), externalDNSList, listOpts); err != nil {
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
		&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(credSecretToExtDNS),
		predicate.NewPredicateFuncs(isInNS(config.SourceNamespace)),
	); err != nil {
		return nil, err
	}

	// Watch secrets from the target namespace
	// and if a secret was indexed as belonging to ExternalDNS
	// we send the reconcile requests with all the ExternalDNS resources
	// which referenced it
	if err := c.Watch(
		&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(credSecretToExtDNSTargetNS),
		predicate.NewPredicateFuncs(isInNS(config.TargetNamespace)),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile reconciles an ExternalDNS and its associated credentials secret
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("externaldns", request.NamespacedName)
	reqLogger.Info("reconciling credentials secret for externalDNS instance")

	extDNS := &operatorv1alpha1.ExternalDNS{}
	if err := r.client.Get(ctx, request.NamespacedName, extDNS); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("externalDNS not found; reconciliation will be skipped")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to get externalDNS %q: %w", request.NamespacedName, err)
	}

	srcSecretName := types.NamespacedName{
		Namespace: r.config.SourceNamespace,
		Name:      getExternalDNSCredentialsSecretName(extDNS, r.config.IsOpenShift),
	}

	if _, _, err := r.ensureCredentialsSecret(ctx, srcSecretName, extDNS); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to ensure credentials secret for externalDNS %q: %w", extDNS.Name, err)
	}

	reqLogger.Info("credentials secret is reconciled for externalDNS instance")

	return reconcile.Result{}, nil
}

// hasSecret returns true if ExternalDNS references a secret
func hasSecret(o client.Object, isOpenShift bool) bool {
	ed := o.(*operatorv1alpha1.ExternalDNS)
	return len(getExternalDNSCredentialsSecretName(ed, isOpenShift)) != 0
}

// isInNS returns a predicate which checks the belonging to the given namespace
func isInNS(namespace string) func(o client.Object) bool {
	return func(o client.Object) bool {
		return o.GetNamespace() == namespace
	}
}

// getExternalDNSCredentialsSecretName returns the name of the credentials secret retrieved from externalDNS resource
func getExternalDNSCredentialsSecretName(externalDNS *operatorv1alpha1.ExternalDNS, isOpenShift bool) string {
	if isOpenShift && operatorutils.ManagedCredentialsProvider(externalDNS) {
		return extdnscontroller.SecretFromCloudCredentialsOperator
	}
	switch externalDNS.Spec.Provider.Type {
	case operatorv1alpha1.ProviderTypeAWS:
		if externalDNS.Spec.Provider.AWS != nil {
			return externalDNS.Spec.Provider.AWS.Credentials.Name
		}
	case operatorv1alpha1.ProviderTypeAzure:
		if externalDNS.Spec.Provider.Azure != nil {
			return externalDNS.Spec.Provider.Azure.ConfigFile.Name
		}
	case operatorv1alpha1.ProviderTypeGCP:
		if externalDNS.Spec.Provider.GCP != nil {
			return externalDNS.Spec.Provider.GCP.Credentials.Name
		}
	case operatorv1alpha1.ProviderTypeBlueCat:
		if externalDNS.Spec.Provider.BlueCat != nil {
			return externalDNS.Spec.Provider.BlueCat.ConfigFile.Name
		}
	case operatorv1alpha1.ProviderTypeInfoblox:
		if externalDNS.Spec.Provider.Infoblox != nil {
			return externalDNS.Spec.Provider.Infoblox.Credentials.Name
		}
	}
	return ""
}
