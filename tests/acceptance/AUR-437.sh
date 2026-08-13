#!/usr/bin/env bash
#
# Acceptance program for card AUR-437, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The GitHub client restored from commit c12d7ab
#   (internal/git/githubclient) reads a pull request's changes and publishes
#   the found problems as comments, and refuses to publish when the token
#   has no write permission — with the four measured restoration defects
#   fixed (consumed-body retry, unretried rate-limit 403, context-deaf
#   backoff sleep, and the first-"b/" path extraction that broke on a
#   directory ending in "b").
#
# WHY EVERYTHING IS OFFLINE
#
#   The sealed profile denies network. Every proof runs against a
#   loopback-only httptest/net.Listen fake GitHub built from the card's
#   fixtures in tests/fixtures/scm/github. Tokens are synthetic strings
#   assembled at runtime; no credential-shaped byte exists in any tracked
#   file, satisfying the runner's credential gate.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure — never valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-437'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR437|IntegrationAUR437|E2EAUR437|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a437.XXXXXX")" || infra mktemp
# Cleanup must never turn an already-decided result into a failure: `cp -R`
# below preserves the read-only mode of the materialized input tree, so force
# write permission back on before removing and swallow residual errors — only
# this script's own fail/infra calls decide the exit code.
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"
export GOMAXPROCS=1 GOMEMLIMIT=192MiB

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what the proofs need: this card's owned
# library and fixtures plus the read-only module files.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod
  [[ -f "$repo_root/go.sum" ]] && copy "$root" go.sum
  copy "$root" internal/git/githubclient tests/fixtures/scm/github
  # The materialized input tree is read-only including directory modes;
  # force the staged scratch copy writable (see cleanup_root's note).
  chmod -R u+w -- "$root"
}

# nominal_case is AC-001's core behavioral proof: the e2e script drives the
# library as a user-facing consumer would — read the PR's changes, publish
# the problems as comments with a write token, run twice for determinism,
# and get refused with the read-only token.
nominal_case() {
  local root="$run_dir/root-nominal"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-437.sh
  chmod -R u+w -- "$root"
  (cd "$root" && bash tests/e2e/AUR-437.sh E2EAUR437) || fail behavior-missing
  cleanup_root "$root"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-437.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur437_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR437UnitBridge(t *testing.T) { TestAUR437(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR437UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR437:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR437:zero-tests

  # The restored historical httptest suites live inside the package itself
  # (client_test.go, integration_test.go, both from c12d7ab, adapted); they
  # must pass in the sealed profile too.
  set +e
  out="$(cd "$root" && go test -mod=mod -p 1 -timeout 300s ./internal/git/githubclient -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:package-tests:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:package-tests:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-437.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur437_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR437IntegrationBridge(t *testing.T) { IntegrationAUR437(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR437IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR437:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR437:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-437.sh
  chmod -R u+w -- "$root"
  (cd "$root" && bash tests/e2e/AUR-437.sh E2EAUR437) || fail e2e-failed
  cleanup_root "$root"
}

# mutation_case is MUT-001: it edits a writable staged copy of client.go so
# requireWritePermission returns nil before checking anything — publishing
# without verifying permission — rebuilds from the mutated copy, and proves
# the read-only-token vector then fails exactly where it must: the e2e
# refusal check reports readonly_not_refused. The committed source is never
# touched; the mutation exists only in this case's own scratch copy.
mutation_case() {
  local root="$run_dir/root-mut"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-437.sh
  chmod -R u+w -- "$root"

  local target="$root/internal/git/githubclient/client.go"
  local anchor
  anchor="$(printf '\tok, err := c.HasWritePermission(ctx, owner, repo)')"
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  local before after
  before="$(cksum "$target")" || fail 'MUT-001/cksum-failed'
  sed -i "s/^${anchor}\$/\treturn nil\n${anchor}/" "$target"
  after="$(cksum "$target")" || fail 'MUT-001/cksum-failed'
  [[ "$before" != "$after" ]] || fail 'MUT-001/mutation-not-applied'

  local out rc
  set +e
  out="$(cd "$root" && bash tests/e2e/AUR-437.sh E2EAUR437 2>&1)"
  rc=$?
  set -e

  if ((rc == 0)); then
    fail 'MUT-001/not-rejected'
  fi
  grep -Fq 'readonly_not_refused' <<<"$out" || fail 'MUT-001/wrong-failure-mode'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mutation_case
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR437) unit_case ;;
  IntegrationAUR437) integration_case ;;
  E2EAUR437) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
