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
