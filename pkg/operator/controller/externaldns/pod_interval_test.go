package externaldnscontroller

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openshift/external-dns-operator/api/v1beta1"
)

func TestIntervalArg(t *testing.T) {
	for _, tc := range []struct {
		name             string
		interval         *metav1.Duration
		expectInterval   bool
		expectedInterval string
	}{
		{
			name:           "interval omitted",
			interval:       nil,
			expectInterval: false,
		},
		{
			name:             "interval set",
			interval:         &metav1.Duration{Duration: 5 * time.Minute},
			expectInterval:   true,
			expectedInterval: "--interval=5m0s",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &externalDNSContainerBuilder{
				externalDNS: &v1beta1.ExternalDNS{
					Spec: v1beta1.ExternalDNSSpec{
						Interval: tc.interval,
						Source: v1beta1.ExternalDNSSource{
							ExternalDNSSourceUnion: v1beta1.ExternalDNSSourceUnion{
								Type: v1beta1.SourceTypeRoute,
								OpenShiftRoute: &v1beta1.ExternalDNSOpenShiftRouteOptions{
									RouterName: "default",
								},
							},
							HostnameAnnotationPolicy: v1beta1.HostnameAnnotationPolicyIgnore,
						},
					},
				},
				provider: externalDNSProviderTypeInfoblox,
				source:   "openshift-route",
			}

			container := b.defaultContainer("external-dns")
			if err := b.fillProviderAgnosticFields(0, "", container); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var intervalArg string
			for _, arg := range container.Args {
				if strings.HasPrefix(arg, "--interval=") {
					intervalArg = arg
				}
			}

			if tc.expectInterval {
				if intervalArg != tc.expectedInterval {
					t.Fatalf("expected %q, got %q from %v", tc.expectedInterval, intervalArg, container.Args)
				}
				return
			}

			if intervalArg != "" {
				t.Fatalf("unexpected interval arg %q in %v", intervalArg, container.Args)
			}
		})
	}
}

func TestInfobloxMaxResultsArg(t *testing.T) {
	for _, tc := range []struct {
		name         string
		maxResults   *int
		expectedArgs []string
	}{
		{
			name:         "max results omitted",
			maxResults:   nil,
			expectedArgs: nil,
		},
		{
			name:         "max results set",
			maxResults:   ptr.To[int](2000),
			expectedArgs: []string{"--infoblox-max-results=2000"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &externalDNSContainerBuilder{
				externalDNS: &v1beta1.ExternalDNS{
					Spec: v1beta1.ExternalDNSSpec{
						Provider: v1beta1.ExternalDNSProvider{
							Type: v1beta1.ProviderTypeInfoblox,
							Infoblox: &v1beta1.ExternalDNSInfobloxProviderOptions{
								GridHost:    "gridhost.example.com",
								WAPIPort:    443,
								WAPIVersion: "2.12.2",
								MaxResults:  tc.maxResults,
							},
						},
					},
				},
				secretName: "infoblox-credentials",
			}

			container := b.defaultContainer("external-dns")
			b.fillInfobloxFields(container)

			var got []string
			for _, arg := range container.Args {
				if strings.HasPrefix(arg, "--infoblox-max-results=") {
					got = append(got, arg)
				}
			}

			if tc.expectedArgs == nil {
				if len(got) != 0 {
					t.Fatalf("unexpected max results args %v", got)
				}
				return
			}

			if len(got) != 1 || got[0] != tc.expectedArgs[0] {
				t.Fatalf("expected max results args %v, got %v from %v", tc.expectedArgs, got, container.Args)
			}
		})
	}
}
