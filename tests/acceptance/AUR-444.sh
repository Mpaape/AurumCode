#!/usr/bin/env bash
#
# Acceptance program for card AUR-444, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The Action publicised by scripts/action-entrypoint.sh, Dockerfile and
#   action.yml can actually run the code review it advertises: the
#   entrypoint invokes cmd/aurumcode's review command with flags that
#   ACTUALLY EXIST (never a hand-written list -- derived from cmd/aurumcode's
#   own source, see tests/unit/AUR-444.go), the image the Dockerfile builds
#   contains that binary at the path the entrypoint resolves it to
#   (tests/integration/AUR-444.go), and none of the three false claims the
#   card's "Achado medido" measured survives in the file that carried it
#   (Dockerfile and action.yml both carried the gomarkdoc claim).
#
# THE --check DISCREPANCY, MEASURED, NOT ASSUMED
#
#   The card's own "Achado medido" lists --check among "the real flags,
#   today". Measured independently against this worktree's actual tip
#   (`go build ./cmd/aurumcode && ./aurumcode review --help`): --check does
#   NOT exist. `git merge-base --is-ancestor` confirms the AUR-439 commit
#   that adds it (cf3273f) is not an ancestor of this card's base_sha; only a
#   board bookkeeping commit (9e78890) moved AUR-439 to done, without ever
#   integrating that code. Per this card's own instruction to verify every
#   claim independently rather than trust a hand list, this fix does not
#   emit --check. tests/unit/AUR-444.go derives the real set dynamically
#   (go/parser over cmd/aurumcode's source), so if AUR-439 is later
#   integrated for real, this proof keeps working without being edited.
#
# WHY THE READ_PATHS GAP MATTERS TO HOW THIS IS PROVED
#
#   AUR-444's read_paths lists cmd/aurumcode but not the internal/... packages
#   it imports (internal/analyzer, internal/llm, internal/review,
#   internal/security, internal/git, internal/prompt, pkg/types). Confirmed
#   empirically: staging only the declared read_paths and running
#   `go build ./cmd/aurumcode` fails offline ("finding module for package
#   .../internal/analyzer" et al.) because those packages are not on disk and
#   GOPROXY is off. This card cannot repair read_paths itself (.board/cards
#   is a forbidden_path), so every REQUIRED assertion below is derived from
#   source text (go/parser, string containment) or from driving the
#   entrypoint against a stand-in binary (tests/e2e/AUR-444.sh) -- nothing
#   required here needs cmd/aurumcode's dependency closure to be present.
#   tests/e2e/AUR-444.sh additionally attempts the real binary, best-effort,
#   whenever that closure happens to be materialized (a normal working tree);
#   its absence there is reported and skipped, never treated as red.
#
# MUT-001
#   Reintroducing a flag cmd/aurumcode does not register into the
#   entrypoint's review invocation makes the same Unit proof, re-run against
#   the mutated copy, fail with a "behavior-missing" label naming the bad
#   flag; restoring reproduces the exact GREEN (mutation_case below).
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

readonly card='AUR-444'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR428|IntegrationAUR428|E2EAUR428|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a444.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export GOMAXPROCS=1 GOMEMLIMIT=2GiB
export HOME="$run_dir/home"

