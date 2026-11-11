# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8-operator@sha256:382016dcb417804d7d5a728d7c08635d9b850ccfa80e7f79be6cce1fe097a794'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8@sha256:92380bee0076475b3150feaf350eb25de2d87072142bb08ee333c00d0c6cf65b'
# kube-rbac-proxy
# Latest version of v4.15 tag is used.
# Catalog link (health grade B): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6af2e9186f432953c6dbd80b
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:f1c228b649b488d24105a956545c3ed2a331624c9430571c0339089a88d0e5d8'
