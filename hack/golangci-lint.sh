#!/bin/sh
set -e

GOLANGCI_VERSION="1.59.1"

OUTPUT_PATH=${1:-./bin/golangci-lint}

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

case $GOOS in
  linux)
    CHECKSUM="c30696f1292cff8778a495400745f0f9c0406a3f38d8bb12cef48d599f6c7791"
    ;;
  darwin)
    CHECKSUM="2f945063394a489c0037f0369c0ce21bc007565c08f90b924a35f4c04721cbc0"
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
