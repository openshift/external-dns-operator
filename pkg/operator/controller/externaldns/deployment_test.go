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
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	namespace   = "externaldns"
	name        = "test"
	image       = "bitname/external-dns:latest"
	publicZone  = "my-dns-public-zone"
	privateZone = "my-dns-private-zone"
)

var (
	allSvcTypes = []corev1.ServiceType{
		corev1.ServiceTypeNodePort,
		corev1.ServiceTypeLoadBalancer,
		corev1.ServiceTypeClusterIP,
		corev1.ServiceTypeExternalName,
	}
)

func TestDesiredExternalDNSDeployment(t *testing.T) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	testCases := []struct {
		name                    string
		inputSecret             *corev1.Secret
		inputExternalDNS        *operatorv1alpha1.ExternalDNS
		expectedTemplatePodSpec corev1.PodSpec
	}{
		{
			name:             "Nominal AWS",
			inputSecret:      testAWSSecret(),
			inputExternalDNS: testAWSExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
						Env: []corev1.EnvVar{
							{
								Name: "AWS_ACCESS_KEY_ID",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "awssecret",
										},
										Key: "aws_access_key_id",
									},
								},
							},
							{
								Name: "AWS_SECRET_ACCESS_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "awssecret",
										},
										Key: "aws_secret_access_key",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials AWS",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testAWSExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
		{
			name:             "Nominal Azure",
			inputSecret:      testAzureSecret(),
			inputExternalDNS: testAzureExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "azure-config-file",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "azuresecret",
								Items: []corev1.KeyToPath{
									{
										Key:  "azure.json",
										Path: "azure.json",
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=azure",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--azure-config-file=/etc/kubernetes/azure.json",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "azure-config-file",
								ReadOnly:  true,
								MountPath: "/etc/kubernetes",
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Azure",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testAzureExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=azure",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
		{
			name:             "Nominal GCP",
			inputSecret:      testGCPSecret(),
			inputExternalDNS: testGCPExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "gcp-credentials-file",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "gcpsecret",
								Items: []corev1.KeyToPath{
									{
										Key:  "gcp-credentials.json",
										Path: "gcp-credentials.json",
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--google-project=external-dns-gcp-project",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "GOOGLE_APPLICATION_CREDENTIALS",
								Value: "/etc/kubernetes/gcp-credentials.json",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "gcp-credentials-file",
								ReadOnly:  true,
								MountPath: "/etc/kubernetes",
							},
						},
					},
				},
			},
		},
		{
			name:             "No project, no credentials GCP",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testGCPExternalDNSNoProject(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
		{
			name:             "Nominal Bluecat",
			inputSecret:      testBlueCatSecret(),
			inputExternalDNS: testBlueCatExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "bluecat-config-file",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "bluecatsecret",
								Items: []corev1.KeyToPath{
									{
										Key:  "bluecat.json",
										Path: "bluecat.json",
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=bluecat",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--bluecat-config-file=/etc/kubernetes/bluecat.json",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "bluecat-config-file",
								ReadOnly:  true,
								MountPath: "/etc/kubernetes",
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Bluecat",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testBlueCatExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=bluecat",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
		{
			name:             "Nominal Infoblox",
			inputSecret:      testInfobloxSecret(),
			inputExternalDNS: testInfobloxExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=infoblox",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--infoblox-wapi-port=443",
							"--infoblox-grid-host=gridhost.example.com",
							"--infoblox-wapi-version=2.3.1",
						},
						Env: []corev1.EnvVar{
							{
								Name: "EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "infobloxsecret",
										},
										Key: "EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME",
									},
								},
							},
							{
								Name: "EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "infobloxsecret",
										},
										Key: "EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Infoblox",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testInfobloxExternalDNS(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=infoblox",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
		{
			name:             "Hostname allowed, no clusterip type",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testAWSExternalDNSHostnameAllow(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=LoadBalancer",
						},
					},
				},
			},
		},
		{
			name:             "Many zones",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testAWSExternalDNSZones([]string{publicZone, privateZone}),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
					{
						Name:  "external-dns-n656hcdh5d9hf6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7980",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-private-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
		{
			name:             "Annotation filter",
			inputSecret:      &corev1.Secret{},
			inputExternalDNS: testAWSExternalDNSAnnotationFilter(map[string]string{"testannotation": "yes"}),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: name,
				NodeSelector: map[string]string{
					osLabel:             linuxOS,
					masterNodeRoleLabel: "",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      masterNodeRoleLabel,
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-nfbh54h648h6q",
						Image: "bitname/external-dns:latest",
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--namespace=testns",
							"--annotation-filter=testannotation=yes",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			depl, err := desiredExternalDNSDeployment(namespace, image, serviceAccount, tc.inputSecret, tc.inputExternalDNS)
			if err != nil {
				t.Errorf("expected no error from calling desiredExternalDNSDeployment, but received %v", err)
			}
			diffOpts := cmpopts.IgnoreFields(corev1.Container{}, "TerminationMessagePolicy", "ImagePullPolicy")
			if diff := cmp.Diff(tc.expectedTemplatePodSpec, depl.Spec.Template.Spec, diffOpts); diff != "" {
				t.Errorf("wrong desired POD spec (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExternalDNSDeploymentChanged(t *testing.T) {
	testCases := []struct {
		description        string
		originalDeployment *appsv1.Deployment
		mutate             func(*appsv1.Deployment)
		expect             bool
		expectedDeployment *appsv1.Deployment
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *appsv1.Deployment) {},
			expect:      false,
		},
		{
			description: "if externalDNS image changes",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Image = "foo.io/test:latest"
			},
			expect:             true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{testContainerWithImage("foo.io/test:latest")}),
		},
		{
			description: "if externalDNS container args",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Args = []string{"Nada"}
			},
			expect:             true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{testContainerWithArgs([]string{"Nada"})}),
		},
		{
			description: "if externalDNS container args order changes",
			mutate: func(depl *appsv1.Deployment) {
				// swap the last and the first elements
				last := len(depl.Spec.Template.Spec.Containers[0].Args) - 1
				tmp := depl.Spec.Template.Spec.Containers[0].Args[0]
				depl.Spec.Template.Spec.Containers[0].Args[0] = depl.Spec.Template.Spec.Containers[0].Args[last]
				depl.Spec.Template.Spec.Containers[0].Args[last] = tmp
			},
			expect: false,
		},
		{
			description: "if externalDNS misses container",
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers = append(depl.Spec.Template.Spec.Containers, testContainerWithName("second"))
			},
			expect: true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{
				testContainer(),
				testContainerWithName("second"),
			}),
		},
		{
			description: "if externalDNS has extra container",
			originalDeployment: testDeploymentWithContainers([]corev1.Container{
				testContainer(),
				testContainerWithName("second"),
			}),
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers = []corev1.Container{testContainer()}
			},
			expect: true,
			expectedDeployment: testDeploymentWithContainers([]corev1.Container{
				testContainer(),
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			original := testDeployment()
			if tc.originalDeployment != nil {
				original = tc.originalDeployment
			}

			mutated := original.DeepCopy()
			tc.mutate(mutated)
			if changed, updated := externalDNSDeploymentChanged(original, mutated); changed != tc.expect {
				t.Errorf("Expect externalDNSDeploymentChanged to be %t, got %t", tc.expect, changed)
			} else if changed {
				if changedAgain, updatedAgain := externalDNSDeploymentChanged(mutated, updated); changedAgain {
					t.Errorf("ExternalDNSDeploymentChanged does not behave as a fixed point function")
				} else {
					if !reflect.DeepEqual(updatedAgain, tc.expectedDeployment) {
						t.Errorf("Expect updated deployment to be %v, got %v", tc.expectedDeployment, updatedAgain)
					}
				}
			}
		})
	}
}

