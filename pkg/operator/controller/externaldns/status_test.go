package externaldnscontroller

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/externaldns/test"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestComputeDeploymentAvailableCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		expectedResult     metav1.Condition
	}{
		{
			name:               "Deployment in progress should return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "DeploymentAvailabilityUnknown",
				Message: "The deployment has no Available status condition set",
			},
		},
		{
			name:               "Deployment available status false should return ConditionAvailable False for ExternalDNS ",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionFalse, 8, "25%", "25%", 0, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "DeploymentUnavailable",
				Message: "The deployment has Available status condition set to False ",
			},
		},
		{
			name:               "Deployment available status true should return ConditionAvailable true for ExternalDNS ",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentAvailable",
				Message: "The deployment has Available status condition set to True",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeDeploymentAvailableCondition(&tc.existingDeployment)
			if cond.Type != tc.expectedResult.Type {
				t.Errorf("expected condition type %v; got condition type %v", tc.expectedResult.Type, cond.Type)
			}
			if cond.Status != tc.expectedResult.Status {
				t.Errorf("expected condition status %v; got condition status %v", tc.expectedResult.Status, cond.Status)
			}
			if cond.Reason != tc.expectedResult.Reason {
				t.Errorf("expected condition reason %v; got condition reason %v", tc.expectedResult.Reason, cond.Reason)
			}
			if !strings.Contains(cond.Message, tc.expectedResult.Message) {
				t.Errorf("expected condition message %v; got condition message %v", tc.expectedResult.Message, cond.Message)
			}
		})
	}
}

func TestComputeMinReplicasCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		expectedResult     metav1.Condition
	}{
		{
			name:               "AvailableReplica = spec.replica (25% maxUnavailable) should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionFalse, 8, "25%", "25%", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentMinimumReplicasMet",
				Message: "Minimum replicas requirement is met",
			},
		},
		{
			name:               "AvailableReplica = spec.replica - maxUnavailable shoud return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentMinimumReplicasMet",
				Message: "Minimum replicas requirement is met",
			},
		},
		{
			name:               "AvailableReplica < spec.replica - maxUnavailable shoud return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 2, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "DeploymentMinimumReplicasNotMet",
				Message: "2/8 of replicas are available, max unavailable is 2",
			},
		},
		{
			name:               "maxUnavailable unparsable shoud return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "a", "25%", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "InvalidMaxUnavailableValue",
				Message: "invalid value for max unavailable",
			},
		},
		{
			name:               "maxSurge unparsable shoud return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "b", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "InvalidMaxSurgeValue",
				Message: "invalid value for max surge",
			},
		},
		{
			name:               "maxSurge = maxUnavailable = 0 should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "0%", "0%", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentMinimumReplicasMet",
				Message: "Minimum replicas requirement is met",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeMinReplicasCondition(&tc.existingDeployment)
			if cond.Type != tc.expectedResult.Type {
				t.Errorf("expected condition type %v; got condition type %v", tc.expectedResult.Type, cond.Type)
			}
			if cond.Status != tc.expectedResult.Status {
				t.Errorf("expected condition status %v; got condition status %v", tc.expectedResult.Status, cond.Status)
			}
			if cond.Reason != tc.expectedResult.Reason {
				t.Errorf("expected condition reason %v; got condition reason %v", tc.expectedResult.Reason, cond.Reason)
			}
			if !strings.Contains(cond.Message, tc.expectedResult.Message) {
				t.Errorf("expected condition message %v; got condition message %v", tc.expectedResult.Message, cond.Message)
			}
		})
	}
}

func TestComputeAllReplicasCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		expectedResult     metav1.Condition
	}{
		{
			name:               "AvailableReplica = spec.replica should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasAllAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentReplicasAvailable",
				Message: "All replicas are available",
			},
		},
		{
			name:               "AvailableReplica < spec.replica shoud return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasAllAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "DeploymentReplicasNotAvailable",
				Message: "6/8 of replicas are available",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeAllReplicasCondition(&tc.existingDeployment)
			if cond.Type != tc.expectedResult.Type {
				t.Errorf("expected condition type %v; got condition type %v", tc.expectedResult.Type, cond.Type)
			}
			if cond.Status != tc.expectedResult.Status {
				t.Errorf("expected condition status %v; got condition status %v", tc.expectedResult.Status, cond.Status)
			}
			if cond.Reason != tc.expectedResult.Reason {
				t.Errorf("expected condition reason %v; got condition reason %v", tc.expectedResult.Reason, cond.Reason)
			}
			if !strings.Contains(cond.Message, tc.expectedResult.Message) {
				t.Errorf("expected condition message %v; got condition message %v", tc.expectedResult.Message, cond.Message)
			}
		})
	}
}

func TestComputeDeploymentPodsScheduledCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		existingPods       []corev1.Pod
		expectedResult     metav1.Condition
	}{
		//all podScheduledConditions are true
		{
			name:               "All pods are scheduled should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			existingPods: []corev1.Pod{
				fakePod("pod", "external-dns-operator", "external-dns-operator", corev1.ConditionTrue, "Scheduled"),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "AllPodsScheduled",
				Message: "All pods are scheduled",
			},
		},
		//deployment selector invalid
		{
			name:               "Deployment selector empty or invalid should return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, ""),
			existingPods: []corev1.Pod{
				fakePod("pod", "external-dns-operator", "external-dns-operator", corev1.ConditionTrue, "Scheduled"),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "InvalidLabelSelector",
				Message: "Deployment has an invalid label selector.",
			},
		},
		//pods filtered empty, unrelated
		{
			name:               "No pods matching deployment selector should return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			existingPods: []corev1.Pod{
				fakePod("pod", "external-dns-operator", "not-external-dns", corev1.ConditionTrue, "Scheduled"),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "NoLabelMatchingPods",
				Message: "no matching pods found for label selector",
			},
		},
		//pods has mix of related  & unrelated
		{
			name:               "part of podList matches selector should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			existingPods: []corev1.Pod{
				fakePod("otherPod", "external-dns-operator", "not-external-dns", corev1.ConditionTrue, "Scheduled"),
				fakePod("extDNSPod", "external-dns-operator", "external-dns-operator", corev1.ConditionTrue, "Scheduled"),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "AllPodsScheduled",
				Message: "All pods are scheduled",
			},
		},
		//some pods are unschedulable
		{
			name:               "Pod unschedulable in list should return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			existingPods: []corev1.Pod{
				fakePod("pod", "external-dns-operator", "external-dns-operator", corev1.ConditionFalse, corev1.PodReasonUnschedulable),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "PodsNotScheduled",
				Message: "Make sure you have sufficient worker nodes.",
			},
		},
		//some pods not yet scheduled
		{
			name:               "unscheduled pods in list should return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			existingPods: []corev1.Pod{
				fakePod("pod", "external-dns-operator", "external-dns-operator", corev1.ConditionFalse, ""),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "PodsNotScheduled",
				Message: "Some pods are not scheduled",
			},
		},
	}

	for _, tc := range testCases {
		fakeObjects := append(fakeRuntimeObjectFromPodList(tc.existingPods), &tc.existingDeployment)
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(fakeObjects...).Build()
		t.Run(tc.name, func(t *testing.T) {
			cond := computeDeploymentPodsScheduledCondition(context.TODO(), cl, &tc.existingDeployment)
			if cond.Type != tc.expectedResult.Type {
				t.Errorf("expected condition type %v; got condition type %v", tc.expectedResult.Type, cond.Type)
			}
			if cond.Status != tc.expectedResult.Status {
				t.Errorf("expected condition status %v; got condition status %v", tc.expectedResult.Status, cond.Status)
			}
			if cond.Reason != tc.expectedResult.Reason {
				t.Errorf("expected condition reason %v; got condition reason %v", tc.expectedResult.Reason, cond.Reason)
			}
			if !strings.Contains(cond.Message, tc.expectedResult.Message) {
				t.Errorf("expected condition message %v; got condition message %v", tc.expectedResult.Message, cond.Message)
			}
		})
	}
}

func TestMergeConditions(t *testing.T) {
	testCases := []struct {
		name               string
		existingConditions []metav1.Condition
		updateCondition    metav1.Condition
		expectedResult     []metav1.Condition
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
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conditions := mergeConditions(tc.existingConditions, tc.updateCondition)
			if len(conditions) != len(tc.expectedResult) {
				t.Errorf("expected number of conditions %v; got %v conditions", len(tc.expectedResult), len(conditions))
			}
			for _, expectedCondition := range tc.expectedResult {
				isConditionTypeFound := false
				for _, cond := range conditions {
					if cond.Type == expectedCondition.Type {
						isConditionTypeFound = true
						if cond.Status != expectedCondition.Status {
							t.Errorf("expected condition status %v; got condition status %v", expectedCondition.Status, cond.Status)
						}
						if cond.Reason != expectedCondition.Reason {
							t.Errorf("expected condition reason %v; got condition reason %v", expectedCondition.Reason, cond.Reason)
						}
						if !strings.Contains(cond.Message, expectedCondition.Message) {
							t.Errorf("expected condition message %v; got condition message %v", expectedCondition.Message, cond.Message)
						}
					}
				}
				if !isConditionTypeFound {
					t.Errorf("expected condition type %v was not found in the result", expectedCondition.Type)
				}
			}
		})
	}
}

