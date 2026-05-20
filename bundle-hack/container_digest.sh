# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:35ed62702739decfeb8cc0039476d8e5b2c5547460559c93a337ca4c679b34ab'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:d0e09840bad1051d91c204655ab445a377235db78e7d9a45fa0bc3c69e86e188'
# kube-rbac-proxy
# Latest version of v4.17 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a0daee8c15dfff5626b21b5
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0185c66b6ff250fddb837d5bcafcd87f089012e9157254d356f72a2620327eab'
