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
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
	"github.com/openshift/external-dns-operator/pkg/operator/controller/utils/test"
	"github.com/openshift/external-dns-operator/pkg/utils"
)

var (
	replicas    = int32(1)
	allSvcTypes = []corev1.ServiceType{
		corev1.ServiceTypeNodePort,
		corev1.ServiceTypeLoadBalancer,
		corev1.ServiceTypeClusterIP,
		corev1.ServiceTypeExternalName,
	}
	serviceAccount = &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.OperandName,
		},
	}
)

const (
	testSecretHash              = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	awsSecret                   = "awssecret"
	azureSecret                 = "azuresecret"
	gcpSecret                   = "gcpsecret"
	infobloxsecret              = "infobloxsecret"
	bluecatsecret               = "bluecatsecret"
	ExternalDNSContainerName    = "external-dns-nfbh54h648h6q"
	ExternalDNSContainerNoZones = "external-dns-n56fh6dh59ch5fcq"
	httpProxy                   = "http://proxy-user1:XXXYYYZZZ@ec2-3-100-200-30.us-east-2.compute.amazonaws.com:3128"
	httpsProxy                  = "https://proxy-user1:XXXYYYZZZ@ec2-3-100-200-30.us-east-2.compute.amazonaws.com:3128"
	noProxy                     = ".cluster.local,.svc,.us-east-2.compute.internal,10.0.0.0/16,127.0.0.1,100.200.300.400"
	externalDNSKind             = "ExternalDNS"
	ExternalDNSBaseName         = "external-dns"
	testName                    = "test"
)

