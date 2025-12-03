# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-3-rhel-9/external-dns-operator-container-ext-dns-optr-1-3-rhel-9@sha256:2f2c2f56cac0c993293cbb2f1460313e19bf6c757267bf107ded07b8718695b3'
# Controller
export OPERAND_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-3-rhel-9/external-dns-container-ext-dns-optr-1-3-rhel-9@sha256:8a88bc8c4ad8e5cf2da8b003040a9d7e3e353e52982e3588b6539a5135763cb2'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
