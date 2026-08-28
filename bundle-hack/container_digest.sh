# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:bfb278a17acde1c4c2fa0d60d0f97432a5650ad5011d97bce468ca8481431923'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:9e02d30bca8fbf7678f81fe947582e76eec6cd5c2692cb90103f173f961d526b'
# kube-rbac-proxy
# Latest version of v4.22 tag is used.
# Catalog link (health grade A):https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a8440ea5bb0a84dedb3d79a
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:463b54511bf50ee29f2e14f1fd00e7a4a148386d5571268277f7b945cf702451'
