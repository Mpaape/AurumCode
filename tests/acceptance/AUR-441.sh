#!/usr/bin/env bash
#
# Acceptance program for card AUR-441, scenario AC-001.
#
# WHAT THIS PROVES
#
#   On a second `aurumcode review --base HEAD~1` invocation against an
#   UNCHANGED repository, no file is resent to the model, and the command
#   reports how many files were reused. The engine sends its whole reviewed
#   diff to the model in one prompt, one Complete call per invocation
#   (internal/review/reviewer.go, internal/prompt/builder.go) -- there is no
#   per-file send for a cache to intercept -- so cmd/aurumcode filters
#   diff.Files down to the internal/review/cache misses BEFORE calling
#   GenerateReview (zero misses skips the call to the model entirely),
#   merges the cache hits' previously-found issues into the printed result,
#   and reports the reused count on stderr. AURUMCODE_PROMPT_CAPTURE
#   (internal/review/fakeprovider.go, read-only, written only when
#   FakeProvider.Complete actually runs) is the real assertion: the second
#   run's capture file is absent entirely (zero "### File:" sections,
#   because the model is never called at all on a full cache hit), while
#   the first run's carries one section per changed file.
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sealed profile (bootstrap-readonly-v1) denies network.
#   AURUMCODE_LLM_FIXTURE points the binary's provider selection at a
#   canned, deterministic response file; internal/review.FakeProvider
#   implements the same llm.Provider interface a vendor provider would.
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

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-441'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR441|IntegrationAUR441|E2EAUR441|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Input preflight. Deliverables this card owns fail behavioral (their
# absence IS the missing behavior); everything else is an environment gap.
owned_inputs=(
  tests/unit/AUR-441.go
  tests/integration/AUR-441.go
  tests/e2e/AUR-441.sh
  internal/review/cache/cache.go
  cmd/aurumcode/review_cache.go
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/llm
  internal/prompt
  internal/review
  internal/security
  internal/git
  internal/documentation
  internal/pipeline
  cmd/regenerate-docs
  pkg/types
  tests/fixtures/repos/git-demo/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a441.XXXXXX")" || infra mktemp
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
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"
export GOMAXPROCS=1

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
# cmd/aurumcode is one package shared with concurrently-integrated cards
# (AUR-426 added docs.go, AUR-438 added pr.go), so the full closure those
# already-integrated files import has to resolve too, even though this card
# owns none of their behavior -- mirroring how AUR-426.sh and AUR-438.sh
# stage the same closure for the same reason.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/git
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/documentation/extractors internal/documentation/incremental \
    internal/documentation/normalizer internal/documentation/site \
    internal/documentation/welcome
  copy "$root" internal/pipeline
  copy "$root" internal/analyzer internal/prompt internal/review internal/llm \
    internal/security pkg/types
  copy "$root" tests/fixtures/repos/git-demo
  # The materialized input tree can be read-only, directories included;
  # force the staged scratch copy writable so this card's own cache writes
  # (default AURUMCODE_CACHE_DIR-less runs write into the reviewed repo's
  # own directory -- see internal/review/cache.ResolveDir), the mutation
  # case's sed, and cleanup_root can all operate on it.
  chmod -R u+w -- "$root"
}

# write_fixture plants this card's deterministic model response: exactly
# one finding on config/demo-tokens.txt, the file commit 3 of git-demo adds
# (the same planted problem tests/acceptance/AUR-430.sh's own fixture
# uses).
write_fixture() {
  local dir="$1"
  mkdir -p "$dir"
  cat >"$dir/known-problem.json" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN).",
      "suggestion": "Remove the secret from version control and load it from the environment instead."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF
}

count_sections() {
  # Prints the number of captured "### File:" sections, or -1 when the
  # capture file does not exist at all -- the signal a full cache hit never
  # even calls the model.
  local f="$1"
  if [[ ! -e "$f" ]]; then
    printf -- '-1\n'
    return 0
  fi
  grep -c '### File:' "$f" || true
}

