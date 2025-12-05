# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-3-rhel-9/external-dns-operator-container-ext-dns-optr-1-3-rhel-9@sha256:e93c9273342e383f728f605303daccb37e6f1185375872d0725aef80d623a7c9'
# Controller
export OPERAND_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-3-rhel-9/external-dns-container-ext-dns-optr-1-3-rhel-9@sha256:01ada09821f11b7ca474a3e2f7e6b03fc15f41ba9f78529a586fd2b80b9dbb3d'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
