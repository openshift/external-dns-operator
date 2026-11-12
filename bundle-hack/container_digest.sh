# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8-operator@sha256:d7842d6473348d5d781180f8a28a3f38ada2c107d16b4154da4620120717150d'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8@sha256:a1080f3cce10e3d1ff990820b19b4b032e761b0a2bfe4854ef6b7d352d9098c7'
# kube-rbac-proxy
# Latest version of v4.15 tag is used.
# Catalog link (health grade B): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6af2e9186f432953c6dbd80b
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:f1c228b649b488d24105a956545c3ed2a331624c9430571c0339089a88d0e5d8'