func testDeployment() *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "testns",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"testlbl": "yes",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"testlbl": "yes",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "testsa",
					NodeSelector: map[string]string{
						"testlbl": "yes",
					},
					Containers: []corev1.Container{testContainer()},
				},
			},
		},
	}
}

func testDeploymentWithContainers(containers []corev1.Container) *appsv1.Deployment {
	depl := testDeployment()
	depl.Spec.Template.Spec.Containers = containers
	return depl
}

func testContainer() corev1.Container {
	return corev1.Container{
		Name:                     "first",
		Image:                    image,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		Args: []string{
			"--flag1=value1",
			"--flag2=value2",
		},
	}
}

func testContainerWithName(name string) corev1.Container {
	cont := testContainer()
	cont.Name = name
	return cont
}

func testContainerWithImage(image string) corev1.Container {
	cont := testContainer()
	cont.Image = image
	return cont
}

func testContainerWithArgs(args []string) corev1.Container {
	cont := testContainer()
	cont.Args = args
	return cont
}

func testExternalDNSInstance(provider operatorv1alpha1.ExternalDNSProviderType,
	source operatorv1alpha1.ExternalDNSSourceType,
	svcType []corev1.ServiceType,
	annotationFilter map[string]string,
	hostnamePolicy operatorv1alpha1.HostnameAnnotationPolicy,
	fqdnTemplate string,
	zones []string) *operatorv1alpha1.ExternalDNS {
	sourceNamespace := "testns"
	extDNS := &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: operatorv1alpha1.ExternalDNSSpec{
			Provider: operatorv1alpha1.ExternalDNSProvider{
				Type: provider,
			},
			Source: operatorv1alpha1.ExternalDNSSource{
				ExternalDNSSourceUnion: operatorv1alpha1.ExternalDNSSourceUnion{
					Type:      source,
					Namespace: &sourceNamespace,
					Service: &operatorv1alpha1.ExternalDNSServiceSourceOptions{
						ServiceType: svcType,
					},
					AnnotationFilter: annotationFilter,
				},
				HostnameAnnotationPolicy: hostnamePolicy,
				FQDNTemplate:             fqdnTemplate,
			},
			Zones: zones,
		},
	}

	return extDNS
}

