module github.com/openshift/external-dns-operator

go 1.16

require (
	github.com/Azure-Samples/azure-sdk-for-go-samples v0.0.0-20210506191746-b49c4162aa1d
	github.com/Azure/azure-sdk-for-go v59.3.0+incompatible
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/aws/aws-sdk-go v1.41.6
	github.com/go-logr/logr v0.4.0
	github.com/gofrs/uuid v4.1.0+incompatible // indirect
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.2.0 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/openshift/api v0.0.0-20211019100638-b2cbe79f2e4b
	github.com/pkg/errors v0.9.1
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56 // indirect
	google.golang.org/api v0.58.0 // indirect
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/controller-runtime/tools/setup-envtest v0.0.0-20211004203041-b1efff64d3d2
	sigs.k8s.io/controller-tools v0.6.0
)
