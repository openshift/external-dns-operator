#!/usr/bin/env bash

# This update_bundle script was inspired from the external-dns-operator render templates on CPaaS with the intention of minimal changes during Konflux migration
# https://gitlab.cee.redhat.com/cpaas-midstream/external-dns-operator/-/blob/ext-dns-optr-1.3-rhel-9/distgit/containers/external-dns-operator-bundle/render_templates?ref_type=heads
set -x
set -e

export VERSION=$(cat VERSION)

source ./container_digest.sh
source ./bundle_vars.sh

# Check for environment variables pertaining to the bundle
if  [ -z "${OPERATOR_IMAGE_PULLSPEC}" ] ||
    [ -z "${OPERAND_IMAGE_PULLSPEC}" ] ||
    [ -z "${KUBE_RBAC_PROXY_IMAGE_PULLSPEC}" ] ||
    [ -z "${MANIFESTS_DIR}" ] ||
    [ -z "${METADATA_DIR}" ] ||
    [ -z "${SUPPORTED_OCP_VERSIONS}" ] ||
    [ -z "${VERSION}" ]; then
  echo "ERROR: Not all required environment variables are set"
  echo "    OPERATOR_IMAGE_PULLSPEC"
  echo "    OPERAND_IMAGE_PULLSPEC"
  echo "    KUBE_RBAC_PROXY_IMAGE_PULLSPEC"
  echo "    MANIFESTS_DIR"
  echo "    METADATA_DIR"
  echo "    SUPPORTED_OCP_VERSIONS"
  echo "    VERSION"
  exit 2
fi

# Corrected CSV_FILE variable definition with closing parenthesis
CSV_FILE="${MANIFESTS_DIR:-/manifests}/external-dns-operator.clusterserviceversion.yaml"

sed -i -e "s|quay.io/openshift/origin-external-dns-operator:latest|${OPERATOR_IMAGE_PULLSPEC}|g" \
       -e "s|k8s.gcr.io/external-dns/external-dns:.*$|${OPERAND_IMAGE_PULLSPEC}|g" \
       -e "s|quay.io/openshift/origin-external-dns:.*$|${OPERAND_IMAGE_PULLSPEC}|g" \
       -e "s|quay.io/openshift/origin-external-dns@.*$|${OPERAND_IMAGE_PULLSPEC}|g" \
       -e "s|quay.io/external-dns-operator/external-dns:.*$|${OPERAND_IMAGE_PULLSPEC}|g" \
       -e "s|quay.io/external-dns-operator/external-dns@.*$|${OPERAND_IMAGE_PULLSPEC}|g" \
       -e "s|quay.io/openshift/origin-kube-rbac-proxy:latest|${KUBE_RBAC_PROXY_IMAGE_PULLSPEC}|g" "${CSV_FILE}"

export EPOC_TIMESTAMP=$(date +%s)
export TARGET_CSV_FILE="${CSV_FILE}"

# Combined all Python logic into a single block
python3 - << CSV_UPDATE
import os
from sys import exit as sys_exit
from datetime import datetime
from ruamel.yaml import YAML

yaml = YAML()

def load_manifest(pathn):
   if not pathn.endswith(".yaml"):
      return None
   try:
      with open(pathn, "r") as f:
         return yaml.load(f)
   except FileNotFoundError:
      print("File can not be found")
      exit(6)

def dump_manifest(pathn, manifest):
   with open(pathn, "w") as f:
      yaml.dump(manifest, f)
   return

timestamp = int(os.getenv('EPOC_TIMESTAMP'))
datetime_time = datetime.fromtimestamp(timestamp)
version = os.getenv('VERSION')
replaces = os.getenv('REPLACES_VERSION')
operator_pullspec = os.getenv('OPERATOR_IMAGE_PULLSPEC', '')
operand_pullspec = os.getenv('OPERAND_IMAGE_PULLSPEC', '')
kube_rbac_proxy_pullspec = os.getenv('KUBE_RBAC_PROXY_IMAGE_PULLSPEC', '')
csv = load_manifest(os.getenv('TARGET_CSV_FILE'))

# Update CSV metadata and spec
csv['metadata']['annotations']['createdAt'] = datetime_time.strftime('%Y-%m-%dT%H:%M:%S')
csv['metadata']['annotations']['containerImage'] = operator_pullspec
csv['metadata']['name'] = 'external-dns-operator.v{}'.format(version)
csv['metadata']['annotations']['olm.skipRange'] = '<{}'.format(version)
csv['spec']['version'] = version

if replaces:
    csv['spec']['replaces'] = 'external-dns-operator.v{}'.format(replaces)

# Add relatedImages
if '@sha256:' not in operator_pullspec:
    print(f"ERROR: operator image does not contain SHA256: {operator_pullspec}")
    exit(3)

operator_sha = operator_pullspec.split('@sha256:')[1]
annotation_image_name = f'external-dns-rhel9-operator-{operator_sha}-annotation'
csv['spec']['relatedImages'] = [
    {'name': annotation_image_name, 'image': operator_pullspec},
    {'name': 'external-dns-operator', 'image': operator_pullspec},
    {'name': 'kube-rbac-proxy', 'image': kube_rbac_proxy_pullspec},
    {'name': 'external_dns', 'image': operand_pullspec}
]

dump_manifest(os.getenv('TARGET_CSV_FILE'), csv)
CSV_UPDATE

[ $? -ne 0 ] && { echo "ERROR: Error rendering CSV file."; exit 3; }

# Add OCP annotations
python3 - << END
import os
from ruamel.yaml import YAML
yaml = YAML()
with open(os.getenv('METADATA_DIR') + "/annotations.yaml", 'r') as f:
    y = yaml.load(f)
    y['annotations']['com.redhat.openshift.versions'] = os.getenv('SUPPORTED_OCP_VERSIONS')
with open(os.getenv('METADATA_DIR') + "/annotations.yaml", 'w') as f:
    yaml.dump(y, f)
END
[ $? -ne 0 ] && { echo "ERROR: Error rendering annotations.yaml file."; exit 3; }

exit 0
