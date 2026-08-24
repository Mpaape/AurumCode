#!/usr/bin/env bash
#
# Acceptance program for card AUR-448, scenario AC-001.
#
# WHAT THIS PROVES
#
#   A user who follows the no-provider-configured message literally can no
#   longer be told "No issues found." with exit 0 while a real finding was
#   silently discarded by the AUR-434 rule gate. Two things, both required:
#
#     1. `selectProvider`'s message (cmd/aurumcode/main.go) shows the
#        COMPLETE fixture shape the engine accepts -- rule_id included,
#        with a real id from the embedded catalog (security/hardcoded-
#        secret) as the example -- instead of the pre-AUR-448 shape that
#        omitted rule_id.
#     2. Whenever internal/review.enforceRuleCitations (AUR-434) discards
#        one or more findings for a missing or unknown rule_id,
#        `aurumcode review` names how many and why on stderr. The happy
#        path (every finding cites a resolvable rule, including "the model
#        reported none") is byte-identical on BOTH stdout and stderr: the
#        warning line appears only when something was actually discarded.
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sealed profile (bootstrap-readonly-v1) denies network.
#   AURUMCODE_LLM_FIXTURE points the binary's provider selection at canned,
#   deterministic response files; internal/review.FakeProvider implements
#   the same llm.Provider interface a vendor provider would.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
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

readonly card='AUR-448'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR448|IntegrationAUR448|E2EAUR448|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Input preflight: this acceptance builds and runs the real binary, so the
# engine's packages and the demo repository must have been materialized. A
# missing input is an environment/contract problem (the card's
# paths/read_paths did not admit it), never behavioral RED.
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/config
  internal/llm
  internal/prompt
  internal/review
  internal/security
  internal/git
  internal/documentation/extractors
  internal/documentation/incremental
  internal/documentation/normalizer
  internal/documentation/site
  internal/documentation/welcome
  internal/pipeline
  cmd/regenerate-docs
  pkg/types
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
  tests/unit/AUR-448.go
  tests/integration/AUR-448.go
  tests/e2e/AUR-448.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a448.XXXXXX")" || infra mktemp
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

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 (single build/test process) for every go
# invocation in this file.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" )
}

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs:
# this card's owned packages plus the read-only packages it imports
# (cmd/aurumcode is shared with concurrently-dispatched cards -- pr.go
# imports internal/git/githubclient, docs.go imports
# internal/documentation/* and internal/pipeline -- both have to resolve),
# plus the fixtures the CLI is exercised against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode cmd/regenerate-docs
  copy "$root" internal/analyzer internal/config internal/prompt internal/review internal/security internal/llm internal/git internal/pipeline
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  # The materialized input this copies from can be read-only, directories
  # included; force the staged copy writable so mutation_case's sed and
  # cleanup_root can operate. The copy is scratch from here on.
  chmod -R u+w -- "$root"
}

# write_fixtures plants this card's own deterministic model responses in
# $1: a mixed response (one grounded finding, two ungrounded), an
# all-discarded response (one ungrounded finding, the exact repro of the
# defect this card fixes), and an all-grounded response (the happy path,
# reusing zero-discard semantics but distinct from known-problem-
# response.json so nominal_case is not solely dependent on a shared
# fixture).
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
      "message": "grounded"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "unknown rule_id"
    }
  ],
  "summary": "Mixed fixture for AUR-448."
}
EOF
  cat >"$dir/all-discarded.json" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all"
    }
  ],
  "summary": "All-discarded fixture for AUR-448."
}
EOF
}

# build_shared builds the binary exactly once per acceptance run and reuses
# it for nominal_case and e2e_case; mutation_case rebuilds only its mutated
# copy on the same warm GOCACHE (see tests/acceptance/AUR-430.sh for why
# cold per-case builds are avoided under the profile's memory ceiling).
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! run_go "$shared_root" build -o "$shared_bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    infra build_failed
  fi
  shared_built=1
}

