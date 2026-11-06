#!/usr/bin/env bash
# Bumps the app version, commits it and creates the release tag.
#
#   scripts/bump-version.sh patch|minor|major|X.Y.Z [-m "release notes"] [--no-tag]
#
#   patch  0.1.0 -> 0.1.1   bug fixes and small changes
#   minor  0.1.1 -> 0.2.0   new features
#   major  0.2.0 -> 1.0.0   breaking changes
#
# Updates wails.json (productVersion, shown in Windows file properties) and
# frontend/package.json, commits "Release vX.Y.Z" and creates the annotated tag
# vX.Y.Z. The tag message (-m, or the commit subjects since the previous tag)
# becomes the release notes shown in the app's update banner.
# Nothing is pushed; push with: git push --follow-tags
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

die() { echo "error: $*" >&2; exit 1; }

LEVEL=""
NOTES=""
TAG=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    -m | --message) NOTES="${2:?-m needs a value}"; shift 2 ;;
    --no-tag) TAG=0; shift ;;
    -h | --help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    -*) die "unknown option $1 (see --help)" ;;
    *) LEVEL="$1"; shift ;;
  esac
done
[[ -n "$LEVEL" ]] || die "say which part to bump: patch, minor, major or X.Y.Z (see --help)"

CURRENT="$(sed -n 's/.*"productVersion": *"\([^"]*\)".*/\1/p' wails.json)"
[[ "$CURRENT" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || die "wails.json productVersion '$CURRENT' is not X.Y.Z"
MA="${BASH_REMATCH[1]}" MI="${BASH_REMATCH[2]}" PA="${BASH_REMATCH[3]}"

# The latest tag can be ahead of wails.json (e.g. tags made by hand); never go backwards.
LAST_TAG="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
if [[ "${LAST_TAG#v}" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  t=("${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}")
  if ((t[0] > MA || (t[0] == MA && (t[1] > MI || (t[1] == MI && t[2] > PA))))); then
    MA=${t[0]} MI=${t[1]} PA=${t[2]}
    CURRENT="${LAST_TAG#v}"
  fi
fi

case "$LEVEL" in
  patch) NEXT="$MA.$MI.$((PA + 1))" ;;
  minor) NEXT="$MA.$((MI + 1)).0" ;;
  major) NEXT="$((MA + 1)).0.0" ;;
  [0-9]*.[0-9]*.[0-9]*) NEXT="$LEVEL" ;;
  *) die "unknown level '$LEVEL' (patch, minor, major or X.Y.Z)" ;;
esac
[[ "$NEXT" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "invalid version '$NEXT'"
git rev-parse -q --verify "refs/tags/v$NEXT" >/dev/null && die "tag v$NEXT already exists"

if [[ $TAG -eq 1 ]]; then
  git diff --quiet && git diff --cached --quiet ||
    die "commit or stash your changes first; the release tag must point at a clean commit"
fi

echo "==> $CURRENT -> $NEXT"
sed -i.bak "s/\"productVersion\": *\"[^\"]*\"/\"productVersion\": \"$NEXT\"/" wails.json && rm -f wails.json.bak
sed -i.bak "0,/\"version\": *\"[^\"]*\"/s//\"version\": \"$NEXT\"/" frontend/package.json && rm -f frontend/package.json.bak
# package-lock.json repeats the version at the top and for the root package.
sed -i.bak "1,12s/\"version\": *\"[^\"]*\"/\"version\": \"$NEXT\"/" frontend/package-lock.json && rm -f frontend/package-lock.json.bak

[[ $TAG -eq 1 ]] || { echo "==> Files updated (no commit, no tag)"; exit 0; }

if [[ -z "$NOTES" ]]; then
  range="HEAD"
  [[ -n "$LAST_TAG" ]] && range="$LAST_TAG..HEAD"
  NOTES="$(git log --no-merges --pretty='- %s' "$range" | grep -v '^- Release v' | head -n 50 || true)"
  [[ -n "$NOTES" ]] || NOTES="- Maintenance release"
fi

git add wails.json frontend/package.json frontend/package-lock.json
git commit -q --no-verify -m "Release v$NEXT"
git tag -a "v$NEXT" -m "v$NEXT" -m "$NOTES"
echo "==> Tagged v$NEXT. Build with 'make release-all', then publish with: git push --follow-tags"
