# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel9-operator@sha256:80a58d6389fa37acb505e9d0613a333b3032763140c6024da310fd5b8fa3fa89'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel9@sha256:7c118dc2dc23eef9a039770ea45c863e56728ab3295c14bc48ebb222b9df256c'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