# run_bin runs the built binary as a user would, from the given directory,
# with a clean provider/canary/cache-less environment plus any extra
# KEY=VALUE pairs given anywhere among the remaining arguments. Raw exit
# code lands in rc; stdout/stderr land in $run_dir/out.{stdout,stderr} --
# NEVER a combined capture: stdout is a raw *os.File and stderr travels
# through the AUR-432 redaction writer with its own Flush in
# cmd/aurumcode/main.go's run(), so their relative interleaving is not
# guaranteed and this program never assumes it.
run_bin() {
  local bin="$1" dir="$2"; shift 2
  local extra_env=() bin_args=() a
  for a in "$@"; do
    if [[ "$a" =~ ^[A-Za-z_][A-Za-z0-9_]*=.*$ ]]; then
      extra_env+=("$a")
    else
      bin_args+=("$a")
    fi
  done
  set +e
  (cd "$dir" && env -u AURUM_SECRET_CANARY -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u AURUMCODE_CACHE_DIR \
    "${extra_env[@]}" "$bin" "${bin_args[@]}") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# nominal_case is AC-001's core behavioral proof: run the built binary
# exactly as a user would.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local known_fixture="$shared_root/tests/fixtures/review/known-problem-response.json"
  write_fixtures "$run_dir/fixtures"

  # --- 1. No provider configured: the message shows the COMPLETE shape. ---
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1
  [[ "$rc" -eq 1 ]] || fail behavior-missing
  [[ -s "$run_dir/out.stdout" ]] && fail behavior-missing
  grep -Fq 'no LLM provider configured' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq '"issues"' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq '"file"' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq '"line"' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq '"severity"' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq '"rule_id"' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq '"message"' "$run_dir/out.stderr" || fail behavior-missing
  # The example must be a REAL id of the embedded catalog, not a
  # placeholder: it is exercised below (mixed.json cites it for its
  # surviving finding) and resolved by internal/review's own rules_test.go.
  grep -Fq 'security/hardcoded-secret' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq 'discarded' "$run_dir/out.stderr" || fail behavior-missing
  grep -Fq 'tests/fixtures/review/known-problem-response.json' "$run_dir/out.stderr" || fail behavior-missing

  # --- 2. Happy path: zero discards. Byte-identical stdout, EMPTY stderr. ---
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$known_fixture"
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  local want_happy='config/demo-tokens.txt:4: [error] A credential-shaped value was committed in plain text (DEMO_API_TOKEN). (rule security/hardcoded-secret: Hardcoded Secrets)'
  [[ "$(cat "$run_dir/out.stdout")" == "$want_happy" ]] || fail behavior-missing
  [[ ! -s "$run_dir/out.stderr" ]] || fail happy-path-stderr-not-empty

  # --- 3. Mixed discard: stdout hides ungrounded findings; stderr names how many and why. ---
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/mixed.json"
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt:3' "$run_dir/out.stdout" || fail behavior-missing
  grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' "$run_dir/out.stdout" || fail behavior-missing
  if grep -Fq 'no rule_id at all' "$run_dir/out.stdout"; then fail behavior-missing; fi
  if grep -Fq 'unknown rule_id' "$run_dir/out.stdout"; then fail behavior-missing; fi
  local want_mixed_stderr='aurumcode review: 2 finding(s) discarded: 1 with no rule_id, 1 citing an unknown rule_id (security/definitely-not-a-rule)'
  [[ "$(cat "$run_dir/out.stderr")" == "$want_mixed_stderr" ]] || fail behavior-missing

  # Determinism: same input, same stdout AND same stderr.
  local mixed_out1 mixed_err1 mixed_out2 mixed_err2
  mixed_out1="$(cat "$run_dir/out.stdout")"; mixed_err1="$(cat "$run_dir/out.stderr")"
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/mixed.json"
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  mixed_out2="$(cat "$run_dir/out.stdout")"; mixed_err2="$(cat "$run_dir/out.stderr")"
  [[ "$mixed_out1" == "$mixed_out2" ]] || fail non-deterministic
  [[ "$mixed_err1" == "$mixed_err2" ]] || fail non-deterministic

  # --- 4. Every finding discarded: the exact defect this card fixes. ---
  # Before AUR-448: "No issues found." with NOTHING on stderr -- a
  # confidently wrong "your code is clean" for a fixture that planted a
  # real finding.
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/all-discarded.json"
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  [[ "$(cat "$run_dir/out.stdout")" == "No issues found." ]] || fail behavior-missing
  local want_all_discarded_stderr='aurumcode review: 1 finding(s) discarded: 1 with no rule_id'
  [[ "$(cat "$run_dir/out.stderr")" == "$want_all_discarded_stderr" ]] || fail behavior-missing
}

