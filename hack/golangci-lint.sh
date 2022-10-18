#!/bin/sh
set -e

GOLANGCI_VERSION="1.45.2"

OUTPUT_PATH=${1:-./bin/golangci-lint}

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

case $GOOS in
  linux)
    CHECKSUM="595ad6c6dade4c064351bc309f411703e457f8ffbb7a1806b3d8ee713333427f"
    ;;
  darwin)
    CHECKSUM="995e509e895ca6a64ffc7395ac884d5961bdec98423cb896b17f345a9b4a19cf"
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
