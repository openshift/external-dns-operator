# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8-operator@sha256:fe2818584447961accaaa6459c22a8d5c2b7d4c698f5df3bf2ac853f6b93fd18'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8@sha256:65b8adf165bf8ad7fabcc8915f63e3d99bc172f8b9441fcdea5b9856b73ea980'
# kube-rbac-proxy
# Latest version of v4.17 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a0daee8c15dfff5626b21b5
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0185c66b6ff250fddb837d5bcafcd87f089012e9157254d356f72a2620327eab'
