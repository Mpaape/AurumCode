#!/usr/bin/env bash
#
# Acceptance program for card AUR-430, scenario AC-001.
#
# WHAT THIS PROVES
#
#   Running `aurumcode review --base HEAD~1` against a git repository with a
#   known planted problem (tests/fixtures/repos/git-demo: commit 3 adds
#   config/demo-tokens.txt, a file of synthetic, credential-shaped values)
#   prints that finding with file, line and severity, deterministically,
#   with no file the user has to prepare beforehand.
#
# WHY NO `git` IS INVOKED
#
#   The card's sealed profile is bootstrap-readonly-v1: bash and a Go
#   toolchain, no `git` binary, no network (see AUR-011's acceptance
#   program for the same, independently-confirmed fact). cmd/aurumcode
#   therefore reads the git object database directly in pure Go (see
#   internal/analyzer/gitrepo.go) instead of shelling out to `git diff`.
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sandbox denies network. AURUMCODE_LLM_FIXTURE points the binary's
#   provider selection (see cmd/aurumcode/main.go's selectProvider) at a
#   canned, deterministic response file instead of a real vendor endpoint;
#   internal/review.FakeProvider implements the same llm.Provider interface
#   a real vendor provider would.
#
# EXIT CODES:
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   69 = inconclusive environment (missing tool, build failure) -- never
#        valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-430'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR430|IntegrationAUR430|E2EAUR430|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a430.XXXXXX")" || infra mktemp
# Cleanup must never turn an already-decided nominal/mutation result into a
# failure: `cp -R` below preserves the read-only mode of the materialized
# `paths`/`read_paths` input tree, so a later `rm -rf` on that copy can hit
# "Permission denied" on individual files even though the containing
# directories (created by this script, under its own writable mktemp) are
# perfectly removable once writable. Force write permission back on before
# removing, and never let a residual removal error propagate: only the
# script's own explicit `fail`/`infra` calls decide the exit code.
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs:
# this card's own owned packages plus the read-only packages they import,
# and the fixtures the CLI is exercised against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/prompt internal/review
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
}

# build_shared builds the binary exactly once per acceptance run and
# reuses it for nominal_case, mutation_case and e2e_case: the profile's
# memory ceiling (256MB) is tight for a Go build that pulls in
# net/http+crypto/tls (via internal/llm/provider/litellm), so repeating a
# cold build per case risked -- and, before this fix, caused -- the
# compiler being OOM-killed. GOCACHE is shared process-wide (exported
# above), so unit_case/integration_case's own `go test` compiles reuse the
# same warmed package cache instead of starting cold too.
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# nominal_case is AC-001's core behavioral proof: run the built binary
# exactly as a user would.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local fixture="$shared_root/tests/fixtures/review/known-problem-response.json"

  local out1
  out1="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1)" || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt' <<<"$out1" || fail behavior-missing
  grep -Fq '[error]' <<<"$out1" || fail behavior-missing

  local out2
  out2="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1)" || fail non-deterministic
  [[ "$out1" == "$out2" ]] || fail non-deterministic

  # --base is required: omitting it must fail closed, never silently
  # review nothing.
  if (cd "$repo_dir" && "$shared_bin" review >/dev/null 2>&1); then
    fail missing-base-should-fail
  fi
}

# mutation_case is MUT-001: with the review engine's declared behavior
# replaced by "return an empty issue list" (see reviewer.go's
# AURUM_A430_MUTATION hook), the same known-problem diff must come back
# with no finding -- proving the nominal case above is actually exercising
# the promised behavior, not a coincidental pass. The mutation is an
# environment toggle read at runtime (not a rebuild), so it reuses the
# exact same binary nominal_case just proved GREEN.
mutation_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local fixture="$shared_root/tests/fixtures/review/known-problem-response.json"

  local out
  out="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" AURUM_A430_MUTATION=empty-issues "$shared_bin" review --base HEAD~1)" || fail mutation-run-failed
  if grep -Fq 'config/demo-tokens.txt' <<<"$out"; then
    fail 'MUT-001/not-rejected'
  fi
  grep -Fq 'No issues found.' <<<"$out" || fail 'MUT-001/unexpected-output'

  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-430.go
  cat >"$root/tests/unit/aur430_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR430UnitBridge(t *testing.T) { TestAUR430(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 ./tests/unit -run '^TestAUR430UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  ((rc == 0)) || fail "selector:TestAUR430:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR430:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-430.go
  cat >"$root/tests/integration/aur430_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR430IntegrationBridge(t *testing.T) { IntegrationAUR430(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 ./tests/integration -run '^TestAUR430IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  ((rc == 0)) || fail "selector:IntegrationAUR430:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR430:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-430.sh
  # Reuse the already-built, already-proven binary and the shared, warm
  # GOCACHE (both exported/set above) instead of letting the nested script
  # cold-build its own copy -- see build_shared's comment.
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-430.sh E2EAUR430) || fail e2e-failed
  cleanup_root "$root"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mutation_case
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR430) unit_case ;;
  IntegrationAUR430) integration_case ;;
  E2EAUR430) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
