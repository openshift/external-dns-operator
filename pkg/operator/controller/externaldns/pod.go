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
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

const (
	defaultMetricsAddress   = "127.0.0.1"
	defaultOwnerPrefix      = "external-dns"
	defaultMetricsStartPort = 7979
	//
	// AWS
	//
	awsAccessKeyIDEnvVar     = "AWS_ACCESS_KEY_ID"
	awsAccessKeySecretEnvVar = "AWS_SECRET_ACCESS_KEY"
	awsAccessKeyIDKey        = "aws_access_key_id"
	awsAccessKeySecretKey    = "aws_secret_access_key"
	//
	// Azure
	//
	azureConfigVolumeName = "azure-config-file"
	azureConfigMountPath  = "/etc/kubernetes"
	azureConfigFileName   = "azure.json"
	azureConfigFileKey    = "azure.json"
	//
	// GCP
	//
	gcpCredentialsVolumeName = "gcp-credentials-file"
	gcpCredentialsMountPath  = "/etc/kubernetes"
	gcpCredentialsFileKey    = "gcp-credentials.json"
	gcpCredentialsFileName   = "gcp-credentials.json"
	gcpAppCredentialsEnvVar  = "GOOGLE_APPLICATION_CREDENTIALS"
	//
	// BlueCat
	//
	blueCatConfigVolumeName = "bluecat-config-file"
	blueCatConfigMountPath  = "/etc/kubernetes"
	blueCatConfigFileName   = "bluecat.json"
	blueCatConfigFileKey    = "bluecat.json"
	//
	// Infoblox
	//
	infobloxWAPIUsernameEnvVar = "EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME"
	infobloxWAPIPasswordEnvVar = "EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD"
	infobloxWAPIUsernameKey    = "EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME"
	infobloxWAPIPasswordKey    = "EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD"
)

// externalDNSContainerBuilder builds the definition of the containers for ExternalDNS POD
type externalDNSContainerBuilder struct {
	image       string
	provider    string
	source      string
	volumes     []corev1.Volume
	secretName  string
	externalDNS *operatorv1alpha1.ExternalDNS
	counter     int
}

// newExternalDNSContainerBuilder returns an instance of container builder
func newExternalDNSContainerBuilder(image, provider, source, secretName string, volumes []corev1.Volume, externalDNS *operatorv1alpha1.ExternalDNS) *externalDNSContainerBuilder {
	return &externalDNSContainerBuilder{
		image:       image,
		provider:    provider,
		source:      source,
		secretName:  secretName,
		volumes:     volumes,
		externalDNS: externalDNS,
		counter:     0,
	}
}

// build returns the definition of a single container for the given DNS zone with unique metrics port
func (b *externalDNSContainerBuilder) build(zone string) *corev1.Container {
	seq := b.counter
	b.counter++
	return b.buildSeq(seq, zone)
}

// buildSeq returns the definition of a single container for the given DNS zone
// sequence param is used to create the unique metrics port
func (b *externalDNSContainerBuilder) buildSeq(seq int, zone string) *corev1.Container {
	container := b.defaultContainer(controller.ExternalDNSContainerName(zone))
	b.fillProviderAgnosticFields(seq, zone, container)
	b.fillProviderSpecificFields(container)
	return container
}

// defaultContainer returns the initial definition of any container of ExternalDNS POD
func (b *externalDNSContainerBuilder) defaultContainer(name string) *corev1.Container {
	return &corev1.Container{
		Name:                     name,
		Image:                    b.image,
		Args:                     []string{},
		ImagePullPolicy:          corev1.PullIfNotPresent,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}
}

