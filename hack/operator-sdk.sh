#!/bin/sh

set -e

VERSION="1.15.0"

OUTPUT_PATH=${1:-./bin/operator-sdk}
VERIFY=${2:-yes}

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)
BIN="operator-sdk"
BIN_ARCH="${BIN}_${GOOS}_${GOARCH}"
OPERATOR_SDK_DL_URL="https://github.com/operator-framework/operator-sdk/releases/download/v${VERSION}"

case ${GOOS} in
  linux)
    CHECKSUM="d2065f1f7a0d03643ad71e396776dac0ee809ef33195e0f542773b377bab1b2a"
    ;;
  darwin)
    CHECKSUM="5fc30d04a31736449adb5c9b0b44e78ebeaa5cf968cc7afcbdf533135b72e31a"
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

command -v curl &> /dev/null || { echo "can't find curl command" && exit 1; }
command -v sha256sum &> /dev/null || { echo "can't find sha256sum command" && exit 1; }

TEMPDIR=$(mktemp -d)
BIN_PATH="${TEMPDIR}/${BIN_ARCH}"

echo "> downloading binary"
curl --silent --location -o "${BIN_PATH}" "${OPERATOR_SDK_DL_URL}/operator-sdk_${GOOS}_${GOARCH}"

if [ "${VERIFY}" == "yes" ]; then
    echo "> verifying binary"
    echo "${CHECKSUM} ${BIN_PATH}" | sha256sum -c --quiet
fi

echo "> installing binary"
mv "${BIN_PATH}" "${OUTPUT_PATH}"
chmod +x "${OUTPUT_PATH}"
rm -rf "${TEMPDIR}"
