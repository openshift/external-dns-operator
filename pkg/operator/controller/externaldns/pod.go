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
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
)

const (
	defaultMetricsAddress   = "127.0.0.1"
	defaultOwnerPrefix      = "external-dns"
	defaultMetricsStartPort = 7979
	defaultConfigMountPath  = "/etc/kubernetes"
	defaultTXTRecordPrefix  = "external-dns-"
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
	azureConfigMountPath  = defaultConfigMountPath
	azureConfigFileName   = "azure.json"
	azureConfigFileKey    = "azure.json"
	//
	// GCP
	//
	gcpCredentialsVolumeName = "gcp-credentials-file"
	gcpCredentialsMountPath  = defaultConfigMountPath
	gcpCredentialsFileKey    = "gcp-credentials.json"
	gcpCredentialsFileName   = "gcp-credentials.json"
	gcpAppCredentialsEnvVar  = "GOOGLE_APPLICATION_CREDENTIALS"
	//
	// BlueCat
	//
	blueCatConfigVolumeName = "bluecat-config-file"
	blueCatConfigMountPath  = defaultConfigMountPath
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
func (b *externalDNSContainerBuilder) build(zone string) (*corev1.Container, error) {
	seq := b.counter
	b.counter++
	return b.buildSeq(seq, zone)
}

// buildSeq returns the definition of a single container for the given DNS zone
// sequence param is used to create the unique metrics port
func (b *externalDNSContainerBuilder) buildSeq(seq int, zone string) (*corev1.Container, error) {
	container := b.defaultContainer(controller.ExternalDNSContainerName(zone))
	err := b.fillProviderAgnosticFields(seq, zone, container)
	if err != nil {
		return nil, err
	}
	b.fillProviderSpecificFields(container)
	return container, nil
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
func (b *externalDNSContainerBuilder) fillProviderAgnosticFields(seq int, zone string, container *corev1.Container) error {
	args := []string{
		fmt.Sprintf("--metrics-address=%s:%d", defaultMetricsAddress, defaultMetricsStartPort+seq),
		fmt.Sprintf("--txt-owner-id=%s-%s", defaultOwnerPrefix, b.externalDNS.Name),
		fmt.Sprintf("--provider=%s", b.provider),
		fmt.Sprintf("--source=%s", b.source),
		"--policy=sync",
		"--registry=txt",
		"--log-level=debug",
	}

	if zone != "" {
		args = append(args, fmt.Sprintf("--zone-id-filter=%s", zone))
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

	if b.externalDNS.Spec.Source.OpenShiftRoute != nil && len(b.externalDNS.Spec.Source.OpenShiftRoute.RouterName) > 0 {
		args = append(args, fmt.Sprintf("--openshift-router-name=%s", b.externalDNS.Spec.Source.OpenShiftRoute.RouterName))
	}

	filterArgs, err := b.domainFilters()
	if err != nil {
		return err
	}

	container.Args = append(container.Args, filterArgs...)
	container.Args = append(container.Args, args...)
	return nil

}

func (b *externalDNSContainerBuilder) domainFilters() ([]string, error) {
	var args, includePatterns, excludePatterns []string
	for _, d := range b.externalDNS.Spec.Domains {
		switch d.FilterType {
		case operatorv1alpha1.FilterTypeInclude:
			switch d.MatchType {
			case operatorv1alpha1.DomainMatchTypeExact:
				if d.Name == nil {
					return nil, fmt.Errorf("name for domain cannot be empty")
				}
				args = append(args, fmt.Sprintf("--domain-filter=%s", *d.Name))
			case operatorv1alpha1.DomainMatchTypeRegex:
				if d.Pattern == nil {
					return nil, fmt.Errorf("pattern for domain cannot be empty")
				}
				_, err := regexp.Compile(*d.Pattern)
				if err != nil {
					return nil, fmt.Errorf("input pattern %s is invalid: %w", *d.Pattern, err)
				}
				includePatterns = append(includePatterns, *d.Pattern)
			default:
				return nil, fmt.Errorf("unknown match type in domains: %s", d.MatchType)
			}
		case operatorv1alpha1.FilterTypeExclude:
			switch d.MatchType {
			case operatorv1alpha1.DomainMatchTypeExact:
				if d.Name == nil {
					return nil, fmt.Errorf("name for domain cannot be empty")
				}
				args = append(args, fmt.Sprintf("--exclude-domains=%s", *d.Name))
			case operatorv1alpha1.DomainMatchTypeRegex:
				if d.Pattern == nil {
					return nil, fmt.Errorf("pattern for domain cannot be empty")
				}
				_, err := regexp.Compile(*d.Pattern)
				if err != nil {
					return nil, fmt.Errorf("exclude pattern %s is invalid: %w", *d.Pattern, err)
				}
				excludePatterns = append(excludePatterns, *d.Pattern)
			default:
				return nil, fmt.Errorf("unknown match type in domains: %s", d.MatchType)
			}
		}
	}
	if len(includePatterns) > 0 {
		args = append(args, fmt.Sprintf("--regex-domain-filter=%s", combineRegexps(includePatterns)))
	}
	if len(excludePatterns) > 0 {
		args = append(args, fmt.Sprintf("--regex-domain-exclusion=%s", combineRegexps(excludePatterns)))
	}
	return args, nil
}

func combineRegexps(patterns []string) string {
	if len(patterns) == 1 {
		return patterns[0]
	}
	parenthesisedPatterns := make([]string, len(patterns))
	for i, p := range patterns {
		parenthesisedPatterns[i] = fmt.Sprintf("(%s)", p)
	}
	return strings.Join(parenthesisedPatterns, "|")
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
	container.Args = addTXTPrefixFlag(container.Args)
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
	// https://github.com/kubernetes-sigs/external-dns/issues/2082
	container.Args = addTXTPrefixFlag(container.Args)

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
	// https://github.com/kubernetes-sigs/external-dns/issues/262
	container.Args = addTXTPrefixFlag(container.Args)

	// don't add empty args if GCP provider is not given

	if !operatorv1alpha1.IsOpenShift {
		if b.externalDNS.Spec.Provider.GCP == nil {
			return
		}

		if b.externalDNS.Spec.Provider.GCP.Project != nil && len(*b.externalDNS.Spec.Provider.GCP.Project) > 0 {
			container.Args = append(container.Args, fmt.Sprintf("--google-project=%s", *b.externalDNS.Spec.Provider.GCP.Project))
		}
	} else {
		container.Args = append(container.Args, fmt.Sprintf("--google-project=%s", operatorv1alpha1.PlatformStatus.GCP.ProjectID))
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
	// only standard CNAME records are supported
	// https://docs.bluecatnetworks.com/r/Address-Manager-API-Guide/ENUM-number-generic-methods/9.2.0
	container.Args = addTXTPrefixFlag(container.Args)

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

// addTXTPrefixFlag adds the txt prefix flag with default value
// needed if CNAME records are used: https://github.com/kubernetes-sigs/external-dns#note
func addTXTPrefixFlag(args []string) []string {
	return append(args, fmt.Sprintf("--txt-prefix=%s", defaultTXTRecordPrefix))
}
