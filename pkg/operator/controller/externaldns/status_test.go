package externaldnscontroller

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/externaldns/test"
)

// Option for comparison of conditions : ignore LastTransitionTime
var ignoreTimeOpt = cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime")

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
				Message: "The deployment has Available status condition set to False (reason: Not really important for test) with message: Not really important for test",
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
			if diff := cmp.Diff(tc.expectedResult, cond, ignoreTimeOpt); diff != "" {
				t.Errorf("expected condition %v; got condition %v: \n %s", tc.expectedResult, cond, diff)
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
			name:               "AvailableReplica = spec.replica - maxUnavailable should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DeploymentMinimumReplicasMet",
				Message: "Minimum replicas requirement is met",
			},
		},
		{
			name:               "AvailableReplica < spec.replica - maxUnavailable should return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 2, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "DeploymentMinimumReplicasNotMet",
				Message: "2/8 of replicas are available, max unavailable is 2",
			},
		},
		{
			name:               "maxUnavailable not parseable should return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "a", "25%", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "InvalidMaxUnavailableValue",
				Message: "invalid value for max unavailable: invalid value for IntOrString: invalid type: string is not a percentage",
			},
		},
		{
			name:               "maxSurge not parseable should return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "b", 8, "external-dns-operator"),
			expectedResult: metav1.Condition{
				Type:    ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  "InvalidMaxSurgeValue",
				Message: "invalid value for max surge: invalid value for IntOrString: invalid type: string is not a percentage",
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
			if diff := cmp.Diff(tc.expectedResult, cond, ignoreTimeOpt); diff != "" {
				t.Errorf("expected condition %v; got condition %v: \n %s", tc.expectedResult, cond, diff)
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
			name:               "AvailableReplica < spec.replica shouLd return ConditionFalse",
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
			if diff := cmp.Diff(tc.expectedResult, cond, ignoreTimeOpt); diff != "" {
				t.Errorf("expected condition %v; got condition %v: \n %s", tc.expectedResult, cond, diff)
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
				Message: "No matching pods found for label selector: &LabelSelector{MatchLabels:map[string]string{name: external-dns-operator,},MatchExpressions:[]LabelSelectorRequirement{},}",
			},
		},
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
		{
			name:               "part of podList matches selector while another doesnt and is not scheduled should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8, "external-dns-operator"),
			existingPods: []corev1.Pod{
				fakePod("otherPod", "external-dns-operator", "not-external-dns", corev1.ConditionFalse, corev1.PodReasonUnschedulable),
				fakePod("extDNSPod", "external-dns-operator", "external-dns-operator", corev1.ConditionTrue, "Scheduled"),
			},
			expectedResult: metav1.Condition{
				Type:    ExternalDNSPodsScheduledConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "AllPodsScheduled",
				Message: "All pods are scheduled",
			},
		},
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
				Message: "Some pods are not scheduled: Pod \"pod\" cannot be scheduled:  Make sure you have sufficient worker nodes.",
			},
		},
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
				Message: "Some pods are not scheduled: Pod \"pod\" is not yet scheduled: : ",
			},
		},
	}

	for _, tc := range testCases {
		fakeObjects := append(fakeRuntimeObjectFromPodList(tc.existingPods), &tc.existingDeployment)
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(fakeObjects...).Build()
		t.Run(tc.name, func(t *testing.T) {
			cond := computeDeploymentPodsScheduledCondition(context.TODO(), cl, &tc.existingDeployment)
			if diff := cmp.Diff(tc.expectedResult, cond, ignoreTimeOpt); diff != "" {
				t.Errorf("expected condition %v; got condition %v: \n %s", tc.expectedResult, cond, diff)
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
			conditionCmpOpts := []cmp.Option{
				cmpopts.EquateEmpty(),
				ignoreTimeOpt,
				cmpopts.SortSlices(func(a, b metav1.Condition) bool { return a.Type < b.Type }),
			}
			if diff := cmp.Diff(tc.expectedResult, conditions, conditionCmpOpts...); diff != "" {
				t.Errorf("mergeConditions result differs from expected:\n%s", diff)
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
		} else if !tc.errExpected {
			if err != nil {
				t.Errorf("expected no error but got %v", err)
			}
			if len(podList) != len(tc.expectedResult) {
				t.Errorf("expected podList to contain %d pods but got %d", len(tc.expectedResult), len(podList))
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
		} else if !tc.errExpected {
			if err != nil {
				t.Errorf("expected no error but got %v", err)
			}
			outputExtDNS := &operatorv1alpha1.ExternalDNS{}
			if err := r.client.Get(context.TODO(), namespacedName, outputExtDNS); err != nil {
				if errors.IsNotFound(err) {
					t.Error("outputExtDNS not found")
				}
			}
			conditionCmpOpts := []cmp.Option{
				cmpopts.EquateEmpty(),
				ignoreTimeOpt,
				cmpopts.SortSlices(func(a, b metav1.Condition) bool { return a.Type < b.Type }),
			}
			if diff := cmp.Diff(tc.expectedResult.Status.Conditions, outputExtDNS.Status.Conditions, conditionCmpOpts...); diff != "" {
				t.Errorf("updateExternalDNSStatus result differs from expected:\n%s", diff)
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
