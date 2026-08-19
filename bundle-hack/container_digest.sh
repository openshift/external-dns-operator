# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:7c05523f81076231dde95b511bbf7e62675dcf7076ca3654ffc47c62042b5f39'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:bff2bf48be1d62005dda3b939066fb9d3ba2877799373ec0d1b02feac7728bc9'
# kube-rbac-proxy
# Latest version of v4.19 tag is used.
# Catalog link (health grade A):https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a69e7e6e3094faea0ff8769
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:98f9ae97d479f92cabc782534c6dfad56588f434797f1e3d866a97810db2d323'
