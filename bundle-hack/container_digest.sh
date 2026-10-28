# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8-operator@sha256:72dd749aa726b98c549dc8a92cfb3985030ee784724c382b482f5819f44670be'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8@sha256:f2a33c26aaf5b48e3eafe00ac1244d54786cfcfede9f4437ef65e8f7fea17d90'
# kube-rbac-proxy
# Latest version of v4.15 tag is used.
# Catalog link (health grade B): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6a047feb2defcf172d2f13ab
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:814e0ec7d531113a01b327a1f8719e4d42ec4b6683b96728c5bcfab4a3a4ebcf'
