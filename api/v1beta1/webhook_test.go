package v1beta1

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
				AWS:  &ExternalDNSAWSProviderOptions{Credentials: SecretReference{Name: "credentials"}},
			},
			Source: ExternalDNSSource{
				ExternalDNSSourceUnion: ExternalDNSSourceUnion{
					Type: SourceTypeService,
				},
				HostnameAnnotationPolicy: HostnameAnnotationPolicyIgnore,
				FQDNTemplate:             []string{"{{.Name}}"},
			},
			Domains: domains,
		},
	}
}

var _ = Describe("ExternalDNS admission webhook when platform is OCP", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
		isOpenShift = true

	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
		isOpenShift = false

	})

	Context("platform OpenShift", func() {
		Context("resource with AWS provider", func() {
			It("ignores when credential not specified", func() {
				resource := makeExternalDNS("test-missing-aws-credentials", nil)
				resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS}
				err := k8sClient.Create(context.Background(), resource)
				Expect(err).Should(Succeed())
			})
		})
		Context("resource with Azure provider", func() {
			It("ignores when provider Azure credentials are not specified", func() {
				resource := makeExternalDNS("test-missing-azure-config", nil)
				resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAzure}
				err := k8sClient.Create(context.Background(), resource)
				Expect(err).Should(Succeed())
			})
		})

		Context("resource with GCP provider", func() {
			It("ignores when provider GCP credentials are not specified", func() {
				resource := makeExternalDNS("test-missing-gcp-credentials", nil)
				resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeGCP}
				err := k8sClient.Create(context.Background(), resource)
				Expect(err).Should(Succeed())
			})
		})

		Context("resource with Bluecat provider", func() {
			It("rejected when provider Bluecat credentials are not specified", func() {
				resource := makeExternalDNS("test-missing-bluecat-config", nil)
				resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeBlueCat}
				err := k8sClient.Create(context.Background(), resource)
				Expect(err).ShouldNot(Succeed())
				Expect(err.Error()).Should(ContainSubstring("config file name must be specified when provider type is BlueCat"))
			})
		})
	})

})

