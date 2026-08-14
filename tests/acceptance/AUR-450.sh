#!/usr/bin/env bash
#
# Acceptance program for card AUR-450, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The 2026-08-14 re-test measured: code carrying hashlib.md5(password)
#   (security/weak-crypto) and "<b>" + name + "</b>" (security/xss) gets
#   `Security findings (standards/security-review): No security findings.`
#   with exit 0. Five of the embedded catalog's eight security-category
#   rules are metadata only -- AUR-442 gave three of them (sql-injection,
#   command-injection, hardcoded-secret) a real matcher, and documented,
#   scoped reasons for leaving the other five patternless -- but the CLI
#   never says which ran. A user reads silence as full coverage. This is
#   not a matcher false positive or false negative: it is the output
#   omitting its own scope. This program proves the fix: `--seguranca`
#   names, on stderr, how many and which security-category rules of the
#   embedded catalog it actually applied against how many the category
#   declares in total -- identically whether the pass found something or
#   found nothing -- while stdout (the findings themselves, their content
#   and order, and the AUR-442/AUR-449 byte contract with a provider
#   configured) stays completely untouched. See docs/specs/AUR-450.md.
#
# WHY STDERR, NOT STDOUT
#
#   tests/acceptance/AUR-449.sh pins an EXACT sha256 of git-demo's
#   `--seguranca` stdout with a configured provider
#   (63c649af1c90e38b473e1bd45b4152b1f96ecad17d5d9c05c17bb94df7b8240f) as a
#   regression guard this card does not own and must not disturb. Any new
#   byte on that exact stdout would break that pin. This codebase already
#   has an established idiom for exactly this situation -- additive,
#   caller-facing information that must not touch a byte-pinned stdout
#   contract goes to stderr with the "aurumcode review: " prefix (the
#   AUR-449 skip note, the AUR-448 discard warning, the AUR-441 cache-reuse
#   note, the AUR-433 cost lines all do this) -- so the coverage note
#   follows the same convention. nominal_case below re-pins the exact same
#   AUR-449 stdout sha256 to prove it, byte for byte, unaffected.
#
# WHY THE LLM CALL IS A FIXTURE (where one is used at all)
#
#   The sealed profile (bootstrap-readonly-v1) denies network. Most of this
#   card's proof needs no provider at all (the security pass is a
#   deterministic regex matcher, AUR-442/AUR-449); the one scenario that
#   does configure a provider (proving that path is unaffected) uses
#   AURUMCODE_LLM_FIXTURE, the same canned deterministic response already
#   committed for the sibling review cards.
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

readonly card='AUR-450'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR450|IntegrationAUR450|E2EAUR450|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
command -v sha256sum >/dev/null 2>&1 || infra missing_sha256sum

