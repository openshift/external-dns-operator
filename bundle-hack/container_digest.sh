# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:781c9c3a70f1ef9f26dc914e31af4fcaddde5d6726cac1fdd8ac93782fc5d765'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:2404ca0a5e4f348c77687aa88154b982b8db79108af230799e43d1dcf4ce2b4b'
# kube-rbac-proxy
# Latest version of v4.22 tag is used.
# Catalog link (health grade A):https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a8440ea5bb0a84dedb3d79a
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:463b54511bf50ee29f2e14f1fd00e7a4a148386d5571268277f7b945cf702451'
