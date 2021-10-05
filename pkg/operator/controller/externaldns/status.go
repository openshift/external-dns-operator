package externaldnscontroller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
)

const (
	ExternalDNSAdmittedConditionType                       = "Admitted"
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

func createDeploymentAvailabilityUnknownCondition() metav1.Condition {
	return metav1.Condition{
		Type:               ExternalDNSDeploymentAvailableConditionType,
		Status:             metav1.ConditionUnknown,
		Reason:             "DeploymentAvailabilityUnknown",
		Message:            "The deployment has no Available status condition set",
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}
}