func TestDesiredExternalDNSDeployment(t *testing.T) {
	testCases := []struct {
		name                        string
		inputSecretName             string
		inputExternalDNS            *operatorv1beta1.ExternalDNS
		inputIsOpenShift            bool
		inputPlatformStatus         *configv1.PlatformStatus
		inputTrustedCAConfigMapName string
		inputEnvVars                map[string]string
		expectedTemplatePodSpec     corev1.PodSpec
	}{
		{
			name:             "Nominal AWS",
			inputSecretName:  awsSecret,
			inputExternalDNS: testAWSExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: awsCredentialsVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "awssecret",
								Items: []corev1.KeyToPath{
									{
										Key:  awsCredentialsFileKey,
										Path: awsCredentialsFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name:  awsCredentialEnvVarName,
								Value: awsCredentialsFilePath,
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      awsCredentialsVolumeName,
								MountPath: awsCredentialsMountPath,
								ReadOnly:  true,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials AWS",
			inputExternalDNS: testAWSExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:                        "Trusted CA AWS",
			inputExternalDNS:            testAWSExternalDNS(operatorv1beta1.SourceTypeService),
			inputTrustedCAConfigMapName: test.TrustedCAConfigMapName,
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: "trusted-ca",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: test.TrustedCAConfigMapName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  "ca-bundle.crt",
										Path: "tls-ca-bundle.pem",
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "SSL_CERT_DIR",
								Value: "/etc/pki/ca-trust/extracted/pem",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "trusted-ca",
								ReadOnly:  true,
								MountPath: "/etc/pki/ca-trust/extracted/pem",
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:                "Nominal AWS Gov",
			inputSecretName:     awsSecret,
			inputExternalDNS:    testAWSExternalDNS(operatorv1beta1.SourceTypeService),
			inputPlatformStatus: testPlatformStatusAWSGov("us-gov-west-1"),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: awsCredentialsVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "awssecret",
								Items: []corev1.KeyToPath{
									{
										Key:  awsCredentialsFileKey,
										Path: awsCredentialsFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--aws-prefer-cname",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "AWS_REGION",
								Value: "us-gov-west-1",
							},
							{
								Name:  "AWS_SHARED_CREDENTIALS_FILE",
								Value: "/etc/kubernetes/aws-credentials",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      awsCredentialsVolumeName,
								MountPath: awsCredentialsMountPath,
								ReadOnly:  true,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal Azure",
			inputSecretName:  azureSecret,
			inputExternalDNS: testAzureExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: azureConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: azureSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  azureConfigFileName,
										Path: azureConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=azure",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
							"--azure-config-file=/etc/kubernetes/azure.json",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Private Zone Azure",
			inputSecretName:  azureSecret,
			inputExternalDNS: testAzureExternalDNSPrivateZones([]string{test.AzurePrivateDNSZone}, operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: azureConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: azureSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  azureConfigFileName,
										Path: azureConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "external-dns-n64ch5cch658h64bq",
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=/subscriptions/xxxx/resourceGroups/test-az-2f9kj-rg/providers/Microsoft.Network/privateDnsZones/test-az.example.com",
							"--provider=azure-private-dns",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--azure-config-file=/etc/kubernetes/azure.json",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Azure",
			inputExternalDNS: testAzureExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=azure",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No Zones Azure",
			inputSecretName:  azureSecret,
			inputExternalDNS: testAzureExternalDNSNoZones(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: azureConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: azureSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  azureConfigFileName,
										Path: azureConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=azure",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
							"--azure-config-file=/etc/kubernetes/azure.json",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
					{
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7980",
							"--txt-owner-id=external-dns-test",
							"--provider=azure-private-dns",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
							"--azure-config-file=/etc/kubernetes/azure.json",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal GCP",
			inputSecretName:  gcpSecret,
			inputExternalDNS: testGCPExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: gcpCredentialsVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: gcpSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  gcpCredentialsFileKey,
										Path: gcpCredentialsFileKey,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--google-project=external-dns-gcp-project",
						},
						Env: []corev1.EnvVar{
							{
								Name:  gcpAppCredentialsEnvVar,
								Value: "/etc/kubernetes/gcp-credentials.json",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      gcpCredentialsVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No project GCP",
			inputExternalDNS: testGCPExternalDNSNoProject(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:                "Platform project GCP",
			inputExternalDNS:    testGCPExternalDNSNoProject(operatorv1beta1.SourceTypeService),
			inputIsOpenShift:    true,
			inputPlatformStatus: testPlatformStatusGCP("external-dns-gcp-project"),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--google-project=external-dns-gcp-project",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal Bluecat",
			inputSecretName:  bluecatsecret,
			inputExternalDNS: testBlueCatExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
								SecretName: bluecatsecret,
								Items: []corev1.KeyToPath{
									{
										Key:  blueCatConfigFileName,
										Path: blueCatConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=bluecat",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
							"--bluecat-config-file=/etc/kubernetes/bluecat.json",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "bluecat-config-file",
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Bluecat",
			inputExternalDNS: testBlueCatExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=bluecat",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--txt-prefix=external-dns-",
							"--fqdn-template={{.Name}}.test.com",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal Infoblox",
			inputSecretName:  infobloxsecret,
			inputExternalDNS: testInfobloxExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=infoblox",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
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
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name: infobloxWAPIUsernameEnvVar,
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: infobloxsecret,
										},
										Key: infobloxWAPIUsernameEnvVar,
									},
								},
							},
							{
								Name: infobloxWAPIPasswordEnvVar,
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: infobloxsecret,
										},
										Key: infobloxWAPIPasswordEnvVar,
									},
								},
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Infoblox",
			inputExternalDNS: testInfobloxExternalDNS(operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=infoblox",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Hostname allowed, no clusterip type",
			inputExternalDNS: testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeService, ""),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=LoadBalancer",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Many FQDN templates",
			inputExternalDNS: testAWSExternalDNSManyFQDN(),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=LoadBalancer",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com,{{.Name}}.{{.Namespace}}.example.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Many zones",
			inputExternalDNS: testAWSExternalDNSZones([]string{test.PublicZone, test.PrivateZone}, operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
					{
						Name:  "external-dns-n656hcdh5d9hf6q",
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7980",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-private-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Annotation filter",
			inputExternalDNS: testAWSExternalDNSLabelFilter(utils.MustParseLabelSelector("testannotation=yes,app in (web,external)"), operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--label-filter=app in (external,web),testannotation=yes",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No zones && no domain filter",
			inputExternalDNS: testAWSExternalDNSZones([]string{}, operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No zones + Domain filter",
			inputExternalDNS: testAWSExternalDNSDomainFilter([]string{}, operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--domain-filter=abc.com",
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Zone + Domain filter",
			inputExternalDNS: testAWSExternalDNSDomainFilter([]string{test.PublicZone}, operatorv1beta1.SourceTypeService),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--domain-filter=abc.com",
							"--zone-id-filter=my-dns-public-zone",
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=aws",
							"--source=service",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--service-type-filter=NodePort",
							"--service-type-filter=LoadBalancer",
							"--service-type-filter=ClusterIP",
							"--service-type-filter=ExternalName",
							"--publish-internal-services",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		// OCP Route Source
		{
			name:             "Nominal AWS Route",
			inputSecretName:  awsSecret,
			inputExternalDNS: testAWSExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: awsCredentialsVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "awssecret",
								Items: []corev1.KeyToPath{
									{
										Key:  awsCredentialsFileKey,
										Path: awsCredentialsFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name:  awsCredentialEnvVarName,
								Value: awsCredentialsFilePath,
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      awsCredentialsVolumeName,
								MountPath: awsCredentialsMountPath,
								ReadOnly:  true,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials AWS Route",
			inputExternalDNS: testAWSExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "FQDNTemplate set AWS Route",
			inputSecretName:  awsSecret,
			inputExternalDNS: testAWSExternalDNSFQDNTemplate(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: awsCredentialsVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "awssecret",
								Items: []corev1.KeyToPath{
									{
										Key:  awsCredentialsFileKey,
										Path: awsCredentialsFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							"--fqdn-template={{.Name}}.test.com",
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name:  awsCredentialEnvVarName,
								Value: awsCredentialsFilePath,
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      awsCredentialsVolumeName,
								MountPath: awsCredentialsMountPath,
								ReadOnly:  true,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal Azure Route",
			inputSecretName:  azureSecret,
			inputExternalDNS: testAzureExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: azureConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: azureSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  azureConfigFileName,
										Path: azureConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=azure",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--azure-config-file=/etc/kubernetes/azure.json",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Azure Route",
			inputExternalDNS: testAzureExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=azure",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No zones Azure Route",
			inputSecretName:  azureSecret,
			inputExternalDNS: testAzureExternalDNSNoZones(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: azureConfigVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: azureSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  azureConfigFileName,
										Path: azureConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=azure",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--azure-config-file=/etc/kubernetes/azure.json",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
					{
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7980",
							"--txt-owner-id=external-dns-test",
							"--provider=azure-private-dns",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--azure-config-file=/etc/kubernetes/azure.json",
							"--txt-prefix=external-dns-",
							"--txt-wildcard-replacement=any",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      azureConfigVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal GCP Route",
			inputSecretName:  gcpSecret,
			inputExternalDNS: testGCPExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name: gcpCredentialsVolumeName,
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: gcpSecret,
								Items: []corev1.KeyToPath{
									{
										Key:  gcpCredentialsFileKey,
										Path: gcpCredentialsFileKey,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--google-project=external-dns-gcp-project",
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name:  gcpAppCredentialsEnvVar,
								Value: "/etc/kubernetes/gcp-credentials.json",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      gcpCredentialsVolumeName,
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No project GCP Route",
			inputExternalDNS: testGCPExternalDNSNoProject(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=google",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal Bluecat Route",
			inputSecretName:  bluecatsecret,
			inputExternalDNS: testBlueCatExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
								SecretName: bluecatsecret,
								Items: []corev1.KeyToPath{
									{
										Key:  blueCatConfigFileName,
										Path: blueCatConfigFileName,
									},
								},
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=bluecat",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--bluecat-config-file=/etc/kubernetes/bluecat.json",
							"--txt-prefix=external-dns-",
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "bluecat-config-file",
								ReadOnly:  true,
								MountPath: defaultConfigMountPath,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Bluecat Route",
			inputExternalDNS: testBlueCatExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=bluecat",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Nominal Infoblox Route",
			inputSecretName:  infobloxsecret,
			inputExternalDNS: testInfobloxExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=infoblox",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--infoblox-wapi-port=443",
							"--infoblox-grid-host=gridhost.example.com",
							"--infoblox-wapi-version=2.3.1",
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name: "EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: infobloxsecret,
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
											Name: infobloxsecret,
										},
										Key: "EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD",
									},
								},
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No credentials Infoblox Route",
			inputExternalDNS: testInfobloxExternalDNS(operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=infoblox",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Hostname allowed, no clusterip type",
			inputExternalDNS: testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeRoute, ""),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Many zones Route",
			inputExternalDNS: testAWSExternalDNSZones([]string{test.PublicZone, test.PrivateZone}, operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
					{
						Name:  "external-dns-n656hcdh5d9hf6q",
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7980",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-private-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Annotation filter Route",
			inputExternalDNS: testAWSExternalDNSLabelFilter(utils.MustParseLabelSelector("testannotation=yes"), operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--label-filter=testannotation=yes",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No zones && no domain filter Route",
			inputExternalDNS: testAWSExternalDNSZones([]string{}, operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "No zones + Domain filter Route",
			inputExternalDNS: testAWSExternalDNSDomainFilter([]string{}, operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerNoZones,
						Image: test.OperandImage,
						Args: []string{
							"--domain-filter=abc.com",
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Zone + Domain filter Route",
			inputExternalDNS: testAWSExternalDNSDomainFilter([]string{test.PublicZone}, operatorv1beta1.SourceTypeRoute),
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--domain-filter=abc.com",
							"--zone-id-filter=my-dns-public-zone",
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							"--ignore-hostname-annotation",
							`--fqdn-template={{""}}`,
							"--txt-prefix=external-dns-",
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
		{
			name:             "Propagate proxy settings",
			inputExternalDNS: testAWSExternalDNS(operatorv1beta1.SourceTypeRoute),
			inputEnvVars: map[string]string{
				"HTTP_PROXY":  httpProxy,
				"HTTPS_PROXY": httpsProxy,
				"NO_PROXY":    noProxy,
			},
			expectedTemplatePodSpec: corev1.PodSpec{
				ServiceAccountName: test.OperandName,
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
						Name:  ExternalDNSContainerName,
						Image: test.OperandImage,
						Args: []string{
							"--metrics-address=127.0.0.1:7979",
							"--txt-owner-id=external-dns-test",
							"--zone-id-filter=my-dns-public-zone",
							"--provider=aws",
							"--source=openshift-route",
							"--policy=sync",
							"--registry=txt",
							"--log-level=debug",
							`--fqdn-template={{""}}`,
							"--ignore-hostname-annotation",
							"--txt-prefix=external-dns-",
						},
						Env: []corev1.EnvVar{
							{
								Name:  "HTTP_PROXY",
								Value: httpProxy,
							},
							{
								Name:  "HTTPS_PROXY",
								Value: httpsProxy,
							},
							{
								Name:  "NO_PROXY",
								Value: noProxy,
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{allCapabilities},
							},
							Privileged:               pointer.Bool(false),
							RunAsNonRoot:             pointer.Bool(true),
							AllowPrivilegeEscalation: pointer.Bool(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.inputEnvVars {
				if err := os.Setenv(k, v); err != nil {
					t.Errorf("failed to set environment variable %q: %v", k, err)
				}
			}
			defer func() {
				for k := range tc.inputEnvVars {
					if err := os.Unsetenv(k); err != nil {
						t.Errorf("failed to unset environment variable %q: %v", k, err)
					}
				}
			}()
			depl, err := desiredExternalDNSDeployment(&deploymentConfig{
				test.OperandNamespace,
				test.OperandImage,
				serviceAccount,
				tc.inputExternalDNS,
				tc.inputIsOpenShift,
				tc.inputPlatformStatus,
				tc.inputSecretName,
				testSecretHash,
				tc.inputTrustedCAConfigMapName, "",
			})
			if err != nil {
				t.Errorf("expected no error from calling desiredExternalDNSDeployment, but received %v", err)
			}
			ignoreFieldsOpts := cmpopts.IgnoreFields(corev1.Container{}, "TerminationMessagePolicy", "ImagePullPolicy")
			sortArgsOpt := cmp.Transformer("Sort", func(spec corev1.PodSpec) corev1.PodSpec {
				if len(spec.Containers) == 0 {
					return spec
				}
				cpy := *spec.DeepCopy()
				for i := range cpy.Containers {
					sort.Strings(cpy.Containers[i].Args)
				}
				return cpy
			})
			if diff := cmp.Diff(tc.expectedTemplatePodSpec, depl.Spec.Template.Spec, ignoreFieldsOpts, sortArgsOpt); diff != "" {
				t.Errorf("wrong desired POD spec (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExternalDNSDeploymentChanged(t *testing.T) {
	updatedSecretHashAnnotation := make(map[string]string)
	updatedSecretHashAnnotation[credentialsAnnotation] = "31f4ea504e2efd429769e1d09b586449f0b339eb"
	testCases := []struct {
		description        string
		originalDeployment *appsv1.Deployment
		mutate             func(*appsv1.Deployment)
		expect             bool
		expectedDeployment *appsv1.Deployment
	}{
		{
			description: "if nothing changes",
			expect:      false,
			mutate:      func(_ *appsv1.Deployment) {},
		},
		{
			description: "if externalDNS test.OperandImage changes",
			expect:      true,
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Image = "foo.io/test:latest"
			},
			expectedDeployment: testDeploymentWithContainers(testContainerWithImage("foo.io/test:latest")),
		},
		{
			description: "if externalDNS container args change",
			expect:      true,
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers[0].Args = []string{"Nada"}
			},
			expectedDeployment: testDeploymentWithContainers(testContainerWithArgs("Nada")),
		},
		{
			description: "if externalDNS container args order changes",
			expect:      false,
			mutate: func(depl *appsv1.Deployment) {
				// swap the last and the first elements
				last := len(depl.Spec.Template.Spec.Containers[0].Args) - 1
				tmp := depl.Spec.Template.Spec.Containers[0].Args[0]
				depl.Spec.Template.Spec.Containers[0].Args[0] = depl.Spec.Template.Spec.Containers[0].Args[last]
				depl.Spec.Template.Spec.Containers[0].Args[last] = tmp
			},
		},
		{
			description: "if externalDNS misses container",
			expect:      true,
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers = append(depl.Spec.Template.Spec.Containers, testContainerWithName("second"))
			},
			expectedDeployment: testDeploymentWithContainers(testContainer(), testContainerWithName("second")),
		},
		{
			description:        "if externalDNS has extra container",
			expect:             true,
			originalDeployment: testDeploymentWithContainers(testContainer(), testContainerWithName("extra")),
			mutate: func(depl *appsv1.Deployment) {
				depl.Spec.Template.Spec.Containers = []corev1.Container{testContainer()}
			},
			expectedDeployment: testDeploymentWithContainers(testContainer()),
		},
		{
			description:        "if externalDNS annotation changes",
			expect:             true,
			originalDeployment: testDeploymentWithContainers(testContainer()),
			mutate: func(dep1 *appsv1.Deployment) {
				dep1.Spec.Template.Annotations = updatedSecretHashAnnotation
			},
			expectedDeployment: testDeploymentWithAnnotations(updatedSecretHashAnnotation),
		},
		{
			description:        "if externalDNS annotation is not present",
			expect:             true,
			originalDeployment: testDeploymentWithoutAnnotations(),
			mutate: func(dep1 *appsv1.Deployment) {
				dep1.Spec.Template.Annotations = updatedSecretHashAnnotation
			},
			expectedDeployment: testDeploymentWithAnnotations(updatedSecretHashAnnotation),
		},
		{
			description:        "if externalDNS security context is added",
			expect:             true,
			originalDeployment: testDeploymentWithContainers(testContainer()),
			mutate: func(dep1 *appsv1.Deployment) {
				dep1.Spec.Template.Spec.Containers = []corev1.Container{
					testContainerWithSecurityContext(&corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{allCapabilities},
						},
						Privileged:               pointer.Bool(false),
						RunAsNonRoot:             pointer.Bool(true),
						AllowPrivilegeEscalation: pointer.Bool(false),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					}),
				}
			},
			expectedDeployment: testDeploymentWithContainers(
				testContainerWithSecurityContext(&corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{allCapabilities},
					},
					Privileged:               pointer.Bool(false),
					RunAsNonRoot:             pointer.Bool(true),
					AllowPrivilegeEscalation: pointer.Bool(false),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				})),
		},
		{
			description: "if externalDNS security context is updated",
			expect:      true,
			originalDeployment: testDeploymentWithContainers(
				testContainerWithSecurityContext(&corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{allCapabilities},
					},
					Privileged:               pointer.Bool(false),
					RunAsNonRoot:             pointer.Bool(true),
					AllowPrivilegeEscalation: pointer.Bool(true),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				})),
			mutate: func(dep1 *appsv1.Deployment) {
				dep1.Spec.Template.Spec.Containers = []corev1.Container{
					testContainerWithSecurityContext(&corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{allCapabilities},
						},
						Privileged:               pointer.Bool(false),
						RunAsNonRoot:             pointer.Bool(true),
						AllowPrivilegeEscalation: pointer.Bool(false),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					}),
				}
			},
			expectedDeployment: testDeploymentWithContainers(
				testContainerWithSecurityContext(&corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{allCapabilities},
					},
					Privileged:               pointer.Bool(false),
					RunAsNonRoot:             pointer.Bool(true),
					AllowPrivilegeEscalation: pointer.Bool(false),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				})),
		},
		{
			description: "if externalDNS security context is same",
			expect:      false,
			originalDeployment: testDeploymentWithContainers(
				testContainerWithSecurityContext(&corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{allCapabilities},
					},
					Privileged:               pointer.Bool(false),
					RunAsNonRoot:             pointer.Bool(true),
					AllowPrivilegeEscalation: pointer.Bool(false),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
				})),
			mutate: func(dep *appsv1.Deployment) {
			},
		},
		{
			description: "if externalDNS has extra volume",
			expect:      false,
			originalDeployment: testDeploymentWithVolumes(
				testSecretVolume("testcreds", "testsecret", "creds", "/run/secrets"),
				testConfigMapVolume("extravolume", "extra", "files", "/etc"),
			),
			mutate: func(dep *appsv1.Deployment) {
				dep.Spec.Template.Spec.Volumes = []corev1.Volume{testSecretVolume("testcreds", "testsecret", "creds", "/run/secrets")}
			},
		},
		{
			description:        "if API server sets defaultMode on secret volume",
			expect:             false,
			originalDeployment: testDeploymentWithVolumes(testSecretVolume("testcreds", "testsecret", "creds", "/run/secrets")),
			mutate: func(dep *appsv1.Deployment) {
				dep.Spec.Template.Spec.Volumes[0].VolumeSource.Secret.DefaultMode = nil
			},
		},
		{
			description:        "if API server sets defaultMode on configmap volume",
			expect:             false,
			originalDeployment: testDeploymentWithVolumes(testConfigMapVolume("testcerts", "testcerts", "key", "/etc/pki/trust")),
			mutate: func(dep *appsv1.Deployment) {
				dep.Spec.Template.Spec.Volumes[0].VolumeSource.ConfigMap.DefaultMode = nil
			},
		},
		{
			description: "if volumes change ordering",
			expect:      false,
			originalDeployment: testDeploymentWithVolumes(
				testSecretVolume("testcreds", "testsecret", "creds", "/run/secrets"),
				testConfigMapVolume("testconfig", "config", "files", "/etc"),
			),
			mutate: func(dep *appsv1.Deployment) {
				dep.Spec.Template.Spec.Volumes = []corev1.Volume{
					testConfigMapVolume("testconfig", "config", "files", "/etc"),
					testSecretVolume("testcreds", "testsecret", "creds", "/run/secrets"),
				}
			},
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
			// original is the object that's in the API.
			// mutated is the new desired object, which mutates the original API object based on some update:
			// changes to the externaldns, or changes to the operator itself.
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

func TestEnsureExternalDNSDeployment(t *testing.T) {
	testCases := []struct {
		name               string
		existingObjects    []runtime.Object
		credSecret         *corev1.Secret
		trustCAConfigMap   *corev1.ConfigMap
		expectedExist      bool
		expectedDeployment appsv1.Deployment
		errExpected        bool
		extDNS             operatorv1beta1.ExternalDNS
		ocpRouterNames     []string
	}{
		{
			name:            "Does not exist with route source",
			extDNS:          *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeRoute, ""),
			existingObjects: []runtime.Object{},
			credSecret:      testSecret(),
			expectedExist:   true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=openshift-route",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "Exist as expected with route source",
			extDNS: *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeRoute, ""),
			existingObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.OperandName,
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               externalDNSKind,
								Name:               test.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/instance": testName,
									"app.kubernetes.io/name":     ExternalDNSBaseName,
								},
								Annotations: map[string]string{credentialsAnnotation: testSecretHash},
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: test.OperandName,
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
										Name: awsCredentialsVolumeName,
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "external-dns-credentials-test",
												Items: []corev1.KeyToPath{
													{
														Key:  awsCredentialsFileKey,
														Path: awsCredentialsFileName,
													},
												},
											},
										},
									},
								},
								Containers: []corev1.Container{
									{
										Name:  ExternalDNSContainerName,
										Image: test.OperandImage,
										Args: []string{
											"--metrics-address=127.0.0.1:7979",
											"--txt-owner-id=external-dns-test",
											"--zone-id-filter=my-dns-public-zone",
											"--provider=aws",
											"--source=openshift-route",
											"--policy=sync",
											"--registry=txt",
											"--log-level=debug",
											"--txt-prefix=external-dns-",
										},
										Env: []corev1.EnvVar{
											{
												Name:  awsCredentialEnvVarName,
												Value: awsCredentialsFilePath,
											},
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      awsCredentialsVolumeName,
												MountPath: awsCredentialsMountPath,
												ReadOnly:  true,
											},
										},
										SecurityContext: &corev1.SecurityContext{
											Capabilities: &corev1.Capabilities{
												Drop: []corev1.Capability{allCapabilities},
											},
											Privileged:               pointer.Bool(false),
											RunAsNonRoot:             pointer.Bool(true),
											AllowPrivilegeEscalation: pointer.Bool(false),
											SeccompProfile: &corev1.SeccompProfile{
												Type: corev1.SeccompProfileTypeRuntimeDefault,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			credSecret:    testSecret(),
			expectedExist: true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=openshift-route",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "Exist as expected with one Router Names added as flag",
			extDNS: *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeRoute, "default"),
			existingObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.OperandName,
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               externalDNSKind,
								Name:               test.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/instance": testName,
									"app.kubernetes.io/name":     ExternalDNSBaseName,
								},
								Annotations: map[string]string{credentialsAnnotation: testSecretHash},
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: test.OperandName,
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
										Name: awsCredentialsVolumeName,
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "external-dns-credentials-test",
												Items: []corev1.KeyToPath{
													{
														Key:  awsCredentialsFileKey,
														Path: awsCredentialsFileName,
													},
												},
											},
										},
									},
								},
								Containers: []corev1.Container{
									{
										Name:  ExternalDNSContainerName,
										Image: test.OperandImage,
										Args: []string{
											"--metrics-address=127.0.0.1:7979",
											"--txt-owner-id=external-dns-test",
											"--zone-id-filter=my-dns-public-zone",
											"--provider=aws",
											"--source=openshift-route",
											"--policy=sync",
											"--registry=txt",
											"--log-level=debug",
										},
										Env: []corev1.EnvVar{
											{
												Name:  awsCredentialEnvVarName,
												Value: awsCredentialsFilePath,
											},
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      awsCredentialsVolumeName,
												MountPath: awsCredentialsMountPath,
												ReadOnly:  true,
											},
										},
										SecurityContext: &corev1.SecurityContext{
											Capabilities: &corev1.Capabilities{
												Drop: []corev1.Capability{allCapabilities},
											},
											Privileged:               pointer.Bool(false),
											RunAsNonRoot:             pointer.Bool(true),
											AllowPrivilegeEscalation: pointer.Bool(false),
											SeccompProfile: &corev1.SeccompProfile{
												Type: corev1.SeccompProfileTypeRuntimeDefault,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			credSecret:     testSecret(),
			ocpRouterNames: []string{"default"},
			expectedExist:  true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=openshift-route",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--txt-prefix=external-dns-",
										"--openshift-router-name=default",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "Exist and drifted with route source",
			extDNS: *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeRoute, ""),
			existingObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.OperandName,
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               externalDNSKind,
								Name:               test.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/instance": testName,
									"app.kubernetes.io/name":     ExternalDNSBaseName,
								},
								Annotations: map[string]string{credentialsAnnotation: testSecretHash},
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: test.OperandName,
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
										Name:  "external-dns-unsolicited",
										Image: test.OperandImage,
									},
								},
							},
						},
					},
				},
			},
			credSecret:    testSecret(),
			expectedExist: true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=openshift-route",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:             "Does not exist with trusted CA configmap",
			extDNS:           *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeRoute, ""),
			existingObjects:  []runtime.Object{},
			credSecret:       testSecret(),
			trustCAConfigMap: testTrustedCAConfigMap(),
			expectedExist:    true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{
								"externaldns.olm.openshift.io/credentials-secret-hash":   testSecretHash,
								"externaldns.olm.openshift.io/trusted-ca-configmap-hash": "745c799ac442a05bf2ff97c75089850a00471d9dd51392ef9ff9f6ea610d1071",
							},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: "trusted-ca",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: test.TrustedCAConfigMapName,
											},
											Items: []corev1.KeyToPath{
												{
													Key:  "ca-bundle.crt",
													Path: "tls-ca-bundle.pem",
												},
											},
										},
									},
								},
								{
									Name: "aws-credentials",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  "credentials",
													Path: "aws-credentials",
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=openshift-route",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  "SSL_CERT_DIR",
											Value: "/etc/pki/ca-trust/extracted/pem",
										},
										{
											Name:  "AWS_SHARED_CREDENTIALS_FILE",
											Value: "/etc/kubernetes/aws-credentials",
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "trusted-ca",
											MountPath: "/etc/pki/ca-trust/extracted/pem",
											ReadOnly:  true,
										},
										{
											Name:      "aws-credentials",
											MountPath: "/etc/kubernetes",
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:            "Does not exist",
			existingObjects: []runtime.Object{},
			credSecret:      testSecret(),
			extDNS:          *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeService, ""),
			expectedExist:   true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=service",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--service-type-filter=LoadBalancer",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "Exist as expected",
			extDNS: *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeService, ""),
			existingObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.OperandName,
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               externalDNSKind,
								Name:               test.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/instance": testName,
									"app.kubernetes.io/name":     ExternalDNSBaseName,
								},
								Annotations: map[string]string{credentialsAnnotation: testSecretHash},
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: test.OperandName,
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
										Name: awsCredentialsVolumeName,
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "external-dns-credentials-test",
												Items: []corev1.KeyToPath{
													{
														Key:  awsCredentialsFileKey,
														Path: awsCredentialsFileName,
													},
												},
											},
										},
									},
								},
								Containers: []corev1.Container{
									{
										Name:  ExternalDNSContainerName,
										Image: test.OperandImage,
										Args: []string{
											"--metrics-address=127.0.0.1:7979",
											"--txt-owner-id=external-dns-test",
											"--zone-id-filter=my-dns-public-zone",
											"--provider=aws",
											"--source=service",
											"--policy=sync",
											"--registry=txt",
											"--log-level=debug",
											"--service-type-filter=LoadBalancer",
											"--txt-prefix=external-dns-",
										},
										Env: []corev1.EnvVar{
											{
												Name:  awsCredentialEnvVarName,
												Value: awsCredentialsFilePath,
											},
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      awsCredentialsVolumeName,
												MountPath: awsCredentialsMountPath,
												ReadOnly:  true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			credSecret:    testSecret(),
			expectedExist: true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=service",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--service-type-filter=LoadBalancer",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "Exist and drifted",
			extDNS: *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeService, ""),
			existingObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.OperandName,
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               externalDNSKind,
								Name:               test.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/instance": testName,
									"app.kubernetes.io/name":     ExternalDNSBaseName,
								},
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: test.OperandName,
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
										Name:  "external-dns-unsolicited-1",
										Image: test.OperandImage,
									},
									{
										Name:  "external-dns-unsolicited-2",
										Image: test.OperandImage,
									},
								},
							},
						},
					},
				},
			},
			credSecret:    testSecret(),
			expectedExist: true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=service",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--service-type-filter=LoadBalancer",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		// TODO: Add Exist and drifted on security context fields
		{
			name:   "Exist and drifted on volumes and envs",
			extDNS: *testAWSExternalDNSHostnameAllow(operatorv1beta1.SourceTypeService, ""),
			existingObjects: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.OperandName,
						Namespace: test.OperandNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         operatorv1beta1.GroupVersion.String(),
								Kind:               externalDNSKind,
								Name:               test.Name,
								Controller:         &test.TrueVar,
								BlockOwnerDeletion: &test.TrueVar,
							},
						},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app.kubernetes.io/instance": testName,
									"app.kubernetes.io/name":     ExternalDNSBaseName,
								},
								Annotations: map[string]string{credentialsAnnotation: testSecretHash},
							},
							Spec: corev1.PodSpec{
								ServiceAccountName: test.OperandName,
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
										Name: awsCredentialsVolumeName,
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "external-dns-credentials-test",
												Items: []corev1.KeyToPath{
													{
														Key:  "wrongkey",
														Path: "wrongpath",
													},
												},
											},
										},
									},
									{
										Name: "unsolicited-vol",
										VolumeSource: corev1.VolumeSource{
											Secret: &corev1.SecretVolumeSource{
												SecretName: "secretname",
												Items: []corev1.KeyToPath{
													{
														Key:  "key",
														Path: "path",
													},
												},
											},
										},
									},
								},
								Containers: []corev1.Container{
									{
										Name:  ExternalDNSContainerName,
										Image: test.OperandImage,
										Args: []string{
											"--metrics-address=127.0.0.1:7979",
											"--txt-owner-id=external-dns-test",
											"--zone-id-filter=my-dns-public-zone",
											"--provider=aws",
											"--source=service",
											"--policy=sync",
											"--registry=txt",
											"--log-level=debug",
											"--service-type-filter=LoadBalancer",
											"--txt-prefix=external-dns-",
										},
										Env: []corev1.EnvVar{
											{
												Name:  awsCredentialEnvVarName,
												Value: "wrongvalue",
											},
											{
												Name:  "unsolicited-env",
												Value: "somevalue",
											},
										},
										VolumeMounts: []corev1.VolumeMount{
											{
												Name:      awsCredentialsVolumeName,
												MountPath: "wrongpath",
												ReadOnly:  true,
											},
											{
												Name:      "unsolicited-vm",
												MountPath: "somepath",
												ReadOnly:  true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			credSecret:    testSecret(),
			expectedExist: true,
			expectedDeployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.OperandName,
					Namespace: test.OperandNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         operatorv1beta1.GroupVersion.String(),
							Kind:               externalDNSKind,
							Name:               test.Name,
							Controller:         &test.TrueVar,
							BlockOwnerDeletion: &test.TrueVar,
						},
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app.kubernetes.io/instance": testName,
							"app.kubernetes.io/name":     ExternalDNSBaseName,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/instance": testName,
								"app.kubernetes.io/name":     ExternalDNSBaseName,
							},
							Annotations: map[string]string{credentialsAnnotation: testSecretHash},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: test.OperandName,
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
									Name: awsCredentialsVolumeName,
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "external-dns-credentials-test",
											Items: []corev1.KeyToPath{
												{
													Key:  awsCredentialsFileKey,
													Path: awsCredentialsFileName,
												},
											},
										},
									},
								},
								{
									Name: "unsolicited-vol",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "secretname",
											Items: []corev1.KeyToPath{
												{
													Key:  "key",
													Path: "path",
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  ExternalDNSContainerName,
									Image: test.OperandImage,
									Args: []string{
										"--metrics-address=127.0.0.1:7979",
										"--txt-owner-id=external-dns-test",
										"--zone-id-filter=my-dns-public-zone",
										"--provider=aws",
										"--source=service",
										"--policy=sync",
										"--registry=txt",
										"--log-level=debug",
										"--service-type-filter=LoadBalancer",
										"--txt-prefix=external-dns-",
									},
									Env: []corev1.EnvVar{
										{
											Name:  awsCredentialEnvVarName,
											Value: awsCredentialsFilePath,
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      awsCredentialsVolumeName,
											MountPath: awsCredentialsMountPath,
											ReadOnly:  true,
										},
										{
											Name:      "unsolicited-vm",
											MountPath: "somepath",
											ReadOnly:  true,
										},
									},
									SecurityContext: &corev1.SecurityContext{
										Capabilities: &corev1.Capabilities{
											Drop: []corev1.Capability{allCapabilities},
										},
										Privileged:               pointer.Bool(false),
										RunAsNonRoot:             pointer.Bool(true),
										AllowPrivilegeEscalation: pointer.Bool(false),
										SeccompProfile: &corev1.SeccompProfile{
											Type: corev1.SeccompProfileTypeRuntimeDefault,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().WithScheme(test.Scheme).WithRuntimeObjects(tc.existingObjects...).Build()
			r := &reconciler{
				client: cl,
				scheme: test.Scheme,
				log:    zap.New(zap.UseDevMode(true)),
			}

			gotExist, gotDepl, err := r.ensureExternalDNSDeployment(context.TODO(), test.OperandNamespace, test.OperandImage, serviceAccount, tc.credSecret, tc.trustCAConfigMap, &tc.extDNS)
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("unexpected error received: %v", err)
				}
				return
			}
			if tc.errExpected {
				t.Fatalf("Error expected but wasn't received")
			}
			if gotExist != tc.expectedExist {
				t.Errorf("expected deployment's exist to be %t, got %t", tc.expectedExist, gotExist)
			}
			deplOpt := cmpopts.IgnoreFields(appsv1.Deployment{}, "ResourceVersion", "Kind", "APIVersion")
			contOpt := cmpopts.IgnoreFields(corev1.Container{}, "TerminationMessagePolicy", "ImagePullPolicy")
			sortArgsOpt := cmp.Transformer("Sort", func(d appsv1.Deployment) appsv1.Deployment {
				if len(d.Spec.Template.Spec.Containers) == 0 {
					return d
				}
				cpy := *d.DeepCopy()
				for i := range cpy.Spec.Template.Spec.Containers {
					sort.Strings(cpy.Spec.Template.Spec.Containers[i].Args)
				}
				return cpy
			})
			if diff := cmp.Diff(tc.expectedDeployment, *gotDepl, deplOpt, contOpt, sortArgsOpt); diff != "" {
				t.Errorf("unexpected deployment (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildSecretHash(t *testing.T) {
	testCases := []struct {
		name            string
		inputSecretData map[string][]byte
		expectedHash    string
		errExpected     bool
	}{
		{
			name: "correct hash",
			inputSecretData: map[string][]byte{
				"aws_access_key_id":     []byte("aws_access_key_id"),
				"aws_secret_access_key": []byte("aws_secret_access_key"),
			},
			expectedHash: "93fd56cba8fc84aba59b5f6743b2ea34aca7690fa829aa98b8cdcbf42808d213",
			errExpected:  false,
		},
		{
			name:            "empty data",
			inputSecretData: map[string][]byte{},
			expectedHash:    testSecretHash,
			errExpected:     false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotHash, err := buildMapHash(tc.inputSecretData)
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("unexpected error received: %v", err)
				}
				return
			}
			if tc.errExpected {
				t.Fatalf("Error expected but wasn't received")
			}
			if gotHash != tc.expectedHash {
				t.Errorf("unexpected secret hash: %s", gotHash)
			}
		})
	}
}

func TestSecurityContextChanged(t *testing.T) {
	for _, tc := range []struct {
		name      string
		currentSC *corev1.SecurityContext
		desiredSC *corev1.SecurityContext
		updatedSC *corev1.SecurityContext
		changed   bool
	}{
		{
			name:      "current RunAsNonRoot is nil",
			currentSC: &corev1.SecurityContext{},
			desiredSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			changed:   true,
		},
		{
			// should be ignored to handle defaulting
			name:      "desired RunAsNonRoot is nil",
			currentSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			desiredSC: &corev1.SecurityContext{},
			updatedSC: &corev1.SecurityContext{},
			changed:   false,
		},
		{
			name:      "RunAsNonRoot changes true->false",
			currentSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			changed:   true,
		},
		{
			name:      "RunAsNonRoot changes false->true",
			currentSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(false)},
			changed:   true,
		},
		{
			name:      "RunAsNonRoot changes is same",
			currentSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(true)},
			updatedSC: &corev1.SecurityContext{RunAsNonRoot: pointer.Bool(true)},
			changed:   false,
		},
		{
			name:      "current Privileged is nil",
			currentSC: &corev1.SecurityContext{},
			desiredSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			changed:   true,
		},
		{
			// should be ignored to handle defaulting
			name:      "desired Privileged is nil",
			desiredSC: &corev1.SecurityContext{},
			currentSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			changed:   false,
		},
		{
			name:      "Privileged changes true->false",
			currentSC: &corev1.SecurityContext{Privileged: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			changed:   true,
		},
		{
			name:      "Privileged changes false->true",
			currentSC: &corev1.SecurityContext{Privileged: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{Privileged: pointer.Bool(false)},
			changed:   true,
		},
		{
			name:      "Privileged is same",
			currentSC: &corev1.SecurityContext{Privileged: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{Privileged: pointer.Bool(true)},
			updatedSC: &corev1.SecurityContext{Privileged: pointer.Bool(true)},
			changed:   false,
		},
		{
			name:      "current AllowPrivilegeEscalation is nil",
			currentSC: &corev1.SecurityContext{},
			desiredSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			changed:   true,
		},
		{
			// should be ignored to handle defaulting
			name:      "desired AllowPrivilegeEscalation is nil",
			desiredSC: &corev1.SecurityContext{},
			currentSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			changed:   false,
		},
		{
			name:      "AllowPrivilegeEscalation changes true->false",
			currentSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			changed:   true,
		},
		{
			name:      "AllowPrivilegeEscalation changes false->true",
			currentSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			updatedSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(false)},
			changed:   true,
		},
		{
			name:      "AllowPrivilegeEscalation is same",
			currentSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(true)},
			desiredSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(true)},
			updatedSC: &corev1.SecurityContext{AllowPrivilegeEscalation: pointer.Bool(true)},
			changed:   false,
		},
		{
			name:      "Add Capabilities are the same",
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"C", "B", "A"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"C", "B", "A"}}},
			changed:   false,
		},
		{
			name:      "Add Capabilities are the different",
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"C", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"C", "B", "C"}}},
			changed:   true,
		},
		{
			name:      "current Capabilities are nil",
			currentSC: &corev1.SecurityContext{},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			changed:   true,
		},
		{
			// ignore the desired because the capabilities might be defaulting or set by something else.
			name:      "desired Capabilities are nil",
			desiredSC: &corev1.SecurityContext{},
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			changed:   false,
		},
		{
			name:      "current Add Capabilities are nil",
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{}},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			changed:   true,
		},
		{
			name:      "desired Add Capabilities are nil",
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{}},
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"A", "B", "C"}}},
			changed:   true,
		},
		{
			name:      "Drop Capabilities are the same",
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"A", "B", "C"}}},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"C", "B", "A"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"C", "B", "A"}}},
			changed:   false,
		},
		{
			name:      "Drop Capabilities are the different",
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"A", "B", "C"}}},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"C", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"C", "B", "C"}}},
			changed:   true,
		},
		{
			name:      "current Drop Capabilities are nil",
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{}},
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"A", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"A", "B", "C"}}},
			changed:   true,
		},
		{
			name:      "desired Drop Capabilities are nil",
			desiredSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{}},
			currentSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"A", "B", "C"}}},
			updatedSC: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"A", "B", "C"}}},
			changed:   true,
		},
		{
			name:      "current SeccompProfile is nil",
			currentSC: &corev1.SecurityContext{},
			desiredSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost}},
			updatedSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost}},
			changed:   true,
		},
		{
			// ignore the desired seccompprofile if it is being defaulted elsewhere
			name:      "desired SeccompProfile is nil",
			desiredSC: &corev1.SecurityContext{},
			currentSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost}},
			updatedSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost}},
			changed:   false,
		},
		{
			name:      "SeccompProfile is different",
			currentSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			desiredSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost}},
			updatedSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeLocalhost}},
			changed:   true,
		},
		{
			name:      "SeccompProfile is same",
			currentSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			desiredSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			updatedSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			changed:   false,
		},
		{
			name:      "SeccompProfile is empty",
			currentSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{}},
			desiredSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			updatedSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			changed:   true,
		},
		{
			name:      "Only update SeccompProfile in security context",
			currentSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{}},
			desiredSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			updatedSC: &corev1.SecurityContext{
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
				RunAsUser:      pointer.Int64(1007),
			},
			changed: true,
		},
		{
			name:      "Don't update security context if diff in other fields",
			currentSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}},
			desiredSC: &corev1.SecurityContext{SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}, RunAsUser: pointer.Int64(1007)},
			updatedSC: &corev1.SecurityContext{
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			changed: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed, updated := securityContextChanged(tc.currentSC, tc.updatedSC, tc.desiredSC)
			if changed != tc.changed {
				t.Errorf("expected %v, instead was %v", tc.changed, changed)
			}

			if tc.changed {
				if !equality.Semantic.DeepDerivative(tc.desiredSC, updated) {
					t.Errorf("expected %v, instead was %v", tc.desiredSC, updated)
				}

				if !equality.Semantic.DeepDerivative(tc.updatedSC, updated) {
					t.Errorf("expected %v, instead was %v", tc.updatedSC, updated)
				}
			}
		})
	}
}

func testDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
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
					Annotations: map[string]string{
						credentialsAnnotation: testSecretHash,
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

func testDeploymentWithoutAnnotations() *appsv1.Deployment {
	depl := testDeployment()
	depl.Spec.Template.Annotations = nil
	return depl
}

func testDeploymentWithContainers(containers ...corev1.Container) *appsv1.Deployment {
	depl := testDeployment()
	depl.Spec.Template.Spec.Containers = containers
	return depl
}

func testDeploymentWithAnnotations(annotations map[string]string) *appsv1.Deployment {
	depl := testDeployment()
	depl.Spec.Template.Annotations = annotations
	return depl
}

func testDeploymentWithVolumes(volumes ...corev1.Volume) *appsv1.Deployment {
	depl := testDeployment()
	depl.Spec.Template.Spec.Volumes = volumes
	return depl
}

func testContainer() corev1.Container {
	return corev1.Container{
		Name:                     "first",
		Image:                    test.OperandImage,
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		Args: []string{
			"--flag1=value1",
			"--flag2=value2",
		},
	}
}

func testContainerWithSecurityContext(sc *corev1.SecurityContext) corev1.Container {
	cont := testContainer()
	cont.SecurityContext = sc
	return cont
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

func testContainerWithArgs(args ...string) corev1.Container {
	cont := testContainer()
	cont.Args = args
	return cont
}

func testConfigMapVolume(name, cmname, key, path string) corev1.Volume {
	mode := int32(0644)
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cmname,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  key,
						Path: path,
					},
				},
				DefaultMode: &mode,
			},
		},
	}
}