# copy materializes one repo path into a staged root. A missing source is an
# input this card does not own that was never materialized here -- an
# environment gap, never a verdict.
copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_static materializes exactly what the Unit and Integration proofs
# need: cmd/aurumcode's .go sources (parsed as TEXT via go/parser, never
# built), the three artifacts the card's Outcome spans, and go.mod/go.sum
# for module identity -- neither tests/unit/AUR-444.go nor
# tests/integration/AUR-444.go imports anything beyond the standard library.
stage_static() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" scripts/action-entrypoint.sh Dockerfile action.yml
  chmod -R u+w -- "$root"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_static "$root"
  copy "$root" tests/unit/AUR-444.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur444_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR444UnitBridge(t *testing.T) { TestAUR428(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR444UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    fail "selector-exit:TestAUR428:$rc"
  fi
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail 'selector:TestAUR428:zero-tests'
  grep -Fq -- '--- PASS: TestAUR444UnitBridge' <<<"$out" || fail 'selector-did-not-run:TestAUR428'
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_static "$root"
  copy "$root" tests/integration/AUR-444.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur444_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR444IntegrationBridge(t *testing.T) { IntegrationAUR428(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR444IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    fail "selector-exit:IntegrationAUR428:$rc"
  fi
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail 'selector:IntegrationAUR428:zero-tests'
  grep -Fq -- '--- PASS: TestAUR444IntegrationBridge' <<<"$out" || fail 'selector-did-not-run:IntegrationAUR428'
  cleanup_root "$root"
}

e2e_case() {
  local root="$run_dir/root-e2e"
  stage_static "$root"
  copy "$root" tests/e2e/AUR-444.sh
  chmod -R u+w -- "$root"
  local rc
  set +e
  ( cd "$root" && bash tests/e2e/AUR-444.sh E2EAUR428 ) >"$run_dir/e2e.out" 2>&1
  rc=$?
  set -e
  cat "$run_dir/e2e.out"
  if (( rc == 79 )); then
    infra "e2e-inconclusive:$(tail -n1 "$run_dir/e2e.out")"
  fi
  (( rc == 0 )) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# mutation_case is MUT-001: in a writable staged copy, reintroduce exactly
# the class of defect the card measured -- a flag cmd/aurumcode does not
# register -- into the entrypoint's review invocation, and prove the SAME
# Unit proof this card ships, re-run unmodified against the mutated copy,
# now fails with the card's own behavior-missing label naming the bad flag.
# The committed source is never touched, so restoration is by construction:
# unit_case (the unmutated proof) still passes afterwards.
mutation_case() {
  local root="$run_dir/root-mut"
  stage_static "$root"
  copy "$root" tests/unit/AUR-444.go
  chmod -R u+w -- "$root"

  local target="$root/scripts/action-entrypoint.sh"
  local anchor='    if "$AURUMCODE_CLI" review --base="${AURUMCODE_BASE_REF}"; then'
  local count
  count="$(grep -Fc -- "$anchor" "$target")" || true
  case "$count" in
    1) ;;
    0) fail 'MUT-001/anchor-not-found' ;;
    *) fail 'MUT-001/anchor-not-unique' ;;
  esac

  local content before after replacement
  content="$(cat "$target")"
  before="$(cksum "$target")" || fail 'MUT-001/cksum-failed'
  replacement='    if "$AURUMCODE_CLI" review --base="${AURUMCODE_BASE_REF}" --provider="bogus"; then'
  content="${content//$anchor/$replacement}"
  printf '%s\n' "$content" >"$target"
  after="$(cksum "$target")" || fail 'MUT-001/cksum-failed'
  [[ "$before" != "$after" ]] || fail 'MUT-001/mutation-not-applied'
  grep -Fq -- '--provider="bogus"' "$target" || fail 'MUT-001/mutation-not-applied'

  cat >"$root/tests/unit/aur444_mut_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR444MutBridge(t *testing.T) { TestAUR428(t) }
EOF

  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR444MutBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  if (( rc == 0 )); then
    fail 'MUT-001/not-rejected'
  fi
  grep -Fq "$card/$scenario/behavior-missing" <<<"$out" || fail "MUT-001/wrong-failure-mode"
  grep -Fq -- 'provider' <<<"$out" || fail 'MUT-001/wrong-flag-named'
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"

  cleanup_root "$root"

  # Restoration: the unmutated proof still passes -- the GREEN reproduces
  # exactly.
  unit_case
}

spec_case() {
  local spec="$repo_root/docs/specs/AUR-444.md"
  [[ -s "$spec" ]] || fail 'behavior-missing:docs/specs/AUR-444.md absent or empty'
  grep -Fq 'AURUMCODE_BASE_REF' "$spec" || fail 'behavior-missing:spec never documents AURUMCODE_BASE_REF'
  grep -Fq -- '--check' "$spec" || fail 'behavior-missing:spec never records the --check discrepancy'
  grep -Fq 'AUR-424' "$spec" || fail 'behavior-missing:spec never cites AUR-424 for the gomarkdoc fix'
}

run_all() {
  unit_case
  integration_case
  e2e_case
  mutation_case
  spec_case
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR428) unit_case ;;
  IntegrationAUR428) integration_case ;;
  E2EAUR428) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
