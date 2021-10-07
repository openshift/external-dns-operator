package externaldnscontroller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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
		errExpected        bool
	}{
		{
			name:               "Deployment in progress should return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6),
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
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionFalse, 8, "25%", "25%", 0),
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
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8),
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

func TestComputeMinReplicasCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		expectedResult     metav1.Condition
		errExpected        bool
	}{
		{
			name:               "AvailableReplica = spec.replica (25% maxUnavailable) should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionFalse, 8, "25%", "25%", 8),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             "DeploymentMinimumReplicasMet",
				Message:            "Minimum replicas requirement is met",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		{
			name:               "AvailableReplica = spec.replica - maxUnavailable shoud return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             "DeploymentMinimumReplicasMet",
				Message:            "Minimum replicas requirement is met",
				LastTransitionTime: metav1.NewTime(clock.Now())},
			errExpected: false,
		},
		{
			name:               "AvailableReplica < spec.replica - maxUnavailable shoud return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 2),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "DeploymentMinimumReplicasNotMet",
				Message:            "Not relevant for this test",
				LastTransitionTime: metav1.NewTime(clock.Now())},
			errExpected: false,
		},
		{
			name:               "maxUnavailable unparsable shoud return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "a", "25%", 8),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:             metav1.ConditionUnknown,
				Reason:             "InvalidMaxUnavailableValue",
				Message:            "Not relevant for this test",
				LastTransitionTime: metav1.NewTime(clock.Now())},
			errExpected: false,
		},
		{
			name:               "maxSurge unparsable shoud return ConditionUnknown",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "b", 8),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:             metav1.ConditionUnknown,
				Reason:             "InvalidMaxSurgeValue",
				Message:            "Not relevant for this test",
				LastTransitionTime: metav1.NewTime(clock.Now())},
			errExpected: false,
		},
		{
			name:               "maxSurge = maxUnavailable = 0 should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "0%", "0%", 8),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasMinAvailableConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             "DeploymentMinimumReplicasMet",
				Message:            "Minimum replicas requirement is met",
				LastTransitionTime: metav1.NewTime(clock.Now())},
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeMinReplicasCondition(&tc.existingDeployment)
			require.EqualValues(t, tc.expectedResult.Type, cond.Type)
			require.EqualValues(t, tc.expectedResult.Status, cond.Status)
		})
	}
}

func TestComputeAllReplicasCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		expectedResult     metav1.Condition
		errExpected        bool
	}{
		{
			name:               "AvailableReplica = spec.replica should return ConditionTrue",
			existingDeployment: fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionTrue, 8, "25%", "25%", 8),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasAllAvailableConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             "DeploymentReplicasAvailable",
				Message:            "All replicas are available",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		{
			name:               "AvailableReplica < spec.replica shoud return ConditionFalse",
			existingDeployment: fakeDeployment(appsv1.DeploymentProgressing, corev1.ConditionFalse, 8, "25%", "25%", 6),
			expectedResult: metav1.Condition{
				Type:               ExternalDNSDeploymentReplicasAllAvailableConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "DeploymentReplicasNotAvailable",
				Message:            "Irrelevant for test",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeAllReplicasCondition(&tc.existingDeployment)
			require.EqualValues(t, tc.expectedResult.Type, cond.Type)
			require.EqualValues(t, tc.expectedResult.Status, cond.Status)
		})
	}
}

