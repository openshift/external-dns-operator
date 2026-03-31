#!/bin/sh
set -e

GOLANGCI_VERSION="2.10.1"

OUTPUT_PATH=${1:-./bin/golangci-lint}

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

case $GOOS in
  linux)
    CHECKSUM="dfa775874cf0561b404a02a8f4481fc69b28091da95aa697259820d429b09c99"
    ;;
  darwin)
    CHECKSUM="66fb0da81b8033b477f97eea420d4b46b230ca172b8bb87c6610109f3772b6b6"
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
