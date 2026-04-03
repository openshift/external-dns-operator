# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel8-operator@sha256:dd821a23f972f65aee5a22f217a43e4783a1944a707d8a76127ed0d9ea3d6b80'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel8@sha256:3f8977a694032dd6bd1675c03ead6561ce1d9b939dd04c6aa304a7c5f67711e9'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
