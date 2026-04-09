# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel8-operator@sha256:818b2238a0f4600e32794ec24056c24f741337cf1077329104d8227e97ec30f0'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel8@sha256:9f60c682b44497d9736a04991c0d2b3485d477f6c89a87c4a44a211a3d1f3cd4'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
