#!/usr/bin/env bash

export MANIFESTS_DIR="/manifests"
export METADATA_DIR="/metadata"

export SUPPORTED_OCP_VERSIONS="${SUPPORTED_OCP_VERSIONS:-v4.17}"

if [ -z "${REPLACES_VERSION:-}" ] && [ -n "${VERSION:-}" ]; then
  if [ "${VERSION##*.}" -ne 0 ]; then
    export REPLACES_VERSION="${VERSION%.*}.$((${VERSION##*.} - 1))"
  fi
fi