func TestComputeDeploymentPodsScheduledCondition(t *testing.T) {
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		existingPods       []corev1.Pod
		expectedResult     metav1.Condition
		errExpected        bool
	}{
		//all podScheduledConditions are true
		{
			name: "All pods are scheduled should return ConditionTrue",
			existingDeployment: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"name": "external-dns-operator",
						},
					},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "external-dns-operator",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			expectedResult: metav1.Condition{
				Type:               ExternalDNSPodsScheduledConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             "AllPodsScheduled",
				Message:            "All pods are scheduled",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		//deployment selector invalid
		{
			name: "Deployment selector empty or invalid should return ConditionUnknown",
			existingDeployment: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "external-dns-operator",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			expectedResult: metav1.Condition{
				Type:               ExternalDNSPodsScheduledConditionType,
				Status:             metav1.ConditionUnknown,
				Reason:             "InvalidLabelSelector",
				Message:            "Deployment has an invalid label selector.",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		//pods filtered empty, unrelated
		{
			name: "No pods matching deployment selector should return ConditionFalse",
			existingDeployment: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"name": "external-dns-operator",
						},
					},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "not-external-dns",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			expectedResult: metav1.Condition{
				Type:               ExternalDNSPodsScheduledConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "NoLabelMatchingPods",
				Message:            "no matching pods found for label selector",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		//pods has mix of related  & unrelated
		{
			name: "part of podList matches selector should return ConditionTrue",
			existingDeployment: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"name": "external-dns-operator",
						},
					},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "not-external-dns",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "external-dns-operator",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						}},
					},
				},
			},
			expectedResult: metav1.Condition{
				Type:               ExternalDNSPodsScheduledConditionType,
				Status:             metav1.ConditionTrue,
				Reason:             "AllPodsScheduled",
				Message:            "All pods are scheduled",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		//some pods are unschedulable
		{
			name: "Pod unschedulable in list should return ConditionFalse",
			existingDeployment: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"name": "external-dns-operator",
						},
					},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "external-dns-operator",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:    corev1.PodScheduled,
							Status:  corev1.ConditionFalse,
							Reason:  corev1.PodReasonUnschedulable,
							Message: "0/3 nodes are available: 3 node(s) didn't match node selector.",
						}},
					},
				},
			},
			expectedResult: metav1.Condition{
				Type:               ExternalDNSPodsScheduledConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "PodsNotScheduled",
				Message:            "Not relevant to the test",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
		//some pods not yet scheduled
		{
			name: "unscheduled pods in list should return ConditionFalse",
			existingDeployment: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"name": "external-dns-operator",
						},
					},
				},
			},
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod",
						Labels: map[string]string{
							"name": "external-dns-operator",
						},
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionFalse,
						}},
					},
				},
			},
			expectedResult: metav1.Condition{
				Type:               ExternalDNSPodsScheduledConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "PodsNotScheduled",
				Message:            "Not relevant to the test",
				LastTransitionTime: metav1.NewTime(clock.Now()),
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := computeDeploymentPodsScheduledCondition(&tc.existingDeployment, tc.existingPods)
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
			existingObjects: fakePodListAsRuntimeObject(),
			expectedResult:  fakePodList(),
			errExpected:     false,
		},
	}
	for _, tc := range testCases {
		//Question: is it better to use envtest.Environment instead
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()

		podList, err := getPodsList(cl, context.TODO())
		if tc.errExpected {
			require.Error(t, err)
		} else {
			require.Len(t, podList, 2)
		}
	}
}

func TestUpdateExternalDNSStatus(t *testing.T) {
	aDeployment := fakeDeployment(appsv1.DeploymentAvailable, corev1.ConditionFalse, 8, "25%", "25%", 8)
	testCases := []struct {
		name               string
		existingDeployment appsv1.Deployment
		existingObjects    []runtime.Object
		existingExtDNS     *operatorv1alpha1.ExternalDNS
		errExpected        bool
	}{
		//nominal case
		{
			name:               "Nominal case",
			existingDeployment: aDeployment,
			existingObjects:    append(fakePodListAsRuntimeObject(), &aDeployment),
			existingExtDNS: &operatorv1alpha1.ExternalDNS{
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
			},
			errExpected: false,
		},
	}
	for _, tc := range testCases {
		//Question: is it better to use envtest.Environment instead
		cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()
		err := updateExternalDNSStatus(cl, context.TODO(), tc.existingExtDNS, true, &tc.existingDeployment)
		if tc.errExpected {
			require.Error(t, err)
		} else {
			require.Len(t, tc.existingExtDNS.Status.Conditions, 4)
		}
	}
}
func fakePodList() []corev1.Pod {
	return []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
				Labels: map[string]string{
					"name": "not-external-dns",
				},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
				Labels: map[string]string{
					"name": "external-dns-operator",
				},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	}
}
func fakePodListAsRuntimeObject() []runtime.Object {
	return []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "notextdns",
				Labels: map[string]string{
					"name": "not-external-dns",
				},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod",
				Labels: map[string]string{
					"name": "external-dns-operator",
				},
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	}
}
func fakeDeployment(condType appsv1.DeploymentConditionType, isAvailable corev1.ConditionStatus, specReplicas int32, maxUnavailable string, maxSurge string, availableReplicas int32) appsv1.Deployment {
	pointerToInt32 := func(i int32) *int32 { return &i }
	pointerToIntVal := func(val intstr.IntOrString) *intstr.IntOrString { return &val }
	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointerToInt32(specReplicas),
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