# build_shared builds the binary exactly once per acceptance run and
# reuses it for the behavioral and e2e cases; mutation_case rebuilds only
# its mutated copy of cmd/aurumcode on the same warm GOCACHE (see
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
# exactly as a user would, twice, against an unchanged repository.
nominal_case() {
  build_shared
  write_fixture "$run_dir/fixtures"
  local fixture="$run_dir/fixtures/known-problem.json"

  local demo_repo="$run_dir/repo-nominal.git"
  cp -R "$shared_root/tests/fixtures/repos/git-demo/repo.git" "$demo_repo"
  chmod -R u+w -- "$demo_repo"
  local cache_dir="$run_dir/cache-nominal"

  local out1 err1
  out1="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$cache_dir" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture1.txt" "$shared_bin" review --base HEAD~1 2>"$run_dir/err1.txt")" || fail behavior-missing
  err1="$(cat "$run_dir/err1.txt")"
  grep -Fq 'config/demo-tokens.txt' <<<"$out1" || fail behavior-missing
  grep -Fq '[error]' <<<"$out1" || fail behavior-missing
  [[ "$(count_sections "$run_dir/capture1.txt")" == "2" ]] || fail cold-run-must-send-both-files
  if grep -Fq 'reused' <<<"$err1"; then fail cold-run-must-not-claim-reuse; fi

  local out2 err2
  out2="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$cache_dir" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture2.txt" "$shared_bin" review --base HEAD~1 2>"$run_dir/err2.txt")" || fail warm-run-failed
  err2="$(cat "$run_dir/err2.txt")"
  [[ "$out2" == "$out1" ]] || fail non-deterministic
  [[ "$(count_sections "$run_dir/capture2.txt")" == "-1" ]] || fail behavior-missing
  grep -Fq 'reused 2 file' <<<"$err2" || fail reuse-count-not-reported

  # --fail-on must still see a cache-hit finding: the gate reads the merged
  # result, not just what was freshly sent.
  local gate_dir="$run_dir/cache-gate"
  local rc
  set +e
  (cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$gate_dir" "$shared_bin" review --base HEAD~1 --fail-on high) >/dev/null 2>&1
  rc=$?
  (cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$gate_dir" "$shared_bin" review --base HEAD~1 --fail-on high) >/dev/null 2>&1
  local rc2=$?
  set -e
  [[ "$rc" -eq 3 && "$rc2" -eq 3 ]] || fail gate-must-see-cached-finding

  # The pre-existing "nothing changed" contract (--base HEAD) is
  # unaffected: the model is still called even with zero files to send.
  local noop_dir="$run_dir/cache-noop"
  (cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$noop_dir" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-noop.txt" "$shared_bin" review --base HEAD >/dev/null) || fail noop-run-failed
  [[ "$(count_sections "$run_dir/capture-noop.txt")" != "-1" ]] || fail zero-file-diff-must-still-call-model

  # The default cache directory (AURUMCODE_CACHE_DIR unset) is
  # process-scoped, not shared: two separate invocations against the
  # identical repository must NOT reuse each other's cache by default --
  # each independently sends both files. A repo-scoped default was this
  # card's first design; it collided with AUR-433's already-published
  # --limite contract, whose own acceptance program runs this binary
  # several times in a row against one repository and expects every
  # invocation to independently reach the model (see
  # internal/review/cache.ResolveDir's doc for the full account). This is
  # the regression test for that reversal.
  local default_repo="$run_dir/repo-default.git"
  cp -R "$shared_root/tests/fixtures/repos/git-demo/repo.git" "$default_repo"
  chmod -R u+w -- "$default_repo"
  local d1
  d1="$(cd "$default_repo" && AURUMCODE_LLM_FIXTURE="$fixture" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-d1.txt" "$shared_bin" review --base HEAD~1)" || fail default-first-run-failed
  [[ "$(count_sections "$run_dir/capture-d1.txt")" == "2" ]] || fail default-cold-run-must-send-both-files
  local d2 err_d2
  d2="$(cd "$default_repo" && AURUMCODE_LLM_FIXTURE="$fixture" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-d2.txt" "$shared_bin" review --base HEAD~1 2>"$run_dir/err-d2.txt")" || fail default-second-run-failed
  err_d2="$(cat "$run_dir/err-d2.txt")"
  [[ "$d2" == "$d1" ]] || fail default-runs-disagree-on-the-finding
  [[ "$(count_sections "$run_dir/capture-d2.txt")" == "2" ]] || fail default-second-run-must-not-share-the-first-run-s-cache
  if grep -Fq 'reused' <<<"$err_d2"; then fail default-isolated-run-must-not-claim-reuse; fi

  # A metered (--limite) and an unmetered run against the SAME repository,
  # under a cache dir the caller explicitly shares, must NOT collide: the
  # cache key folds in the resolved cost-tracker model key
  # (llm.ModelResolver) precisely so these two differently-guarded
  # requests never serve one another's cached answer.
  local limite_dir="$run_dir/cache-limite"
  local out_unmetered out_metered
  out_unmetered="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$limite_dir" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-unmetered.txt" "$shared_bin" review --base HEAD~1)" || fail limite-collision-unmetered-run-failed
  [[ "$(count_sections "$run_dir/capture-unmetered.txt")" == "2" ]] || fail limite-collision-unmetered-must-call-model
  out_metered="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_CACHE_DIR="$limite_dir" \
    AURUMCODE_PROMPT_CAPTURE="$run_dir/capture-metered.txt" "$shared_bin" review --base HEAD~1 --limite 0.50 2>"$run_dir/metered.err")" || fail limite-collision-metered-run-failed
  [[ "$(count_sections "$run_dir/capture-metered.txt")" == "2" ]] || fail limite-metered-run-must-not-be-served-the-unmetered-cache
  grep -Fq 'actual cost $' "$run_dir/metered.err" || fail limite-metered-run-missing-real-cost

  # Security: the planted canary must never reach the cache directory.
  local canary="aurum-canary-441-$$"
  local canary_dir="$run_dir/cache-canary"
  local out_canary err_canary
  out_canary="$(cd "$demo_repo" && AURUM_SECRET_CANARY="$canary" AURUMCODE_LLM_FIXTURE="$fixture" \
    AURUMCODE_CACHE_DIR="$canary_dir" "$shared_bin" review --base HEAD~1 2>"$run_dir/canary.err")" || fail canary-run-failed
  err_canary="$(cat "$run_dir/canary.err")"
  if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked-to-output; fi
  if grep -Frq "$canary" "$canary_dir" 2>/dev/null; then fail canary-leaked-to-cache; fi
}

