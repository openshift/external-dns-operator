# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:c3ed5e5a53d1e05c365352ef00170d652a0e79bcf5c1effaa5ede6dd3e03f791'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:3c7abdadd8028b0cc3334302414bd130b7cfe19a02f13f7b6ba838d3d8987be8'
# kube-rbac-proxy
# Latest version of v4.19 tag is used.
# Catalog link (health grade A):https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a69e7e6e3094faea0ff8769
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:98f9ae97d479f92cabc782534c6dfad56588f434797f1e3d866a97810db2d323'
