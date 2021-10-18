package externaldnscontroller

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	ExternalDNSPodsScheduledConditionType                  = "PodsScheduled"
	ExternalDNSDeploymentAvailableConditionType            = "DeploymentAvailable"
	ExternalDNSDeploymentReplicasMinAvailableConditionType = "DeploymentReplicasMinAvailable"
	ExternalDNSDeploymentReplicasAllAvailableConditionType = "DeploymentReplicasAllAvailable"
)

// clock is to enable unit testing
var clock utilclock.Clock = utilclock.RealClock{}

// updateExternalDNSStatus recomputes all conditions given the current deployment and its status
// and pushes the new externalDNS custom resource with updated status through a call to the client.Update function
func (r *reconciler) updateExternalDNSStatus(ctx context.Context, externalDNS *operatorv1alpha1.ExternalDNS, currentDeployment *appsv1.Deployment) error {
	extDNSWithStatus := externalDNS.DeepCopy()
	if currentDeployment != nil {
		extDNSWithStatus.Status.Conditions = mergeConditions(extDNSWithStatus.Status.Conditions,
			computeDeploymentAvailableCondition(currentDeployment),
			computeMinReplicasCondition(currentDeployment),
			computeAllReplicasCondition(currentDeployment),
			computeDeploymentPodsScheduledCondition(ctx, r.client, currentDeployment),
		)
	} else {
		extDNSWithStatus.Status.Conditions = mergeConditions(extDNSWithStatus.Status.Conditions, createDeploymentAvailabilityUnknownCondition())
	}
	extDNSWithStatus.Status.ObservedGeneration = extDNSWithStatus.Generation
	extDNSWithStatus.Status.Zones = extDNSWithStatus.Spec.Zones
	if !externalDNSStatusesEqual(extDNSWithStatus.Status, externalDNS.Status) {
		return r.client.Status().Update(ctx, extDNSWithStatus)
	}
	return nil
}

// computeDeploymentAvailableCondition returns an externalDNS condition based on the deployment status & its conditions
func computeDeploymentAvailableCondition(deployment *appsv1.Deployment) metav1.Condition {
	for _, cond := range deployment.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			switch cond.Status {
			case corev1.ConditionFalse:
				return metav1.Condition{
					Type:    ExternalDNSDeploymentAvailableConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  "DeploymentUnavailable",
					Message: fmt.Sprintf("The deployment has Available status condition set to False (reason: %s) with message: %s", cond.Reason, cond.Message),
				}
			case corev1.ConditionTrue:
				return metav1.Condition{
					Type:    ExternalDNSDeploymentAvailableConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  "DeploymentAvailable",
					Message: "The deployment has Available status condition set to True",
				}
			}
			break
		}
	}
	return createDeploymentAvailabilityUnknownCondition()

}

// computeMinReplicasCondition returns an externalDNS condition based on the deployment, its number of desired pods,
// its maxUnavailable, maxSurge, and the number of available replicas.
// The condition is true if the number of available replicas is at least the number of desired replicas minus maxUnavailable.
func computeMinReplicasCondition(deployment *appsv1.Deployment) metav1.Condition {
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	pointerTo := func(val intstr.IntOrString) *intstr.IntOrString { return &val }
	maxUnavailableIntStr := pointerTo(intstr.FromString("25%"))
	maxSurgeIntStr := pointerTo(intstr.FromString("25%"))
	if deployment.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && deployment.Spec.Strategy.RollingUpdate != nil {
		if deployment.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			maxUnavailableIntStr = deployment.Spec.Strategy.RollingUpdate.MaxUnavailable
		}
		if deployment.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			maxSurgeIntStr = deployment.Spec.Strategy.RollingUpdate.MaxSurge
		}
	}
	maxSurge, err := intstr.GetScaledValueFromIntOrPercent(maxSurgeIntStr, int(replicas), true)
	if err != nil {
		return metav1.Condition{
			Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "InvalidMaxSurgeValue",
			Message: fmt.Sprintf("invalid value for max surge: %v", err),
		}
	}
	maxUnavailable, err := intstr.GetScaledValueFromIntOrPercent(maxUnavailableIntStr, int(replicas), false)
	if err != nil {
		return metav1.Condition{
			Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "InvalidMaxUnavailableValue",
			Message: fmt.Sprintf("invalid value for max unavailable: %v", err),
		}
	}
	if maxSurge == 0 && maxUnavailable == 0 {
		//Use a default value here
		maxUnavailable = 1
	}
	if int(deployment.Status.AvailableReplicas) < int(replicas)-maxUnavailable {
		return metav1.Condition{
			Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "DeploymentMinimumReplicasNotMet",
			Message: fmt.Sprintf("%d/%d of replicas are available, max unavailable is %d", deployment.Status.AvailableReplicas, replicas, maxUnavailable),
		}
	}

	return metav1.Condition{
		Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "DeploymentMinimumReplicasMet",
		Message: "Minimum replicas requirement is met",
	}
}