func TestGetPodsList(t *testing.T) {
	testCases := []struct {
		name            string
		existingObjects []runtime.Object
		expectedResult  []corev1.Pod
		errExpected     bool
	}{
		//nominal case
		{
			name:            "Nominal case",
			existingObjects: fakeRuntimeObjectFromPodList(fakePodList()),
			expectedResult: []corev1.Pod{
				fakePod("pod", "external-dns-operator", "external-dns-operator", corev1.ConditionTrue, "Scheduled"),
			},
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()
		aSelector := &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"name": "external-dns-operator",
			},
		}
		labelSelector, err := metav1.LabelSelectorAsSelector(aSelector)
		if err != nil {
			t.Error("Unable to create labelSelector for label external-dns-operator")
		}
		podList, err := getFilteredPodsList(context.TODO(), cl, "external-dns-operator", labelSelector)
		if tc.errExpected && err == nil {
			t.Error("expected an error but got none")
		} else {
			if err != nil {
				t.Errorf("expected no error but got %v", err)
			}
			if len(podList) != 1 {
				t.Errorf("expected podList to contain 2 pods but got %v", len(podList))
			}
			for _, expectedPod := range tc.expectedResult {
				isPodFound := false
				for _, pod := range podList {
					if pod.Name == expectedPod.Name {
						isPodFound = true
						if !reflect.DeepEqual(pod.Status, expectedPod.Status) {
							t.Errorf("pod status %v is different from expected %v", pod.Status, expectedPod.Status)
						}
					}
				}
				if !isPodFound {
					t.Errorf("expected pod %v was not found in the result", expectedPod)
				}
			}
		}
	}
}

func TestUpdateExternalDNSStatus(t *testing.T) {
	aDeployment := fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator")
	anExternalDNS := fakeExternalDNS()
	namespacedName := types.NamespacedName{
		Namespace: "",
		Name:      test.Name,
	}
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		existingObjects    []runtime.Object
		existingExtDNS     *operatorv1alpha1.ExternalDNS
		errExpected        bool
		expectedResult     operatorv1alpha1.ExternalDNS
	}{
		//nominal case
		{
			name:               "Nominal case",
			existingDeployment: aDeployment,
			existingObjects:    append(fakeRuntimeObjectFromPodList(fakePodList()), &aDeployment, anExternalDNS),
			existingExtDNS:     anExternalDNS,
			errExpected:        false,
			expectedResult:     fakeExternalDNSWithStatus(),
		},
	}
	for _, tc := range testCases {
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()
		r := &reconciler{
			client: cl,
			scheme: test.Scheme,
			config: testConfig(),
			log:    zap.New(zap.UseDevMode(true)),
		}

		err := r.updateExternalDNSStatus(context.TODO(), tc.existingExtDNS, &tc.existingDeployment)
		if tc.errExpected && err == nil {
			t.Error("expected an error but got none")
		} else {
			if err != nil {
				t.Errorf("expected no error but got %v", err)
			}
			outputExtDNS := &operatorv1alpha1.ExternalDNS{}
			if err := r.client.Get(context.TODO(), namespacedName, outputExtDNS); err != nil {
				if errors.IsNotFound(err) {
					t.Error("outputExtDNS not found")
				}
			}
			if len(outputExtDNS.Status.Conditions) != 4 {
				t.Errorf("expected externalDNS.Status to contain 4 conditions but got %v", len(tc.existingExtDNS.Status.Conditions))
			}
			for _, expectedCondition := range tc.expectedResult.Status.Conditions {
				isConditionTypeFound := false
				for _, cond := range outputExtDNS.Status.Conditions {
					if cond.Type == expectedCondition.Type {
						isConditionTypeFound = true
						if cond.Status != expectedCondition.Status {
							t.Errorf("expected condition status %v; got condition status %v", expectedCondition.Status, cond.Status)
						}
						if cond.Reason != expectedCondition.Reason {
							t.Errorf("expected condition reason %v; got condition reason %v", expectedCondition.Reason, cond.Reason)
						}
						if !strings.Contains(cond.Message, expectedCondition.Message) {
							t.Errorf("expected condition message %v; got condition message %v", expectedCondition.Message, cond.Message)
						}
					}
				}
				if !isConditionTypeFound {
					t.Errorf("expected condition type %v was not found in the result", expectedCondition.Type)
				}
			}
		}
	}
}

