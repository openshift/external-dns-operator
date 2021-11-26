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
	"context"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorutil "github.com/openshift/cluster-ingress-operator/pkg/util"

	awsutil "github.com/openshift/cluster-ingress-operator/pkg/util/aws"

	corev1 "k8s.io/api/core/v1"

	"os"

	configv1 "github.com/openshift/api/config/v1"

	kberror "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"

	"regexp"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"

	utilErrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var kClient client.Client

func initKubeClient() error {
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kube config: %w", err)
	}

	kClient, err = client.New(kubeConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create kube client: %w", err)
	}
	return nil
}

const (
	kind    = "OpenShiftAPIServer"
	group   = "operator.openshift.io"
	version = "v1"
	// kubeCloudConfigName is the name of the kube cloud config ConfigMap
	kubeCloudConfigName = "kube-cloud-config"
	// cloudCABundleKey is the key in the kube cloud config ConfigMap where the custom CA bundle is located
	cloudCABundleKey                      = "ca-bundle.pem"
	GlobalMachineSpecifiedConfigNamespace = "openshift-config-managed"
)

// webhookLog is for logging in this package.
var webhookLog = logf.Log.WithName("validating-webhook")
var IsOpenShift bool
var PlatformStatus *configv1.PlatformStatus
var PlatformSpec *configv1.InfrastructureSpec
var DNSClusterSpec *configv1.DNSSpec
var creds *corev1.Secret
var zones []configv1.DNSZone
var ZoneIDList []string

