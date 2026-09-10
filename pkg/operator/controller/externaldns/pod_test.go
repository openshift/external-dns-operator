package externaldnscontroller

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/openshift/external-dns-operator/api/v1beta1"
)

func TestDomainFilters(t *testing.T) {
	for _, tc := range []struct {
		name          string
		domainInput   []v1beta1.ExternalDNSDomain
		expectErr     bool
		expectedArgs  []string
		expectedError string
	}{
		{
			name: "only one domain included",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("abc.com"),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{"--domain-filter=abc.com"},
		},
		{
			name: "multiple domains included",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("abc.com"),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("def.com"),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("ghi.com"),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{"--domain-filter=abc.com", "--domain-filter=def.com", "--domain-filter=ghi.com"},
		},
		{
			name: "single regex include filter",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.abc\.com`),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{`--regex-domain-filter=(.*)\.abc\.com`},
		},
		{
			name: "invalid regex include filter",

			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*]\.abc\.com`),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
			},
			expectErr:     true,
			expectedError: `input pattern (.*]\.abc\.com is invalid`,
		},
		{
			name: "multiple regex include filter",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.abc\.com`),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.def\.com`),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{`--regex-domain-filter=((.*)\.abc\.com)|((.*)\.def\.com)`},
		},
		{
			name: "only one domain excluded",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("abc.com"),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{"--exclude-domains=abc.com"},
		},
		{
			name: "multiple domains excluded",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("abc.com"),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("def.com"),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("ghi.com"),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{"--exclude-domains=abc.com", "--exclude-domains=def.com", "--exclude-domains=ghi.com"},
		},
		{
			name: "single regex exclude filter",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.abc\.com`),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{`--regex-domain-exclusion=(.*)\.abc\.com`},
		},
		{
			name: "invalid regex exclude filter",

			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*]\.abc\.com`),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
			},
			expectErr:     true,
			expectedError: `exclude pattern (.*]\.abc\.com is invalid`,
		},
		{
			name: "multiple regex exclude filter",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.abc\.com`),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.def\.com`),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{
				`--regex-domain-exclusion=((.*)\.abc\.com)|((.*)\.def\.com)`,
			},
		},
		{
			name: "mixed domain filters",
			domainInput: []v1beta1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("abc.com"),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeExact,
						Name:      ptr.To[string]("def.com"),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.ghi\.com`),
					},
					FilterType: v1beta1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1beta1.ExternalDNSDomainUnion{
						MatchType: v1beta1.DomainMatchTypeRegex,
						Pattern:   ptr.To[string](`(.*)\.pqr\.com`),
					},
					FilterType: v1beta1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{"--domain-filter=abc.com", "--exclude-domains=def.com", `--regex-domain-filter=(.*)\.ghi\.com`, `--regex-domain-exclusion=(.*)\.pqr\.com`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &externalDNSContainerBuilder{
				externalDNS: &v1beta1.ExternalDNS{
					Spec: v1beta1.ExternalDNSSpec{
						Domains: tc.domainInput,
					},
				},
			}
			args, err := b.domainFilters()
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tc.expectErr && err == nil {
				t.Errorf("expected error, got nil")
				return
			}
			if !tc.expectErr {
				if !reflect.DeepEqual(args, tc.expectedArgs) {
					t.Errorf("expected arguments %v, got %v", tc.expectedArgs, args)
				}
			} else {
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestNumMetricsPorts(t *testing.T) {
	testCases := []struct {
		name     string
		extDNS   *v1beta1.ExternalDNS
		expected int
	}{
		{
			name: "no zones, non-Azure provider",
			extDNS: &v1beta1.ExternalDNS{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1beta1.ExternalDNSSpec{
					Provider: v1beta1.ExternalDNSProvider{Type: v1beta1.ProviderTypeAWS},
				},
			},
			expected: 1,
		},
		{
			name: "no zones, Azure provider",
			extDNS: &v1beta1.ExternalDNS{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1beta1.ExternalDNSSpec{
					Provider: v1beta1.ExternalDNSProvider{Type: v1beta1.ProviderTypeAzure},
				},
			},
			expected: 2,
		},
		{
			name: "3 zones",
			extDNS: &v1beta1.ExternalDNS{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: v1beta1.ExternalDNSSpec{
					Provider: v1beta1.ExternalDNSProvider{Type: v1beta1.ProviderTypeAWS},
					Zones:    []string{"zone1", "zone2", "zone3"},
				},
			},
			expected: 3,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := numMetricsPorts(tc.extDNS)
			if got != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, got)
			}
		})
	}
}

func TestMetricsPortNameForSeq(t *testing.T) {
	testCases := []struct {
		seq      int
		expected string
	}{
		{0, "metrics"},
		{1, "metrics-1"},
		{2, "metrics-2"},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("seq=%d", tc.seq), func(t *testing.T) {
			got := metricsPortNameForSeq(tc.seq)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestMetricsCertVolume(t *testing.T) {
	secretName := "test-metrics-cert"
	vol := metricsCertVolume(secretName)

	if vol.Name != metricsCertVolumeName {
		t.Errorf("expected volume name %q, got %q", metricsCertVolumeName, vol.Name)
	}
	if vol.Secret == nil {
		t.Fatal("expected secret volume source")
	}
	if vol.Secret.SecretName != secretName {
		t.Errorf("expected secret name %q, got %q", secretName, vol.Secret.SecretName)
	}
}

func TestEqualContainerPorts(t *testing.T) {
	testCases := []struct {
		name     string
		current  []corev1.ContainerPort
		expected []corev1.ContainerPort
		equal    bool
	}{
		{
			name:     "both empty",
			current:  nil,
			expected: nil,
			equal:    true,
		},
		{
			name: "identical",
			current: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			expected: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			equal: true,
		},
		{
			name: "different length",
			current: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			expected: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
				{Name: "https-1", ContainerPort: 8444, Protocol: corev1.ProtocolTCP},
			},
			equal: false,
		},
		{
			name: "different port number",
			current: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			expected: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 9999, Protocol: corev1.ProtocolTCP},
			},
			equal: false,
		},
		{
			name: "different protocol",
			current: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			expected: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolUDP},
			},
			equal: false,
		},
		{
			name: "missing port name",
			current: []corev1.ContainerPort{
				{Name: "other", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			expected: []corev1.ContainerPort{
				{Name: "https", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			},
			equal: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := equalContainerPorts(tc.current, tc.expected)
			if got != tc.equal {
				t.Errorf("expected %v, got %v", tc.equal, got)
			}
		})
	}
}