// fillProviderAgnosticFields fills the given container with the data agnostic to any provider
func (b *externalDNSContainerBuilder) fillProviderAgnosticFields(seq int, zone string, container *corev1.Container) {
	args := []string{
		fmt.Sprintf("--metrics-address=%s:%d", defaultMetricsAddress, defaultMetricsStartPort+seq),
		fmt.Sprintf("--txt-owner-id=%s-%s", defaultOwnerPrefix, b.externalDNS.Name),
		fmt.Sprintf("--zone-id-filter=%s", zone),
		fmt.Sprintf("--provider=%s", b.provider),
		fmt.Sprintf("--source=%s", b.source),
		"--policy=sync",
		"--registry=txt",
		"--log-level=debug",
	}

	if b.externalDNS.Spec.Source.Namespace != nil && len(*b.externalDNS.Spec.Source.Namespace) > 0 {
		args = append(args, fmt.Sprintf("--namespace=%s", *b.externalDNS.Spec.Source.Namespace))
	}

	if len(b.externalDNS.Spec.Source.AnnotationFilter) > 0 {
		annotationFilter := ""
		for key, value := range b.externalDNS.Spec.Source.AnnotationFilter {
			annotationFilter += fmt.Sprintf("%s=%s,", key, value)
		}
		args = append(args, fmt.Sprintf("--annotation-filter=%s", annotationFilter[0:len(annotationFilter)-1]))
	}

	if b.externalDNS.Spec.Source.Service != nil && len(b.externalDNS.Spec.Source.Service.ServiceType) > 0 {
		publishInternal := false
		for _, serviceType := range b.externalDNS.Spec.Source.Service.ServiceType {
			args = append(args, fmt.Sprintf("--service-type-filter=%s", string(serviceType)))
			if serviceType == corev1.ServiceTypeClusterIP {
				publishInternal = true
			}
		}

		// legacy option before the service-type-filter was introduced
		// must be there though, ClusterIP endpoints won't be added without it
		if publishInternal {
			args = append(args, "--publish-internal-services")
		}
	}

	if b.externalDNS.Spec.Source.HostnameAnnotationPolicy == operatorv1alpha1.HostnameAnnotationPolicyIgnore {
		args = append(args, "--ignore-hostname-annotation")
	}

	if len(b.externalDNS.Spec.Source.FQDNTemplate) > 0 {
		args = append(args, fmt.Sprintf("--fqdn-template=%s", strings.Join(b.externalDNS.Spec.Source.FQDNTemplate, ",")))
	}

	//TODO: Add logic for the CRD source.

	container.Args = append(container.Args, args...)
}

// fillProviderSpecificFields fills the fields specific to the provider of given ExternalDNS
func (b *externalDNSContainerBuilder) fillProviderSpecificFields(container *corev1.Container) {
	switch b.provider {
	case externalDNSProviderTypeAWS:
		b.fillAWSFields(container)
	case externalDNSProviderTypeAzure:
		b.fillAzureFields(container)
	case externalDNSProviderTypeGCP:
		b.fillGCPFields(container)
	case externalDNSProviderTypeBlueCat:
		b.fillBlueCatFields(container)
	case externalDNSProviderTypeInfoblox:
		b.fillInfobloxFields(container)
	}
}

// fillAWSFields fills the given container with the data specific to AWS provider
func (b *externalDNSContainerBuilder) fillAWSFields(container *corev1.Container) {
	// don't add empty credentials environment variables if no secret was given
	if len(b.secretName) == 0 {
		return
	}

	env := []corev1.EnvVar{
		{
			Name: awsAccessKeyIDEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: b.secretName,
					},
					Key: awsAccessKeyIDKey,
				},
			},
		},
		{
			Name: awsAccessKeySecretEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: b.secretName,
					},
					Key: awsAccessKeySecretKey,
				},
			},
		},
	}

	container.Env = append(container.Env, env...)
}

// fillAzureFields fills the given container with the data specific to Azure provider
func (b *externalDNSContainerBuilder) fillAzureFields(container *corev1.Container) {
	// no volume mounts will be added if there is no config volume added before
	for _, v := range b.volumes {
		// config volume
		if v.Name == azureConfigVolumeName {
			container.Args = append(container.Args, fmt.Sprintf("--azure-config-file=%s/%s", azureConfigMountPath, azureConfigFileName))
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      v.Name,
				MountPath: azureConfigMountPath,
				ReadOnly:  true,
			})
		}
	}
}

// fillGCPFields fills the given container with the data specific to Google provider
func (b *externalDNSContainerBuilder) fillGCPFields(container *corev1.Container) {
	// don't add empty args GCP provider is not given
	if b.externalDNS.Spec.Provider.GCP == nil {
		return
	}

	if b.externalDNS.Spec.Provider.GCP.Project != nil && len(*b.externalDNS.Spec.Provider.GCP.Project) > 0 {
		container.Args = append(container.Args, fmt.Sprintf("--google-project=%s", *b.externalDNS.Spec.Provider.GCP.Project))
	}

	for _, v := range b.volumes {
		// credentials volume
		if v.Name == gcpCredentialsVolumeName {
			container.Env = append(container.Env, corev1.EnvVar{Name: gcpAppCredentialsEnvVar, Value: filepath.Join(gcpCredentialsMountPath, gcpCredentialsFileKey)})
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      v.Name,
				MountPath: gcpCredentialsMountPath,
				ReadOnly:  true,
			})
		}
	}
}

