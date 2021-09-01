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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:path=externaldnses,scope=Cluster
// +kubebuilder:subresource:status
//
// ExternalDNS describes a managed ExternalDNS controller instance for a cluster.
// The controller is responsible for creating external DNS records in supported
// DNS providers based off of instances of select Kubernetes resources.
type ExternalDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the specification of the desired behavior of the ExternalDNS.
	Spec ExternalDNSSpec `json:"spec"`
	// status is the most recently observed status of the ExternalDNS.
	Status ExternalDNSStatus `json:"status,omitempty"`
}

// ExternalDNSSpec defines the desired state of the ExternalDNS.
type ExternalDNSSpec struct {
	// Domains specifies which domains that ExternalDNS should
	// create DNS records for. Multiple domain values
	// can be specified such that subdomains of an included domain
	// can effectively be ignored using the "Include" and "Exclude"
	// domain filter options.
	//
	// An empty list of domains means ExternalDNS will create
	// DNS records for any included source resource regardless
	// of the resource's desired hostname.
	//
	// Populating Domains with only excluded options means ExternalDNS
	// will create DNS records for any included source resource that do not
	// match the provided excluded domain options.
	//
	// Excluding DNS records that were previous included via a resource update
	// will *not* result in the original DNS records being deleted.
	//
	// +optional
	Domains []ExternalDNSDomain `json:"domains,omitempty"`

	// Provider refers to the DNS provider that ExternalDNS
	// should publish records to. Note that each ExternalDNS
	// is tied to a single provider.
	//
	// +kubebuilder:validation:Required
	// +required
	Provider ExternalDNSProvider `json:"provider"`

	// Source describes which source resource
	// ExternalDNS will be configured to create
	// DNS records for.
	//
	// Multiple ExternalDNS CRs must be
	// created if multiple ExternalDNS source resources
	// are desired.
	//
	// +kubebuilder:validation:Required
	// +required
	Source ExternalDNSSource `json:"source"`

	// Zones describes which DNS Zone IDs
	// ExternalDNS should publish records to.
	//
	// Updating this field after creation
	// will cause all DNS records in the previous
	// zone(s) to be left behind.
	//
	// +kubebuilder:validation:MaxItems=10
	// +optional
	Zones []string `json:"zones,omitempty"`
}

// ExternalDNSDomain describes how sets of included
// or excluded domains are to be constructed.
type ExternalDNSDomain struct {
	ExternalDNSDomainUnion `json:",inline"`

	// FilterType marks the Name or Pattern field
	// as an included or excluded set of domains.
	//
	// In the event of contradicting domain options,
	// preference is given to excluded domains.
	//
	// This field accepts the following values:
	//
	//  "Include": Include the domain set specified
	//  by name or pattern.
	//
	//  "Exclude": Exclude the domain set specified
	//  by name or pattern.
	//
	// +kubebuilder:validation:Required
	// +required
	FilterType ExternalDNSFilterType `json:"filterType"`
}

// ExternalDNSDomainUnion describes optional fields of an External domain
// that should be captured.
// +union
type ExternalDNSDomainUnion struct {
	// MatchType specifies the type of match to be performed
	// by ExternalDNS when determining whether or not to publish DNS
	// records for a given source resource based on the resource's
	// requested hostname.
	//
	// This field accepts the following values:
	//
	//  "Exact": Explicitly match the full domain string
	//   specified via the Name field, including any subdomains
	//   of Name.
	//
	//  "Pattern": Match potential domains against
	//  the provided regular expression pattern string.
	//
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	// +required
	MatchType DomainMatchType `json:"matchType"`

	// Name is a string representing a single domain
	// value. Subdomains are included.
	//
	// e.g. my-app.my-cluster-domain.com
	// would also include
	// foo.my-app.my-cluster-domain.com
	//
	// +optional
	Name *string `json:"names,omitempty"`

	// Pattern is a regular expression used to
	// match a set of domains. Any provided
	// regular expressions should follow the syntax
	// used by the go regexp package (RE2).
	// See https://golang.org/pkg/regexp/ for more information.
	//
	// +optional
	Pattern *string `json:"pattern,omitempty"`
}

// +kubebuilder:validation:Enum=Exact;Pattern
type DomainMatchType string

