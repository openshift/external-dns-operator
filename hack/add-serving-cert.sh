#!/usr/bin/env bash

# Meant to secure the communication between the API and the validating webhook endpoint
# using OpenShift's service serving certificate

set -e

usage() {
  cat <<EOF
Make the service serving certificates and add the CA bundle to the validating webhook's client config.
usage: ${0} [OPTIONS]
The following flags are required.
    --namespace        Namespace where webhook service resides.
    --service          Service name of webhook.
    --secret           Secret name for CA certificate and server certificate/key pair.
    --webhook          Webhook config name.
EOF
  exit 1
}

while [ $# -gt 0 ]; do
  case ${1} in
      --service)
          service="$2"
          shift
          ;;
      --webhook)
          webhook="$2"
          shift
          ;;
      --secret)
          secret="$2"
          shift
          ;;
      --namespace)
          namespace="$2"
          shift
          ;;
      *)
          usage
          ;;
  esac
  shift
done

[ -z "${service}" ] && echo "ERROR: --service flag is required" && exit 1
[ -z "${webhook}" ] && echo "ERROR: --webhook flag is required" && exit 1
[ -z "${secret}" ] && echo "ERROR: --secret flag is required" && exit 1
[ -z "${namespace}" ] && echo "ERROR: --namespace flag is required" && exit 1

if [ ! -x "$(command -v oc)" ]; then
  echo "ERROR: oc not found"
  exit 1
fi

oc -n "${namespace}" annotate service "${service}" "service.beta.openshift.io/serving-cert-secret-name=${secret}" --overwrite=true
oc annotate validatingwebhookconfigurations "${webhook}" "service.beta.openshift.io/inject-cabundle=true" --overwrite=true
