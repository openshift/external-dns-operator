# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:85c924c961be7b7cd43ae163bde145ebf1c901d134b9d5be45ca87b941d0b62d'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:fbdf7b34328642c38580e175bb6f85614bacb7db4bd80433527762dba75e38c1'
# kube-rbac-proxy
# Latest version of v4.18 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a202a5cc6ecfa8d7f4125bf
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0fc6a16b71e2719d9d01d6dfeb83077c38562c08d628d1f1ae03fabe3a5b9a91'
