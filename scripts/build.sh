#!/usr/bin/env bash
# Builds API test kits desktop binaries and packages them into release/.
#
#   scripts/build.sh [options] [platform...]
#
# Platforms: linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/universal,
# or the shortcuts linux, windows, darwin, all (= every platform this host can build), native.
#
# Options:
#   -v, --version X   version stamped into the binary (default: git describe, else wails.json)
#   --nsis            also build a Windows installer (needs makensis)
#   --no-package      leave binaries in build/bin only, skip archives and checksums
#   --skip-tests      skip go test / frontend tests before building
#   --feed URL        update manifest URL baked into the app (default: $UPDATE_URL;
#                     empty disables in-app updates)
#   -h, --help        show this help
#
# Linux builds use CGO with GTK/WebKitGTK, so they only build for the host
# architecture (set CC to a cross compiler with a matching sysroot to override).
# Windows builds need no C toolchain and can be cross-compiled from Linux.
# macOS builds require a macOS host.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP="api-test-kits"
MODULE="api-test-kits"
RELEASE_DIR="$ROOT/release"

VERSION=""
FEED="${UPDATE_URL:-}"
NSIS=0
PACKAGE=1
TESTS=1
PLATFORMS=()

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }
usage() { sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v | --version) VERSION="${2:?--version needs a value}"; shift 2 ;;
    --nsis) NSIS=1; shift ;;
    --no-package) PACKAGE=0; shift ;;
    --skip-tests) TESTS=0; shift ;;
    --feed) FEED="$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) die "unknown option $1 (see --help)" ;;
    *) PLATFORMS+=("$1"); shift ;;
  esac
done

HOST_OS="$(go env GOHOSTOS)"
HOST_ARCH="$(go env GOHOSTARCH)"

# Expand shortcuts into os/arch targets.
expand() {
  case "$1" in
    native) echo "$HOST_OS/$HOST_ARCH" ;;
    linux) echo "linux/$HOST_ARCH" ;;
    windows) echo "windows/amd64 windows/arm64" ;;
    darwin | macos) echo "darwin/universal" ;;
    all)
      case "$HOST_OS" in
        linux) echo "linux/$HOST_ARCH windows/amd64 windows/arm64" ;;
        windows) echo "windows/amd64 windows/arm64" ;;
        darwin) echo "darwin/universal windows/amd64 windows/arm64" ;;
      esac
      ;;
    */*) echo "$1" ;;
    *) die "unknown platform '$1'" ;;
  esac
}

[[ ${#PLATFORMS[@]} -eq 0 ]] && PLATFORMS=(native)
TARGETS=()
for p in "${PLATFORMS[@]}"; do
  for t in $(expand "$p"); do
    [[ " ${TARGETS[*]-} " == *" $t "* ]] || TARGETS+=("$t")
  done
done

if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || true)"
  [[ -n "$VERSION" ]] || VERSION="$(sed -n 's/.*"productVersion": *"\([^"]*\)".*/\1/p' wails.json)"
  VERSION="${VERSION#v}"
fi
[[ "$VERSION" =~ ^[0-9A-Za-z.+_-]+$ ]] || die "invalid version '$VERSION'"

# ---- preflight ----

command -v go >/dev/null || die "Go is not installed (https://go.dev/dl/)"
command -v npm >/dev/null || die "Node.js/npm is not installed"
WAILS="$(command -v wails || true)"
[[ -n "$WAILS" ]] || WAILS="$(go env GOPATH)/bin/wails"
[[ -x "$WAILS" ]] || die "wails CLI not found; run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2"

linux_tags() {
  command -v pkg-config >/dev/null || die "pkg-config is missing; run: sudo apt-get install -y pkg-config"
  pkg-config --exists gtk+-3.0 || die "GTK 3 headers missing; run: sudo apt-get install -y libgtk-3-dev"
  if pkg-config --exists webkit2gtk-4.1; then
    echo "webkit2_41"
  elif pkg-config --exists webkit2gtk-4.0; then
    echo ""
  else
    die "WebKitGTK headers missing; run: sudo apt-get install -y libwebkit2gtk-4.1-dev"
  fi
}

check_target() {
  local os="${1%/*}" arch="${1#*/}"
  case "$os" in
    linux)
      [[ "$HOST_OS" == linux ]] || die "$1: Linux builds need a Linux host"
      [[ "$arch" == "$HOST_ARCH" || -n "${CC:-}" ]] ||
        die "$1: cross-compiling Linux needs CC set to a $arch C compiler with GTK/WebKitGTK for $arch"
      ;;
    windows) [[ "$arch" == amd64 || "$arch" == arm64 ]] || die "$1: unsupported Windows architecture" ;;
    darwin) [[ "$HOST_OS" == darwin ]] || die "$1: macOS builds need a macOS host" ;;
    *) die "unsupported platform $1" ;;
  esac
}
for t in "${TARGETS[@]}"; do check_target "$t"; done

