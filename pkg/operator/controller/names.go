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

	"k8s.io/apimachinery/pkg/util/rand"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	// ExternalDNSBaseName is the base name for any ExternalDNS resource.
	ExternalDNSBaseName = "external-dns"
)

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

func hashString(str string) string {
	hasher := getHasher()
	hasher.Write([]byte(str))
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum(nil)))
}

func getHasher() hash.Hash {
	return fnv.New32a()
}