var _ = Describe("ExternalDNS admission webhook", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("resource with domain filters", func() {
		It("without pattern rejected", func() {
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
		It("invalid pattern rejected", func() {
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
		It("without name rejected", func() {
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
		It("with multiple valid names and patterns accepted", func() {
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

	Context("hostname annotation", func() {
		It("should reject resource without fqdnTemplates when annotation policy is Ignore", func() {
			resource := makeExternalDNS("test-missing-fqdn-template", nil)
			resource.Spec.Source.HostnameAnnotationPolicy = HostnameAnnotationPolicyIgnore
			resource.Spec.Source.FQDNTemplate = []string{}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"fqdnTemplate" must be specified when "hostnameAnnotation" is "Ignore"`))
		})
	})

	Context("resource with AWS provider", func() {
		It("rejected when credential or assume role options not specified", func() {
			resource := makeExternalDNS("test-missing-aws-credentials", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("credentials secret or assume role options must be specified when provider type is AWS"))
		})

		It("rejected when credential and assume role options both specified", func() {
			arn := "arn:aws:iam::123456789012:role/my-role"
			resource := makeExternalDNS("test-mutually-exclusive-aws-credentials", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS, AWS: &ExternalDNSAWSProviderOptions{
				Credentials: SecretReference{Name: "aws-credentials"},
				AssumeRole:  &ExternalDNSAWSAssumeRoleOptions{ID: &arn, Strategy: ExternalDNSAWSAssumeRoleOptionIRSA},
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("credentials and assume role options are mutually exclusive but both are specified when provider type is AWS"))
		})

		It("rejected when assume role is specified without a role arn", func() {
			resource := makeExternalDNS("test-missing-aws-role-arn", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS, AWS: &ExternalDNSAWSProviderOptions{
				AssumeRole: &ExternalDNSAWSAssumeRoleOptions{Strategy: ExternalDNSAWSAssumeRoleOptionIRSA},
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("assume role arn must be specified when assume role strategy is used and provider type is AWS"))
		})

		It("rejected when role arn is invalid", func() {
			arn := "role-arn"
			resource := makeExternalDNS("test-invalid-aws-role-arn", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS, AWS: &ExternalDNSAWSProviderOptions{
				AssumeRole: &ExternalDNSAWSAssumeRoleOptions{ID: &arn, Strategy: ExternalDNSAWSAssumeRoleOptionIRSA},
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("is not a valid AWS ARN"))
		})

		It("resource with irsa assume role configuration", func() {
			arn := "arn:aws:iam::123456789012:role/my-role"
			resource := makeExternalDNS("test-irsa-aws-assume-role", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS, AWS: &ExternalDNSAWSProviderOptions{
				AssumeRole: &ExternalDNSAWSAssumeRoleOptions{ID: &arn, Strategy: ExternalDNSAWSAssumeRoleOptionIRSA},
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).Should(Succeed())
		})

		It("resource with kiam assume role configuration", func() {
			arn := "arn:aws:iam::123456789012:role/my-role"
			resource := makeExternalDNS("test-kiam-aws-assume-role", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS, AWS: &ExternalDNSAWSProviderOptions{
				AssumeRole: &ExternalDNSAWSAssumeRoleOptions{ID: &arn, Strategy: ExternalDNSAWSAssumeRoleOptionKIAM},
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).Should(Succeed())
		})

		It("resource with kube2iam assume role configuration", func() {
			arn := "arn:aws:iam::123456789012:role/my-role"
			resource := makeExternalDNS("test-kube2iam-aws-assume-role", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS, AWS: &ExternalDNSAWSProviderOptions{
				AssumeRole: &ExternalDNSAWSAssumeRoleOptions{ID: &arn, Strategy: ExternalDNSAWSAssumeRoleOptionKube2IAM},
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).Should(Succeed())
		})
	})

	Context("resource with Azure provider", func() {
		It("rejected when provider Azure credentials are not specified", func() {
			resource := makeExternalDNS("test-missing-azure-config", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAzure}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("config file name must be specified when provider type is Azure"))
		})
	})

	Context("resource with GCP provider", func() {
		It("rejected when provider GCP credentials are not specified", func() {
			resource := makeExternalDNS("test-missing-gcp-credentials", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeGCP}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("credentials secret must be specified when provider type is GCP"))
		})
	})

	Context("resource with Bluecat provider", func() {
		It("rejected when provider Bluecat credentials are not specified", func() {
			resource := makeExternalDNS("test-missing-bluecat-config", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeBlueCat}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("config file name must be specified when provider type is BlueCat"))
		})
	})

	Context("resource with Infobox provider", func() {
		It("rejected when provider WAPIVersion not specified", func() {
			resource := makeExternalDNS("test-missing-bluecat-config", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeInfoblox, Infoblox: &ExternalDNSInfobloxProviderOptions{
				Credentials: SecretReference{Name: "infoblox-credentials"},
				GridHost:    "127.0.0.1",
				WAPIPort:    1977,
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"WAPIVersion", "WAPIPort", "GridHost" and credentials file must be specified when provider is Infoblox`))
		})

		It("rejected when provider WAPIPort not specified", func() {
			resource := makeExternalDNS("test-missing-bluecat-config", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeInfoblox, Infoblox: &ExternalDNSInfobloxProviderOptions{
				Credentials: SecretReference{Name: "infoblox-credentials"},
				GridHost:    "127.0.0.1",
				WAPIVersion: "v1",
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"WAPIVersion", "WAPIPort", "GridHost" and credentials file must be specified when provider is Infoblox`))
		})

		It("rejected when provider GridHost not specified", func() {
			resource := makeExternalDNS("test-missing-bluecat-config", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeInfoblox, Infoblox: &ExternalDNSInfobloxProviderOptions{
				Credentials: SecretReference{Name: "infoblox-credentials"},
				WAPIVersion: "v1",
				WAPIPort:    1977,
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"WAPIVersion", "WAPIPort", "GridHost" and credentials file must be specified when provider is Infoblox`))
		})

		It("rejected when provider credentials not specified", func() {
			resource := makeExternalDNS("test-missing-bluecat-config", nil)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeInfoblox, Infoblox: &ExternalDNSInfobloxProviderOptions{
				WAPIVersion: "v1",
				WAPIPort:    1977,
				GridHost:    "127.0.0.1",
			}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"WAPIVersion", "WAPIPort", "GridHost" and credentials file must be specified when provider is Infoblox`))
		})
	})

	Context("resource with multiple missing fields", func() {
		It("should be rejected with all errors", func() {
			resource := makeExternalDNS(
				"test-multierror",
				[]ExternalDNSDomain{
					{
						FilterType:             FilterTypeInclude,
						ExternalDNSDomainUnion: ExternalDNSDomainUnion{MatchType: DomainMatchTypeRegex},
					},
				},
			)
			resource.Spec.Provider = ExternalDNSProvider{Type: ProviderTypeAWS}
			resource.Spec.Source = ExternalDNSSource{HostnameAnnotationPolicy: HostnameAnnotationPolicyIgnore, ExternalDNSSourceUnion: ExternalDNSSourceUnion{Type: SourceTypeService}}
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring(`"Pattern" cannot be empty when match type is "Pattern"`))
			Expect(err.Error()).Should(ContainSubstring(`"fqdnTemplate" must be specified when "hostnameAnnotation" is "Ignore"`))
			Expect(err.Error()).Should(ContainSubstring(`credentials secret or assume role options must be specified when provider type is AWS`))
		})
	})

	Context("resource with crd source", func() {
		It("should be rejected as not implemented", func() {
			resource := makeExternalDNS("test-crd-source", nil)
			resource.Spec.Source.Type = SourceTypeCRD
			err := k8sClient.Create(context.Background(), resource)
			Expect(err).ShouldNot(Succeed())
			Expect(err.Error()).Should(ContainSubstring("CRD source is not implemented"))
		})
	})
})
