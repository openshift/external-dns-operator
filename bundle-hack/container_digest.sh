# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel9-operator@sha256:e93c9273342e383f728f605303daccb37e6f1185375872d0725aef80d623a7c9'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.stage.redhat.io/edo/external-dns-rhel9@sha256:36a24459014086c2df189ba457f01fb41d566a46e1986a6701ef113e14ac34af'
# kube-rbac-proxy
# Latest version of v4.14 tag is used.
# Catalog link (health grade A): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=691eb72e6d4c48dbffa76548
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:ba9ff4c933739f1774bc8277d636053c5306863221a8c7b7b9ddc4470eb7feff'
