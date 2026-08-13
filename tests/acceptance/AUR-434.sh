#!/usr/bin/env bash
#
# Acceptance program for card AUR-434, scenario AC-001.
#
# WHAT THIS PROVES
#
#   Running `aurumcode review --base HEAD~1` against a git repository with a
#   known planted problem prints every finding WITH the rule of the project
#   review standard that sustains it, and a finding that cannot cite such a
#   rule (missing or unknown rule_id) never reaches the user. The rule
#   catalog is embedded in the binary (internal/review/rules/*.yml, restored
#   from commit c12d7ab's .aurumcode/rules); an empty catalog is a loud
#   error, never a silent zero-rule pass-through.
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sealed profile (bootstrap-readonly-v1) denies network.
#   AURUMCODE_LLM_FIXTURE points the binary's provider selection at canned,
#   deterministic response files; internal/review.FakeProvider implements
#   the same llm.Provider interface a vendor provider would. The ungrounded
#   findings this program plants use fixtures it writes itself.
#
# EXIT CODES:
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
#   64 = unknown scenario selector
#   69 = inconclusive environment (missing tool, missing materialized
#        input, build failure) -- never valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-434'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR434|IntegrationAUR434|E2EAUR434|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Input preflight: this acceptance builds and runs the real binary, so the
# engine's packages and the demo repository must have been materialized.
# A missing input is an environment/contract problem (the card's
# paths/read_paths did not admit it), never behavioral RED.
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/llm
  internal/prompt
  internal/review
  pkg/types
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
  tests/unit/AUR-434.go
  tests/integration/AUR-434.go
  tests/e2e/AUR-434.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a434.XXXXXX")" || infra mktemp
# Cleanup must never turn an already-decided result into a failure: the
# materialized input tree can be read-only, so force write permission back
# on before removing, and never let a residual removal error propagate (see
# tests/acceptance/AUR-430.sh for the original rationale).
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
# this card's owned package plus the read-only packages it imports, and the
# fixtures the CLI is exercised against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/prompt internal/review internal/security
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  # The materialized input this copies from can be read-only, directories
  # included; force the staged copy writable so mutation_case's sed and
  # cleanup_root can operate. The copy is scratch from here on.
  chmod -R u+w -- "$root"
}

# write_fixtures plants this card's own deterministic model responses in
# $1: one response mixing a grounded finding with two ungrounded ones, and
# one response with only an ungrounded finding.
write_fixtures() {
  local dir="$1"
  mkdir -p "$dir"
  cat >"$dir/mixed.json" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "UNGROUNDED-BAD-RULE planted finding citing a nonexistent rule."
    }
  ],
  "summary": "Mixed fixture for AUR-434."
}
EOF
  cat >"$dir/ungrounded.json" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    }
  ],
  "summary": "Only ungrounded findings."
}
EOF
}

# build_shared builds the binary exactly once per acceptance run and reuses
# it for nominal_case and e2e_case; mutation_case rebuilds only its mutated
# copy of internal/review on the same warm GOCACHE (see
# tests/acceptance/AUR-430.sh for why cold per-case builds are avoided
# under the profile's memory ceiling).
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    infra build_failed
  fi
  shared_built=1
}

# nominal_case is AC-001's core behavioral proof: run the built binary
# exactly as a user would. RED before the implementation is behavioral --
# the binary builds and runs, but its findings carry no rule citation --
# and reports `behavior-missing`.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local known_fixture="$shared_root/tests/fixtures/review/known-problem-response.json"
  write_fixtures "$run_dir/fixtures"

  # The known-problem finding (which cites a catalog rule) must print WITH
  # the rule of the project standard that sustains it.
  local out_known
  out_known="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$known_fixture" "$shared_bin" review --base HEAD~1)" || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt' <<<"$out_known" || fail behavior-missing
  grep -Fq '[error]' <<<"$out_known" || fail behavior-missing
  grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' <<<"$out_known" || fail behavior-missing

  # Mixed response: the two ungrounded findings never reach the user; the
  # grounded one survives, cited.
  local out1
  out1="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$run_dir/fixtures/mixed.json" "$shared_bin" review --base HEAD~1)" || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt:3' <<<"$out1" || fail behavior-missing
  grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' <<<"$out1" || fail behavior-missing
  if grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out1"; then fail behavior-missing; fi
  if grep -Fq 'UNGROUNDED-BAD-RULE' <<<"$out1"; then fail behavior-missing; fi

  # Determinism: same input, same output.
  local out2
  out2="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$run_dir/fixtures/mixed.json" "$shared_bin" review --base HEAD~1)" || fail non-deterministic
  [[ "$out1" == "$out2" ]] || fail non-deterministic

  # Every finding ungrounded: the unchanged AUR-430 no-findings output.
  local out3
  out3="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$run_dir/fixtures/ungrounded.json" "$shared_bin" review --base HEAD~1)" || fail behavior-missing
  grep -Fq 'No issues found.' <<<"$out3" || fail behavior-missing
  if grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out3"; then fail behavior-missing; fi
}

# mutation_case is MUT-001: accepting a finding without a cited rule must
# make the acceptance fail. It edits a writable staged copy of
# internal/review/reviewer.go so the rule gate stops discarding ungrounded
# findings (the exact `continue` that rejects them), rebuilds, and proves
# the planted ungrounded finding then DOES reach the user -- i.e. that
# nominal_case's absence assertion is load-bearing. The committed source is
# never touched; the mutation exists only in this case's own staged copy.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.
  write_fixtures "$run_dir/fixtures"

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/internal/review/reviewer.go"
  local anchor
  anchor="$(printf '\t\t\tcontinue')"
  [[ "$(grep -c "^${anchor}\$" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  sed -i "s/^${anchor}\$/\t\t\t_ = issue \/\/ MUT-001: accept finding without rule/" "$target"
  grep -Fq 'MUT-001: accept finding without rule' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  local out
  out="$(cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$run_dir/fixtures/mixed.json" "$bin" review --base HEAD~1)" || fail 'MUT-001/mutation-run-failed'
  # The mutant accepts the ungrounded finding, so it must surface in the
  # output; nominal_case's absence assertion is what catches it. A mutant
  # whose output still hides the finding means the gate was not what
  # rejected it -- the mutation survived, so the acceptance fails.
  grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out" || fail 'MUT-001/not-rejected'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-434.go
  cat >"$root/tests/unit/aur434_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR434UnitBridge(t *testing.T) { TestAUR434(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR434UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR434:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR434:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-434.go
  cat >"$root/tests/integration/aur434_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR434IntegrationBridge(t *testing.T) { IntegrationAUR434(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR434IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR434:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR434:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-434.sh
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-434.sh E2EAUR434) || fail e2e-failed
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
  TestAUR434) unit_case ;;
  IntegrationAUR434) integration_case ;;
  E2EAUR434) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
