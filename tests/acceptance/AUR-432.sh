#!/usr/bin/env bash
#
# Acceptance program for card AUR-432, scenario AC-001.
#
# WHAT THIS PROVES
#
#   A secret present in the code reviewed by `aurumcode review --base
#   HEAD~1` never reaches the model, stdout, stderr, the prompt capture,
#   or the report -- while the finding that cites the secret's line still
#   arrives, because the redaction replaces values, not context. Three
#   wiring points carry the guarantee: the assembled prompt is redacted
#   before any provider receives it (internal/review), the model's parsed
#   output is redacted at the same boundary before it can reach a sink,
#   and the stderr channel (including the --modelo endpoint note, fixed at
#   origin with url.Redacted) writes through the AUR-009 redaction writer.
#   For a secret-free review the published output format of
#   AUR-430/431/436 is byte-identical, pinned literally below.
#
# HOW THE SECRET IS OBSERVED
#
#   The sandbox denies network, so the model is the deterministic offline
#   provider (AURUMCODE_LLM_FIXTURE, review.FakeProvider). What that
#   provider RECEIVES is captured via AURUMCODE_PROMPT_CAPTURE -- the
#   exact bytes a real vendor provider would have been handed -- and the
#   planted synthetic values of tests/fixtures/repos/git-demo must not be
#   in them. The wire-level equivalent (raw bytes reaching a local
#   loopback endpoint) is proved by tests/integration/AUR-432.go.
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

readonly card='AUR-432'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR432|IntegrationAUR432|E2EAUR432|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a432.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: staged copies preserve
# read-only modes of the materialized input tree; force write permission
# back before removing, and never let a residual removal error decide the
# exit code.
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

# The three synthetic values the demo repository's HEAD commit plants in
# config/demo-tokens.txt (see tests/fixtures/repos/git-demo/manifest.json,
# synthetic_secrets). The webhook value doubles as the registered canary.
readonly planted_token='AURUM-FAKE-TOKEN-0000-0001'
readonly planted_password='AURUM-FAKE-PASSWORD-0000-0002'
readonly planted_webhook='AURUM-FAKE-WEBHOOK-0000-0003'

# copy materializes one repo path into a staged root. Every path it copies
# is either owned by this card (paths) or read by it; a missing source is
# an input this card does not own that was never materialized -- an
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

# stage_source materializes exactly what `go build ./cmd/aurumcode` and the
# review run need: this card's owned internal/review and cmd/aurumcode,
# the read-only packages they import (including internal/security/redaction,
# the filter this card wires in), and the fixtures the CLI is exercised
# against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/review
  copy "$root" internal/git internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/pipeline
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/analyzer internal/prompt internal/llm pkg/types
  copy "$root" internal/security/redaction
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review/secret
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# Secret-free deterministic responses, written here so the byte-identity
# pins depend on nothing this program does not fully control. The
# adversarial secret-echoing response is this card's tracked fixture,
# tests/fixtures/review/secret/response-echoes-secret.json.
fixture_clean="$run_dir/response-clean.json"
cat >"$fixture_clean" <<'EOF'
{
  "issues": [],
  "summary": "Nothing to report."
}
EOF

# build_shared builds the binary once per acceptance run and reuses it for
# every case; GOCACHE is shared process-wide so later compiles start warm
# (the sealed profile's 256MB ceiling is tight for cold builds -- see
# tests/acceptance/AUR-430.sh's build_shared note).
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

