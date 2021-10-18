#!/usr/bin/env bash

# Taken from https://github.com/newrelic/k8s-webhook-cert-manager/blob/master/generate_certificate.sh
# and modified for use with newer versions of Kubernetes/Openshift.

set -e

usage() {
  cat <<EOF
Generate a TLS key and certificate suitable for use with any Kubernetes Webhook. The key and \
certificate are self signed. The server key and cert are stored in a k8s secret. The certificate \
is also also injected into the Webhook configuration resource.
usage: ${0} [OPTIONS]
The following flags are required.
    --service          Service name of webhook.
    --webhook          Webhook config name.
    --namespace        Namespace where webhook service and secret reside.
    --secret           Secret name for CA certificate and server certificate/key pair.
The following flags are optional.
    --webhook-kind     Webhook kind, either MutatingWebhookConfiguration or
                       ValidatingWebhookConfiguration (defaults to ValidatingWebhookConfiguration)
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
      --webhook-kind)
          kind="$2"
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

fullServiceDomain="${service}.${namespace}.svc"

# THE CN has a limit of 64 characters. We could remove the namespace and svc
# and rely on the Subject Alternative Name (SAN), but there is a bug in EKS
# that discards the SAN when signing the certificates.
#
# https://github.com/awslabs/amazon-eks-ami/issues/341
if [ ${#fullServiceDomain} -gt 64 ] ; then
  echo "ERROR: common name exceeds the 64 character limit: ${fullServiceDomain}"
  exit 1
fi

if [ ! -x "$(command -v openssl)" ]; then
  echo "ERROR: openssl not found"
  exit 1
fi


tmpdir=$(mktemp -d)
echo "INFO: Creating certs in tmpdir ${tmpdir} "

openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
  -keyout "${tmpdir}/tls.key" -out "${tmpdir}/tls.crt" -subj "/CN=${fullServiceDomain}" \
  -addext "subjectAltName=DNS:${fullServiceDomain}"

# create the secret with CA cert and server cert/key
kubectl create secret tls "${secret}" \
      --key="${tmpdir}/tls.key" \
      --cert="${tmpdir}/tls.crt" \
      --dry-run=client -o yaml |
  kubectl -n "${namespace}" apply -f -

caBundle=$(base64 < "${tmpdir}/tls.crt" | tr -d '\n')

echo "INFO: Trying to patch webhook adding the caBundle."
kubectl patch "${kind:-validatingwebhookconfiguration}" "${webhook}" --type='json' -p "[{'op': 'add', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${caBundle}'}]"

