package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func makeExternalDNS(name string, domains []ExternalDNSDomain) *ExternalDNS {
	return &ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: ExternalDNSSpec{
			Provider: ExternalDNSProvider{
				Type: ProviderTypeAWS,
			},
			Source: ExternalDNSSource{
				ExternalDNSSourceUnion: ExternalDNSSourceUnion{
					Type: SourceTypeCRD,
				},
				HostnameAnnotationPolicy: HostnameAnnotationPolicyIgnore,
			},
			Domains: domains,
		},
	}
}

var _ = Describe("ExternalDNS admission", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("webhook", func() {
		It("should reject resource without domain filter pattern", func() {
			resource := makeExternalDNS("test-no-pattern", []ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: ExternalDNSDomainUnion{
						MatchType: DomainMatchTypeRegex,
					},
					FilterType: FilterTypeInclude,
				},
			})
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"Pattern" cannot be empty when match type is "Pattern"`))
		})
		It("should reject resource bad domain filter pattern", func() {
			resource := makeExternalDNS("test-bad-pattern", []ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: ExternalDNSDomainUnion{
						MatchType: DomainMatchTypeRegex,
						Pattern:   pointer.StringPtr(`(.*\.test.com`),
					},
					FilterType: FilterTypeInclude,
				},
			})
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`invalid pattern for "Pattern" match type`))
		})
		It("should reject resource without domain filter name", func() {
			resource := makeExternalDNS("test-no-name", []ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: ExternalDNSDomainUnion{
						MatchType: DomainMatchTypeExact,
					},
					FilterType: FilterTypeInclude,
				},
			})
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"Name" cannot be empty when match type is "Exact"`))
		})
		It("should accept resource with valid names and patterns", func() {
			resource := makeExternalDNS("test-valid", []ExternalDNSDomain{
				{
					ExternalDNSDomainUnion: ExternalDNSDomainUnion{
						MatchType: DomainMatchTypeExact,
						Name:      pointer.StringPtr("abc.test.com"),
					},
					FilterType: FilterTypeInclude,
				},
				{
					ExternalDNSDomainUnion: ExternalDNSDomainUnion{
						MatchType: DomainMatchTypeExact,
						Name:      pointer.StringPtr(`(.*)\.test\.com`),
					},
					FilterType: FilterTypeInclude,
				},
			})
			Expect(k8sClient.Create(context.Background(), resource)).Should(Succeed())
			Expect(k8sClient.Delete(context.Background(), resource)).Should(Succeed())
		})
	})
})
