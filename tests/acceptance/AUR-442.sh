#!/usr/bin/env bash
#
# Acceptance program for card AUR-442, scenario AC-001.
#
# WHAT THIS PROVES
#
#   AUR-442's 2026-08-13 dogfooding measured that `aurumcode review --base
#   HEAD~1 --seguranca` matched only 2 of the 8 catalog rules
#   (security/sql-injection, security/command-injection), both requiring
#   the narrow `"..." + var` concatenation shape -- so the project's own
#   demo fixture (tests/fixtures/repos/git-demo), whose sole purpose is to
#   plant a plaintext secret, reported "No security findings." This
#   program proves the chosen fix: security/hardcoded-secret now carries a
#   matcher (internal/review/rules/security.yml), proven against a
#   dedicated fixture (tests/fixtures/review/vuln/hardcoded-secret) AND
#   against git-demo itself, without echoing the matched secret value and
#   without disturbing the published no-flag contract. The five remaining
#   patternless security rules (xss, path-traversal, weak-crypto,
#   insecure-random, missing-auth) are a documented scope decision, not an
#   oversight -- see docs/specs/AUR-442.md.
#
# KNOWN, EXPECTED CONSEQUENCE OUTSIDE THIS CARD'S paths
#
#   tests/acceptance/AUR-435.sh's nominal_case and
#   tests/integration/AUR-435.go's IntegrationAUR435 both hardcode that
#   git-demo's --seguranca run prints "No security findings." -- the exact
#   defect this card fixes. Neither file is in AUR-442's `paths`, so this
#   card cannot update them; a follow-up card should re-baseline those two
#   assertions. AUR-435's own MUT-001 (which runs against the unrelated
#   tests/fixtures/review/vuln/repo.git, not git-demo) is unaffected.
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sealed profile (bootstrap-readonly-v1) denies network.
#   AURUMCODE_LLM_FIXTURE points the binary's provider selection at a
#   canned deterministic response (tests/fixtures/review/known-problem-
#   response.json, the same fixture already committed for the sibling
#   review cards). The security pass itself is deterministic and calls no
#   model.
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

readonly card='AUR-442'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR442|IntegrationAUR442|E2EAUR442|AC-001-MUT-001) ;;
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
  tests/fixtures/review/vuln/hardcoded-secret/repo.git
  tests/unit/AUR-442.go
  tests/integration/AUR-442.go
  tests/e2e/AUR-442.sh
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/config
  internal/llm
  internal/prompt
  internal/review
  internal/security/redaction
  pkg/types
  standards/security-review/rules.md
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
  tests/fixtures/review/vuln/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a442.XXXXXX")" || infra mktemp
# Cleanup must never turn an already-decided result into a failure: the
# materialized input tree can be read-only, so force write permission back
# on before removing, and never let a residual removal error propagate.
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
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs:
# this card's owned paths plus the read-only packages the engine imports,
# and the fixtures the CLI is exercised against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/config internal/prompt internal/review internal/security
  copy "$root" internal/git internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/documentation/review internal/pipeline
  copy "$root" cmd/regenerate-docs
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  # The materialized input tree can be read-only, directories included;
  # force the staged scratch copy writable so mutation_case's rewrite and
  # cleanup_root can operate.
  chmod -R u+w -- "$root"
}

readonly sec_header='Security findings (standards/security-review):'
readonly citation='(rule security/hardcoded-secret: Hardcoded Secrets)'
readonly standard_citation='standards/security-review SCR-003'

