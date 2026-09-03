#!/usr/bin/env bash
#
# Acceptance program for card AUR-433, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode review --base HEAD~1 --limite <usd>` lets the user know and
#   cap how much a single review may cost: it prints the estimated cost
#   before the model is ever called, refuses -- calling the model zero
#   times, spending nothing -- when that estimate exceeds --limite, and
#   after a run it went ahead with, prints the actual cost too. Without
#   --limite, the AUR-430/431/435/436 contracts hold byte for byte.
#
# HOW "ZERO CALLS" IS PROVEN OFFLINE (MUT-001)
#
#   The sandbox denies network, so the model is the deterministic offline
#   fixture provider (AURUMCODE_LLM_FIXTURE, review.FakeProvider). That
#   provider already supports AURUMCODE_PROMPT_CAPTURE (wired in
#   cmd/aurumcode/main.go's selectProvider/selectProviderForModel, unchanged
#   by this card): when set, every call to the model writes the exact
#   prompt it received to that path before answering. With one provider and
#   no fallback configured, the file's existence after a run IS the call
#   counter this card's MUT-001 requires: absent means zero calls, present
#   means at least one. tests/integration/AUR-433.go additionally counts
#   real HTTP requests received by a loopback endpoint, the literal numeric
#   form of the same proof.
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

readonly card='AUR-433'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR433|IntegrationAUR433|E2EAUR433|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a433.XXXXXX")" || infra mktemp
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

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 (single build/test process) for every go
# invocation in this file -- the shared cmd/aurumcode closure this card now
# stages (internal/documentation/*, internal/pipeline, cmd/regenerate-docs)
# is large enough that unbounded build parallelism gets the compiler
# OOM-killed under the sealed profile's memory ceiling; -p=1 plus the
# GOMEMLIMIT/ulimit pair in run_go keeps peak memory bounded instead.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

# run_go <dir> <go-args...> runs `go` inside dir with the memory ceiling and
# GOMEMLIMIT the card requires, isolated to a subshell so the ulimit does not
# leak into the rest of this script (e.g. the `cp -R` staging below).
run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" )
}

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

# stage_source materializes what `go build ./cmd/aurumcode` and the review
# run need. cmd/aurumcode is one Go package shared with concurrently
# dispatched cards: AUR-438 (integrated) added cmd/aurumcode/pr.go, which
# imports internal/git/githubclient, and AUR-426 (integrated) added
# cmd/aurumcode/docs.go, which imports internal/documentation/* and
# internal/pipeline (and, transitively, github.com/gorilla/mux). Every one
# of those imports has to resolve for `go build ./cmd/aurumcode` to succeed
# against the integrated tree, even though this card owns none of their
# behavior -- mirroring how AUR-426's own acceptance stages the same
# directory (tests/acceptance/AUR-426.sh's stage_source).
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/git
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/documentation/review
  copy "$root" internal/pipeline
  copy "$root" action.yml
  copy "$root" internal/analyzer internal/config internal/prompt internal/review internal/security internal/llm pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/docs/goproject tests/fixtures/scm/github
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# Deterministic offline model response, written here rather than read from
# the repo, so this acceptance depends on no response fixture it does not
# fully control. One warning finding, in the shape
# internal/prompt.ResponseParser validates, citing an embedded rule so it
# survives the AUR-434 rule-citation gate.
fixture="$run_dir/response.json"
cat >"$fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "warning",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN)."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF

