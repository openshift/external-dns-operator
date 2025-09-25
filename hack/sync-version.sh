#!/usr/bin/env bash

# Sync the version from the VERSION file into:
#   - Containerfile.external-dns-operator-bundle
#   - Containerfile.external-dns-operator
#
# This script was adapted from:
#   https://github.com/openshift/bpfman-operator/blob/main/hack/openshift/sync-version.sh
# Original author: Andy McDermott (frobware).

set -e

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

version_file="${repo_root}/VERSION"

containerfiles=(
    "${repo_root}/Containerfile.external-dns-operator-bundle"
    "${repo_root}/Containerfile.external-dns-operator"
)

if [ ! -f "$version_file" ]; then
    echo "Error: VERSION file not found at $version_file" >&2
    exit 1
fi

version=$(tr -d '\n' < "$version_file")
echo "Target VERSION: $version"

any_updated=false

for containerfile in "${containerfiles[@]}"; do
    if [ ! -f "$containerfile" ]; then
        echo "Warning: Containerfile not found at $containerfile, skipping..." >&2
        continue
    fi

    current_version=$(grep -oP 'version="\K[^"]+' "$containerfile" | head -1)

    if [ -z "$current_version" ]; then
        echo "Error: No version label found in $containerfile" >&2
        exit 1
    fi

    if [ "$current_version" = "$version" ]; then
        echo "[OK] $(basename "$containerfile"): already in sync ($version)"
    else
        echo "[UPDATE] $(basename "$containerfile"): updating from $current_version to $version"
        exit 1
    fi
done

if [ "$any_updated" = true ]; then
    echo ""
    echo "Updated Containerfiles with VERSION=$version"

    if command -v git &> /dev/null; then
        echo ""
        echo "Changes made:"
        git diff --stat "${containerfiles[@]}"
    fi
else
    echo ""
    echo "All Containerfiles already in sync with VERSION=$version"
fi