# mutation_case is MUT-001: accepting a silent discard (the discard still
# happens, but the stderr explanation is suppressed) must make the
# acceptance fail. It edits a writable staged copy of
# cmd/aurumcode/main.go so the discard-warning Fprintf is replaced with a
# no-op that still "uses" the warning variable (so the mutant compiles),
# rebuilds, and proves BOTH halves of the isolation this mutation targets:
# the mutant's stdout STILL hides the discarded finding exactly like the
# unmutated binary (internal/review.enforceRuleCitations, the underlying
# gate, is untouched by this mutation) while its stderr NO LONGER explains
# why -- i.e. that nominal_case's stderr assertion is load-bearing for
# exactly the silence this card's Outcome exists to close, not for the
# gate AUR-434 already proved. The committed source is never touched; the
# mutation exists only in this case's own staged copy.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.
  write_fixtures "$run_dir/fixtures"

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/main.go"
  local anchor
  anchor='fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # A literal (not regex) substring replace via awk's index/substr, so the
  # anchor's own regex metacharacters (the parentheses, the quotes) never
  # need escaping -- bash and awk are the only tools this profile
  # guarantees (bootstrap-readonly-v1: Go and bash, no git, per
  # docs/specs/AUR-443.md's own note). `_ = warning` keeps the mutant
  # compiling (the identifier stays used); the print itself is gone, so
  # the discard becomes observably SILENT while the underlying gate
  # (untouched) keeps discarding.
  local replacement='_ = warning // MUT-001: suppress the discard warning silently'
  # ENVIRON, not -v: awk's -v (and command-line var=value) assignments
  # process C-style backslash escapes, which would silently turn the
  # anchor's literal `\n` (two bytes, matching the Go source's own
  # "%s\n") into an actual newline and break the match. ENVIRON reads the
  # raw environment value instead.
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    {
      idx = index($0, anchor)
      if (idx > 0) {
        print substr($0, 1, idx - 1) repl substr($0, idx + length(anchor))
      } else {
        print $0
      }
    }
  ' "$target" > "$target.mut" && mv "$target.mut" "$target"
  grep -Fq 'MUT-001: suppress the discard warning silently' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  run_bin "$bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/all-discarded.json"
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/mutation-run-failed'
  # The gate itself is untouched: the discarded finding still never reaches
  # stdout, and "No issues found." still prints -- proving this mutation is
  # isolated to the warning, not a second copy of AUR-434's own mutation.
  [[ "$(cat "$run_dir/out.stdout")" == "No issues found." ]] || fail 'MUT-001/gate-also-mutated'
  # The warning is gone: this is the silent discard nominal_case's own
  # stderr assertion (case 4 above) would have caught.
  [[ -s "$run_dir/out.stderr" ]] && fail 'MUT-001/not-rejected'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-448.go
  cat >"$root/tests/unit/aur448_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR448UnitBridge(t *testing.T) { TestAUR448(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR448UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR448:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR448:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-448.go
  cat >"$root/tests/integration/aur448_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR448IntegrationBridge(t *testing.T) { IntegrationAUR448(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR448IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR448:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR448:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-448.sh
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-448.sh E2EAUR448)
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
  TestAUR448) unit_case ;;
  IntegrationAUR448) integration_case ;;
  E2EAUR448) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
