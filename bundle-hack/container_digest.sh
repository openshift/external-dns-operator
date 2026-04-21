# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-1-rhel-8/external-dns-operator-container-ext-dns-optr-1-1-rhel-8@sha256:0000000000000000000000000000000000000000000000000000000000000000'
# Controller
export OPERAND_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-1-rhel-8/external-dns-container-ext-dns-optr-1-1-rhel-8@sha256:7c5b02e47f3f15d9cebeb0f0165616ad40ee5979e38a0aafc1678b6a3c69f9ce'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
