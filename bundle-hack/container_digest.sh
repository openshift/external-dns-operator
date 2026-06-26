# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:e13a2bad5a6c5cea6f85ca638bfc6de99c301e5dfaa1648738e3272cf95ed177'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:7e22f6b017ddf0105c668aff373e37d0f4e79ae99a6d2192d42f843bc549c907'
# kube-rbac-proxy
# Latest version of v4.18 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a202a5cc6ecfa8d7f4125bf
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0fc6a16b71e2719d9d01d6dfeb83077c38562c08d628d1f1ae03fabe3a5b9a91'
