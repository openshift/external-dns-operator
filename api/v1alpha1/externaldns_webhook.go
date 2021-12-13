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
	"errors"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/runtime"

	utilErrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// webhookLog is for logging in this package.
var webhookLog = logf.Log.WithName("validating-webhook")

var isOpenShift bool

func (r *ExternalDNS) SetupWebhookWithManager(mgr ctrl.Manager, openshift bool) error {
	isOpenShift = openshift
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
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
	if r.Spec.Source.Type == SourceTypeRoute {
		// dummy fqdnTemplate is used for Route source
		return nil
	}

	if r.Spec.Source.HostnameAnnotationPolicy == HostnameAnnotationPolicyIgnore && len(r.Spec.Source.FQDNTemplate) == 0 {
		return errors.New(`"fqdnTemplate" must be specified when "hostnameAnnotation" is "Ignore"`)
	}
	return nil
}

func (r *ExternalDNS) validateProviderCredentials() error {
	if isOpenShift && (r.Spec.Provider.Type == ProviderTypeAWS || r.Spec.Provider.Type == ProviderTypeGCP || r.Spec.Provider.Type == ProviderTypeAzure) {
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
