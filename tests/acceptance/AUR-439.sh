#!/usr/bin/env bash
#
# Acceptance program for card AUR-439, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar
#   --check` publishes one commit status (internal/git/githubclient.SetStatus,
#   AUR-437) on the pull request's head commit: "failure" when at least one
#   finding is grave (error severity), "success" otherwise -- so a branch
#   protection rule requiring this check blocks the merge until the grave
#   finding is fixed. --check needs no --na-linha of its own, inherits the
#   commit-SHA and write-permission fail-closed gates AUR-438/AUR-437
#   already proved instead of a second, weaker path around them, and the
#   pre-existing `--pr ... --publicar --na-linha` (no --check) contract is
#   untouched. MUT-001: reporting success while a grave finding is present
#   must be caught.
#
# WHY EVERYTHING IS OFFLINE
#
#   The sealed profile denies network. Every proof runs against a
#   loopback-only fake GitHub built from AUR-438's fixtures
#   (tests/fixtures/scm/github) and deterministic offline model responses
#   the proof programs write themselves. Tokens are synthetic strings
#   assembled at runtime; no credential-shaped byte exists in any tracked
#   file.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure -- never valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-439'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR439|IntegrationAUR439|E2EAUR439|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a439.XXXXXX")" || infra mktemp
# cp -R below preserves the read-only mode bits of the materialized input
# tree; force the staged scratch copy writable before removing it, and
# never let a residual removal error override an already-decided result
# (see tests/acceptance/AUR-430.sh's cleanup_root, the origin of this
# idiom).
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

# copy materializes one repo path into a staged root. Every path it copies
# is either owned by this card (paths) or read by it (read_paths); a
# missing source is an input this card does not own that was never
# materialized -- an environment gap, never a verdict.
copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs
# for the --check path this card adds: this card's owned cmd/aurumcode
# plus the read-only packages `go list -deps ./cmd/aurumcode` actually
# names (verified against this card's own read_paths, not copied on
# faith) -- internal/git/githubclient (the AUR-437 client SetStatus lives
# in), internal/security/redaction (not the whole internal/security tree:
# this card's read_paths name only the redaction subpackage, and nothing
# cmd/aurumcode imports reaches further), and the documentation/pipeline
# closure cmd/aurumcode/docs.go (AUR-426, already integrated on this tip)
# imports -- plus the fixtures this card's own proofs read.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/analyzer internal/config internal/prompt internal/review internal/llm \
    internal/git/githubclient internal/security/redaction pkg/types
  copy "$root" internal/documentation/extractors internal/documentation/incremental \
    internal/documentation/normalizer internal/documentation/site internal/documentation/review \
    internal/documentation/welcome internal/documentation/review internal/pipeline
  copy "$root" tests/fixtures/scm/github tests/fixtures/repos/git-demo tests/fixtures/review

  # See the note above copy(): the staged copy is scratch from here on, so
  # force it writable for the mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# build_shared builds the binary once per acceptance run and reuses it for
# every case; GOCACHE is shared process-wide, so the go test compiles and
# tests/e2e/AUR-439.sh's own fakegithub build start warm instead of cold
# (see tests/acceptance/AUR-430.sh's build_shared note, the origin of this
# idiom).
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

# run_e2e runs tests/e2e/AUR-439.sh -- which proves AC-001 in full: grave
# vs clean vs zero-issue status content, the exit code that must never
# read as success, fail-closed inheritance, and determinism -- against the
# given binary, from the given staged root. The nested script's own
# exit-code vocabulary is preserved: its 79 is an environment gap and must
# be re-emitted as infra here, never collapsed into behavioral RED.
run_e2e() {
  local root="$1" bin="$2"
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$bin" bash tests/e2e/AUR-439.sh E2EAUR439) \
    >"$run_dir/e2e.stdout" 2>"$run_dir/e2e.stderr"
  rc=$?
  set -e
  printf '%s\n' "$rc"
}

