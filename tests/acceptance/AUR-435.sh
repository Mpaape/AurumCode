#!/usr/bin/env bash
#
# Acceptance program for card AUR-435, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode review --base HEAD~1 --seguranca` runs a security pass that
#   finds a real (synthetic) vulnerability in the reviewed diff using the
#   project's security standard, and reports it in a section SEPARATE from
#   the quality findings. The matcher is the piece the c12d7ab restoration
#   measured as missing: the embedded security-category rules
#   (internal/review/rules/security.yml) now carry patterns, the pass scans
#   only added lines, every security finding cites its security-category
#   rule (the AUR-434 gate applies unchanged) plus the standards/security-review
#   rule that scopes it, and without the flag stdout stays byte-identical to
#   the published AUR-430/431/432/434/436 contract.
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sealed profile (bootstrap-readonly-v1) denies network.
#   AURUMCODE_LLM_FIXTURE points the binary's provider selection at a canned
#   deterministic response (internal/review.FakeProvider). The quality-only
#   response this program plants makes the quality block and the security
#   section distinguishable; the security pass itself is deterministic and
#   calls no model.
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

readonly card='AUR-435'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR435|IntegrationAUR435|E2EAUR435|AC-001-MUT-001) ;;
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
  tests/fixtures/review/vuln/repo.git
  tests/unit/AUR-435.go
  tests/integration/AUR-435.go
  tests/e2e/AUR-435.sh
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
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a435.XXXXXX")" || infra mktemp
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
  copy "$root" internal/git internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/pipeline
  copy "$root" cmd/regenerate-docs
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  # The materialized input tree can be read-only, directories included;
  # force the staged scratch copy writable so mutation_case's sed and
  # cleanup_root can operate.
  chmod -R u+w -- "$root"
}

# write_fixture plants this card's deterministic model response: exactly one
# QUALITY finding, so the quality block and the security section are
# distinguishable in the same output.
write_fixture() {
  local dir="$1"
  mkdir -p "$dir"
  cat >"$dir/quality.json" <<'EOF'
{
  "issues": [
    {
      "file": "src/db.py",
      "line": 4,
      "severity": "info",
      "rule_id": "quality/poor-naming",
      "message": "PLANTED-QUALITY-435 finding for the separation proof."
    }
  ],
  "summary": "Quality-only fixture for AUR-435."
}
EOF
}

readonly sec_header='Security findings (standards/security-review):'
readonly sec_citation='(rule security/sql-injection: SQL Injection Vulnerability)'
# AUR-442 gave security/hardcoded-secret a matcher (internal/review/rules/
# security.yml), so the git-demo assertion below no longer measures an
# honest absence: git-demo plants a plaintext secret specifically to be
# found, and it now is. See docs/specs/AUR-442.md.
readonly hardcoded_secret_citation='(rule security/hardcoded-secret: Hardcoded Secrets)'

