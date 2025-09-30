#!/bin/sh
set -e

GOLANGCI_VERSION="1.64.8"

OUTPUT_PATH=${1:-./bin/golangci-lint}

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

case $GOOS in
  linux)
    CHECKSUM="b6270687afb143d019f387c791cd2a6f1cb383be9b3124d241ca11bd3ce2e54e"
    ;;
  darwin)
    CHECKSUM="b52aebb8cb51e00bfd5976099083fbe2c43ef556cef9c87e58a8ae656e740444"
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