# Input preflight. Deliverables this card owns fail behavioral (their
# absence IS the missing behavior); everything else is an environment gap.
owned_inputs=(
  tests/unit/AUR-450.go
  tests/integration/AUR-450.go
  tests/e2e/AUR-450.sh
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
  internal/security/redaction
  pkg/types
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a450.XXXXXX")" || infra mktemp
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
# and the fixtures the CLI is exercised against. Same set AUR-449's own
# acceptance program stages, since this card touches the same command.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/prompt internal/review internal/security
  copy "$root" internal/git internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/pipeline
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

# The AUR-450 coverage note. Catalog-wide (not diff-derived): the embedded
# catalog (internal/review/rules/security.yml) declares 8 security-category
# rules, exactly 3 of which (sql-injection, command-injection,
# hardcoded-secret) carry a matcher -- see docs/specs/AUR-442.md's "The
# decision" for why the other 5 are deliberately patternless. If a future
# card changes that count, this constant (and the sibling ones in
# tests/unit/AUR-450.go, tests/integration/AUR-450.go and
# tests/e2e/AUR-450.sh) must be re-derived, not patched blindly.
readonly coverage_prefix='aurumcode review: security pass applied 3 of 8 security rules ('
readonly coverage_pointer='internal/review/rules/security.yml'
coverage_rules=(security/command-injection security/hardcoded-secret security/sql-injection)

# The exact sha256 of git-demo's `--seguranca` stdout with the fixture
# provider configured (AURUMCODE_LLM_FIXTURE=known-problem-response.json) --
# the same pin tests/acceptance/AUR-449.sh carries, re-asserted here because
# this card's whole point is adding NEW output around this exact command
# without disturbing this exact byte sequence. See this file's own "WHY
# STDERR, NOT STDOUT" header note.
readonly expected_with_provider_sha256='63c649af1c90e38b473e1bd45b4152b1f96ecad17d5d9c05c17bb94df7b8240f'

# build_shared builds the binary exactly once per acceptance run and reuses
# it for the behavioral and e2e cases; mutation_case rebuilds only its
# mutated copy of cmd/aurumcode/main.go on the same warm GOCACHE (see
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

# noprov_env strips every provider-selecting variable from the environment
# passed to the binary, so "no provider configured" is never an accident of
# the harness's own environment.
noprov_env() {
  env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL "$@"
}

assert_coverage_note() {
  local errtext="$1"
  grep -Fq "$coverage_prefix" <<<"$errtext" || fail coverage-note-missing
  local rule
  for rule in "${coverage_rules[@]}"; do
    grep -Fq "$rule" <<<"$errtext" || fail "coverage-note-missing-rule:$rule"
  done
  grep -Fq "$coverage_pointer" <<<"$errtext" || fail coverage-note-missing-pointer
}

# nominal_case is AC-001's core behavioral proof: run the built binary
# exactly as a user would. RED before the implementation is behavioral --
# the binary builds, but --seguranca does not deliver the promised
# coverage note -- and reports `coverage-note-missing`.
nominal_case() {
  build_shared
  local fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
  local demo_repo="$shared_root/tests/fixtures/repos/git-demo/repo.git"

  # The card's central proof: the pass finds something (git-demo, no
  # provider needed -- AUR-449's skip path), and the coverage note appears
  # on stderr alongside it.
  local out_sec err_sec
  out_sec="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/sec.err")" || fail behavior-missing
  err_sec="$(cat "$run_dir/sec.err")"
  grep -Fq "$sec_header" <<<"$out_sec" || fail behavior-missing
  local line
  for line in 4 5 6; do
    grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out_sec" || fail "behavior-missing:line-$line"
  done
  assert_coverage_note "$err_sec"

  # Honest absence, but the coverage note is IDENTICAL: a diff with nothing
  # to match (--base HEAD against itself) still prints the header and "No
  # security findings." with exit 0 -- and the SAME catalog-wide coverage
  # note, because it does not depend on the diff. This is the exact case
  # the card's Outcome exists to fix: silence must never read as full
  # coverage.
  local out_clean err_clean
  out_clean="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD --seguranca 2>"$run_dir/clean.err")" || fail clean-run-failed
  err_clean="$(cat "$run_dir/clean.err")"
  grep -Fq "$sec_header" <<<"$out_clean" || fail honest-absence-header-missing
  grep -Fq 'No security findings.' <<<"$out_clean" || fail honest-absence-missing
  assert_coverage_note "$err_clean"
  [[ "$err_clean" == "$err_sec" ]] || fail "coverage-note-differs-between-empty-and-nonempty"

  # Determinism: same input, same bytes, on both streams.
  local out_again err_again
  out_again="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/again.err")" || fail rerun-failed
  err_again="$(cat "$run_dir/again.err")"
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic-stdout
  [[ "$err_sec" == "$err_again" ]] || fail non-deterministic-stderr

  # Composed with --fail-on: unaffected by the new note.
  local rc
  set +e
  (cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
  rc=$?
  set -e
  [[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

  # An explicit --modelo that cannot be served still fails loudly, before
  # any --seguranca output -- coverage note included -- ever prints.
  local out_modelo err_modelo
  set +e
  out_modelo="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --modelo local 2>"$run_dir/modelo.err")"
  rc=$?
  set -e
  err_modelo="$(cat "$run_dir/modelo.err")"
  [[ "$rc" -eq 1 ]] || fail "explicit-modelo-must-still-fail:$rc"
  grep -Fq 'model "local" is unavailable' <<<"$err_modelo" || fail explicit-modelo-error-missing
  if grep -Fq "$sec_header" <<<"$out_modelo"; then fail explicit-modelo-must-not-print-security-section; fi
  if grep -Fq "$coverage_prefix" <<<"$err_modelo"; then fail explicit-modelo-must-not-print-coverage-note; fi

  # Without --seguranca, no coverage note anywhere: it is scoped to the
  # security pass, never printed on every review.
  local out_plain err_plain
  out_plain="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 2>"$run_dir/plain.err")" || fail plain-run-failed
  err_plain="$(cat "$run_dir/plain.err")"
  if grep -Fq "$coverage_prefix" <<<"$out_plain$err_plain"; then fail coverage-note-must-not-appear-without-seguranca; fi

  # With a provider configured, stdout is byte-identical to what
  # AUR-442/AUR-449 already published -- proved here by the exact same
  # sha256 pin AUR-449 carries, because this card's whole point is adding
  # new output around this exact command without disturbing this exact
  # byte sequence -- and the same coverage note now appears on stderr.
  local out_prov prov_sha err_prov
  # Redirected to a file, not captured via `$(...)`: command substitution
  # strips trailing newlines, which would hash different bytes than the
  # program actually printed and than the file-based sha256sum this
  # constant was pinned against (see tests/acceptance/AUR-449.sh's own
  # note and tests/acceptance/AUR-433.sh's baseline_stdout_sha, the
  # precedent this idiom follows).
  (cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca) >"$run_dir/prov.out" 2>"$run_dir/prov.err" || fail provider-run-failed
  out_prov="$(cat "$run_dir/prov.out")"
  err_prov="$(cat "$run_dir/prov.err")"
  prov_sha="$(sha256sum "$run_dir/prov.out" | awk '{print $1}')"
  [[ "$prov_sha" == "$expected_with_provider_sha256" ]] || fail "provider-stdout-changed:$prov_sha"
  grep -Fq "$citation" <<<"$out_prov" || fail provider-citation-missing
  grep -Fq "$standard_citation" <<<"$out_prov" || fail provider-standard-missing
  assert_coverage_note "$err_prov"

  # The secret canary never reaches a sink alongside the new note.
  local canary="aurum-canary-450-$$"
  local out_canary err_canary
  out_canary="$(cd "$demo_repo" && AURUM_SECRET_CANARY="$canary" noprov_env "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
  err_canary="$(cat "$run_dir/canary.err")"
  if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi
}

# mutation_case is MUT-001: "esconder do usuario quais regras foram
# aplicadas faz o aceite falhar." It edits a writable staged copy of
# cmd/aurumcode/main.go, silencing exactly the coverage note's print call
# (the same technique tests/acceptance/AUR-448.sh's mutation_case uses for
# its own discard-warning print: an awk substring replace via ENVIRON, so
# the anchor's own parentheses never need regex escaping), rebuilds, and
# proves the coverage note vanishes while everything else -- the findings,
# their order, the security header, --fail-on -- is untouched. The
# committed source is never touched: the mutation exists only in this
# case's own staged copy.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/main.go"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor='printSecurityCoverage(stderr, coverageApplied, coverageTotal)'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  local replacement='_ = coverageApplied; _ = coverageTotal // MUT-001: suppress the coverage note silently'
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
  ' "$target" >"$target.mut" && mv "$target.mut" "$target"
  grep -Fq 'MUT-001: suppress the coverage note silently' "$target" || fail 'MUT-001/mutation-not-applied'
  [[ "$(grep -Fc "$anchor" "$target")" == 0 ]] || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local demo_repo="$root/tests/fixtures/repos/git-demo/repo.git"
  local out err rc
  set +e
  out="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/mut.err")"
  rc=$?
  set -e
  err="$(cat "$run_dir/mut.err")"

  # The mutant must still find and print the security section (the gate
  # itself is untouched) -- but the coverage note that names which rules
  # ran is gone. That silent omission is exactly the defect this card
  # exists to fix, so the mutant must be rejected.
  [[ "$rc" -eq 0 ]] || fail "MUT-001/mutation-changed-exit:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail 'MUT-001/mutation-broke-unrelated-behavior'
  local line
  for line in 4 5 6; do
    grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out" || fail 'MUT-001/mutation-broke-unrelated-behavior'
  done
  if grep -Fq "$coverage_prefix" <<<"$err"; then
    fail 'MUT-001/mutation-survived:coverage-note-still-present'
  fi

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-450.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur450_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR450UnitBridge(t *testing.T) { TestAUR450(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR450UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR450:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR450:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-450.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur450_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR450IntegrationBridge(t *testing.T) { IntegrationAUR450(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR450IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR450:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR450:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-450.sh
  chmod -R u+w -- "$root"
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-450.sh E2EAUR450) || fail e2e-failed
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
  TestAUR450) unit_case ;;
  IntegrationAUR450) integration_case ;;
  E2EAUR450) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
