# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:f70fcc7f67e2be7d3bf793e03290599e5bc8c5a7a46ec644fae3933c51487261'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:d0e09840bad1051d91c204655ab445a377235db78e7d9a45fa0bc3c69e86e188'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
