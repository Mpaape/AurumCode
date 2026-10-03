#!/usr/bin/env bash
#
# Acceptance program for card AUR-544.
#
# WHAT THIS PROVES
#
#   tests/acceptance/AUR-439.sh (a done card) stages cmd, internal and pkg
#   whole, honors a caller-exported GOCACHE, and now exits 0 offline.
#   Before, its enumerated package list omitted internal/artifacts, dtrack,
#   gate, grammar, reviewprofile, sbom, supplychain and xbom, so the build
#   died as `module lookup disabled by GOPROXY=off`.
#
# SELECTORS
#   all             AC-001, AC-002 and AC-001-MUT-001
#   AC-001          runs tests/acceptance/AUR-439.sh all and requires exit 0
#   AC-002          docs/specs/AUR-544.md cites the measured cause
#   AC-001-MUT-001  reverting the staging fix in a scratch copy makes the
#                   AUR-439 build fail (exit 1, build_failed, missing module)
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 holds, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-544'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in tests/acceptance/AUR-439.sh tests/e2e/AUR-439.sh tests/unit/AUR-439.go \
  tests/integration/AUR-439.go docs/specs/AUR-544.md go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a544.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# One GOCACHE shared by every nested go invocation keeps this inside the
# sealed 600 s budget.
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE
export GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"

ac001() {
  local out rc
  set +e
  out="$(bash "$repo_root/tests/acceptance/AUR-439.sh" all 2>&1)"
  rc=$?
  set -e
  if ((rc != 0)); then
    printf '%s\n' "$out" | tail -n 40 >&2
    fail "AC-001/aur439-exit:$rc"
  fi
  grep -Fq 'AUR-439/AC-001/ok' <<<"$out" || fail 'AC-001/missing-ok-line'
  printf '%s/AC-001/ok\n' "$card"
}

ac002() {
  local spec="$repo_root/docs/specs/AUR-544.md" pkg
  for pkg in artifacts dtrack gate grammar reviewprofile sbom supplychain xbom; do
    grep -Fq "internal/$pkg" "$spec" || fail "AC-002/spec-missing:internal/$pkg"
  done
  grep -Fq 'cannot find module providing package' "$spec" || fail 'AC-002/spec-missing-error-text'
  grep -Fq 'module lookup disabled by GOPROXY=off' "$spec" || fail 'AC-002/spec-missing-symptom'
  grep -Fq 'head.sha' "$spec" || fail 'AC-002/spec-missing-metadata-cause'
  printf '%s/AC-002/ok\n' "$card"
}

mut001() {
  local root="$run_dir/mut"
  mkdir -p "$root/tests/acceptance"
  cp -R "$repo_root/go.mod" "$repo_root/go.sum" "$repo_root/cmd" "$repo_root/internal" "$repo_root/pkg" "$root/"
  mkdir -p "$root/tests"
  cp -R "$repo_root/tests/fixtures" "$repo_root/tests/unit" "$repo_root/tests/integration" "$repo_root/tests/e2e" "$root/tests/"
  cp "$repo_root/tests/acceptance/AUR-439.sh" "$root/tests/acceptance/AUR-439.sh"
  chmod -R u+w -- "$root"
  local f="$root/tests/acceptance/AUR-439.sh"
  [[ "$(grep -Fc '  copy "$root" cmd internal pkg' "$f")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # Revert: the old enumerated list, which lacks internal/gate and friends.
  sed -i 's#^  copy "\$root" cmd internal pkg$#  copy "$root" cmd/aurumcode internal/analysis internal/analyzer internal/apply internal/changelog internal/config internal/context internal/git/githubclient internal/llm internal/memory internal/prompt internal/render internal/review internal/security/redaction internal/testgen pkg/types#' "$f"
  grep -Fq 'cmd/aurumcode internal/analysis' "$f" || fail 'MUT-001/mutation-not-applied'
  local out rc
  set +e
  out="$(bash "$f" AC-001-MUT-001 2>&1)"
  rc=$?
  set -e
  ((rc != 0)) || fail 'MUT-001/not-rejected'
  ((rc == 1)) || fail "MUT-001/unexpected-exit:$rc"
  grep -Fq 'build_failed' <<<"$out" || fail 'MUT-001/wrong-failure-mode'
  grep -Fq 'cannot find module providing package' <<<"$out" || fail 'MUT-001/wrong-failure-text'
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  all) ac001; ac002; mut001; printf '%s/all/ok\n' "$card" ;;
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-001-MUT-001) mut001 ;;
esac