info "Version $VERSION, targets: ${TARGETS[*]}"

# ---- frontend and tests (once for all targets) ----

info "Installing frontend dependencies"
(cd frontend && npm ci --no-audit --no-fund)

if [[ $TESTS -eq 1 ]]; then
  info "Frontend lint and tests"
  (cd frontend && npm run lint && npm test)
  info "Building frontend (go test embeds frontend/dist)"
  (cd frontend && npm run build)
  info "Go tests"
  go test ./...
fi

# ---- desktop builds ----

LDFLAGS="-s -w -X $MODULE/internal/service.Version=$VERSION -X $MODULE/internal/updater.FeedURL=$FEED -X $MODULE/internal/backend.BaseURL=${BACKEND_URL:-}"
[[ -n "$FEED" ]] && info "In-app updates from $FEED" || info "No update feed: in-app updates disabled"
mkdir -p "$RELEASE_DIR"
ARTIFACTS=()

package() {
  local os="$1" arch="$2" src="$3"
  local name="$APP-$VERSION-$os-$arch"
  local stage
  stage="$(mktemp -d)"
  mkdir -p "$stage/$name"
  cp -R "$src" "$stage/$name/"
  cp README.md "$stage/$name/"
  [[ -f LICENSE ]] && cp LICENSE "$stage/$name/"
  # The bare executable is what portable installs download to update themselves.
  if [[ "$os" == linux ]]; then
    install -m755 "$src" "$RELEASE_DIR/$name"
    ARTIFACTS+=("$name")
  elif [[ "$os" == windows ]]; then
    install -m755 "$src" "$RELEASE_DIR/$name.exe"
    ARTIFACTS+=("$name.exe")
  fi
  if [[ "$os" == linux ]]; then
    cp build/appicon.png "$stage/$name/$APP.png"
    tar -C "$stage" -czf "$RELEASE_DIR/$name.tar.gz" "$name"
    ARTIFACTS+=("$name.tar.gz")
  else
    command -v zip >/dev/null || die "zip is not installed"
    (cd "$stage" && zip -qr "$RELEASE_DIR/$name.zip" "$name")
    ARTIFACTS+=("$name.zip")
  fi
  rm -rf "$stage"
}

# The frontend is already built when tests ran; otherwise the first target builds it.
frontend_built=$TESTS
first=1
for t in "${TARGETS[@]}"; do
  os="${t%/*}"
  arch="${t#*/}"
  args=(build -platform "$t" -ldflags "$LDFLAGS" -trimpath -m)
  [[ $frontend_built -eq 1 ]] && args+=(-s)
  [[ $first -eq 1 ]] && args+=(-clean)
  frontend_built=1
  first=0

  case "$os" in
    linux)
      out="$APP-$os-$arch"
      tags="$(linux_tags)"
      [[ -n "$tags" ]] && args+=(-tags "$tags")
      args+=(-o "$out")
      ;;
    windows)
      out="$APP-$arch.exe"
      [[ $NSIS -eq 1 ]] && args+=(-nsis)
      args+=(-o "$out")
      ;;
    darwin) out="" ;;
  esac

  info "Building $t"
  "$WAILS" "${args[@]}"

  if [[ "$os" == darwin ]]; then
    out="$(cd build/bin && ls -d ./*.app 2>/dev/null | head -n1)"
    out="${out#./}"
  fi
  [[ -n "$out" && -e "build/bin/$out" ]] || die "$t: build output not found in build/bin"
  if [[ $PACKAGE -eq 1 ]]; then
    package "$os" "$arch" "build/bin/$out"
    # The versioned copy in release/ is the output; drop wails' unversioned
    # intermediate so there is only one file to pick up.
    [[ "$os" == darwin ]] || rm -f "build/bin/$out"
    if [[ "$os" == windows && $NSIS -eq 1 ]]; then
      for inst in build/bin/*installer*.exe; do
        [[ -e "$inst" ]] || continue
        dest="$APP-$VERSION-$os-$arch-installer.exe"
        mv "$inst" "$RELEASE_DIR/$dest"
        ARTIFACTS+=("$dest")
      done
    fi
  fi
done

if [[ $PACKAGE -eq 1 && ${#ARTIFACTS[@]} -gt 0 ]]; then
  info "Writing checksums"
  (cd "$RELEASE_DIR" && sha256sum "${ARTIFACTS[@]}" > "SHA256SUMS-$VERSION.txt")
  info "Done. Artifacts in release/:"
  (cd "$RELEASE_DIR" && ls -lh "${ARTIFACTS[@]}" "SHA256SUMS-$VERSION.txt")
else
  info "Done. Binaries in build/bin/"
fi
