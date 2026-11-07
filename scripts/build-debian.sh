#!/usr/bin/env bash
# Builds API test kits inside a Debian 12 container and packages it as a .deb.
#
#   scripts/build-debian.sh [-v VERSION] [--feed URL]
#
# The binary links against Debian 12's glibc (2.36) and WebKitGTK 4.1, so the package
# installs on Debian 12+, Ubuntu 22.04+ and derivatives. Builds for the host architecture.
# Needs Docker (or set DOCKER=podman). Output: release/api-test-kits_<version>_<arch>.deb
#
# Install:  sudo apt install ./release/api-test-kits_<version>_<arch>.deb
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP="api-test-kits"
MODULE="api-test-kits"
IMAGE="$APP-build-debian:bookworm"
DOCKER="${DOCKER:-docker}"
VERSION=""
FEED="${UPDATE_URL:-}"

die() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v | --version) VERSION="${2:?--version needs a value}"; shift 2 ;;
    --feed) FEED="$2"; shift 2 ;;
    -h | --help) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    --in-container) IN_CONTAINER=1; shift ;;
    *) die "unknown argument $1 (see --help)" ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || true)"
  VERSION="${VERSION#v}"
fi
# Debian versions must start with a digit; untagged commits become <productVersion>+git<hash>.
BASE_VERSION="$(sed -n 's/.*"productVersion": *"\([^"]*\)".*/\1/p' wails.json)"
if [[ -z "$VERSION" ]]; then
  VERSION="$BASE_VERSION"
elif [[ ! "$VERSION" =~ ^[0-9] ]]; then
  VERSION="$BASE_VERSION+git$VERSION"
fi
DEB_VERSION="${VERSION//-/.}"
[[ "$DEB_VERSION" =~ ^[0-9][0-9A-Za-z.+~]*$ ]] || die "invalid version '$VERSION'"

# ---- host side: build the image and re-run this script inside it ----

if [[ -z "${IN_CONTAINER:-}" ]]; then
  command -v "$DOCKER" >/dev/null || die "$DOCKER is not installed"
  info "Building image $IMAGE"
  "$DOCKER" build -t "$IMAGE" -f build/linux/debian.Dockerfile build/linux
  info "Building $APP $DEB_VERSION in Debian 12"
  mkdir -p .cache release
  "$DOCKER" run --rm \
    --user "$(id -u):$(id -g)" \
    -e HOME=/src/.cache/home \
    -e GOCACHE=/src/.cache/go-build \
    -e GOMODCACHE=/src/.cache/go-mod \
    -e npm_config_cache=/src/.cache/npm \
    -e BACKEND_URL="${BACKEND_URL:-}" \
    -v "$ROOT:/src" \
    "$IMAGE" scripts/build-debian.sh --in-container -v "$DEB_VERSION" --feed "$FEED"
  exit 0
fi

# ---- container side ----

mkdir -p "$HOME"
ARCH="$(dpkg --print-architecture)"
OUT="$APP-debian-$ARCH"

info "Building frontend"
(cd frontend && npm ci --no-audit --no-fund && npm run build)

info "Building binary"
wails build -s -m -trimpath -tags webkit2_41 \
  -platform "linux/$ARCH" -o "$OUT" \
  -ldflags "-s -w -X $MODULE/internal/service.Version=$DEB_VERSION -X $MODULE/internal/updater.FeedURL=$FEED -X $MODULE/internal/updater.Package=deb -X $MODULE/internal/backend.BaseURL=${BACKEND_URL:-}"
BIN="build/bin/$OUT"
[[ -x "$BIN" ]] || die "build output $BIN not found"

info "Packaging .deb"
STAGE="$(mktemp -d)"
PKG="$STAGE/pkg"
install -Dm755 "$BIN" "$PKG/usr/bin/$APP"
# App menu / dock icon in the standard theme sizes, rendered from the SVG source.
install -Dm644 build/appicon.svg "$PKG/usr/share/icons/hicolor/scalable/apps/$APP.svg"
for size in 16 24 32 48 64 128 256 512; do
  dir="$PKG/usr/share/icons/hicolor/${size}x${size}/apps"
  install -d "$dir"
  rsvg-convert -w "$size" -h "$size" build/appicon.svg -o "$dir/$APP.png"
done
install -Dm644 build/appicon.png "$PKG/usr/share/pixmaps/$APP.png"
install -Dm644 README.md "$PKG/usr/share/doc/$APP/README.md"
install -d "$PKG/usr/share/applications"
cat > "$PKG/usr/share/applications/$APP.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=API test kits
Comment=Local-first API testing and regression detection
Exec=$APP
Icon=$APP
Terminal=false
Categories=Development;
StartupWMClass=api-test-kits
EOF

# Work out library dependencies (with Debian 12 minimum versions) from the binary itself.
mkdir -p "$STAGE/debian"
printf 'Source: %s\n\nPackage: %s\nArchitecture: any\n' "$APP" "$APP" > "$STAGE/debian/control"
DEPENDS="$(cd "$STAGE" && dpkg-shlibdeps -O "pkg/usr/bin/$APP" 2>/dev/null | sed -n 's/^shlibs:Depends=//p')"
[[ -n "$DEPENDS" ]] || die "dpkg-shlibdeps found no dependencies"

install -d "$PKG/DEBIAN"
# Conflicts/Replaces: installing this removes the package shipped under the earlier name.
cat > "$PKG/DEBIAN/control" <<EOF
Package: $APP
Version: $DEB_VERSION
Architecture: $ARCH
Maintainer: API test kits contributors
Installed-Size: $(du -sk "$PKG/usr" | cut -f1)
Depends: $DEPENDS
Conflicts: test-tool-kit
Replaces: test-tool-kit
Section: devel
Priority: optional
Description: Local-first API testing and regression detection
 Desktop app for running API test collections and detecting regressions
 between runs. Data is stored locally in ~/.config/api-test-kits.
EOF

DEB="release/${APP}_${DEB_VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "$PKG" "$DEB" >/dev/null
rm -rf "$STAGE"

info "Done: $DEB"
dpkg-deb --field "$DEB" Package Version Architecture Depends
