#!/bin/sh
set -e

GOLANGCI_VERSION="1.51.2"

OUTPUT_PATH=${1:-./bin/golangci-lint}

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

case $GOOS in
  linux)
    CHECKSUM="4de479eb9d9bc29da51aec1834e7c255b333723d38dbd56781c68e5dddc6a90b"
    ;;
  darwin)
    CHECKSUM="0549cbaa2df451cf3a2011a9d73a9cb127784d26749d9cd14c9f4818af104d44"
    ;;
    *)
    echo "Unsupported OS $GOOS"
    exit 1
    ;;
esac

if [ "$GOARCH" != "amd64" ]; then
  echo "Unsupported architecture $GOARCH"
  exit 1
fi

TEMPDIR=$(mktemp -d)
curl --silent --location -o "$TEMPDIR/golangci-lint.tar.gz" "https://github.com/golangci/golangci-lint/releases/download/v$GOLANGCI_VERSION/golangci-lint-$GOLANGCI_VERSION-$GOOS-$GOARCH.tar.gz"
tar xzf "$TEMPDIR/golangci-lint.tar.gz" --directory="$TEMPDIR"

echo "$CHECKSUM" "$TEMPDIR/golangci-lint.tar.gz" | sha256sum -c --quiet

BIN=$TEMPDIR/golangci-lint-$GOLANGCI_VERSION-$GOOS-$GOARCH/golangci-lint
mv "$BIN" "$OUTPUT_PATH"
rm -rf "$TEMPDIR"
