#!/usr/bin/env bash
# Builds the headless CLI (api-test-kits-cli) for every platform and packages it into release/.
#
#   scripts/build-cli.sh [-v VERSION] [os/arch ...]
#
# The CLI is pure Go (no CGO, no GUI libraries), so every target cross-compiles from any host.
# Default targets: linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP="api-test-kits-cli"
MODULE="api-test-kits"
RELEASE_DIR="$ROOT/release"
VERSION=""
TARGETS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v | --version) VERSION="${2:?--version needs a value}"; shift 2 ;;
    -h | --help) sed -n '2,7p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) echo "error: unknown option $1" >&2; exit 1 ;;
    *) TARGETS+=("$1"); shift ;;
  esac
done
[[ ${#TARGETS[@]} -gt 0 ]] || TARGETS=(linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64)
if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)"
  VERSION="${VERSION#v}"
fi
[[ "$VERSION" =~ ^[0-9A-Za-z.+_-]+$ ]] || { echo "error: invalid version '$VERSION'" >&2; exit 1; }

mkdir -p "$RELEASE_DIR"
ARTIFACTS=()
for t in "${TARGETS[@]}"; do
  os="${t%/*}" arch="${t#*/}"
  name="$APP-$VERSION-$os-$arch"
  stage="$(mktemp -d)"
  mkdir -p "$stage/$name"
  bin="$APP"
  [[ "$os" == windows ]] && bin="$APP.exe"
  echo "==> Building $t"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X $MODULE/internal/service.Version=$VERSION" -o "$stage/$name/$bin" ./cmd/api-test-kits-cli
  cp docs/cli.md "$stage/$name/README.md"
  [[ -f LICENSE ]] && cp LICENSE "$stage/$name/"
  if [[ "$os" == windows ]]; then
    (cd "$stage" && zip -qr "$RELEASE_DIR/$name.zip" "$name")
    ARTIFACTS+=("$name.zip")
  else
    tar -C "$stage" -czf "$RELEASE_DIR/$name.tar.gz" "$name"
    ARTIFACTS+=("$name.tar.gz")
  fi
  rm -rf "$stage"
done
(cd "$RELEASE_DIR" && sha256sum "${ARTIFACTS[@]}" > "SHA256SUMS-cli-$VERSION.txt")
(cd "$RELEASE_DIR" && ls -lh "${ARTIFACTS[@]}" "SHA256SUMS-cli-$VERSION.txt")
