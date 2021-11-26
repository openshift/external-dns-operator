module github.com/openshift/external-dns-operator

go 1.16

require (
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.18
	github.com/Azure/go-autorest/autorest/adal v0.9.13
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/aws/aws-sdk-go v1.41.6
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.6
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/openshift/api v0.0.0-20211021122928-16dd969d5550
	github.com/openshift/cloud-credential-operator v0.0.0-20211118210017-9066dcc747fa
	github.com/openshift/cluster-ingress-operator v0.0.0-20211129201418-28dc157f3dae
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/api v0.58.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/controller-runtime/tools/setup-envtest v0.0.0-20211004203041-b1efff64d3d2
	sigs.k8s.io/controller-tools v0.6.0
)