const (
	DomainMatchTypeExact DomainMatchType = "Exact"
	DomainMatchTypeRegex DomainMatchType = "Pattern"
)

// +kubebuilder:validation:Enum=Include;Exclude
type ExternalDNSFilterType string

const (
	FilterTypeInclude ExternalDNSFilterType = "Include"
	FilterTypeExclude ExternalDNSFilterType = "Exclude"
)

// ExternalDNSProvider specifies configuration
// options for the desired ExternalDNS DNS provider.
// +union
type ExternalDNSProvider struct {
	// Type describes which DNS provider
	// ExternalDNS should publish records to.
	// The following DNS providers are supported:
	//
	//  * AWS (Route 53)
	//  * GCP (Google DNS)
	//  * Azure
	//  * BlueCat
	//  * Infoblox
	//
	// +kubebuilder:validation:Required
	// +unionDiscriminator
	// +required
	Type ExternalDNSProviderType `json:"type"`

	// AWS describes provider configuration options
	// specific to AWS (Route 53).
	//
	// +optional
	AWS *ExternalDNSAWSProviderOptions `json:"aws,omitempty"`

	// GCP describes provider configuration options
	// specific to GCP (Google DNS).
	//
	// +optional
	GCP *ExternalDNSGCPProviderOptions `json:"gcp,omitempty"`

	// Azure describes provider configuration options
	// specific to Azure DNS.
	//
	// +optional
	Azure *ExternalDNSAzureProviderOptions `json:"azure,omitempty"`

	// BlueCat describes provider configuration options
	// specific to BlueCat DNS.
	//
	// +optional
	BlueCat *ExternalDNSBlueCatProviderOptions `json:"blueCat,omitempty"`

	// Infoblox describes provider configuration options
	// specific to Infoblox DNS.
	//
	// +optional
	Infoblox *ExternalDNSInfobloxProviderOptions `json:"infoblox,omitempty"`
}

type ExternalDNSAWSProviderOptions struct {
	// Credentials is a reference to a secret containing
	// the following keys (with corresponding values):
	//
	// * aws_access_key_id
	// * aws_secret_access_key
	//
	// See
	// https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/aws.md
	// for more information.
	//
	// +kubebuilder:validation:Required
	// +required
	Credentials SecretReference `json:"credentials"`
	// TODO: Additionally support access for:
	// - kiam/kube2iam enabled clusters ("iam.amazonaws.com/role" POD's annotation to assume IAM role)
	// - EKS clusters ("eks.amazonaws.com/role-arn" ServiceAccount's annotation to assume IAM role)
}

type ExternalDNSGCPProviderOptions struct {
	// Project is the GCP project to use for
	// creating DNS records. This field is not necessary
	// when running on GCP as externalDNS auto-detects
	// the GCP project to use when running on GCP.
	//
	// +optional
	Project *string `json:"project,omitempty"`

	// Credentials is a reference to a secret containing
	// the necessary GCP service account keys.
	// The secret referenced by Credentials should
	// contain a key named `service_account.json`
	// presumably generated by the gcloud CLI.
	//
	// +kubebuilder:validation:Required
	// +required
	Credentials SecretReference `json:"credentials"`
}

type ExternalDNSAzureProviderOptions struct {
	// ConfigFile is a reference to a secret containing
	// the necessary information to use the Azure provider.
	// The secret referenced by ConfigFile should contain
	// a key named `azure.json` similar to the following:
	//
	// {
	//   "tenantId": "123",
	//   "subscriptionId": "456",
	//   "resourceGroup": "MyDnsResourceGroup",
	//   "aadClientId": "789",
	//   "aadClientSecret": "123"
	// }
	//
	// See
	// https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/azure.md
	// for more information on the necessary configuration key/values and how to obtain them.
	//
	// +kubebuilder:validation:Required
	// +required
	ConfigFile SecretReference `json:"configFile"`
}

type ExternalDNSBlueCatProviderOptions struct {
	// ConfigFile is a reference to a secret containing
	// the necessary information to use the BlueCat provider.
	// The secret referenced by ConfigFile should contain
	// an object named `bluecat.json` similar to the following:
	//
	// {
	//   "gatewayHost": "https://bluecatgw.example.com",
	//   "gatewayUsername": "user",
	//   "gatewayPassword": "pass",
	//   "dnsConfiguration": "Example",
	//   "dnsView": "Internal",
	//   "rootZone": "example.com",
	//   "skipTLSVerify": false
	// }
	//
	// See
	// https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/bluecat.md
	// for more information on the necessary configuration values and how to obtain them.
	//
	// +kubebuilder:validation:Required
	// +required
	ConfigFile SecretReference `json:"configFile"`
}