func testSecretVolume(name, sname, key, path string) corev1.Volume {
	mode := int32(0644)
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: sname,
				Items: []corev1.KeyToPath{
					{
						Key:  key,
						Path: path,
					},
				},
				DefaultMode: &mode,
			},
		},
	}
}

func testExternalDNSInstance(provider operatorv1beta1.ExternalDNSProviderType,
	source operatorv1beta1.ExternalDNSSourceType,
	svcType []corev1.ServiceType,
	labelFilter *metav1.LabelSelector,
	hostnamePolicy operatorv1beta1.HostnameAnnotationPolicy,
	fqdnTemplate []string,
	zones []string, routerName string,
) *operatorv1beta1.ExternalDNS {
	extDnsSource := &operatorv1beta1.ExternalDNSSource{
		ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
			Type: source,
			Service: &operatorv1beta1.ExternalDNSServiceSourceOptions{
				ServiceType: svcType,
			},
			LabelFilter: labelFilter,
		},
		HostnameAnnotationPolicy: hostnamePolicy,
		FQDNTemplate:             fqdnTemplate,
	}
	// As FQDNTemplate: not needed for openshift-route source
	extDnsSourceForRoute := &operatorv1beta1.ExternalDNSSource{
		ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
			Type: source,
			OpenShiftRoute: &operatorv1beta1.ExternalDNSOpenShiftRouteOptions{
				RouterName: routerName,
			},
			LabelFilter: labelFilter,
		},
		HostnameAnnotationPolicy: hostnamePolicy,
	}
	extDNS := &operatorv1beta1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: test.Name,
		},
		Spec: operatorv1beta1.ExternalDNSSpec{
			Provider: operatorv1beta1.ExternalDNSProvider{
				Type: provider,
			},

			Zones: zones,
		},
	}
	if source == operatorv1beta1.SourceTypeRoute {
		extDNS.Spec.Source = *extDnsSourceForRoute
		return extDNS
	}

	if source == operatorv1beta1.SourceTypeService {
		extDNS.Spec.Source = *extDnsSource
		return extDNS
	}
	return extDNS
}

