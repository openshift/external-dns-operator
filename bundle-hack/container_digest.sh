# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel9-operator@sha256:fcf22cf052f9f35b7aa9b2fc886f64d9493ad92ca663a56ce5a3868f620423b3'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel9@sha256:9890936faaada0886ce0782315f0a4da6d34a0783cdce5f6dab185e4117168d6'
# kube-rbac-proxy
# Latest version of v4.17 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a0daee8c15dfff5626b21b5
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0185c66b6ff250fddb837d5bcafcd87f089012e9157254d356f72a2620327eab'