# build_shared builds the binary exactly once per acceptance run and reuses
# it for the behavioral and e2e cases; mutation_case rebuilds only its
# mutated copy of internal/review on the same warm GOCACHE (see
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
# the binary builds, but `--seguranca` does not deliver the promised
# security pass -- and reports `behavior-missing`.
nominal_case() {
  build_shared
  write_fixture "$run_dir/fixtures"
  local fixture="$run_dir/fixtures/quality.json"
  local vuln_repo="$shared_root/tests/fixtures/review/vuln/repo.git"
  local demo_repo="$shared_root/tests/fixtures/repos/git-demo/repo.git"

  # The pass claims the project security standard; the standard must
  # actually define the cited rule. This reads the read_paths artifact, so
  # its absence would have been an infra gap (see required_inputs).
  grep -Fq '### Rule: SCR-001' "$repo_root/standards/security-review/rules.md" \
    || fail standard-does-not-define-SCR-001

  # Without the flag: the published contract, byte for byte, no security
  # section anywhere.
  local out_base
  out_base="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1)" || fail base-run-failed
  grep -Fq 'PLANTED-QUALITY-435' <<<"$out_base" || fail base-quality-missing
  if grep -Fq "$sec_header" <<<"$out_base"; then fail base-grew-a-security-section; fi

  # With the flag: the security pass reports the planted synthetic SQL
  # injection in its own section, appended after the byte-identical base
  # output.
  local out_sec
  out_sec="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
  [[ "${out_sec:0:${#out_base}}" == "$out_base" ]] || fail base-bytes-rewritten
  grep -Fq "$sec_header" <<<"$out_sec" || fail behavior-missing
  grep -Fq 'src/db.py:8: [error]' <<<"$out_sec" || fail behavior-missing
  grep -Fq "$sec_citation" <<<"$out_sec" || fail behavior-missing
  grep -Fq 'standards/security-review SCR-001' <<<"$out_sec" || fail standard-citation-missing

  # Separation both ways: the security citation appears only after the
  # header, the quality marker only before it.
  local before_header="${out_sec%%"$sec_header"*}"
  local after_header="${out_sec#*"$sec_header"}"
  if grep -Fq "$sec_citation" <<<"$before_header"; then fail security-leaked-into-quality; fi
  if grep -Fq 'PLANTED-QUALITY-435' <<<"$after_header"; then fail quality-leaked-into-security; fi

  # Determinism: same input, same bytes.
  local out_again
  out_again="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic

  # git-demo plants a plaintext secret specifically to be found (three
  # lines: config/demo-tokens.txt:4-6). AUR-442 gave security/hardcoded-
  # secret a matcher, so the deterministic pass now reports exactly that
  # planted secret -- not the SQL-injection rule this repository has
  # nothing to trigger, and not an invented finding beyond the three
  # planted lines.
  local out_demo demo_line
  out_demo="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca)" || fail demo-run-failed
  grep -Fq "$sec_header" <<<"$out_demo" || fail demo-security-section-missing
  if grep -Fq "$sec_citation" <<<"$out_demo"; then fail invented-finding; fi
  grep -Fq "$hardcoded_secret_citation" <<<"$out_demo" || fail demo-secret-not-found
  grep -Fq 'standards/security-review SCR-003' <<<"$out_demo" || fail demo-standard-citation-missing
  for demo_line in 4 5 6; do
    grep -Fq "config/demo-tokens.txt:${demo_line}: [error]" <<<"$out_demo" || fail "demo-secret-not-found:line-$demo_line"
  done
  if grep -Fq 'No security findings.' <<<"$out_demo"; then fail demo-still-reports-absence; fi

  # The secret canary never reaches a sink. The value is synthetic and
  # assembled at runtime; accepted by nothing.
  local canary="aurum-canary-435-$$"
  local out_canary err_canary
  out_canary="$(cd "$vuln_repo" && AURUM_SECRET_CANARY="$canary" AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
  err_canary="$(cat "$run_dir/canary.err")"
  if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi
}

# mutation_case is MUT-001: reusing the QUALITY rubric in the security pass
# must make the acceptance fail by the absence of the expected finding. It
# edits a writable staged copy of internal/review/securitypass.go so the
# pass selects the quality-category rules instead of the security-category
# ones, rebuilds, and proves the planted vulnerability then VANISHES from
# the output -- i.e. that nominal_case's presence assertion is load-bearing.
# The committed source is never touched; the mutation exists only in this
# case's own staged copy.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.
  write_fixture "$run_dir/fixtures"
  local fixture="$run_dir/fixtures/quality.json"

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/internal/review/securitypass.go"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor='GetByCategory("security")'
  [[ "$(grep -cF "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  sed -i 's/GetByCategory("security")/GetByCategory("quality")/' "$target"
  grep -Fq 'GetByCategory("quality")' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local vuln_repo="$root/tests/fixtures/review/vuln/repo.git"
  local out
  out="$(cd "$vuln_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-001/mutation-run-failed'
  # The mutant still runs the pass (the header prints), but with the
  # quality rubric the expected security finding is absent -- exactly the
  # failure nominal_case's presence assertion would report. A mutant whose
  # output still carries the finding means the category selection was not
  # what produced it: the mutation survived, so the acceptance fails.
  grep -Fq "$sec_header" <<<"$out" || fail 'MUT-001/pass-did-not-run'
  if grep -Fq "$sec_citation" <<<"$out"; then fail 'MUT-001/mutation-survived'; fi
  grep -Fq 'No security findings.' <<<"$out" || fail 'MUT-001/unexpected-shape'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-435.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur435_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR435UnitBridge(t *testing.T) { TestAUR435(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR435UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR435:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR435:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-435.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur435_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR435IntegrationBridge(t *testing.T) { IntegrationAUR435(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR435IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR435:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR435:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-435.sh
  chmod -R u+w -- "$root"
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-435.sh E2EAUR435) || fail e2e-failed
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
  TestAUR435) unit_case ;;
  IntegrationAUR435) integration_case ;;
  E2EAUR435) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
