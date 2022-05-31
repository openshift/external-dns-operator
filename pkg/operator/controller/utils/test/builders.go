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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
)

type ExternalDNSBuilder struct {
	extDNS *operatorv1beta1.ExternalDNS
}

func NewExternalDNS(name string) *ExternalDNSBuilder {
	return &ExternalDNSBuilder{
		extDNS: &operatorv1beta1.ExternalDNS{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (b *ExternalDNSBuilder) WithProviderType(ptype operatorv1beta1.ExternalDNSProviderType) *ExternalDNSBuilder {
	b.extDNS.Spec.Provider = operatorv1beta1.ExternalDNSProvider{Type: ptype}
	return b
}

func (b *ExternalDNSBuilder) WithAWS() *ExternalDNSBuilder {
	return b.WithProviderType(operatorv1beta1.ProviderTypeAWS)
}

func (b *ExternalDNSBuilder) WithAzure() *ExternalDNSBuilder {
	return b.WithProviderType(operatorv1beta1.ProviderTypeAzure)
}

func (b *ExternalDNSBuilder) WithGCP() *ExternalDNSBuilder {
	return b.WithProviderType(operatorv1beta1.ProviderTypeGCP)
}

func (b *ExternalDNSBuilder) WithSourceType(src operatorv1beta1.ExternalDNSSourceType) *ExternalDNSBuilder {
	b.extDNS.Spec.Source = operatorv1beta1.ExternalDNSSource{
		ExternalDNSSourceUnion: operatorv1beta1.ExternalDNSSourceUnion{
			Type: src,
		},
	}
	return b
}

func (b *ExternalDNSBuilder) WithRouteSource() *ExternalDNSBuilder {
	return b.WithSourceType(operatorv1beta1.SourceTypeRoute)
}

func (b *ExternalDNSBuilder) WithServiceSource() *ExternalDNSBuilder {
	return b.WithSourceType(operatorv1beta1.SourceTypeService)
}

func (b *ExternalDNSBuilder) WithZones(ids ...string) *ExternalDNSBuilder {
	b.extDNS.Spec.Zones = ids
	return b
}

func (b *ExternalDNSBuilder) Build() *operatorv1beta1.ExternalDNS {
	return b.extDNS
}
