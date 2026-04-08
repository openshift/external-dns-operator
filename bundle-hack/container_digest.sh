# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:19a9358cc8b53971b22b8cd1246b7a648e0d47f9b61650c5e6ff7cb5a9ee5dbb'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:5c07498bf278ae1c7f62fe264686e78519ad3995e75e90b599595529ac2e3fae'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
