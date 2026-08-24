#!/usr/bin/env bash
#
# Acceptance program for card AUR-444, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The Action publicised by scripts/action-entrypoint.sh, Dockerfile and
#   action.yml can actually run the code review it advertises: the
#   entrypoint invokes cmd/aurumcode's review command with flags that
#   ACTUALLY EXIST -- never a hand-written list; derived both from
#   cmd/aurumcode's own source (go/parser, tests/unit/AUR-444.go) AND from
#   the real, built binary's own `--help` output (tests/integration/AUR-444.go)
#   -- the image the Dockerfile builds contains that binary at the path the
#   entrypoint resolves it to, and none of the three false claims the card's
#   "Achado medido" measured survives in the file that carried it (Dockerfile
#   and action.yml both carried the gomarkdoc claim).
#
# THE --check DISCREPANCY: MEASURED TWICE, DIFFERENTLY, AND WHY
#
#   An earlier measurement against a stale board state found --check absent:
#   the card's own "Achado medido" named it as real, but AUR-439's code
#   commit (cf3273f) was not yet an ancestor of this line of history -- only
#   a board bookkeeping commit (9e78890) had moved that card to `done`
#   without integrating it. That gap is now closed (the coordinator merged
#   the approved SHA); measured again on this tip, `--check` DOES exist. This
#   fix still does not wire it into scripts/action-entrypoint.sh: `--check`
#   is a parameter of cmd/aurumcode's `--pr` path only
#   (cmd/aurumcode/main.go passes it to runPRReview, never used on the
#   `--base` path), so wiring it would mean also wiring the whole
#   `--pr`/`--repo`/`--publicar` family -- inventing pull-request-number
#   extraction from GITHUB_EVENT_PATH this entrypoint has never done, and a
#   live `HasWritePermission` GitHub API call this card's own Preconditions
#   rule out testing (the sandbox denies network). That is deliberately out
#   of this card's scope; see docs/specs/AUR-444.md for the full reasoning
#   and the follow-up this leaves for a future card.
#
# THE read_paths CLOSURE, ALSO FIXED SINCE AN EARLIER MEASUREMENT
#
#   An earlier measurement found AUR-444's read_paths listing cmd/aurumcode
#   but none of the internal/... packages it imports, so a plain
#   `go build ./cmd/aurumcode` failed offline inside the sealed sandbox
#   ("finding module for package .../internal/analyzer" et al.). read_paths
#   now carries the full closure (internal/analyzer, internal/prompt,
#   internal/review, internal/llm, internal/security/redaction,
#   internal/git/githubclient, pkg/types, internal/documentation/...,
#   internal/pipeline), so stage_static below stages it and both
#   tests/integration/AUR-444.go and tests/e2e/AUR-444.sh build and run the
#   REAL binary inside this staged root -- no longer best-effort, a required
#   assertion.
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
  AC-001|TestAUR444|IntegrationAUR444|E2EAUR444|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Resolved BEFORE HOME is redirected below: the default module cache derives
# from HOME, and this run is offline (GOPROXY=off). cmd/aurumcode's closure
# pulls in gopkg.in/yaml.v3 (internal/documentation/normalizer), an external
# module -- not a local package -- so it must already be cached on the host
# running this acceptance; GOMODCACHE is pinned to that host cache below so
# the staged builds can still resolve it without network (same technique as
# tests/e2e/AUR-425.sh).
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a444.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache"
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

# stage_static materializes cmd/aurumcode's FULL compile closure -- now in
# this card's own read_paths (internal/analyzer, internal/prompt,
# internal/review, internal/llm, internal/security/redaction,
# internal/git/githubclient, pkg/types, the internal/documentation/... and
# internal/pipeline packages cmd/aurumcode's docs subcommand reuses) -- plus
# the three artifacts the card's Outcome spans and go.mod/go.sum. Unit's own
# proof (go/parser, source text) never needed this closure and still
# doesn't; Integration's proof now also builds and runs the REAL binary
# (`go build ./cmd/aurumcode`) inside this staged root, so it needs the
# closure to be materialized here exactly as it now is in the true sealed
# `oci-run` container.
#
# THIS LIST DUPLICATES `.board/cards/ready/AUR-444.md`'s `read_paths` and
# can drift from it: if a future card adds an import to cmd/aurumcode and
# updates that card's read_paths, this list needs the same addition or the
# staged build here fails `infra` (the source card is the authority; sync
# from there, not from memory of this comment).
stage_static() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/analyzer internal/config internal/prompt internal/review internal/llm \
    internal/security/redaction internal/git/githubclient pkg/types
  copy "$root" internal/documentation/extractors internal/documentation/incremental \
    internal/documentation/normalizer internal/documentation/site \
    internal/documentation/welcome internal/pipeline
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

func TestAUR444UnitBridge(t *testing.T) { TestAUR444(t) }
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
    fail "selector-exit:TestAUR444:$rc"
  fi
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail 'selector:TestAUR444:zero-tests'
  grep -Fq -- '--- PASS: TestAUR444UnitBridge' <<<"$out" || fail 'selector-did-not-run:TestAUR444'
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

func TestAUR444IntegrationBridge(t *testing.T) { IntegrationAUR444(t) }
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
    fail "selector-exit:IntegrationAUR444:$rc"
  fi
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail 'selector:IntegrationAUR444:zero-tests'
  grep -Fq -- '--- PASS: TestAUR444IntegrationBridge' <<<"$out" || fail 'selector-did-not-run:IntegrationAUR444'
  cleanup_root "$root"
}

e2e_case() {
  local root="$run_dir/root-e2e"
  stage_static "$root"
  copy "$root" tests/e2e/AUR-444.sh
  chmod -R u+w -- "$root"
  local rc
  set +e
  ( cd "$root" && bash tests/e2e/AUR-444.sh E2EAUR444 ) >"$run_dir/e2e.out" 2>&1
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

func TestAUR444MutBridge(t *testing.T) { TestAUR444(t) }
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
  TestAUR444) unit_case ;;
  IntegrationAUR444) integration_case ;;
  E2EAUR444) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