# build_shared builds the binary exactly once per acceptance run and reuses
# it for the behavioral and e2e cases; mutation_case rebuilds only its
# mutated copy of internal/review/rules/security.yml on the same warm
# GOCACHE (see tests/acceptance/AUR-430.sh for why cold per-case builds are
# avoided under the profile's memory ceiling).
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
# the binary builds, but `--seguranca` does not deliver the promised
# finding on either fixture -- and reports `behavior-missing`.
nominal_case() {
  build_shared
  local fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
  local secret_repo="$shared_root/tests/fixtures/review/vuln/hardcoded-secret/repo.git"
  local demo_repo="$shared_root/tests/fixtures/repos/git-demo/repo.git"

  # The pass claims the project security standard; the standard must
  # actually define the cited rule.
  grep -Fq '### Rule: SCR-003' "$repo_root/standards/security-review/rules.md" \
    || fail standard-does-not-define-SCR-003

  # Without the flag: the published contract, byte for byte, no security
  # section anywhere, on either repository.
  local repo out_base
  for repo in "$secret_repo" "$demo_repo"; do
    out_base="$(cd "$repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1)" || fail base-run-failed
    if grep -Fq "$sec_header" <<<"$out_base"; then fail base-grew-a-security-section; fi
  done

  # With the flag, on the dedicated fixture: both planted shapes are
  # found, the two deliberately benign lines beside them are not.
  local out_sec
  out_sec="$(cd "$secret_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
  grep -Fq "$sec_header" <<<"$out_sec" || fail behavior-missing
  grep -Fq 'config/secrets.env:4: [error]' <<<"$out_sec" || fail behavior-missing
  grep -Fq 'src/config.py:4: [error]' <<<"$out_sec" || fail behavior-missing
  grep -Fq "$standard_citation" <<<"$out_sec" || fail standard-citation-missing
  local after_header citation_count
  after_header="${out_sec#*"$sec_header"}"
  citation_count="$(grep -Fo "$citation" <<<"$after_header" | wc -l)"
  [[ "$citation_count" -eq 2 ]] || fail "unexpected-citation-count:$citation_count"
  if grep -Fq 'AURUM-FAKE-KEY-9000-2222' <<<"$out_sec"; then fail secret-value-leaked; fi
  if grep -Fq 'AURUM-FAKE-PASSWORD-9000-1111' <<<"$out_sec"; then fail secret-value-leaked; fi

  # Determinism: same input, same bytes.
  local out_again
  out_again="$(cd "$secret_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic

  # The card's central proof: git-demo's own planted plaintext secret is
  # now reported, not silently absent. Checks below are scoped to AFTER
  # the header on purpose: known-problem-response.json's own QUALITY
  # finding (the model pass) independently cites
  # "config/demo-tokens.txt:4: [error] ..." for the exact same planted
  # line, so an unscoped check could pass on the model's citation alone
  # without the deterministic security pass having matched anything.
  local out_demo demo_section
  out_demo="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca)" || fail demo-run-failed
  grep -Fq "$sec_header" <<<"$out_demo" || fail behavior-missing
  demo_section="${out_demo#*"$sec_header"}"
  if grep -Fq 'No security findings.' <<<"$demo_section"; then fail absence-still-reported; fi
  local line
  for line in 4 5 6; do
    grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$demo_section" || fail "behavior-missing:line-$line"
  done
  for value in AURUM-FAKE-TOKEN-0000-0001 AURUM-FAKE-PASSWORD-0000-0002 AURUM-FAKE-WEBHOOK-0000-0003; do
    if grep -Fq "$value" <<<"$out_demo"; then fail "secret-value-leaked:$value"; fi
  done

  # Composed with --fail-on: the matched secret (severity error) now
  # closes the gate on git-demo.
  local rc
  set +e
  (cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
  rc=$?
  set -e
  [[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

  # The secret canary never reaches a sink.
  local canary="aurum-canary-442-$$"
  local out_canary err_canary
  out_canary="$(cd "$demo_repo" && AURUM_SECRET_CANARY="$canary" AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
  err_canary="$(cat "$run_dir/canary.err")"
  if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi
}

# mutation_case is MUT-001: removing the matcher of a covered rule must
# make the acceptance fail because the expected finding disappears. It
# edits a writable staged copy of internal/review/rules/security.yml,
# deleting the `pattern:` line of security/hardcoded-secret (a fixed-string
# removal, not a sed regex substitution, so the pattern's own regex
# metacharacters never need escaping), rebuilds, and proves the finding
# VANISHES from both fixtures -- i.e. that nominal_case's presence
# assertions are load-bearing. The committed source is never touched; the
# mutation exists only in this case's own staged copy, so nominal_case
# (run separately in run_all, against the real, unmutated build) is the
# restoration-stays-green proof.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.
  local fixture="$repo_root/tests/fixtures/review/known-problem-response.json"

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor="pattern: '(?i:[A-Za-z0-9_]*(api"
  [[ "$(grep -cF "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  grep -vF "$anchor" "$target" >"$target.mut" || fail 'MUT-001/rewrite-failed'
  mv "$target.mut" "$target"
  grep -Fq "$anchor" "$target" && fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local demo_repo="$root/tests/fixtures/repos/git-demo/repo.git"
  local out
  out="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-001/mutation-run-failed'
  # The mutant still runs the pass (the header prints), but with the
  # matcher gone the expected finding is absent -- exactly the failure
  # nominal_case's presence assertion would report. A mutant whose output
  # still carries the finding means the matcher was not what produced it:
  # the mutation survived, so the acceptance fails. The check is scoped to
  # AFTER the header on purpose: known-problem-response.json's own QUALITY
  # finding (the model pass, unrelated to this deterministic matcher) cites
  # the same rule text, so checking the whole output would misread the
  # model's citation as the security pass's.
  grep -Fq "$sec_header" <<<"$out" || fail 'MUT-001/pass-did-not-run'
  local section
  section="${out#*"$sec_header"}"
  if grep -Fq "$citation" <<<"$section"; then fail 'MUT-001/mutation-survived'; fi
  grep -Fq 'No security findings.' <<<"$section" || fail 'MUT-001/unexpected-shape'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-442.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur442_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR442UnitBridge(t *testing.T) { TestAUR442(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR442UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR442:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR442:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-442.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur442_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR442IntegrationBridge(t *testing.T) { IntegrationAUR442(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR442IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR442:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR442:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-442.sh
  chmod -R u+w -- "$root"
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-442.sh E2EAUR442) || fail e2e-failed
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
  TestAUR442) unit_case ;;
  IntegrationAUR442) integration_case ;;
  E2EAUR442) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