func testExternalDNSHostnameIgnore(provider operatorv1alpha1.ExternalDNSProviderType,
	source operatorv1alpha1.ExternalDNSSourceType,
	svcTypes []corev1.ServiceType,
	zones []string) *operatorv1alpha1.ExternalDNS {
	return testExternalDNSInstance(provider, source, svcTypes, nil, operatorv1alpha1.HostnameAnnotationPolicyIgnore, "{{.Name}}.test.com", zones)
}

func testExternalDNSHostnameAllow(provider operatorv1alpha1.ExternalDNSProviderType,
	source operatorv1alpha1.ExternalDNSSourceType,
	svcTypes []corev1.ServiceType,
	zones []string) *operatorv1alpha1.ExternalDNS {
	return testExternalDNSInstance(provider, source, svcTypes, nil, operatorv1alpha1.HostnameAnnotationPolicyAllow, "", zones)
}

func testAWSExternalDNS() *operatorv1alpha1.ExternalDNS {
	return testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeAWS, operatorv1alpha1.SourceTypeService, allSvcTypes, []string{publicZone})
}

func testAWSExternalDNSZones(zones []string) *operatorv1alpha1.ExternalDNS {
	return testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeAWS, operatorv1alpha1.SourceTypeService, allSvcTypes, zones)
}

func testAWSExternalDNSHostnameAllow() *operatorv1alpha1.ExternalDNS {
	return testExternalDNSHostnameAllow(operatorv1alpha1.ProviderTypeAWS, operatorv1alpha1.SourceTypeService, []corev1.ServiceType{corev1.ServiceTypeLoadBalancer}, []string{publicZone})
}

func testAWSExternalDNSAnnotationFilter(annotationFilter map[string]string) *operatorv1alpha1.ExternalDNS {
	extdns := testAWSExternalDNS()
	extdns.Spec.Source.AnnotationFilter = annotationFilter
	return extdns
}

func testAzureExternalDNS() *operatorv1alpha1.ExternalDNS {
	return testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeAzure, operatorv1alpha1.SourceTypeService, allSvcTypes, []string{publicZone})
}

func testGCPExternalDNS() *operatorv1alpha1.ExternalDNS {
	extdns := testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeGCP, operatorv1alpha1.SourceTypeService, allSvcTypes, []string{publicZone})
	project := "external-dns-gcp-project"
	extdns.Spec.Provider.GCP = &operatorv1alpha1.ExternalDNSGCPProviderOptions{
		Project: &project,
	}
	return extdns
}

func testGCPExternalDNSNoProject() *operatorv1alpha1.ExternalDNS {
	return testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeGCP, operatorv1alpha1.SourceTypeService, allSvcTypes, []string{publicZone})
}

func testBlueCatExternalDNS() *operatorv1alpha1.ExternalDNS {
	return testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeBlueCat, operatorv1alpha1.SourceTypeService, allSvcTypes, []string{publicZone})
}

func testInfobloxExternalDNS() *operatorv1alpha1.ExternalDNS {
	extdns := testExternalDNSHostnameIgnore(operatorv1alpha1.ProviderTypeInfoblox, operatorv1alpha1.SourceTypeService, allSvcTypes, []string{publicZone})
	extdns.Spec.Provider.Infoblox = &operatorv1alpha1.ExternalDNSInfobloxProviderOptions{
		GridHost:    "gridhost.example.com",
		WAPIPort:    443,
		WAPIVersion: "2.3.1",
	}
	return extdns
}

func testAWSSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "awssecret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"aws_access_key_id":     []byte("testid"),
			"aws_secret_access_key": []byte("testsecret"),
		},
	}
}

func testAzureSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "azuresecret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"azure.json": []byte("{}"),
		},
	}
}

func testGCPSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gcpsecret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"gcp-credentials.json": []byte("{}"),
		},
	}
}

func testBlueCatSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bluecatsecret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"bluecat.json": []byte("{}"),
		},
	}
}

func testInfobloxSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infobloxsecret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME": []byte("testuser"),
			"EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD": []byte("testpwd"),
		},
	}
}