func (r *ExternalDNS) SetupWebhookWithManager(mgr ctrl.Manager) error {

	if err := initKubeClient(); err != nil {
		fmt.Printf("Failed to init kube client: %v\n", err)
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	IsOpenShift = IsOCP(kubeClient)

	if IsOpenShift {

		var err error
		infraConfig := &configv1.Infrastructure{}
		if err = mgr.GetClient().Get(context.TODO(), types.NamespacedName{Name: "cluster"}, infraConfig); err != nil {
			return fmt.Errorf("failed to get infrastructure 'config': %v", err)
		}
		PlatformStatus = infraConfig.Status.PlatformStatus
		PlatformSpec = &infraConfig.Spec

		dnsConfig := &configv1.DNS{}
		if err = mgr.GetClient().Get(context.TODO(), types.NamespacedName{Name: "cluster"}, dnsConfig); err != nil {
			return fmt.Errorf("failed to get infrastructure 'config': %v", err)
		}
		DNSClusterSpec = &dnsConfig.Spec

		if dnsConfig.Spec.PrivateZone == nil && dnsConfig.Spec.PublicZone == nil {
			return fmt.Errorf("using fake DNS provider because no public or private zone is defined in the cluster DNS configuration")
		}

		if dnsConfig.Spec.PrivateZone != nil {
			zones = append(zones, *dnsConfig.Spec.PrivateZone)
		}
		if dnsConfig.Spec.PublicZone != nil {
			zones = append(zones, *dnsConfig.Spec.PublicZone)
		}

		if PlatformSpec.PlatformSpec.Type == "AWS" {

			creds, err = rootAWSCredentials(kClient)
			if err != nil {
				return fmt.Errorf("failed to get AWS credentials: %w", err)
			}

			dnsProvider, err := createDNSProvider(dnsConfig, PlatformStatus, &infraConfig.Status, creds)
			if err != nil {
				return fmt.Errorf("failed to create DNS provider: %v", err)
			}

			for i := range zones {
				zone := zones[i]
				zoneID, err := dnsProvider.getZoneID(zone)
				if err != nil {
					return fmt.Errorf("failed to find hosted zone : %v", err)
				}
				ZoneIDList = append(ZoneIDList, zoneID)
			}

		} else if PlatformSpec.PlatformSpec.Type == "GCP" {
			for _, zone := range zones {
				ZoneIDList = append(ZoneIDList, zone.ID)
			}
		} else if PlatformSpec.PlatformSpec.Type == "Azure" {
			//TODO
			fmt.Printf("Work in Progress")
		}

	}

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

///// aws  ////////
func rootAWSCredentials(kClient client.Client) (secret *corev1.Secret, err error) {
	secret = &corev1.Secret{}
	secretName := types.NamespacedName{
		Name:      "aws-creds",
		Namespace: "kube-system",
	}
	if err := kClient.Get(context.TODO(), secretName, secret); err != nil {
		return nil, fmt.Errorf("failed to get aws secret from kube-system")
	}
	return secret, nil
}

//+kubebuilder:webhook:path=/validate-externaldns-olm-openshift-io-v1alpha1-externaldns,mutating=false,failurePolicy=fail,sideEffects=None,groups=externaldns.olm.openshift.io,resources=externaldnses,verbs=create;update,versions=v1alpha1,name=vexternaldns.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &ExternalDNS{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ExternalDNS) ValidateCreate() error {
	webhookLog.Info("validate create", "name", r.Name)
	return r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ExternalDNS) ValidateUpdate(_ runtime.Object) error {
	webhookLog.Info("validate update", "name", r.Name)
	return r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ExternalDNS) ValidateDelete() error {
	webhookLog.Info("validate delete", "name", r.Name)
	return nil
}

func (r *ExternalDNS) validate() error {
	var errs []error
	if err := r.validateFilters(); err != nil {
		errs = append(errs, err)
	}
	if err := r.validateHostnameAnnotationPolicy(); err != nil {
		errs = append(errs, err)
	}

	if err := r.validateProviderCredentials(); err != nil {
		errs = append(errs, err)
	}
	return utilErrors.NewAggregate(errs)
}

func (r *ExternalDNS) validateFilters() error {
	for _, f := range r.Spec.Domains {
		switch f.MatchType {
		case DomainMatchTypeExact:
			if f.Name == nil || *f.Name == "" {
				return errors.New(`"Name" cannot be empty when match type is "Exact"`)
			}
		case DomainMatchTypeRegex:
			if f.Pattern == nil || *f.Pattern == "" {
				return errors.New(`"Pattern" cannot be empty when match type is "Pattern"`)
			}
			_, err := regexp.Compile(*f.Pattern)
			if err != nil {
				return fmt.Errorf(`invalid pattern for "Pattern" match type: %w`, err)
			}
		default:
			return fmt.Errorf("unsupported match type %q", f.MatchType)
		}
	}

	return nil
}

func (r *ExternalDNS) validateHostnameAnnotationPolicy() error {
	if r.Spec.Source.HostnameAnnotationPolicy == HostnameAnnotationPolicyIgnore && len(r.Spec.Source.FQDNTemplate) == 0 {
		return errors.New(`"fqdnTemplate" must be specified when "hostnameAnnotation" is "Ignore"`)
	}
	return nil
}

func (r *ExternalDNS) validateProviderCredentials() error {
	if IsOpenShift && (r.Spec.Provider.Type == ProviderTypeAWS || r.Spec.Provider.Type == ProviderTypeGCP || r.Spec.Provider.Type == ProviderTypeAzure) {
		return nil
	}
	provider := r.Spec.Provider
	switch provider.Type {
	case ProviderTypeAWS:
		if provider.AWS == nil || provider.AWS.Credentials.Name == "" {
			return errors.New("credentials secret must be specified when provider type is AWS")
		}
	case ProviderTypeAzure:
		if provider.Azure == nil || provider.Azure.ConfigFile.Name == "" {
			return errors.New("config file name must be specified when provider type is Azure")
		}
	case ProviderTypeGCP:
		if provider.GCP == nil || provider.GCP.Credentials.Name == "" {
			return errors.New("credentials secret must be specified when provider type is GCP")
		}
	case ProviderTypeBlueCat:
		if provider.BlueCat == nil || provider.BlueCat.ConfigFile.Name == "" {
			return errors.New("config file name must be specified when provider type is BlueCat")
		}
	case ProviderTypeInfoblox:
		if provider.Infoblox == nil || provider.Infoblox.WAPIVersion == "" || provider.Infoblox.WAPIPort == 0 || provider.Infoblox.GridHost == "" || provider.Infoblox.Credentials.Name == "" {
			return errors.New(`"WAPIVersion", "WAPIPort", "GridHost" and credentials file must be specified when provider is Infoblox`)
		}
	}
	return nil
}

// Returns true if platform is OCP
func IsOCP(kubeClient discovery.DiscoveryInterface) bool {
	// Since, CRD for OpenShift API Server was introduced in OCP v4.x we can verify if the current cluster is on OCP v4.x by
	// ensuring that resource exists against Group(operator.openshift.io), Version(v1) and Kind(OpenShiftAPIServer)
	// In case it doesn't exist we assume that external dns is running on non OCP 4.x environment
	resources, err := kubeClient.ServerResourcesForGroupVersion(group + "/" + version)
	if err != nil {
		return false
	}

	for _, apiResource := range resources.APIResources {
		if apiResource.Kind == kind {
			return true
		}
	}
	return false
}

// createDNSProvider creates a DNS manager compatible with the given cluster
// configuration.
func createDNSProvider(dnsConfig *configv1.DNS, platformStatus *configv1.PlatformStatus, infraStatus *configv1.InfrastructureStatus, creds *corev1.Secret) (Provider, error) {
	// If no DNS configuration is provided, don't try to set up provider clients.
	// TODO: the provider configuration can be refactored into the provider
	// implementations themselves, so this part of the code won't need to
	// know anything about the provider setup beyond picking the right implementation.
	// Then, it would be safe to always use the appropriate provider for the platform
	// and let the provider surface configuration errors if DNS records are actually
	// created to exercise the provider.
	if dnsConfig.Spec.PrivateZone == nil && dnsConfig.Spec.PublicZone == nil {
		log.Info("using fake DNS provider because no public or private zone is defined in the cluster DNS configuration")
		return Provider{}, nil
	}

	switch platformStatus.Type {
	case configv1.AWSPlatformType:
		cfg := Config{
			Region: platformStatus.AWS.Region,
		}

		sharedCredsFile, err := awsutil.SharedCredentialsFileFromSecret(creds)
		if err != nil {
			return Provider{}, fmt.Errorf("failed to create shared credentials file from Secret: %v", err)
		}
		// since at the end of this function the aws dns provider will be initialized with aws clients, the AWS SDK no
		// longer needs access to the file and therefore it can be removed.
		defer os.Remove(sharedCredsFile)
		cfg.SharedCredentialFile = sharedCredsFile

		if len(platformStatus.AWS.ServiceEndpoints) > 0 {
			cfg.ServiceEndpoints = []ServiceEndpoint{}
			route53Found := false
			elbFound := false
			tagFound := false
			for _, ep := range platformStatus.AWS.ServiceEndpoints {
				switch {
				case route53Found && elbFound && tagFound:
					break
				case ep.Name == Route53Service:
					route53Found = true
					scheme, err := operatorutil.URI(ep.URL)
					if err != nil {
						return Provider{}, fmt.Errorf("failed to validate URI %s: %v", ep.URL, err)
					}
					if scheme != operatorutil.SchemeHTTPS {
						return Provider{}, fmt.Errorf("invalid scheme for URI %s; must be %s", ep.URL, operatorutil.SchemeHTTPS)
					}
					cfg.ServiceEndpoints = append(cfg.ServiceEndpoints, ServiceEndpoint{Name: ep.Name, URL: ep.URL})
				case ep.Name == ELBService:
					elbFound = true
					scheme, err := operatorutil.URI(ep.URL)
					if err != nil {
						return Provider{}, fmt.Errorf("failed to validate URI %s: %v", ep.URL, err)
					}
					if scheme != operatorutil.SchemeHTTPS {
						return Provider{}, fmt.Errorf("invalid scheme for URI %s; must be %s", ep.URL, operatorutil.SchemeHTTPS)
					}
					cfg.ServiceEndpoints = append(cfg.ServiceEndpoints, ServiceEndpoint{Name: ep.Name, URL: ep.URL})
				case ep.Name == TaggingService:
					tagFound = true
					scheme, err := operatorutil.URI(ep.URL)
					if err != nil {
						return Provider{}, fmt.Errorf("failed to validate URI %s: %v", ep.URL, err)
					}
					if scheme != operatorutil.SchemeHTTPS {
						return Provider{}, fmt.Errorf("invalid scheme for URI %s; must be %s", ep.URL, operatorutil.SchemeHTTPS)
					}
					cfg.ServiceEndpoints = append(cfg.ServiceEndpoints, ServiceEndpoint{Name: ep.Name, URL: ep.URL})
				}
			}
		}

		cfg.CustomCABundle, err = customCABundle()
		if err != nil {
			return Provider{}, fmt.Errorf("failed to get the custom CA bundle: %w", err)
		}

		provider, err := NewProvider(cfg)
		if err != nil {
			return Provider{}, fmt.Errorf("failed to create AWS DNS manager: %v", err)
		}

		return *provider, nil
	}
	return Provider{}, nil
}

// customCABundle will get the custom CA bundle, if present, configured in the kube cloud config.
func customCABundle() (string, error) {
	cm := &corev1.ConfigMap{}
	switch err := kClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: GlobalMachineSpecifiedConfigNamespace, Name: kubeCloudConfigName},
		cm,
	); {
	case kberror.IsNotFound(err):
		// no cloud config ConfigMap, so no custom CA bundle
		return "", nil
	case err != nil:
		return "", fmt.Errorf("failed to get kube-cloud-config ConfigMap: %w", err)
	}
	caBundle, ok := cm.Data[cloudCABundleKey]
	if !ok {
		// no "ca-bundle.pem" key in the ConfigMap, so no custom CA bundle
		return "", nil
	}
	return caBundle, nil
}
