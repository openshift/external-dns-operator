/*
Copyright 2023.

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
	"testing"

	operatorv1beta1 "github.com/openshift/external-dns-operator/api/v1beta1"
)

func TestNeedsCredentialSecret(t *testing.T) {
	roleArn := "arn:aws:iam::123456789012:role/my-role"

	type args struct {
		e *operatorv1beta1.ExternalDNS
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "ensure aws with assume role config does not need a credential secret",
			args: args{
				e: &operatorv1beta1.ExternalDNS{
					Spec: operatorv1beta1.ExternalDNSSpec{
						Provider: operatorv1beta1.ExternalDNSProvider{
							AWS: &operatorv1beta1.ExternalDNSAWSProviderOptions{
								AssumeRole: &operatorv1beta1.ExternalDNSAWSAssumeRoleOptions{
									ID:       &roleArn,
									Strategy: operatorv1beta1.ExternalDNSAWSAssumeRoleOptionIRSA,
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "ensure aws with assume role config does not need a credential secret",
			args: args{
				e: &operatorv1beta1.ExternalDNS{
					Spec: operatorv1beta1.ExternalDNSSpec{
						Provider: operatorv1beta1.ExternalDNSProvider{
							AWS: &operatorv1beta1.ExternalDNSAWSProviderOptions{
								Credentials: operatorv1beta1.SecretReference{
									Name: "test",
								},
							},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsCredentialSecret(tt.args.e); got != tt.want {
				t.Errorf("NeedsCredentialSecret() = %v, want %v", got, tt.want)
			}
		})
	}
}