# nominal_case is AC-001's core behavioral proof.
nominal_case() {
  build_shared
  local root="$run_dir/root-nominal"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-439.sh
  chmod -R u+w -- "$root"
  local rc
  rc="$(run_e2e "$root" "$shared_bin")"
  if ((rc == 79)); then
    cat "$run_dir/e2e.stdout" "$run_dir/e2e.stderr" >&2
    infra "nominal-inconclusive:$rc"
  fi
  if ((rc != 0)); then
    cat "$run_dir/e2e.stdout" "$run_dir/e2e.stderr" >&2
    fail "behavior-missing:exit:$rc"
  fi
  cleanup_root "$root"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-439.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur439_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR439UnitBridge(t *testing.T) { TestAUR439(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR439UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR439:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR439:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-439.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur439_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR439IntegrationBridge(t *testing.T) { IntegrationAUR439(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR439IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR439:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR439:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-439.sh
  chmod -R u+w -- "$root"
  local rc
  rc="$(run_e2e "$root" "$shared_bin")"
  ((rc != 79)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# mutation_case is MUT-001: in a writable staged copy, mutate
# cmd/aurumcode/pr.go's publishCheckStatus so the grave count it acts on is
# always forced to zero right after it is computed -- the defect is
# exactly "reportar sucesso com achado grave presente": the published
# status becomes "success" and the exit code becomes 0 even when a grave
# finding was fed in. Rebuild from the mutated copy and prove the same e2e
# proof then fails exactly where it must: tests/e2e/AUR-439.sh's own
# wrong_state_reported_success or grave_wrong_exit assertion (scenario 1,
# the grave-finding case) trips first, because the mutant makes that
# scenario the first to diverge from what it must prove. The committed
# source is never touched, so restoration is by construction: the
# unmutated shared binary still passes.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild below recompiles only cmd/aurumcode.

  local root="$run_dir/root-mut"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-439.sh
  chmod -R u+w -- "$root"

  local target="$root/cmd/aurumcode/pr.go"
  local anchor
  anchor="$(printf '\tgrave := countAtOrAbove(issues, rankError)')"
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  local before after
  before="$(cksum "$target")" || fail 'MUT-001/cksum-failed'
  sed -i "s|^${anchor}\$|${anchor}\n\tgrave = 0 // MUT-001: pretend no grave finding exists|" "$target"
  after="$(cksum "$target")" || fail 'MUT-001/cksum-failed'
  [[ "$before" != "$after" ]] || fail 'MUT-001/mutation-not-applied'
  grep -Fq 'MUT-001: pretend no grave finding exists' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  # Under the mutant, the grave-finding scenario reports success: the e2e
  # proof must fail, and specifically on wrong_state_reported_success or
  # grave_wrong_exit -- never on some unrelated, coincidental failure (a
  # wrong PR number, a build error, a regression in the clean/zero-issue
  # scenarios that run after the grave one and never get reached).
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_BIN="$bin" bash tests/e2e/AUR-439.sh E2EAUR439 2>&1)"
  rc=$?
  set -e
  if ((rc == 0)); then
    fail 'MUT-001/not-rejected'
  fi
  if ((rc == 79)); then
    infra "MUT-001/inconclusive:$out"
  fi
  grep -Eq 'wrong_state_reported_success|grave_wrong_exit' <<<"$out" || fail "MUT-001/wrong-failure-mode:$out"

  # Restoration: the unmutated shared binary still passes the same proof --
  # the GREEN reproduces exactly.
  local root2="$run_dir/root-mut-restore"
  stage_source "$root2"
  copy "$root2" tests/e2e/AUR-439.sh
  chmod -R u+w -- "$root2"
  local rc2
  rc2="$(run_e2e "$root2" "$shared_bin")"
  ((rc2 == 0)) || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  cleanup_root "$root2"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
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
  TestAUR439) unit_case ;;
  IntegrationAUR439) integration_case ;;
  E2EAUR439) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