func testExternalDNSHostnameIgnore(provider operatorv1beta1.ExternalDNSProviderType,
	source operatorv1beta1.ExternalDNSSourceType,
	svcTypes []corev1.ServiceType,
	zones []string, routerName string,
) *operatorv1beta1.ExternalDNS {
	return testExternalDNSInstance(provider, source, svcTypes, nil, operatorv1beta1.HostnameAnnotationPolicyIgnore, []string{"{{.Name}}.test.com"}, zones, routerName)
}

func testExternalDNSHostnameAllow(provider operatorv1beta1.ExternalDNSProviderType,
	source operatorv1beta1.ExternalDNSSourceType,
	svcTypes []corev1.ServiceType,
	zones []string, routerName string,
) *operatorv1beta1.ExternalDNS {
	return testExternalDNSInstance(provider, source, svcTypes, nil, operatorv1beta1.HostnameAnnotationPolicyAllow, nil, zones, routerName)
}

func testAWSExternalDNS(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeAWS, nil, "")
}

func testAWSExternalDNSZones(zones []string, source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeAWS, zones, "")
}

func testAWSExternalDNSHostnameAllow(source operatorv1beta1.ExternalDNSSourceType, routerName string) *operatorv1beta1.ExternalDNS {
	switch source {
	case operatorv1beta1.SourceTypeService:
		return testExternalDNSHostnameAllow(operatorv1beta1.ProviderTypeAWS, source, []corev1.ServiceType{corev1.ServiceTypeLoadBalancer}, []string{test.PublicZone}, routerName)

	case operatorv1beta1.SourceTypeRoute:
		return testExternalDNSHostnameAllow(operatorv1beta1.ProviderTypeAWS, source, nil, []string{test.PublicZone}, routerName)
	}
	return nil
}

