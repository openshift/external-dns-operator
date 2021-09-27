module github.com/openshift/external-dns-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.6
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/controller-runtime v0.9.5
	sigs.k8s.io/controller-tools v0.6.0
)
