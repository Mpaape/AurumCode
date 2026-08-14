#!/usr/bin/env bash
#
# Acceptance program for card AUR-431, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode review --base HEAD~1 --fail-on high` exits with the distinct,
#   documented exit code 3 when the review reports a finding at the chosen
#   severity or above, and exits 0 when no finding reaches the threshold --
#   so the review can gate a CI pipeline. Without `--fail-on`, the command
#   keeps AUR-430's published contract byte for byte: same stdout, exit 0.
#
# SEVERITY LEVELS
#
#   The engine emits exactly three severities (internal/prompt/parser.go
#   validates them): error, warning, info. `--fail-on` accepts those names
#   and the CI-conventional aliases high|medium|low (see
#   docs/specs/AUR-431.md for the mapping table).
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sandbox denies network. AURUMCODE_LLM_FIXTURE points the binary's
#   provider selection (see cmd/aurumcode/main.go's selectProvider) at a
#   deterministic response file; this program writes its own response
#   fixtures (one per severity) into its scratch dir, so the only external
#   fixture it needs is the tests/fixtures/repos/git-demo repository.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure: an input this card does not own was
#        never materialized, a required tool is missing. Never valid red
#        evidence, never a pass.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-431'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR431|IntegrationAUR431|E2EAUR431|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a431.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: the staged copies below
# preserve the read-only modes of the materialized input tree, so force
# write permission back before removing, and never let a residual removal
# error decide the exit code.
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

# copy materializes one repo path into a staged root. Every path it copies
# is either owned by this card (paths) or read by it (read_paths of this or
# a dependency card); a missing source is an input this card does not own
# that was never materialized -- an environment gap, never a verdict.
copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` and the
# review run need: this card's owned cmd/aurumcode plus the read-only
# packages it imports and the git fixture the CLI is exercised against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/git internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/pipeline
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/analyzer internal/prompt internal/review internal/security internal/llm pkg/types
  copy "$root" tests/fixtures/repos/git-demo
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# Deterministic offline model responses, one per severity the engine can
# emit. Written here, not read from the repo, so the acceptance depends on
# no response fixture it does not fully control.
fixture_error="$run_dir/response-error.json"
fixture_info="$run_dir/response-info.json"
cat >"$fixture_error" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN)."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF
cat >"$fixture_info" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "info",
      "rule_id": "quality/magic-numbers",
      "message": "Consider documenting where this demo value comes from."
    }
  ],
  "summary": "Only informational notes."
}
EOF

# build_shared builds the binary once per acceptance run and reuses it for
# nominal_case and e2e_case; GOCACHE is shared process-wide, so the go test
# compiles and the mutation rebuild start warm instead of cold (the sealed
# profile's 256MB memory ceiling is tight for cold net/http+crypto/tls
# builds -- see tests/acceptance/AUR-430.sh's build_shared note).
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

# run_review runs the built binary as a user would (cwd inside the reviewed
# repository) and reports its raw exit code in the global rc without
# tripping errexit. stdout/stderr land in $run_dir/out.{stdout,stderr}.
run_review() {
  local bin="$1" repo_dir="$2" fixture="$3"; shift 3
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# nominal_case is AC-001's core behavioral proof: the gate fires with the
# distinct exit code 3 exactly when a finding reaches the threshold, and
# the pre-existing no-flag contract is untouched.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local rc

  # An error-severity finding at --fail-on high: exit 3, finding printed.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on high
  [[ "$rc" -eq 3 ]] || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail behavior-missing
  grep -Fq '[error]' "$run_dir/out.stdout" || fail behavior-missing
  local gated_stdout
  gated_stdout="$(cat "$run_dir/out.stdout")"

  # Same input, same output, same exit: the gate is deterministic.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on high
  [[ "$rc" -eq 3 ]] || fail non-deterministic
  [[ "$(cat "$run_dir/out.stdout")" == "$gated_stdout" ]] || fail non-deterministic

  # Without --fail-on, AUR-430's published contract is untouched: exit 0
  # and byte-identical stdout (the gate note goes to stderr, never stdout).
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail no-flag-contract-broken
  [[ "$(cat "$run_dir/out.stdout")" == "$gated_stdout" ]] || fail no-flag-stdout-changed

  # "at the chosen severity or above": an error finding also trips the
  # lowest threshold.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on low
  [[ "$rc" -eq 3 ]] || fail at-or-above-not-honored

  # An info-only review below the threshold must NOT trip the gate.
  run_review "$shared_bin" "$repo_dir" "$fixture_info" --base HEAD~1 --fail-on high
  [[ "$rc" -eq 0 ]] || fail false-trigger-below-threshold
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail below-threshold-lost-findings

  # An unknown level is a usage error (2), never a silent pass or gate.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on catastrophic
  [[ "$rc" -eq 2 ]] || fail invalid-level-not-rejected

  # A present-but-empty level (`--fail-on "$VAR"` with VAR empty in CI) is
  # the same usage error, never a silently-open gate that exits 0 despite
  # the error finding.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on ''
  [[ "$rc" -eq 2 ]] || fail empty-level-not-rejected
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on=
  [[ "$rc" -eq 2 ]] || fail empty-level-not-rejected
}

# mutation_case is MUT-001: downgrade the severity of every finding in the
# gate's eyes (severityRank(issue.Severity) -> severityRank("info")) in a
# writable staged copy, rebuild, and prove the gated command then exits 0
# again -- i.e. that nominal_case's exit 3 is actually produced by reading
# each finding's severity, not a coincidence. The committed source is never
# touched, so restoration is by construction: shared_bin still behaves GREEN.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild below recompiles only cmd/aurumcode.

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/main.go"
  local anchor='if severityRank(issue.Severity) >= threshold {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # issue.Severity[:0]+"info" keeps the loop variable in use (so the mutant
  # compiles -- a compile failure would prove nothing) while downgrading the
  # severity every finding presents to the gate to "info".
  sed -i 's/severityRank(issue\.Severity) >= threshold/severityRank(issue.Severity[:0]+"info") >= threshold/' "$target"
  grep -Fq 'severityRank(issue.Severity[:0]+"info") >= threshold' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  local rc
  run_review "$bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on high
  [[ "$rc" -ne 3 ]] || fail 'MUT-001/not-rejected'
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/unexpected-exit'
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail 'MUT-001/unexpected-output'

  # Restoration: the unmutated binary still gates -- the GREEN reproduces.
  run_review "$shared_bin" "$repo_dir" "$fixture_error" --base HEAD~1 --fail-on high
  [[ "$rc" -eq 3 ]] || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-431.go
  cat >"$root/tests/unit/aur431_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR431UnitBridge(t *testing.T) { TestAUR431(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR431UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR431:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR431:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-431.go
  cat >"$root/tests/integration/aur431_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR431IntegrationBridge(t *testing.T) { IntegrationAUR431(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR431IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR431:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR431:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-431.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-build its own copy. The
  # nested script's own exit-code vocabulary is preserved: its 79 is an
  # environment gap and must be re-emitted as infra here, never collapsed
  # into behavioral RED (see EXIT_CODE_CONVENTION.md).
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-431.sh E2EAUR431)
  rc=$?
  set -e
  ((rc != 79)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
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
  TestAUR431) unit_case ;;
  IntegrationAUR431) integration_case ;;
  E2EAUR431) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
