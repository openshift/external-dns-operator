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
	"os"

	"path/filepath"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	controller "github.com/openshift/external-dns-operator/pkg/operator/controller"
	"github.com/openshift/external-dns-operator/pkg/utils"
)

const (
	defaultMetricsAddress    = "127.0.0.1"
	defaultOwnerPrefix       = "external-dns"
	defaultMetricsStartPort  = 7979
	defaultConfigMountPath   = "/etc/kubernetes"
	defaultTXTRecordPrefix   = "external-dns-"
	providerArg              = "--provider="
	httpProxyEnvVar          = "HTTP_PROXY"
	httpsProxyEnvVar         = "HTTPS_PROXY"
	noProxyEnvVar            = "NO_PROXY"
	trustedCAVolumeName      = "trusted-ca"
	trustedCAFileName        = "tls-ca-bundle.pem"
	trustedCAFileKey         = "ca-bundle.crt"
	trustedCAExtractedPEMDir = "/etc/pki/ca-trust/extracted/pem"
	// RHEL path for the trusted certificate bundle may not work for some distributions (e.g. Alpine).
	// This makes the usage of the trusted CAs impossible when the vanilla upstream image is given.
	// SSL_CERT_DIR allows Golang's crypto library to override the default locations.
	// https://pkg.go.dev/crypto/x509#SystemCertPool
	sslCertDirEnvVar = "SSL_CERT_DIR"
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
	image          string
	provider       string
	source         string
	volumes        []corev1.Volume
	secretName     string
	externalDNS    *operatorv1alpha1.ExternalDNS
	isOpenShift    bool
	platformStatus *configv1.PlatformStatus
	counter        int
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
	b.fillProviderSpecificFields(zone, container)
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
	//
	// ARGS
	//
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

	if b.externalDNS.Spec.Source.LabelFilter != nil {
		args = append(args, fmt.Sprintf("--label-filter=%s", metav1.FormatLabelSelector(b.externalDNS.Spec.Source.LabelFilter)))
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
	} else {
		// ExternalDNS needs FQDNTemplate if the hostname annotation is ignored even for Route source.
		// However it doesn't make much sense as the hostname is retrieved from the route's spec.
		// Feeding ExternalDNS with some dummy template just to pass the validation.
		if b.externalDNS.Spec.Source.HostnameAnnotationPolicy == operatorv1alpha1.HostnameAnnotationPolicyIgnore &&
			b.externalDNS.Spec.Source.Type == operatorv1alpha1.SourceTypeRoute {
			args = append(args, "--fqdn-template={{\"\"}}")
		}
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

	//
	// ENV
	//
	if utils.EnvProxySupportedProvider(b.externalDNS) {
		if val := os.Getenv(httpProxyEnvVar); val != "" {
			container.Env = append(container.Env, corev1.EnvVar{Name: httpProxyEnvVar, Value: val})
		}
		if val := os.Getenv(httpsProxyEnvVar); val != "" {
			container.Env = append(container.Env, corev1.EnvVar{Name: httpsProxyEnvVar, Value: val})
		}
		if val := os.Getenv(noProxyEnvVar); val != "" {
			container.Env = append(container.Env, corev1.EnvVar{Name: noProxyEnvVar, Value: val})
		}
	}

	//
	// VOLUME MOUNTS
	//
	for _, v := range b.volumes {
		// if trustedCA volume was added
		if v.Name == trustedCAVolumeName {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      v.Name,
				MountPath: trustedCAExtractedPEMDir,
				ReadOnly:  true,
			})
			container.Env = append(container.Env, corev1.EnvVar{Name: sslCertDirEnvVar, Value: trustedCAExtractedPEMDir})
		}
	}

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
func (b *externalDNSContainerBuilder) fillProviderSpecificFields(zone string, container *corev1.Container) {
	switch b.provider {
	case externalDNSProviderTypeAWS:
		b.fillAWSFields(container)
	case externalDNSProviderTypeAzure, externalDNSProviderTypeAzurePrivate:
		b.fillAzureFields(zone, container)
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
func (b *externalDNSContainerBuilder) fillAzureFields(zone string, container *corev1.Container) {
	// https://github.com/kubernetes-sigs/external-dns/issues/2082
	container.Args = addTXTPrefixFlag(container.Args)

	// check the zone field for the keyword 'privatednszones', this ensures that the
	// provider 'azure-private-dns' is passed to the container
	// to set the operand provider correctly
	if strings.Contains(strings.ToLower(zone), azurePrivateDNSZonesResourceSubStr) {
		for i, x := range container.Args {
			if strings.Contains(x, providerArg) {
				container.Args[i] = providerArg + externalDNSProviderTypeAzurePrivate
				break
			}
		}
	}
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

	if !b.isOpenShift {
		// don't add empty args if GCP provider is not given
		if b.externalDNS.Spec.Provider.GCP == nil {
			return
		}

		if b.externalDNS.Spec.Provider.GCP.Project != nil && len(*b.externalDNS.Spec.Provider.GCP.Project) > 0 {
			container.Args = append(container.Args, fmt.Sprintf("--google-project=%s", *b.externalDNS.Spec.Provider.GCP.Project))
		}
	} else {
		if b.platformStatus != nil && b.platformStatus.GCP != nil && len(b.platformStatus.GCP.ProjectID) > 0 {
			container.Args = append(container.Args, fmt.Sprintf("--google-project=%s", b.platformStatus.GCP.ProjectID))
		}
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

	args = addTXTPrefixFlag(args)

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
	provider               string
	secretName             string
	trustedCAConfigMapName string
}

// newExternalDNSVolumeBuilder returns an instance of volume builder
func newExternalDNSVolumeBuilder(provider, secretName, trustedCAConfigMapName string) *externalDNSVolumeBuilder {
	return &externalDNSVolumeBuilder{
		provider:               provider,
		secretName:             secretName,
		trustedCAConfigMapName: trustedCAConfigMapName,
	}
}

// build returns the definition of all the volumes
func (b *externalDNSVolumeBuilder) build() []corev1.Volume {
	volumes := b.providerAgnosticVolumes()
	return append(volumes, b.providerSpecificVolumes()...)
}

// providerAgnosticVolumes returns the volumes ...
func (b *externalDNSVolumeBuilder) providerAgnosticVolumes() []corev1.Volume {
	if len(b.trustedCAConfigMapName) > 0 {
		return []corev1.Volume{
			{
				Name: trustedCAVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: b.trustedCAConfigMapName,
						},
						Items: []corev1.KeyToPath{
							{
								Key:  trustedCAFileKey,
								Path: trustedCAFileName,
							},
						},
					},
				},
			},
		}
	}
	return nil
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
