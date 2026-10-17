# Do not remove comment lines, they are there to reduce conflicts
# Operator
export OPERATOR_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8-operator@sha256:abd5db8c34755aee55e9f4196c90a0d4e3e08256ac06748fdd0df59b387d0245'
# Controller
export OPERAND_IMAGE_PULLSPEC='registry.redhat.io/edo/external-dns-rhel8@sha256:1acebdf220498dcce7ef8ee2c4ba0af6f1c3009a9dc24df0bc923a5b227e804f'
# kube-rbac-proxy
# Latest version of v4.15 tag is used.
# Catalog link (health grade B): https://catalog.redhat.com/en/software/containers/openshift4/ose-kube-rbac-proxy/5cdb2634dd19c778293b4d98?image=6a047feb2defcf172d2f13ab
export KUBE_RBAC_PROXY_IMAGE_PULLSPEC='registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:814e0ec7d531113a01b327a1f8719e4d42ec4b6683b96728c5bcfab4a3a4ebcf'
