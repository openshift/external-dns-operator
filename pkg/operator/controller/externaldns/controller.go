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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName                                         = "external_dns_controller"
	ExternalDNSAdmittedConditionType                       = "Admitted"
	ExternalDNSPodsScheduledConditionType                  = "PodsScheduled"
	ExternalDNSDeploymentAvailableConditionType            = "DeploymentAvailable"
	ExternalDNSDeploymentReplicasMinAvailableConditionType = "DeploymentReplicasMinAvailable"
	ExternalDNSDeploymentReplicasAllAvailableConditionType = "DeploymentReplicasAllAvailable"
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

// clock is to enable unit testing
var clock utilclock.Clock = utilclock.RealClock{}

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
	if deploymentExists {
		externalDNS.Status.Conditions = MergeConditions(externalDNS.Status.Conditions, computeDeploymentAvailableCondition(currentDeployment))
		externalDNS.Status.ObservedGeneration = externalDNS.Generation
		externalDNS.Status.Zones = externalDNS.Spec.DeepCopy().Zones
	} else {
		unknownCondition := metav1.Condition{
			Type:               ExternalDNSDeploymentAvailableConditionType,
			Status:             metav1.ConditionUnknown,
			Reason:             "DeploymentAvailabilityUnknown",
			Message:            "The deployment has no Available status condition set",
			LastTransitionTime: metav1.NewTime(clock.Now()),
		}
		externalDNS.Status.Conditions = MergeConditions(externalDNS.Status.Conditions, unknownCondition)
	}
	if err := r.client.Status().Update(ctx, externalDNS); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to update externalDNS custom resource %s/%s: %w", externalDNS.Namespace, externalDNS.Name, err)
	}
	return reconcile.Result{}, nil
}

func computeDeploymentAvailableCondition(deployment *appsv1.Deployment) metav1.Condition {
	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			switch cond.Status {
			case corev1.ConditionFalse:
				return metav1.Condition{
					Type:               ExternalDNSDeploymentAvailableConditionType,
					Status:             metav1.ConditionFalse,
					Reason:             "DeploymentUnavailable",
					Message:            fmt.Sprintf("The deployment has Available status condition set to False (reason: %s) with message: %s", cond.Reason, cond.Message),
					LastTransitionTime: cond.LastUpdateTime,
				}
			case corev1.ConditionTrue:
				return metav1.Condition{
					Type:               ExternalDNSDeploymentAvailableConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             "DeploymentAvailable",
					Message:            "The deployment has Available status condition set to True",
					LastTransitionTime: cond.LastUpdateTime,
				}
			}
			break
		}
	}
	return metav1.Condition{
		Type:               ExternalDNSDeploymentAvailableConditionType,
		Status:             metav1.ConditionUnknown,
		Reason:             "DeploymentAvailabilityUnknown",
		Message:            "The deployment has no Available status condition set",
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}

}

func MergeConditions(conditions []metav1.Condition, updates ...metav1.Condition) []metav1.Condition {
	now := metav1.NewTime(clock.Now())
	for i, update := range updates {
		add := true
		for j, cond := range conditions {
			if cond.Type == update.Type {
				add = false
				if conditionChanged(cond, update) {
					conditions[j] = update
					conditions[j].LastTransitionTime = now
					break
				}
			}
		}
		if add {
			updates[i].LastTransitionTime = now
			conditions = append(conditions, updates[i])
		}
	}
	return conditions
}

func conditionChanged(a, b metav1.Condition) bool {
	return a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message
}
