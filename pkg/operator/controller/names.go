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
	"hash"
	"hash/fnv"
	"strings"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
	operatorconfig "github.com/openshift/external-dns-operator/pkg/operator/config"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	// ExternalDNSBaseName is the base name for any ExternalDNS resource.
	ExternalDNSBaseName                = "external-dns"
	CredentialsRequestNamespace        = "openshift-cloud-credential-operator"
	ControllerName                     = "external_dns_controller"
	SecretFromCloudCredentialsOperator = "externaldns-cloud-credentials"
	ServiceAccountName                 = "external-dns-operator"
)

func ExternalDNSCredentialsRequestName(externalDNS *operatorv1alpha1.ExternalDNS) types.NamespacedName {
	return types.NamespacedName{
		// CCO recommendation for the core operators (for which it was primarily designed for) is to use CCO namespace:
		// https://github.com/openshift/cloud-credential-operator#for-openshift-second-level-operators
		// At the same time CCO watches for all the namespaces and the credentials requests can be created anywhere.
		// Which allows OLM operators to create credentials requests in their installation namespaces.
		// However, there are plans to restrict the credentials request watch to CCO namespace only:
		// https://github.com/openshift/cloud-credential-operator/blob/611939bce7694d5b1128cb3e569d794f8cba06a1/pkg/operator/credentialsrequest/credentialsrequest_controller.go#L127-L128
		// So the recommendation from the CCO engineering was to stick to CCO namespace.
		Namespace: CredentialsRequestNamespace,
		Name:      "externaldns-credentials-request-" + strings.ToLower(string(externalDNS.Spec.Provider.Type)),
	}
}

// ExternalDNSResourceName returns the name for the resources unique for the given ExternalDNS instance.
func ExternalDNSResourceName(externalDNS *operatorv1alpha1.ExternalDNS) string {
	return ExternalDNSBaseName + "-" + externalDNS.Name
}

// ExternalDNSGlobalResourceName returns the name for the resources shared among ExternalDNS instances.
func ExternalDNSGlobalResourceName() string {
	return ExternalDNSBaseName
}

// ExternalDNSContainerName returns the container name unique for the given DNS zone.
func ExternalDNSContainerName(zone string) string {
	return ExternalDNSBaseName + "-" + hashString(zone)
}

// ExternalDNSDestCredentialsSecretName returns the namespaced name of the destination (operand) credentials secret
func ExternalDNSDestCredentialsSecretName(operandNamespace, extdnsName string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: operandNamespace,
		Name:      ExternalDNSBaseName + "-credentials-" + extdnsName,
	}
}

// ExternalDNSDestTrustedCAConfigMapName returns the namespaced name of the destination (operand) trusted CA configmap
func ExternalDNSDestTrustedCAConfigMapName(operandNamespace string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: operandNamespace,
		Name:      ExternalDNSBaseName + "-trusted-ca",
	}
}

func ExternalDNSCredentialsSourceNamespace(cfg *operatorconfig.Config) string {
	// TODO: use openshift-config namespace for OpenShift?
	return cfg.OperatorNamespace
}

// ExternalDNSCredentialsSecretNameFromProvider returns the name of the credentials secret retrieved from externalDNS resource
func ExternalDNSCredentialsSecretNameFromProvider(externalDNS *operatorv1alpha1.ExternalDNS) string {
	switch externalDNS.Spec.Provider.Type {
	case operatorv1alpha1.ProviderTypeAWS:
		if externalDNS.Spec.Provider.AWS != nil {
			return externalDNS.Spec.Provider.AWS.Credentials.Name
		}
	case operatorv1alpha1.ProviderTypeAzure:
		if externalDNS.Spec.Provider.Azure != nil {
			return externalDNS.Spec.Provider.Azure.ConfigFile.Name
		}
	case operatorv1alpha1.ProviderTypeGCP:
		if externalDNS.Spec.Provider.GCP != nil {
			return externalDNS.Spec.Provider.GCP.Credentials.Name
		}
	case operatorv1alpha1.ProviderTypeBlueCat:
		if externalDNS.Spec.Provider.BlueCat != nil {
			return externalDNS.Spec.Provider.BlueCat.ConfigFile.Name
		}
	case operatorv1alpha1.ProviderTypeInfoblox:
		if externalDNS.Spec.Provider.Infoblox != nil {
			return externalDNS.Spec.Provider.Infoblox.Credentials.Name
		}
	}
	return ""
}

func hashString(str string) string {
	hasher := getHasher()
	hasher.Write([]byte(str))
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum(nil)))
}

func getHasher() hash.Hash {
	return fnv.New32a()
}