func testAWSExternalDNSFQDNTemplate(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	extDNS := testAWSExternalDNS(source)
	extDNS.Spec.Source.FQDNTemplate = []string{"{{.Name}}.test.com"}
	return extDNS
}

func testAWSExternalDNSManyFQDN() *operatorv1beta1.ExternalDNS {
	extdns := testExternalDNSHostnameIgnore(operatorv1beta1.ProviderTypeAWS, operatorv1beta1.SourceTypeService, []corev1.ServiceType{corev1.ServiceTypeLoadBalancer}, []string{test.PublicZone}, "")
	extdns.Spec.Source.FQDNTemplate = append(extdns.Spec.Source.FQDNTemplate, "{{.Name}}.{{.Namespace}}.example.com")
	return extdns
}

func testAWSExternalDNSLabelFilter(selector *metav1.LabelSelector, source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	extdns := testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeAWS, nil, "")
	extdns.Spec.Source.LabelFilter = selector
	return extdns
}

func testAzureExternalDNS(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeAzure, nil, "")
}

func testAzureExternalDNSNoZones(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testExternalDNSHostnameIgnore(operatorv1beta1.ProviderTypeAzure, source, allSvcTypes, nil, "")
}

func testAzureExternalDNSPrivateZones(zones []string, source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeAzure, zones, "")
}

