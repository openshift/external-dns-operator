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
	"testing"
)

func TestExternalDNSContainerName(t *testing.T) {
	testCases := []struct {
		name   string
		zone   string
		expect string
	}{
		{
			name:   "Nominal alphanumeric",
			zone:   "abc123def234",
			expect: "external-dns-n7dh5d4h677h54fq",
		},
		{
			name:   "Nominal AWS",
			zone:   "Z0323552X0970SB2UHBB",
			expect: "external-dns-n678h689h67dh69q",
		},
		{
			name:   "Very long",
			zone:   "Z0323552X0970SB2UHBBZ0323552X0970SB2UHBBZ0323552X0970SB2UHBBZ0323552X0970SB2UHBBZ0323552X0970SB2UHBBZ0323552X0970SB2UHBBZ0323552X0970SB2UHBB",
			expect: "external-dns-n655hfbh654h557q",
		},
		{
			name:   "No Zone",
			zone:   "",
			expect: "external-dns-n56fh6dh59ch5fcq",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExternalDNSContainerName(tc.zone)
			if got != tc.expect {
				t.Errorf("expect %s container name, got %s", tc.expect, got)
			}
		})
	}
}
