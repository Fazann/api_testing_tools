#!/usr/bin/env bash
# Runs the regression checks: formatting, type checks, vet and all tests.
#
#   scripts/check.sh            check everything (same as `make check`)
#   scripts/check.sh --staged   only the parts touched by staged files (used by the pre-commit hook)
#
# The pre-commit hook is installed with `make hooks`. Skip it once with `git commit --no-verify`.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

run_frontend=1
run_go=1
if [[ "${1:-}" == "--staged" ]]; then
  staged=$(git diff --cached --name-only --diff-filter=ACMR)
  if [[ -z "$staged" ]]; then
    exit 0
  fi
  grep -q '^frontend/' <<<"$staged" || run_frontend=0
  grep -Eq '\.go$|^go\.(mod|sum)$|^internal/|^cmd/' <<<"$staged" || run_go=0
fi

failed=()
step() {
  local name=$1
  shift
  printf '\n\033[1m▶ %s\033[0m\n' "$name"
  if "$@"; then
    printf '\033[32m✓ %s\033[0m\n' "$name"
  else
    printf '\033[31m✗ %s\033[0m\n' "$name"
    failed+=("$name")
  fi
}

if ((run_frontend)); then
  if [[ ! -d frontend/node_modules ]]; then
    echo "frontend/node_modules is missing; run 'make deps' first." >&2
    exit 1
  fi
  step "frontend lint" npm --prefix frontend run -s lint
  step "frontend typecheck" npm --prefix frontend run -s typecheck
  step "frontend tests" npm --prefix frontend test -s
fi

if ((run_go)); then
  gofmt_check() {
    local files
    files=$(gofmt -l $(git ls-files '*.go'))
    if [[ -n "$files" ]]; then
      echo "Not gofmt-formatted (run 'make fmt'):"
      echo "$files"
      return 1
    fi
  }
  # main.go embeds frontend/dist; without a build, test every package but the root one.
  if [[ -d frontend/dist ]]; then
    pkgs=(./...)
  else
    mapfile -t pkgs < <(go list ./... | grep -v "^$(go list -m)\$")
  fi
  step "gofmt" gofmt_check
  step "go vet" go vet "${pkgs[@]}"
  step "go tests" go test "${pkgs[@]}"
fi

if ((${#failed[@]})); then
  printf '\n\033[31mRegression check failed: %s\033[0m\n' "${failed[*]}" >&2
  printf 'Fix the errors above, or commit with --no-verify to skip the check.\n' >&2
  exit 1
fi
if ((run_frontend || run_go)); then
  printf '\n\033[32mAll regression checks passed.\033[0m\n'
fi
