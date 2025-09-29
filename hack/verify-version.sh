#!/bin/bash

set -euo pipefail

function print_failure {
  git status
  git diff
  echo "There are unexpected changes to the tree when running 'make set-version'. Please"
  echo "run the command locally and double-check the Git repository for unexpected changes which may"
  echo "need to be committed."
  exit 1
}

if [ "${OPENSHIFT_CI:-false}" = true ]; then
  echo "> setting version"
  make set-version
  test -z "$(git status --porcelain | \grep -v '^??')" || print_failure
  echo "> verified containerfiles"
fi