// fillBlueCatFields fills the given container with the data specific to BlueCat provider
func (b *externalDNSContainerBuilder) fillBlueCatFields(container *corev1.Container) {
	// no volume mounts will be added if there is no config volume added before
	for _, v := range b.volumes {
		// config volume
		if v.Name == blueCatConfigVolumeName {
			container.Args = append(container.Args, fmt.Sprintf("--bluecat-config-file=%s/%s", blueCatConfigMountPath, blueCatConfigFileName))
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      v.Name,
				MountPath: blueCatConfigMountPath,
				ReadOnly:  true,
			})
		}
	}
}

// fillInfobloxFields fills the given container with the data specific to Infoblox provider
func (b *externalDNSContainerBuilder) fillInfobloxFields(container *corev1.Container) {
	// don't add empty args or env vars if secret or infoblox provider is not given
	if len(b.secretName) == 0 || b.externalDNS.Spec.Provider.Infoblox == nil {
		return
	}

	args := []string{
		fmt.Sprintf("--infoblox-wapi-port=%d", b.externalDNS.Spec.Provider.Infoblox.WAPIPort),
	}

	if len(b.externalDNS.Spec.Provider.Infoblox.GridHost) > 0 {
		args = append(args, fmt.Sprintf("--infoblox-grid-host=%s", b.externalDNS.Spec.Provider.Infoblox.GridHost))
	}

	if len(b.externalDNS.Spec.Provider.Infoblox.WAPIVersion) > 0 {
		args = append(args, fmt.Sprintf("--infoblox-wapi-version=%s", b.externalDNS.Spec.Provider.Infoblox.WAPIVersion))
	}

	env := []corev1.EnvVar{
		{
			Name: infobloxWAPIUsernameEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: b.secretName,
					},
					Key: infobloxWAPIUsernameKey,
				},
			},
		},
		{
			Name: infobloxWAPIPasswordEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: b.secretName,
					},
					Key: infobloxWAPIPasswordKey,
				},
			},
		},
	}

	container.Args = append(container.Args, args...)
	container.Env = append(container.Env, env...)
}

// externalDNSVolumeBuilder builds the definition of the volumes for ExternalDNS POD
type externalDNSVolumeBuilder struct {
	provider   string
	secretName string
}

// newExternalDNSVolumeBuilder returns an instance of volume builder
func newExternalDNSVolumeBuilder(provider, secretName string) *externalDNSVolumeBuilder {
	return &externalDNSVolumeBuilder{
		provider:   provider,
		secretName: secretName,
	}
}

// build returns the definition of all the volumes
func (b *externalDNSVolumeBuilder) build() []corev1.Volume {
	return b.providerSpecificVolumes()
}

// providerSpecificVolumes returns the volumes specific to the provider of given External DNS
func (b *externalDNSVolumeBuilder) providerSpecificVolumes() []corev1.Volume {
	switch b.provider {
	case externalDNSProviderTypeAzure:
		return b.azureVolumes()
	case externalDNSProviderTypeGCP:
		return b.gcpVolumes()
	case externalDNSProviderTypeBlueCat:
		return b.bluecatVolumes()
	}
	return nil
}

// azureVolumes returns volumes needed for Azure provider
func (b *externalDNSVolumeBuilder) azureVolumes() []corev1.Volume {
	if len(b.secretName) == 0 {
		return nil
	}

	return []corev1.Volume{
		{
			Name: azureConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: b.secretName,
					Items: []corev1.KeyToPath{
						{
							Key:  azureConfigFileKey,
							Path: azureConfigFileName,
						},
					},
				},
			},
		},
	}
}

// gcpVolumes returns volumes needed for Google provider
func (b *externalDNSVolumeBuilder) gcpVolumes() []corev1.Volume {
	if len(b.secretName) == 0 {
		return nil
	}

	return []corev1.Volume{
		{
			Name: gcpCredentialsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: b.secretName,
					Items: []corev1.KeyToPath{
						{
							Key:  gcpCredentialsFileKey,
							Path: gcpCredentialsFileName,
						},
					},
				},
			},
		},
	}
}

// bluecatVolumes returns volumes needed for BlueCat provider
func (b *externalDNSVolumeBuilder) bluecatVolumes() []corev1.Volume {
	if len(b.secretName) == 0 {
		return nil
	}

	return []corev1.Volume{
		{
			Name: blueCatConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: b.secretName,
					Items: []corev1.KeyToPath{
						{
							Key:  blueCatConfigFileKey,
							Path: blueCatConfigFileName,
						},
					},
				},
			},
		},
	}
}
