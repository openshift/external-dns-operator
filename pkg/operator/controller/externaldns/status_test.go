package externaldnscontroller

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComputeDeploymentAvailableCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		expectedResult     metav1.Condition
		errExpected        bool
	}{
		{
			name:               "Deployment in progress should return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentAvailableConditionType,
				Status:             metav1.ConditionUnknown,
				Reason:             "DeploymentAvailabilityUnknown",
				Message:            "The deployment has no Available status condition set",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		{
			name:               "Deployment available status false should return ConditionAvailable False for ExternalDNS ",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionFalse),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "DeploymentUnavailable",
				Message: "The deployment has Available status condition set to False ",
			},
			errExpected: false,
		},
		{
			name:               "Deployment available status true should return ConditionAvailable true for ExternalDNS ",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentAvailable",
				Message: "The deployment has Available status condition set to True",
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeDeploymentAvailableCondition(&tc.existingDeployment)
			require.EqualValues(t, tc.expectedResult.Type, cond.Type)
			require.EqualValues(t, tc.expectedResult.Status, cond.Status)
		})
	}
}

func TestMergeConditions(t *testing.T) {
	testCases := []struct {
		name               string
		existingConditions []metav1.Condition
		updateCondition    metav1.Condition
		expectedResult     []metav1.Condition
		errExpected        bool
	}{
		{
			name:               "Starting by empty conditions, with 1 update, results in 1 condition",
			existingConditions: []metav1.Condition{},
			updateCondition: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "DeploymentAvailabilityUnknown",
				Message: "The deployment has no Available status condition set",
			},
			expectedResult: []metav1.Condition{
				{
					Type:    ExternalDNSDeploymentAvailableConditionType,
					Status:  metav1.ConditionUnknown,
					Reason:  "DeploymentAvailabilityUnknown",
					Message: "The deployment has no Available status condition set",
				},
			},
			errExpected: false,
		},
		{
			name: "Starting with 1 condition, with 1 updated condition, results in 1 condition",
			existingConditions: []metav1.Condition{
				{
					Type:    ExternalDNSDeploymentAvailableConditionType,
					Status:  metav1.ConditionTrue,
					Reason:  "DeploymentAvailable",
					Message: "The deployment has Available status condition set to True",
				},
			},
			updateCondition: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "DeploymentUnavailable",
				Message: "The deployment has Available status condition set to False",
			},
			expectedResult: []metav1.Condition{
				{
					Type:    ExternalDNSDeploymentAvailableConditionType,
					Status:  metav1.ConditionFalse,
					Reason:  "DeploymentUnavailable",
					Message: "The deployment has Available status condition set to False",
				},
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conditions := MergeConditions(tc.existingConditions, tc.updateCondition)
			require.Len(t, conditions, len(tc.expectedResult))
		})
	}
}
