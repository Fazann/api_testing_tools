#!/usr/bin/env bash
# Writes release/latest.json, the update manifest installed apps poll.
#
#   scripts/make-manifest.sh -v VERSION [--page-url URL] [--notes TEXT]
#
# Lists every update asset of VERSION found in release/ with its SHA-256:
#   api-test-kits-VERSION-linux-ARCH                  portable Linux binary
#   api-test-kits_VERSION_ARCH.deb                    Debian/Ubuntu package
#   api-test-kits-VERSION-windows-ARCH.exe            portable Windows binary
#   api-test-kits-VERSION-windows-ARCH-installer.exe  NSIS installer
# Asset URLs are relative, so upload latest.json next to the files (e.g. to
# the GitHub release). Release notes default to the vVERSION tag message.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

APP="api-test-kits"
VERSION=""
PAGE_URL="${RELEASE_PAGE_URL:-}"
NOTES=""

die() { echo "error: $*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v | --version) VERSION="${2:?--version needs a value}"; shift 2 ;;
    --page-url) PAGE_URL="${2:?--page-url needs a value}"; shift 2 ;;
    --notes) NOTES="${2:?--notes needs a value}"; shift 2 ;;
    -h | --help) sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument $1 (see --help)" ;;
  esac
done
VERSION="${VERSION#v}"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
  die "release version must be X.Y.Z (got '$VERSION'); tag the release first: make bump-patch"
command -v python3 >/dev/null || die "python3 is required to write JSON"

if [[ -z "$NOTES" ]] && git rev-parse -q --verify "refs/tags/v$VERSION" >/dev/null; then
  NOTES="$(git tag -l --format='%(contents:body)' "v$VERSION" | sed -e '/-----BEGIN PGP/,$d')"
fi
PAGE_URL="${PAGE_URL//\{version\}/$VERSION}"

cd release
args=()
add() { # key file
  [[ -f "$2" ]] || return 0
  args+=("$1" "$2" "$(sha256sum "$2" | cut -d' ' -f1)")
}
for arch in amd64 arm64; do
  add "linux-$arch" "$APP-$VERSION-linux-$arch"
  add "linux-$arch-deb" "${APP}_${VERSION}_${arch}.deb"
  add "windows-$arch" "$APP-$VERSION-windows-$arch.exe"
  add "windows-$arch-installer" "$APP-$VERSION-windows-$arch-installer.exe"
done
[[ ${#args[@]} -gt 0 ]] || die "no $VERSION update assets in release/; build them first (make release-all)"

python3 - "$VERSION" "$PAGE_URL" "$NOTES" "${args[@]}" > latest.json <<'PY'
import json, sys
version, page, notes, *rest = sys.argv[1:]
assets = {rest[i]: {"url": rest[i + 1], "sha256": rest[i + 2]} for i in range(0, len(rest), 3)}
json.dump({"version": version, "notes": notes.strip(), "pageUrl": page, "assets": assets}, sys.stdout, indent=2)
print()
PY
echo "==> release/latest.json ($VERSION, $(( ${#args[@]} / 3 )) update assets)"