# run_review runs a built binary as a user would (cwd inside the reviewed
# repository) with the offline provider, the prompt capture, and the
# canary registration configured, and reports the raw exit code in the
# global rc. stdout/stderr land in $run_dir/out.{stdout,stderr}, the
# captured prompt in $run_dir/prompt.txt.
run_review() {
  local bin="$1" repo_dir="$2" fixture="$3"; shift 3
  set +e
  (cd "$repo_dir" && \
    AURUMCODE_LLM_FIXTURE="$fixture" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/prompt.txt" \
    AURUM_SECRET_CANARY="$planted_webhook" \
    "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# run_review_nocanary is run_review without the registered canary, proving
# the structural rules alone already keep the planted values out of the
# prompt -- the guarantee does not depend on knowing the secret up front.
run_review_nocanary() {
  local bin="$1" repo_dir="$2" fixture="$3"; shift 3
  set +e
  (cd "$repo_dir" && \
    AURUMCODE_LLM_FIXTURE="$fixture" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/prompt.txt" \
    AURUM_SECRET_CANARY= \
    "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# assert_no_secret fails with the given marker if any planted value exists
# in the given file. Typed markers only: this program never echoes a
# planted value to its own output.
assert_no_secret() {
  local file="$1" marker="$2"
  local value
  for value in "$planted_token" "$planted_password" "$planted_webhook"; do
    if grep -Fq -- "$value" "$file"; then fail "$marker"; fi
  done
}

# nominal_case is AC-001's core behavioral proof.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local echo_fixture="$shared_root/tests/fixtures/review/secret/response-echoes-secret.json"
  local cite_fixture="$shared_root/tests/fixtures/review/secret/response-cites-secret-line.json"
  local rc

  # The adversarial case: the reviewed diff plants three synthetic secrets
  # and the model echoes one back. Without the AUR-432 wiring the raw
  # values reach the prompt and stdout -- the exact behavioral RED this
  # card starts from -- so every one of these first assertions reports
  # behavior-missing.
  run_review "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  assert_no_secret "$run_dir/out.stdout" behavior-missing
  test -s "$run_dir/prompt.txt" || fail behavior-missing
  assert_no_secret "$run_dir/prompt.txt" behavior-missing
  assert_no_secret "$run_dir/out.stderr" behavior-missing
  grep -Fq '[REDACTED]' "$run_dir/prompt.txt" || fail behavior-missing

  # The redaction must not destroy the review: the prompt still names the
  # file and the keys, and the finding citing the secret's line still
  # prints, in the exact published AUR-430 format with the AUR-434 rule
  # citation intact.
  grep -Fq 'config/demo-tokens.txt' "$run_dir/prompt.txt" || fail review-context-destroyed
  grep -Fq 'DEMO_API_TOKEN=' "$run_dir/prompt.txt" || fail review-context-destroyed
  local expected_echo='config/demo-tokens.txt:6: [error] Remove the hardcoded webhook credential DEMO_WEBHOOK_SECRET=[REDACTED] and rotate it. (rule security/hardcoded-secret: Hardcoded Secrets)'
  [[ "$(cat "$run_dir/out.stdout")" == "$expected_echo" ]] || fail published-format-drifted
  local echo_stdout echo_prompt
  echo_stdout="$(cat "$run_dir/out.stdout")"
  echo_prompt="$(cat "$run_dir/prompt.txt")"

  # Same input, same redacted prompt, same output: deterministic.
  run_review "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  [[ "$(cat "$run_dir/out.stdout")" == "$echo_stdout" ]] || fail non-deterministic
  [[ "$(cat "$run_dir/prompt.txt")" == "$echo_prompt" ]] || fail non-deterministic

  # The structural rules alone (no registered canary) already keep the
  # planted values out of the prompt.
  run_review_nocanary "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail nocanary-run-failed
  assert_no_secret "$run_dir/prompt.txt" structural-rules-insufficient
  assert_no_secret "$run_dir/out.stdout" structural-rules-insufficient

  # A well-behaved model response over the same secret-bearing diff pins
  # the published secret-free report format byte for byte -- the AUR-430
  # contract survives the wiring untouched.
  run_review "$shared_bin" "$repo_dir" "$cite_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail cite-run-failed
  local expected_cite='config/demo-tokens.txt:4: [error] A hardcoded credential is committed in plain text at this line. (rule security/hardcoded-secret: Hardcoded Secrets)'
  [[ "$(cat "$run_dir/out.stdout")" == "$expected_cite" ]] || fail published-format-drifted
  run_review "$shared_bin" "$repo_dir" "$fixture_clean" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail clean-run-failed
  [[ "$(cat "$run_dir/out.stdout")" == 'No issues found.' ]] || fail published-format-drifted

  # The published composition contracts survive: the AUR-431 gate still
  # closes on the echoed finding, and the AUR-436 selection note still
  # names the chosen model on stderr -- with no secret next to it.
  run_review "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1 --fail-on high
  [[ "$rc" -eq 3 ]] || fail gate-composition-broken
  run_review "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1 --modelo local
  [[ "$rc" -eq 0 ]] || fail modelo-composition-broken
  grep -Fq 'reviewing with model "local"' "$run_dir/out.stderr" || fail modelo-composition-broken
  assert_no_secret "$run_dir/out.stderr" secret-on-stderr
}

# mutation_case is MUT-001, exercised at both redaction points: disable
# the send-path diff redaction and prove the planted secret then reaches
# the provider prompt; separately disable the output-boundary redaction
# and prove the model-echoed secret then reaches stdout. Each is the exact
# leak nominal_case rejects, so the same accept can never pass with either
# defect in place. The committed source is never touched: restoration is
# by construction, and the unmutated shared binary reproduces the GREEN
# exactly after each variant.
mutation_case() {
  build_shared # warm GOCACHE; the rebuilds below recompile only what changed.
  local repo_dir echo_fixture rc

  # Variant A: send path. reviewer.go documents this line as MUT-001's
  # target.
  local root="$run_dir/root-mut"
  stage_source "$root"
  local target="$root/internal/review/reviewer.go"
  local anchor='diff = redactDiff(r.filter, diff)'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  sed -i 's|diff = redactDiff(r\.filter, diff)|_ = redactDiff // MUT-001: send-path redaction disabled|' "$target"
  grep -Fq 'MUT-001: send-path redaction disabled' "$target" || fail 'MUT-001/mutation-not-applied'
  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi
  repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  echo_fixture="$root/tests/fixtures/review/secret/response-echoes-secret.json"
  run_review "$bin" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/mutant-run-failed'
  test -s "$run_dir/prompt.txt" || fail 'MUT-001/mutant-capture-missing'
  # Under the mutant the planted secret reaches the provider prompt -- the
  # exact condition nominal_case's behavior-missing assertions reject.
  grep -Fq -- "$planted_token" "$run_dir/prompt.txt" || fail 'MUT-001/not-rejected'
  # Restoration: the unmutated binary keeps the secret out of the prompt.
  run_review "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/restoration-broken'
  assert_no_secret "$run_dir/prompt.txt" 'MUT-001/restoration-broken'
  cleanup_root "$root"

  # Variant B: output boundary. Disabling redactReviewResult lets the
  # model-echoed secret through to stdout.
  local root_b="$run_dir/root-mut-b"
  stage_source "$root_b"
  local target_b="$root_b/internal/review/reviewer.go"
  local anchor_b='redactReviewResult(r.filter, result)'
  [[ "$(grep -Fc "$anchor_b" "$target_b")" == 1 ]] || fail 'MUT-001/output-anchor-not-unique'
  sed -i 's|redactReviewResult(r\.filter, result)|_ = result // MUT-001: output-boundary redaction disabled|' "$target_b"
  grep -Fq 'MUT-001: output-boundary redaction disabled' "$target_b" || fail 'MUT-001/output-mutation-not-applied'
  local bin_b="$run_dir/aurumcode-mut-b"
  local log_b="$root_b/build-mut-b.log"
  if ! (cd "$root_b" && go build -o "$bin_b" ./cmd/aurumcode) >"$log_b" 2>&1; then
    cat "$log_b" >&2
    fail 'MUT-001/output-build-failed'
  fi
  repo_dir="$root_b/tests/fixtures/repos/git-demo/repo.git"
  echo_fixture="$root_b/tests/fixtures/review/secret/response-echoes-secret.json"
  run_review "$bin_b" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/output-mutant-run-failed'
  # Under this mutant the echoed webhook value reaches stdout -- the exact
  # condition nominal_case's behavior-missing assertions reject.
  grep -Fq -- "$planted_webhook" "$run_dir/out.stdout" || fail 'MUT-001/output-not-rejected'
  # Restoration: the unmutated binary keeps stdout clean.
  run_review "$shared_bin" "$repo_dir" "$echo_fixture" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail 'MUT-001/output-restoration-broken'
  assert_no_secret "$run_dir/out.stdout" 'MUT-001/output-restoration-broken'
  cleanup_root "$root_b"

  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-432.go
  cat >"$root/tests/unit/aur432_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR432UnitBridge(t *testing.T) { TestAUR432(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR432UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR432:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR432:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-432.go
  cat >"$root/tests/integration/aur432_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR432IntegrationBridge(t *testing.T) { IntegrationAUR432(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" GOMAXPROCS=1 go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR432IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR432:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR432:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-432.sh
  # Reuse the already-built binary and the warm GOCACHE instead of letting
  # the nested script cold-build its own copy. The nested script's exit
  # vocabulary is preserved: its 79 is an environment gap and is re-emitted
  # as infra here, never collapsed into behavioral RED.
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-432.sh E2EAUR432)
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
  TestAUR432) unit_case ;;
  IntegrationAUR432) integration_case ;;
  E2EAUR432) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
