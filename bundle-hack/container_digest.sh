# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:891792412de9c2d684bc49459b87a1b6bcd509df9da27b00a1321b39f4a9c977'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:f67e297f864f2eae3a40c4bb2f5e896e22891e62e058f9b8473d24171b4eb1d0'
# kube-rbac-proxy
# Latest version of v4.18 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a202a5cc6ecfa8d7f4125bf
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0fc6a16b71e2719d9d01d6dfeb83077c38562c08d628d1f1ae03fabe3a5b9a91'