# mutation_case is MUT-001: invalidating reuse on every execution must make
# the acceptance fail because the count of model submissions never drops.
# It edits a writable staged copy of cmd/aurumcode/review_cache.go so
# partitionByCache never reports a cache hit -- every file is always
# treated as a miss, so every run resends the whole diff -- rebuilds, and
# reruns tests/e2e/AUR-441.sh's own two-invocation proof against that
# mutant binary. The unmutated proof already asserts the second run's
# capture file is absent (zero-section signal): under the mutant it is
# present again with the same two sections as the first run, so that
# specific assertion is what must fail. The committed source is never
# touched, so restoration is by construction: a fresh run of the same
# unmutated e2e proof against the shared binary still passes.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one small package.

  local root="$run_dir/root-mut"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-441.sh
  chmod -R u+w -- "$root"

  local target="$root/cmd/aurumcode/review_cache.go"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor='if getErr == nil && ok {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  # `&& false` keeps getErr and ok referenced (so the mutant still compiles
  # -- a build failure is never valid MUT-001 evidence, see
  # tests/acceptance/AUR-430.sh's own convention) while making the branch
  # permanently unreachable: every file is always treated as a miss.
  sed -i "s|${anchor}|if getErr == nil \&\& ok \&\& false { // MUT-001: never report a cache hit|" "$target"
  grep -Fq 'if getErr == nil && ok && false { // MUT-001' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_BIN="$bin" bash tests/e2e/AUR-441.sh E2EAUR441 2>&1)"
  rc=$?
  set -e
  if ((rc == 0)); then
    fail 'MUT-001/not-rejected'
  fi
  if ((rc == 79)); then
    infra "MUT-001/inconclusive:$out"
  fi
  grep -Fq 'warm-run-must-never-call-the-model' <<<"$out" || fail "MUT-001/wrong-failure-mode:$out"

  # Restoration: the unmutated shared binary, exercised by the same e2e
  # proof from a fresh root, still passes -- the GREEN reproduces exactly.
  local root2="$run_dir/root-mut-restore"
  stage_source "$root2"
  copy "$root2" tests/e2e/AUR-441.sh
  chmod -R u+w -- "$root2"
  local rc2
  set +e
  (cd "$root2" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-441.sh E2EAUR441) >"$run_dir/restore.log" 2>&1
  rc2=$?
  set -e
  ((rc2 == 0)) || { cat "$run_dir/restore.log" >&2; fail 'MUT-001/restoration-broken'; }

  cleanup_root "$root"
  cleanup_root "$root2"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-441.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur441_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR441UnitBridge(t *testing.T) { TestAUR441(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR441UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR441:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR441:zero-tests
  cleanup_root "$root"
}

integration_case() {
  build_shared # reuse the already-built binary (see tests/integration/AUR-441.go's
               # AURUMCODE_BIN) instead of a sixth cold build of the full closure.
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-441.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur441_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR441IntegrationBridge(t *testing.T) { IntegrationAUR441(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" AURUMCODE_BIN="$shared_bin" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR441IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR441:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR441:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-441.sh
  chmod -R u+w -- "$root"
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-441.sh E2EAUR441) || fail e2e-failed
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
  TestAUR441) unit_case ;;
  IntegrationAUR441) integration_case ;;
  E2EAUR441) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
