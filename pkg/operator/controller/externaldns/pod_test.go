package externaldnscontroller

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/utils/pointer"

	"github.com/openshift/external-dns-operator/api/v1alpha1"
)

func TestDomainFilters(t *testing.T) {
	for _, tc := range []struct {
		name          string
		domainInput   []v1alpha1.ExternalDNSDomain
		expectErr     bool
		expectedArgs  []string
		expectedError string
	}{
		{
			name: "only one domain included",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("abc.com"),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{"--domain-filter=abc.com"},
		},
		{
			name: "multiple domains included",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("abc.com"),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("def.com"),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("ghi.com"),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{"--domain-filter=abc.com", "--domain-filter=def.com", "--domain-filter=ghi.com"},
		},
		{
			name: "single regex include filter",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.abc\.com`),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{`--regex-domain-filter=(.*)\.abc\.com`},
		},
		{
			name: "invalid regex include filter",

			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*]\.abc\.com`),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
			},
			expectErr:     true,
			expectedError: `input pattern (.*]\.abc\.com is invalid`,
		},
		{
			name: "multiple regex include filter",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.abc\.com`),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.def\.com`),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
			},
			expectedArgs: []string{`--regex-domain-filter=((.*)\.abc\.com)|((.*)\.def\.com)`},
		},
		{
			name: "only one domain excluded",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("abc.com"),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{"--exclude-domains=abc.com"},
		},
		{
			name: "multiple domains excluded",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("abc.com"),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("def.com"),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("ghi.com"),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{"--exclude-domains=abc.com", "--exclude-domains=def.com", "--exclude-domains=ghi.com"},
		},
		{
			name: "single regex exclude filter",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.abc\.com`),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{`--regex-domain-exclusion=(.*)\.abc\.com`},
		},
		{
			name: "invalid regex exclude filter",

			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*]\.abc\.com`),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
			},
			expectErr:     true,
			expectedError: `exclude pattern (.*]\.abc\.com is invalid`,
		},
		{
			name: "multiple regex exclude filter",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.abc\.com`),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.def\.com`),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{
				`--regex-domain-exclusion=((.*)\.abc\.com)|((.*)\.def\.com)`,
			},
		},
		{
			name: "mixed domain filters",
			domainInput: []v1alpha1.ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("abc.com"),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeExact,
						Name:      pointer.StringPtr("def.com"),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.ghi\.com`),
					},
					FilterType: v1alpha1.FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: v1alpha1.ExternalDNSDomainUnion{
						MatchType: v1alpha1.DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*)\.pqr\.com`),
					},
					FilterType: v1alpha1.FilterTypeExclude,
				},
			},
			expectedArgs: []string{"--domain-filter=abc.com", "--exclude-domains=def.com", `--regex-domain-filter=(.*)\.ghi\.com`, `--regex-domain-exclusion=(.*)\.pqr\.com`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := &externalDNSContainerBuilder{
				externalDNS: &v1alpha1.ExternalDNS{
					Spec: v1alpha1.ExternalDNSSpec{
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
