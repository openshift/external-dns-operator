# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-2-rhel-8/external-dns-operator-container-ext-dns-optr-1-2-rhel-8@sha256:add347fb3d8cc0ee0d516e3de429e7684b8a3cb804ed279c130f33163331f6b0'
# Controller
export OPERAND_IMAGE_PULLSPEC='quay.io/redhat-user-workloads/external-dns-operator-tenant/ext-dns-optr-1-2-rhel-8/external-dns-container-ext-dns-optr-1-2-rhel-8@sha256:9c5b04c799dc0333226ec691b8d2a2950231d52aebca307d13f8b01d38881358'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
