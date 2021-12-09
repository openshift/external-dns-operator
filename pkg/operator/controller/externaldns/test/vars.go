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

package test

import (
	configv1 "github.com/openshift/api/config/v1"
	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	operatorv1alpha1 "github.com/openshift/external-dns-operator/api/v1alpha1"
)

const (
	Name                = "test"
	OperandNamespace    = "external-dns"
	OperandName         = "external-dns-test"
	OperandImage        = "quay.io/test/external-dns:latest"
	PublicZone          = "my-dns-public-zone"
	PrivateZone         = "my-dns-private-zone"
	AzurePrivateDNSZone = "/subscriptions/xxxx/resourceGroups/test-az-2f9kj-rg/providers/Microsoft.Network/privateDnsZones/test-az.example.com"
)

var (
	TrueVar     = true
	ExternalDNS = &operatorv1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: Name,
		},
	}
	Scheme = runtime.NewScheme()
)

func init() {
	if err := clientgoscheme.AddToScheme(Scheme); err != nil {
		panic(err)
	}
	if err := operatorv1alpha1.AddToScheme(Scheme); err != nil {
		panic(err)
	}
	if err := cco.AddToScheme(Scheme); err != nil {
		panic(err)
	}
	if err := configv1.AddToScheme(Scheme); err != nil {
		panic(err)
	}
}