# build_shared builds the binary once per acceptance run and reuses it for
# every case; GOCACHE is shared process-wide, so the go test compiles and
# the mutation rebuild start warm instead of cold (see
# tests/acceptance/AUR-430.sh's build_shared note).
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! run_go "$shared_root" build -o "$shared_bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# run_review runs the built binary as a user would (cwd inside the reviewed
# repository), with the offline provider configured. Raw exit code lands in
# the global rc without tripping errexit; stdout/stderr land in
# $run_dir/out.{stdout,stderr}.
run_review() {
  local bin="$1" repo_dir="$2"; shift 2
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# run_review_captured is run_review plus AURUMCODE_PROMPT_CAPTURE pointed at
# a fresh path: the file's existence afterward is this card's call counter
# (see the header note above). capture_path is emptied first so a stale
# file from an earlier case can never be mistaken for this run's evidence.
run_review_captured() {
  local bin="$1" repo_dir="$2" capture_path="$3"; shift 3
  rm -f -- "$capture_path"
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$capture_path" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# nominal_case is AC-001's core behavioral proof.
nominal_case() {
  build_shared
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local rc

  # Baseline: no --limite, exit 0, the finding prints, and stdout is the
  # published AUR-430 contract this card must not disturb.
  run_review "$shared_bin" "$repo_dir" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail baseline-broken
  local baseline_stdout baseline_stdout_sha baseline_stderr
  baseline_stdout="$(cat "$run_dir/out.stdout")"
  baseline_stdout_sha="$(sha256sum "$run_dir/out.stdout" | awk '{print $1}')"
  baseline_stderr="$(cat "$run_dir/out.stderr")"
  grep -Fq 'config/demo-tokens.txt' <<<"$baseline_stdout" || fail baseline-missing-finding

  # --limite well above the cost: the review proceeds, exit 0, the finding
  # still prints, and two cost lines land on stderr -- estimated first
  # (before the model can have been called), then actual.
  run_review_captured "$shared_bin" "$repo_dir" "$run_dir/capture-allow.txt" --base HEAD~1 --limite 0.50
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail behavior-missing
  grep -Eq 'estimated cost \$[0-9]+\.[0-9]{4}, diff-only pre-flight \(--limite \$0\.5000\)' "$run_dir/out.stderr" || fail behavior-missing
  grep -Eq 'actual cost \$[0-9]+\.[0-9]{4} \(--limite \$0\.5000\)' "$run_dir/out.stderr" || fail behavior-missing
  [[ "$(grep -c 'estimated cost' "$run_dir/out.stderr")" -eq "$(grep -c 'actual cost' "$run_dir/out.stderr")" ]] || fail cost-lines-mismatched
  # The model WAS called: the capture file (this card's call counter, see
  # the header note) exists.
  [[ -e "$run_dir/capture-allow.txt" ]] || fail call-counter-zero-when-allowed
  # stdout is untouched by the flag: byte-identical to the no-flag run.
  [[ "$(cat "$run_dir/out.stdout")" == "$baseline_stdout" ]] || fail allowed-stdout-changed
  local allowed_stderr
  allowed_stderr="$(cat "$run_dir/out.stderr")"

  # Determinism: same input, same output, same exit -- and --limite's own
  # new output lands on stderr, so that must match too, not only stdout.
  run_review_captured "$shared_bin" "$repo_dir" "$run_dir/capture-allow2.txt" --base HEAD~1 --limite 0.50
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  [[ "$(cat "$run_dir/out.stdout")" == "$baseline_stdout" ]] || fail non-deterministic
  [[ "$(cat "$run_dir/out.stderr")" == "$allowed_stderr" ]] || fail non-deterministic-stderr

  # --limite far below the cost: refused, exit 1, nothing on stdout beyond
  # what a failed run already has -- crucially, no finding was printed, and
  # the call counter proves the model was never reached.
  run_review_captured "$shared_bin" "$repo_dir" "$run_dir/capture-refuse.txt" --base HEAD~1 --limite 0.0001
  [[ "$rc" -eq 1 ]] || fail over-limit-wrong-exit
  grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" && fail over-limit-spent-anyway
  grep -Eq 'estimated cost \$[0-9]+\.[0-9]{4}, diff-only pre-flight \(--limite \$0\.0001\)' "$run_dir/out.stderr" || fail over-limit-missing-estimate
  grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail over-limit-missing-refusal
  grep -Fq 'actual cost' "$run_dir/out.stderr" && fail over-limit-reported-spend-that-never-happened
  [[ -e "$run_dir/capture-refuse.txt" ]] && fail call-counter-nonzero-when-refused

  # The margin case: on this fixture with the default price, the diff-only
  # pre-flight estimate is ~$0.0914, but the enforced check (the larger,
  # fully assembled prompt) does not admit the request until --limite
  # reaches ~$0.11. --limite 0.10 must still refuse even though $0.0914 <
  # $0.10 -- exactly the case printCostEstimate/reportBudgetExceeded's
  # wording is designed not to contradict (see docs/specs/AUR-433.md).
  # NOTE: $0.10 is pinned to the CURRENT size of the prompt template
  # internal/prompt.PromptBuilder assembles (system template + diff). If a
  # future card grows or shrinks that template, this boundary moves and
  # `boundary-wrong-exit` below stops meaning "AUR-433 regressed" and starts
  # meaning "re-derive the boundary" -- re-run the binary search this value
  # came from (bisect --limite between the printed diff-only estimate and a
  # value comfortably large) rather than assume a defect here.
  run_review_captured "$shared_bin" "$repo_dir" "$run_dir/capture-boundary.txt" --base HEAD~1 --limite 0.10
  [[ "$rc" -eq 1 ]] || fail boundary-wrong-exit
  [[ -e "$run_dir/capture-boundary.txt" ]] && fail boundary-call-counter-nonzero
  grep -Eq 'estimated cost \$[0-9]+\.[0-9]{4}, diff-only pre-flight \(--limite \$0\.1000\)' "$run_dir/out.stderr" || fail boundary-missing-estimate
  grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail boundary-missing-refusal
  # The refusal line must not restate a dollar figure as "the" cost that
  # was exceeded -- that would contradict the smaller estimate printed
  # just above it.
  grep -Fq 'estimated cost exceeds' "$run_dir/out.stderr" && fail boundary-contradictory-wording

  # Without --limite the AUR-430 contract is untouched byte for byte, and
  # gains no cost line and no new stderr output at all.
  run_review "$shared_bin" "$repo_dir" --base HEAD~1
  [[ "$rc" -eq 0 ]] || fail no-flag-contract-broken
  [[ "$(sha256sum "$run_dir/out.stdout" | awk '{print $1}')" == "$baseline_stdout_sha" ]] || fail no-flag-stdout-changed
  [[ "$(cat "$run_dir/out.stderr")" == "$baseline_stderr" ]] || fail no-flag-stderr-changed

  # Usage errors: an explicitly empty, non-numeric, zero or negative
  # --limite is exit 2, never a silently-disabled or silently-inverted
  # limit.
  for bad in '' 'not-a-number' '0' '-1'; do
    if [[ -z "$bad" ]]; then
      run_review "$shared_bin" "$repo_dir" --base HEAD~1 --limite ''
    else
      run_review "$shared_bin" "$repo_dir" --base HEAD~1 --limite "$bad"
    fi
    [[ "$rc" -eq 2 ]] || fail "bad-limite-not-rejected:$bad"
  done

  # Composition: --limite plus --fail-on still gates on the same findings
  # (exit 3), still reports cost; --limite plus --modelo still names the
  # chosen model; --limite plus --seguranca still prints its section.
  local fixture_error="$run_dir/response-error.json"
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
  "summary": "err"
}
EOF
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture_error" "$shared_bin" review --base HEAD~1 --limite 0.50 --fail-on high) \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 3 ]] || fail fail-on-composition-broken
  grep -Fq 'actual cost' "$run_dir/out.stderr" || fail fail-on-composition-lost-cost-report

  run_review "$shared_bin" "$repo_dir" --base HEAD~1 --limite 0.50 --modelo local
  [[ "$rc" -eq 0 ]] || fail modelo-composition-broken
  grep -Fq 'reviewing with model "local"' "$run_dir/out.stderr" || fail modelo-composition-lost-note
  grep -Fq 'estimated cost' "$run_dir/out.stderr" || fail modelo-composition-lost-cost

  run_review "$shared_bin" "$repo_dir" --base HEAD~1 --limite 0.50 --seguranca
  [[ "$rc" -eq 0 ]] || fail seguranca-composition-broken
  grep -Fq 'Security findings (standards/security-review):' "$run_dir/out.stdout" || fail seguranca-composition-lost-section
}

# mutation_case is MUT-001: make the one place --limite's value reaches
# enforcement (cost.go's buildCostTracker) admit spend above the ceiling,
# in a writable staged copy, rebuild, and prove the mutant flips the
# behavior nominal_case asserts -- the call counter goes from zero to
# nonzero for the exact same over-limit run. The committed source is never
# touched, so restoration is by construction: shared_bin still refuses
# GREEN.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild below recompiles only cmd/aurumcode.

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/cost.go"
  local anchor='ceilingUSD := limiteUSD'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # The mutant compiles against only what the file already imports: it
  # inflates the ceiling the tracker enforces, far past any --limite value
  # this run passes, so Reserve admits the request regardless of the
  # user's stated limit.
  sed -i 's|ceilingUSD := limiteUSD|ceilingUSD := limiteUSD + 1000000000|' "$target"
  grep -Fq 'ceilingUSD := limiteUSD + 1000000000' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  local rc
  # Under the mutant, the exact over-limit run nominal_case rejects instead
  # runs the model to completion: exit 0, the finding prints, and -- the
  # call counter MUT-001 requires -- the capture file now exists.
  rm -f -- "$run_dir/capture-mut.txt"
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-mut.txt" "$bin" review --base HEAD~1 --limite 0.0001) \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
  [[ "$rc" -ne 1 ]] || fail 'MUT-001/not-rejected'
  [[ -e "$run_dir/capture-mut.txt" ]] || fail 'MUT-001/call-counter-still-zero'

  # Restoration: the unmutated binary still refuses for the same input --
  # the GREEN reproduces, and the call counter is zero again.
  rm -f -- "$run_dir/capture-restore.txt"
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-restore.txt" "$shared_bin" review --base HEAD~1 --limite 0.0001) \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail 'MUT-001/restoration-broken'
  [[ -e "$run_dir/capture-restore.txt" ]] && fail 'MUT-001/restoration-broken'
  grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-433.go
  cat >"$root/tests/unit/aur433_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR433UnitBridge(t *testing.T) { TestAUR433(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR433UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR433:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR433:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-433.go
  cat >"$root/tests/integration/aur433_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR433IntegrationBridge(t *testing.T) { IntegrationAUR433(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR433IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR433:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR433:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-433.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-build its own copy. The
  # nested script's own exit-code vocabulary is preserved: its 79 is an
  # environment gap and must be re-emitted as infra here, never collapsed
  # into behavioral RED (see EXIT_CODE_CONVENTION.md).
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-433.sh E2EAUR433)
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
  TestAUR433) unit_case ;;
  IntegrationAUR433) integration_case ;;
  E2EAUR433) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