type ExternalDNSInfobloxProviderOptions struct {
	// Credentials is a reference to a secret containing
	// the following keys (with proper corresponding values):
	//
	// * EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME
	// * EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD
	//
	// See
	// https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/infoblox.md
	// for more information and configuration options.
	//
	// +kubebuilder:validation:Required
	// +required
	Credentials SecretReference `json:"credentials"`

	// GridHost is the IP of the Infoblox Grid host.
	//
	// kubebuilder:validation:Required
	// +kubebuilder:default:="127.0.0.1"
	// +required
	GridHost string `json:"gridHost"`

	// WAPIPort is the port for the Infoblox WAPI.
	//
	// kubebuilder:validation:Required
	// +kubebuilder:default:=443
	// +required
	WAPIPort int `json:"wapiPort"`

	// WAPIVersion is the version of the Infoblox WAPI.
	//
	// kubebuilder:validation:Required
	// +kubebuilder:default:="2.3.1"
	// +required
	WAPIVersion string `json:"wapiVersion"`
}

// SecretReference contains the information to let you locate the desired secret.
// Secret is expected to be in the operator namespace.
type SecretReference struct {
	// Name is the name of the secret.
	//
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`
}

// +kubebuilder:validation:Enum=AWS;GCP;Azure;BlueCat;Infoblox
type ExternalDNSProviderType string

const (
	ProviderTypeAWS      ExternalDNSProviderType = "AWS"
	ProviderTypeGCP      ExternalDNSProviderType = "GCP"
	ProviderTypeAzure    ExternalDNSProviderType = "Azure"
	ProviderTypeBlueCat  ExternalDNSProviderType = "BlueCat"
	ProviderTypeInfoblox ExternalDNSProviderType = "Infoblox"
	// More providers will ultimately be added in the future.
)

// ExternalDNSSource describes which Source resource
// the ExternalDNS should create DNS records for.
type ExternalDNSSource struct {
	ExternalDNSSourceUnion `json:",inline"`

	// HostnameAnnotationPolicy specifies whether or not ExternalDNS
	// should ignore the "external-dns.alpha.kubernetes.io/hostname"
	// annotation, which overrides DNS hostnames on a given source resource.
	//
	// The following values are accepted:
	//
	//  "Ignore": Ignore any hostname annotation overrides.
	//  "Allow": Allow all hostname annotation overrides.
	//
	// The default behavior of the ExternalDNS is "Ignore".
	//
	// Note that by setting a HostnameAnnotationPolicy of "Allow",
	// may grant privileged DNS permissions to under-privileged cluster
	// users.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=Ignore
	// +required
	HostnameAnnotationPolicy HostnameAnnotationPolicy `json:"hostnameAnnotation"`

	// FQDNTemplate sets a templated string that's used to generate DNS names
	// from sources that don't define a hostname themselves.
	// Multiple global FQDN templates are possible.
	//
	// Should not be empty when HostnameAnnotationPolicy is set to Ignore.
	//
	// Provided templates should follow the syntax defined for text/template Go package,
	// see https://pkg.go.dev/text/template.
	// Annotations inside the template correspond to the definition of the source resource object (e.g. Kubernetes service, OpenShift route).
	// Example: "{{.Name}}.example.com" would be expanded to "myservice.example.com" for service source
	//
	// +kubebuilder:validation:Optional
	// +optional
	FQDNTemplate []string `json:"fqdnTemplate,omitempty"`
}

// ExternalDNSSourceUnion describes optional fields for an ExternalDNS source that should
// be captured.
// +union
type ExternalDNSSourceUnion struct {
	// Type specifies an ExternalDNS source resource
	// to create DNS records for.
	//
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	// +required
	Type ExternalDNSSourceType `json:"type"`

	// AnnotationFilter describes an annotation filter
	// used to filter which source instance resources
	// ExternalDNS publishes records for.
	// The annotation filter uses label selector semantics
	// against source resource annotations.
	//
	// +optional
	AnnotationFilter map[string]string `json:"annotationFilter,omitempty"`

	// Namespace instructs ExternalDNS to only acknowledge
	// source resource instances in a specific namespace.
	//
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// Service describes source configuration options specific
	// to the service source resource.
	//
	// +optional
	Service *ExternalDNSServiceSourceOptions `json:"service,omitempty"`

	// CRD describes source configuration options specific
	// to the CRD source resource.
	// See
	// https://github.com/kubernetes-sigs/external-dns/blob/master/docs/contributing/crd-source.md
	// for more information about the requirements for ExternalDNS Source CRD requirements.
	//
	// +optional
	CRD *ExternalDNSCRDSourceOptions `json:"crd,omitempty"`

	// OpenShiftRoute source currently has no unique configuration options.
}

// +kubebuilder:validation:Enum=OpenShiftRoute;Service;CRD
type ExternalDNSSourceType string

const (
	SourceTypeRoute   ExternalDNSSourceType = "OpenShiftRoute"
	SourceTypeService ExternalDNSSourceType = "Service"
	SourceTypeCRD     ExternalDNSSourceType = "CRD"
)

// +kubebuilder:validation:Enum=Ignore;Allow
type HostnameAnnotationPolicy string

const (
	HostnameAnnotationPolicyIgnore HostnameAnnotationPolicy = "Ignore"
	HostnameAnnotationPolicyAllow  HostnameAnnotationPolicy = "Allow"
)

// ExternalDNSServiceSourceOptions describes options
// specific to the ExternalDNS service source.
type ExternalDNSServiceSourceOptions struct {
	// ServiceType determines what types of Service resources
	// are watched by ExternalDNS. The following types are
	// available options:
	//
	//  "NodePort"
	//  "ExternalName"
	//  "LoadBalancer"
	//  "ClusterIP"
	//
	// One or more Service types can be specified, if desired.
	//
	// Note that using the "ClusterIP" service type will enable
	// the ExternalDNS "--publish-internal-services" flag,
	// which allows ExternalDNS to publish DNS records
	// for ClusterIP services.
	//
	// If no service types are provided, ExternalDNS will be
	// configured to create DNS records for LoadBalancer services
	// only by default.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:default:={"LoadBalancer"}
	// +kubebuilder:validation:MinItems=1
	// +required
	ServiceType []corev1.ServiceType `json:"serviceType,omitempty"`
}

// ExternalDNSCRDSourceOptions describes options for configuring
// the ExternalDNS CRD source. The ExternalDNS CRD Source implementation
// expects CRD resources to have specific fields, including a DNSName field. See
// https://github.com/kubernetes-sigs/external-dns/blob/master/docs/contributing/crd-source.md
// for more information.
//
// A configured CRD source would grant precise control to external DNS resources
// to any user who can create/update/delete the given CRD.
type ExternalDNSCRDSourceOptions struct {
	// Kind is the kind of the CRD
	// source resource type to be
	// consumed by ExternalDNS.
	//
	// e.g. "DNSEndpoint"
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +required
	Kind string `json:"kind"`

	// Version is the API version
	// of the given resource kind for
	// ExternalDNS to use.
	//
	// e.g. "externaldns.k8s.io/v1alpha1"
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +required
	Version string `json:"version"`

	// LabelFilter specifies a label filter
	// to be used to filter CRD resource instances.
	// Only one label filter can be specified on
	// an ExternalDNS instance.
	//
	// +optional
	LabelFilter *metav1.LabelSelector `json:"labelFilter,omitempty"`
}

// ExternalDNSStatus defines the observed state of ExternalDNS.
type ExternalDNSStatus struct {
	// Conditions is a list of operator-specific conditions
	// and their status.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Zones is the configured zones in use by ExternalDNS.
	Zones []string `json:"zones,omitempty"`
}

var (
	// Available indicates that the ExternalDNS is available.
	ExternalDNSAvailableConditionType = "Available"

	// AuthenticationFailed indicates that there were issues starting
	// ExternalDNS pods related to the given provider credentials.
	ExternalDNSProviderAuthFailedReasonType = "AuthenticationFailed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//
// ExternalDNSList contains a list of ExternalDNSes.
type ExternalDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalDNS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalDNS{}, &ExternalDNSList{})
}
