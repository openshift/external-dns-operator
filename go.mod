module github.com/openshift/external-dns-operator

go 1.16

require (
	github.com/Azure/azure-sdk-for-go v60.1.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.23
	github.com/Azure/go-autorest/autorest/adal v0.9.17
	github.com/aws/aws-sdk-go v1.41.6
	github.com/go-logr/logr v0.4.0
	github.com/golang-jwt/jwt/v4 v4.2.0 // indirect
	github.com/google/go-cmp v0.5.6
	github.com/miekg/dns v1.0.14
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/openshift/api v0.0.0-20211019100638-b2cbe79f2e4b
	github.com/openshift/cloud-credential-operator v0.0.0-20211118210017-9066dcc747fa
	golang.org/x/crypto v0.0.0-20211209193657-4570a0811e8b // indirect
	google.golang.org/api v0.58.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/controller-runtime/tools/setup-envtest v0.0.0-20211004203041-b1efff64d3d2
	sigs.k8s.io/controller-tools v0.6.0
)
