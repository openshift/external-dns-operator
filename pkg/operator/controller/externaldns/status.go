package externaldnscontroller

import (
	"context"
	"fmt"
	"sort"

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
	return createDeploymentAvailabilityUnknownCondition()

}
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

func computeDeploymentPodsScheduledCondition(deployment *appsv1.Deployment, pods []corev1.Pod) metav1.Condition {
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil || selector.Empty() {
		return metav1.Condition{
			Type:    ExternalDNSPodsScheduledConditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  "InvalidLabelSelector",
			Message: "Deployment has an invalid label selector.",
		}
	}
	hasMatchingPods := false
	unscheduled := make(map[*corev1.Pod]corev1.PodCondition)
	for i, pod := range pods {
		if !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		hasMatchingPods = true
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
	if !hasMatchingPods {
		return metav1.Condition{
			Type:    ExternalDNSPodsScheduledConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "NoLabelMatchingPods",
			Message: fmt.Sprintf("no matching pods found for label selector %s.", selector.String()),
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

func createDeploymentAvailabilityUnknownCondition() metav1.Condition {
	return metav1.Condition{
		Type:               ExternalDNSDeploymentAvailableConditionType,
		Status:             metav1.ConditionUnknown,
		Reason:             "DeploymentAvailabilityUnknown",
		Message:            "The deployment has no Available status condition set",
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}
}

func (r *reconciler) updateExternalDNSStatus(ctx context.Context, client client.Client, externalDNS *operatorv1alpha1.ExternalDNS, currentDeployment *appsv1.Deployment) error {
	extDNSWithStatus := externalDNS //.DeepCopy()
	if currentDeployment != nil {

		extDNSWithStatus.Status.Conditions = MergeConditions(extDNSWithStatus.Status.Conditions, computeDeploymentAvailableCondition(currentDeployment))
		extDNSWithStatus.Status.Conditions = MergeConditions(extDNSWithStatus.Status.Conditions, computeMinReplicasCondition(currentDeployment))
		extDNSWithStatus.Status.Conditions = MergeConditions(extDNSWithStatus.Status.Conditions, computeAllReplicasCondition(currentDeployment))

		pods, err := getPodsList(client, ctx)
		if err != nil {
			extDNSWithStatus.Status.Conditions = MergeConditions(extDNSWithStatus.Status.Conditions, createPodsScheduledUnknownCondition())
		} else {
			extDNSWithStatus.Status.Conditions = MergeConditions(extDNSWithStatus.Status.Conditions, computeDeploymentPodsScheduledCondition(currentDeployment, pods))
		}

	} else {
		extDNSWithStatus.Status.Conditions = MergeConditions(extDNSWithStatus.Status.Conditions, createDeploymentAvailabilityUnknownCondition())
	}
	extDNSWithStatus.Status.ObservedGeneration = extDNSWithStatus.Generation
	extDNSWithStatus.Status.Zones = extDNSWithStatus.Spec.Zones

	return client.Status().Update(ctx, extDNSWithStatus)
}

func createPodsScheduledUnknownCondition() metav1.Condition {
	return metav1.Condition{
		Type:    ExternalDNSPodsScheduledConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "PodScheduledUnknown",
		Message: "unable to list pods",
	}

}

func getPodsList(client client.Client, ctx context.Context) ([]corev1.Pod, error) {
	pods := &corev1.PodList{}
	if err := client.List(ctx, pods); err != nil {
		return nil, err
	}
	return pods.Items, error(nil)
}