func testGCPExternalDNS(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	extdns := testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeGCP, nil, "")
	project := "external-dns-gcp-project"
	extdns.Spec.Provider.GCP = &operatorv1beta1.ExternalDNSGCPProviderOptions{
		Project: &project,
	}
	return extdns
}

func testCreateDNSFromSourceWRTCloudProvider(source operatorv1beta1.ExternalDNSSourceType, providerType operatorv1beta1.ExternalDNSProviderType, zones []string, routerName string) *operatorv1beta1.ExternalDNS {
	switch source {
	case operatorv1beta1.SourceTypeService:
		// we need to check nil as for the test case No_zones_&&_no_domain_filter and No_zones_+_Domain_filter because if we check len(zones)
		// then it will to else condition and fail as test.PublicZone will be added where we don't want any zones
		if zones != nil {
			return testExternalDNSHostnameIgnore(providerType, source, allSvcTypes, zones, routerName)
		} else {
			return testExternalDNSHostnameIgnore(providerType, source, allSvcTypes, []string{test.PublicZone}, routerName)
		}
	case operatorv1beta1.SourceTypeRoute:
		if zones != nil {
			return testExternalDNSHostnameIgnore(providerType, source, nil, zones, routerName)
		} else {
			return testExternalDNSHostnameIgnore(providerType, source, nil, []string{test.PublicZone}, routerName)
		}
	}
	return nil
}

