# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:61c668b9efd4e7f511033644bcd0fee84808793177179437ab0354fdf2627280'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:f67e297f864f2eae3a40c4bb2f5e896e22891e62e058f9b8473d24171b4eb1d0'
# kube-rbac-proxy
# Latest version of v4.19 tag is used.
# Catalog link (health grade A):https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a69e7e6e3094faea0ff8769
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:98f9ae97d479f92cabc782534c6dfad56588f434797f1e3d866a97810db2d323'
