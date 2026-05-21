# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9-operator@sha256:1595225bc2cd0f84ffd97375b55345490c9b62181bcd3648dda92281bc17af9b'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel9@sha256:ee3ad7fb42cc884be361f3551f8b1f46f9b2a9582849aa4547ba0c7c5f1b6580'
# kube-rbac-proxy
# Latest version of v4.17 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy-rhel9/652809a5244cb343fb4a4b66?image=6a0daee8c15dfff5626b21b5
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy-rhel9@sha256:0185c66b6ff250fddb837d5bcafcd87f089012e9157254d356f72a2620327eab'
