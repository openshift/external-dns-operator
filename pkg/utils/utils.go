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

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

// ManagedCredentialsProvider returns true if the credentials of the ExternalDNS provider can be managed by the platform
func ManagedCredentialsProvider(e *operatorv1alpha1.ExternalDNS) bool {
	switch e.Spec.Provider.Type {
	case operatorv1alpha1.ProviderTypeAWS, operatorv1alpha1.ProviderTypeGCP, operatorv1alpha1.ProviderTypeAzure:
		return true
	}
	return false
}

// EnvProxySupportedProvider returns true if the ExternalDNS provider supports the proxy settings via environment variables HTTP(S)_PROXY, NO_PROXY
func EnvProxySupportedProvider(e *operatorv1alpha1.ExternalDNS) bool {
	switch e.Spec.Provider.Type {
	case operatorv1alpha1.ProviderTypeAWS, operatorv1alpha1.ProviderTypeAzure, operatorv1alpha1.ProviderTypeGCP, operatorv1alpha1.ProviderTypeInfoblox:
		return true
	}
	return false
}

func MustParseLabelSelector(input string) *metav1.LabelSelector {
	selector, err := metav1.ParseToLabelSelector(input)
	if err != nil {
		panic(err)
	}
	return selector
}