func testGCPExternalDNSNoProject(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeGCP, nil, "")
}

func testBlueCatExternalDNS(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	return testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeBlueCat, nil, "")
}

func testInfobloxExternalDNS(source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	extdns := testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeInfoblox, nil, "")
	extdns.Spec.Provider.Infoblox = &operatorv1beta1.ExternalDNSInfobloxProviderOptions{
		GridHost:    "gridhost.example.com",
		WAPIPort:    443,
		WAPIVersion: "2.3.1",
	}
	return extdns
}

func testAWSExternalDNSDomainFilter(zones []string, source operatorv1beta1.ExternalDNSSourceType) *operatorv1beta1.ExternalDNS {
	extdns := testCreateDNSFromSourceWRTCloudProvider(source, operatorv1beta1.ProviderTypeAWS, zones, "")
	extdns.Spec.Domains = []operatorv1beta1.ExternalDNSDomain{
		{
			ExternalDNSDomainUnion: operatorv1beta1.ExternalDNSDomainUnion{
				MatchType: operatorv1beta1.DomainMatchTypeExact,
				Name:      pointer.String("abc.com"),
			},
			FilterType: operatorv1beta1.FilterTypeInclude,
		},
	}
	return extdns
}

func testPlatformStatusGCP(projectID string) *configv1.PlatformStatus {
	return &configv1.PlatformStatus{
		Type: configv1.GCPPlatformType,
		GCP: &configv1.GCPPlatformStatus{
			ProjectID: projectID,
		},
	}
}

func testPlatformStatusAWSGov(region string) *configv1.PlatformStatus {
	return &configv1.PlatformStatus{
		Type: configv1.AWSPlatformType,
		AWS: &configv1.AWSPlatformStatus{
			Region: region,
		},
	}
}

func testTrustedCAConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      test.TrustedCAConfigMapName,
			Namespace: test.OperandNamespace,
		},
		Data: map[string]string{
			"ca-bundle.crt": "--ca bundle--",
		},
	}
}
