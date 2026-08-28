# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8-operator@sha256:7c60e6a395bbf69bfffaa0488d2d0188447a168c056c88c76ba2111c20ad0ea2'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8@sha256:27a4cd83ed008d13a20bb6b306e36805715f6af1390ee4aa338b3c0ffa1f5500'
# kube-rbac-proxy
# Latest version of v4.15 tag is used.
# Catalog link (health grade B): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6a047feb2defcf172d2f13ab
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:814e0ec7d531113a01b327a1f8719e4d42ec4b6683b96728c5bcfab4a3a4ebcf'