func fakePodList() []corev1.Pod {
	return []corev1.Pod{
		fakePod("anotherPod", "external-dns-operator", "not-external-dns", corev1.ConditionTrue, "Scheduled"),
		fakePod("pod", "external-dns-operator", "external-dns-operator", corev1.ConditionTrue, "Scheduled"),
	}
}

func fakeRuntimeObjectFromPodList(podList []corev1.Pod) []runtime.Object {
	runtimeObjects := make([]runtime.Object, len(podList))
	for j := range podList {
		runtimeObjects[j] = &podList[j]
	}
	return runtimeObjects
}

func fakeDeployment(condType appsv1.DeploymentConditionType, isAvailable corev1.ConditionStatus, specReplicas int32, maxUnavailable string, maxSurge string, availableReplicas int32, selectorLabel string) appsv1.Deployment {
	pointerToInt32 := func(i int32) *int32 { return &i }
	pointerToIntVal := func(val intstr.IntOrString) *intstr.IntOrString { return &val }
	selector := &metav1.LabelSelector{}
	if selectorLabel != "" {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"name": selectorLabel,
			},
		}
	}

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(specReplicas),
			Selector: selector,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       pointerToIntVal(intstr.FromString(maxSurge)),
					MaxUnavailable: pointerToIntVal(intstr.FromString(maxUnavailable)),
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  1,
			Replicas:            availableReplicas,
			UpdatedReplicas:     availableReplicas,
			ReadyReplicas:       availableReplicas,
			AvailableReplicas:   availableReplicas,
			UnavailableReplicas: specReplicas - availableReplicas,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:    condType,
					Status:  isAvailable,
					Reason:  "Not really important for test",
					Message: "Not really important for test",
				},
			},
		},
	}
}

func fakeExternalDNS() *operatorv1alpha1.ExternalDNS {
	return &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Provider: operatorv1alpha1.ExternalDNSProvider{
				Type: operatorv1alpha1.ProviderTypeAWS,
				AWS: &operatorv1alpha1.ExternalDNSAWSProviderOptions{
					Credentials: operatorv1alpha1.SecretReference{
						Name: testSecretName,
					},
				},
			},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type: operatorv1alpha1.SourceTypeService,
					Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
						ServiceType: []corev1.ServiceType{
							corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			Zones: []string{"public-zone"},
		},
	}
}

func fakeExternalDNSWithStatus() operatorv1alpha1.ExternalDNS {
	extDNS := fakeExternalDNS()
	condDeploymentAvailable := metav1.Condition{
		Type:    ExternalDNSDeploymentAvailableConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "DeploymentAvailable",
		Message: "The deployment has Available status condition set to True",
	}
	condAllReplicaAvailable := metav1.Condition{
		Type:    ExternalDNSDeploymentReplicasAllAvailableConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "DeploymentReplicasAvailable",
		Message: "All replicas are available",
	}
	condMinReplicaAvailable := metav1.Condition{
		Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "DeploymentMinimumReplicasMet",
		Message: "Minimum replicas requirement is met",
	}
	CondPodScheduled := metav1.Condition{
		Type:    ExternalDNSPodsScheduledConditionType,
		Status:  metav1.ConditionTrue,
		Reason:  "AllPodsScheduled",
		Message: "All pods are scheduled",
	}
	extDNS.Status.Conditions = append(extDNS.Status.Conditions, condDeploymentAvailable)
	extDNS.Status.Conditions = append(extDNS.Status.Conditions, condAllReplicaAvailable)
	extDNS.Status.Conditions = append(extDNS.Status.Conditions, condMinReplicaAvailable)
	extDNS.Status.Conditions = append(extDNS.Status.Conditions, CondPodScheduled)

	return *extDNS
}

func fakePod(name string, namespace string, selectorLabel string, status corev1.ConditionStatus, reason string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"name": selectorLabel,
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodScheduled,
				Status: status,
				Reason: reason,
			}},
		},
	}
}
