#! /bin/bash

set -e

[ -z "${1}" ] && { echo "no BAM IP is given"; exit 1; }

BAM_IP="${1}"
VERSION="${2:-21.5.1}"
#WORKDIR=$(mktemp -d)
WORKDIR=${HOME}/bluecat-gateway
LOGDIR=${WORKDIR}/logs
WORKFLOWS_URL="https://github.com/bluecatlabs/gateway-workflows.git"
WORKFLOWSDIR=${WORKDIR}/gateway-workflows

#chmod 775 "${WORKDIR}"
mkdir -p "${LOGDIR}"
git clone "${WORKFLOWS_URL}" "${WORKFLOWSDIR}"
cd "${WORKFLOWSDIR}"
#chmod -R o=rwx "${WORKFLOWSDIR}"

echo "working dir: ${WORKDIR}"
ls -ltr "${WORKDIR}"

podman run  -d -p 8080:8000 -p 8443:44300 \
            -v ${WORKDIR}:/bluecat_gateway/:Z \
            -v ${LOGDIR}:/logs/:Z \
            -v ${WORKFLOWSDIR}/Examples:/bluecat_gateway/workflows/Examples/:Z \
            -e BAM_IP=${BAM_IP} -e SESSION_COOKIE_SECURE=False \
            quay.io/bluecat/gateway:${VERSION}

xdg-open http://localhost:8080