// computeAllReplicasCondition returns an externalDNS condition based on the deployment, its number of desired pods
// and the number of available replicas.
// The condition is true if the number of available replicas is the same as or greater than the number of desired replicas.
func computeAllReplicasCondition(deployment *appsv1.Deployment) metav1.Condition {
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	if deployment.Status.AvailableReplicas < replicas {
		return metav1.Condition{
			Type:    ExternalDNSDeploymentReplicasAllAvailableConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "DeploymentReplicasNotAvailable",
			Message: fmt.Sprintf("%d/%d of replicas are available", deployment.Status.AvailableReplicas, replicas),
		}
	}

	return metav1.Condition{
		Type:    ExternalDNSDeploymentReplicasAllAvailableConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "DeploymentReplicasAvailable",
		Message: "All replicas are available",
	}
}

// computeDeploymentPodsScheduledCondition lists the pods matching the namespace and the label selector of the deployment.
// Returns condition true when all matching pods are scheduled.
// Returns condition false if no matching pods were found, or if any of the pods is unscheduled.
func computeDeploymentPodsScheduledCondition(ctx context.Context, cl client.Client, deployment *appsv1.Deployment) metav1.Condition {
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil || selector.Empty() {
		return createPodsScheduledUnknownCondition("InvalidLabelSelector", "Deployment has an invalid label selector.")
	}
	pods, err := getFilteredPodsList(ctx, cl, deployment.Namespace, selector)
	if err != nil {
		return createPodsScheduledUnknownCondition("PodScheduledUnknown", "Unable to list pods: "+err.Error())
	}
	if len(pods) == 0 {
		return metav1.Condition{
			Type:    ExternalDNSPodsScheduledConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "NoLabelMatchingPods",
			Message: fmt.Sprintf("No matching pods found for label selector: %v", deployment.Spec.Selector),
		}
	}
	unscheduled := make(map[*corev1.Pod]corev1.PodCondition)
	for i, pod := range pods {
		for j, cond := range pod.Status.Conditions {
			if cond.Type != corev1.PodScheduled {
				continue
			}
			if cond.Status == corev1.ConditionTrue {
				continue
			}
			unscheduled[&pods[i]] = pod.Status.Conditions[j]
		}
	}
	if len(unscheduled) != 0 {
		var haveUnschedulable bool
		message := "Some pods are not scheduled:"
		// Sort keys so that the result is deterministic.
		keys := make([]*corev1.Pod, 0, len(unscheduled))
		for pod := range unscheduled {
			keys = append(keys, pod)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].CreationTimestamp.Equal(&keys[j].CreationTimestamp) {
				return keys[i].UID < keys[j].UID
			}
			return keys[i].CreationTimestamp.Before(&keys[j].CreationTimestamp)
		})
		for _, pod := range keys {
			cond := unscheduled[pod]
			if cond.Reason == corev1.PodReasonUnschedulable {
				haveUnschedulable = true
				message += fmt.Sprintf("%s Pod %q cannot be scheduled: %s", message, pod.Name, cond.Message)
			} else {
				message += fmt.Sprintf("%s Pod %q is not yet scheduled: %s: %s", message, pod.Name, cond.Reason, cond.Message)
			}
		}
		if haveUnschedulable {
			message = message + " Make sure you have sufficient worker nodes."
		}
		return metav1.Condition{
			Type:    ExternalDNSPodsScheduledConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "PodsNotScheduled",
			Message: message,
		}
	}
	return metav1.Condition{
		Type:    ExternalDNSPodsScheduledConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "AllPodsScheduled",
		Message: "All pods are scheduled",
	}

}

// mergeConditions updates the conditions list with new conditions.
// Each condition is added if no condition of the same type already exists.
// Otherwise, the condition is merged with the existing condition of the same type.
func mergeConditions(conditions []metav1.Condition, updates ...metav1.Condition) []metav1.Condition {
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

func externalDNSStatusesEqual(a, b operatorv1alpha1.ExternalDNSStatus) bool {
	if a.ObservedGeneration != b.ObservedGeneration {
		return false
	}
	if !zonesEqual(a.Zones, b.Zones) {
		return false
	}
	return conditionsEqual(a.Conditions, b.Conditions)
}

func zonesEqual(a, b []string) bool {
	zoneCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b string) bool { return a < b }),
	}
	return cmp.Equal(a, b, zoneCmpOpts...)
}
func conditionsEqual(a, b []metav1.Condition) bool {
	conditionCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b metav1.Condition) bool { return a.Type < b.Type }),
	}
	return cmp.Equal(a, b, conditionCmpOpts...)
}

func createDeploymentAvailabilityUnknownCondition() metav1.Condition {
	return metav1.Condition{
		Type:               ExternalDNSDeploymentAvailableConditionType,
		Status:             metav1.ConditionUnknown,
		Reason:             "DeploymentAvailabilityUnknown",
		Message:            "The deployment has no Available status condition set",
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}
}

func createPodsScheduledUnknownCondition(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    ExternalDNSPodsScheduledConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  reason,
		Message: message,
	}
}

// getFilteredPodsList returns the list of PODs matching the provided namespace and label selector
func getFilteredPodsList(ctx context.Context, cl client.Client, namespace string, selector labels.Selector) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}

	if err := cl.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}
	return pods.Items, nil
}
