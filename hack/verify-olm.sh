#!/bin/bash

set -euo pipefail

function print_failure {
  git status
  git diff
  echo "There are unexpected changes to the tree when running 'make bundle catalog'. Please"
  echo "run these commands locally and double-check the Git repository for unexpected changes which may"
  echo "need to be committed."
  exit 1
}

if [ "${OPENSHIFT_CI:-false}" = true ]; then
  echo "> generating the OLM bundle and catalog"
  make bundle catalog
  test -z "$(git status --porcelain | \grep -v '^??')" || print_failure
  echo "> verified generated bundle and catalog"
fi
