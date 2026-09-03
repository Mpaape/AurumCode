#!/usr/bin/env bash
#
# Acceptance program for card AUR-449, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The 2026-08-14 re-test measured: `aurumcode review --base HEAD~1
#   --seguranca` with no LLM provider configured exits 1 with `no LLM
#   provider configured`, even though the security pass it runs (restored
#   by AUR-442) is a deterministic regex matcher over the diff's added
#   lines that calls no model at all. The only free, offline, deterministic
#   path this product has was locked behind the one thing that needs a
#   credential. This program proves the chosen fix: when --seguranca is
#   given, --modelo is NOT, and selectProvider's failure is specifically
#   "nothing is configured at all" (not some other provider failure such as
#   an unreadable fixture path), the command now skips the quality review
#   and runs the security pass alone, reporting its findings with exit 0 --
#   and says so plainly on stderr, never silently. Every path this card
#   does not own -- a configured provider, an explicit --modelo, a broken
#   (attempted) provider configuration, the no-flag no-provider refusal --
#   is proved byte-for-byte unaffected. See docs/specs/AUR-449.md.
#
# WHY THE LLM CALL IS A FIXTURE (where one is used at all)
#
#   The sealed profile (bootstrap-readonly-v1) denies network. The card's
#   own central proof is that NO provider is needed for --seguranca alone;
#   the one scenario that does configure a provider (proving that path is
#   unaffected) uses AURUMCODE_LLM_FIXTURE, the same canned deterministic
#   response already committed for the sibling review cards.
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

readonly card='AUR-449'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR449|IntegrationAUR449|E2EAUR449|AC-001-MUT-001) ;;
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
  tests/unit/AUR-449.go
  tests/integration/AUR-449.go
  tests/e2e/AUR-449.sh
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
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a449.XXXXXX")" || infra mktemp
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
readonly noprovider_text='no LLM provider configured'
readonly skip_text='quality review skipped'

# The exact sha256 of git-demo's `--seguranca` stdout with the fixture
# provider configured (AURUMCODE_LLM_FIXTURE=known-problem-response.json),
# manually verified during this card's development to be byte-identical
# between a binary built from the parent commit (before this card's change)
# and the candidate binary built from this card's tree: both produced
# 63c649af1c90e38b473e1bd45b4152b1f96ecad17d5d9c05c17bb94df7b8240f for
# stdout (and the empty-string hash for stderr). Pinning it here means any
# future change to this path that alters so much as one byte is caught,
# exactly the "byte-identical with a provider present" guarantee the card
# demands.
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

# nominal_case is AC-001's core behavioral proof: run the built binary
# exactly as a user would. RED before the implementation is behavioral --
# the binary builds, but --seguranca does not deliver the promised result
# without a provider -- and reports `behavior-missing`.
nominal_case() {
  build_shared
  local fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
  local demo_repo="$shared_root/tests/fixtures/repos/git-demo/repo.git"

  # Baseline sanity: WITHOUT --seguranca, the pre-existing AUR-430
  # no-provider refusal is completely untouched -- this card's paths
  # widened selectProvider's own behavior not at all, only runReview's
  # handling of its result.
  local out_plain err_plain rc
  set +e
  out_plain="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 2>"$run_dir/plain.err")"
  rc=$?
  set -e
  err_plain="$(cat "$run_dir/plain.err")"
  [[ "$rc" -eq 1 ]] || fail "no-seguranca-no-provider-must-still-fail:$rc"
  [[ -z "$out_plain" ]] || fail no-seguranca-unexpected-stdout
  grep -Fq "$noprovider_text" <<<"$err_plain" || fail no-seguranca-error-missing
  if grep -Fq "$skip_text" <<<"$err_plain"; then fail skip-note-must-not-appear-without-seguranca; fi

  # The card's central proof: --seguranca alone, with NO provider
  # configured at all, runs the security pass and reports its findings.
  local out_sec err_sec
  out_sec="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/sec.err")" || fail behavior-missing
  err_sec="$(cat "$run_dir/sec.err")"
  grep -Fq "$sec_header" <<<"$out_sec" || fail behavior-missing
  local line
  for line in 4 5 6; do
    grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out_sec" || fail "behavior-missing:line-$line"
  done
  if grep -Fq 'No issues found.' <<<"$out_sec"; then fail quality-section-falsely-claimed; fi
  grep -Fq "$noprovider_text" <<<"$err_sec" || fail skip-note-missing
  grep -Fq "$skip_text" <<<"$err_sec" || fail skip-note-missing

  # Determinism: same input, same bytes.
  local out_again
  out_again="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic

  # Honest absence on the skip path: a diff with nothing to match
  # (--base HEAD against itself, no new fixture needed) still prints the
  # header and "No security findings." with exit 0 -- the pass ran and
  # found nothing, which is not the same thing as "did not run."
  local out_clean
  out_clean="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD --seguranca)" || fail clean-run-failed
  grep -Fq "$sec_header" <<<"$out_clean" || fail honest-absence-header-missing
  grep -Fq 'No security findings.' <<<"$out_clean" || fail honest-absence-missing
  if grep -Fq 'No issues found.' <<<"$out_clean"; then fail quality-section-falsely-claimed; fi

  # Composed with --fail-on: the matched secrets (severity error) close
  # the gate even though no quality review ran at all.
  set +e
  (cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
  rc=$?
  set -e
  [[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

  # An explicit --modelo that cannot be served still fails loudly: an
  # explicit model choice must never be silently downgraded into the skip
  # just because --seguranca is present too.
  local out_modelo err_modelo
  set +e
  out_modelo="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --modelo local 2>"$run_dir/modelo.err")"
  rc=$?
  set -e
  err_modelo="$(cat "$run_dir/modelo.err")"
  [[ "$rc" -eq 1 ]] || fail "explicit-modelo-must-still-fail:$rc"
  grep -Fq 'model "local" is unavailable' <<<"$err_modelo" || fail explicit-modelo-error-missing
  if grep -Fq "$sec_header" <<<"$out_modelo"; then fail explicit-modelo-must-not-print-security-section; fi

  # A caller who attempted configuration and got it wrong (an
  # AURUMCODE_LLM_FIXTURE path that does not exist) is a different error
  # than "nothing configured": it must not be silently downgraded either.
  local out_broken err_broken
  set +e
  out_broken="$(cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$run_dir/does-not-exist.json" "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/broken.err")"
  rc=$?
  set -e
  err_broken="$(cat "$run_dir/broken.err")"
  [[ "$rc" -eq 1 ]] || fail "broken-provider-must-still-fail:$rc"
  if grep -Fq "$skip_text" <<<"$err_broken"; then fail broken-provider-must-not-trigger-skip; fi
  if grep -Fq "$sec_header" <<<"$out_broken"; then fail broken-provider-must-fail-before-output; fi

  # With a provider configured, output is byte-identical to what AUR-442
  # already published -- proved here by an exact sha256 pin, manually
  # cross-checked against the parent (pre-AUR-449) binary during
  # development (see the constant's own comment above).
  local out_prov prov_sha
  # Redirected to a file, not captured via `$(...)`: command substitution
  # strips trailing newlines, which would hash different bytes than the
  # program actually printed and than the file-based sha256sum this
  # constant was pinned against (see the constant's own comment above and
  # tests/acceptance/AUR-433.sh's own baseline_stdout_sha, the precedent
  # this idiom follows).
  (cd "$demo_repo" && AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca) >"$run_dir/prov.out" || fail provider-run-failed
  out_prov="$(cat "$run_dir/prov.out")"
  prov_sha="$(sha256sum "$run_dir/prov.out" | awk '{print $1}')"
  [[ "$prov_sha" == "$expected_with_provider_sha256" ]] || fail "provider-output-changed:$prov_sha"
  grep -Fq "$citation" <<<"$out_prov" || fail provider-citation-missing
  grep -Fq "$standard_citation" <<<"$out_prov" || fail provider-standard-missing

  # The secret canary never reaches a sink on the skip path.
  local canary="aurum-canary-449-$$"
  local out_canary err_canary
  out_canary="$(cd "$demo_repo" && AURUM_SECRET_CANARY="$canary" noprov_env "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/canary.err")" || fail canary-run-failed
  err_canary="$(cat "$run_dir/canary.err")"
  if grep -Fq "$canary" <<<"$out_canary$err_canary"; then fail canary-leaked; fi
}

# mutation_case is MUT-001: reverting to require a provider for the
# --seguranca-only path must make the acceptance fail. It edits a writable
# staged copy of cmd/aurumcode/main.go, neutralizing exactly the condition
# that enables the skip (appending "&& false", a fixed-string full-line
# replacement, so the mutation is minimal and precise), rebuilds, and
# proves the OLD, broken behavior returns: --seguranca alone with no
# provider configured exits 1 with the plain no-provider refusal and NO
# security section -- i.e. that nominal_case's presence assertions above
# are load-bearing. The committed source is never touched: the mutation
# exists only in this case's own staged copy.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.

  local root="$run_dir/root-mut"
  stage_source "$root"

  local target="$root/cmd/aurumcode/main.go"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor=$'\tif providerErr != nil && *modelo == "" && *seguranca && errors.Is(providerErr, errNoProviderConfigured) {'
  local replacement=$'\tif providerErr != nil && *modelo == "" && *seguranca && errors.Is(providerErr, errNoProviderConfigured) && false {'
  [[ "$(grep -Fxc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  awk -v anchor="$anchor" -v replacement="$replacement" '
    $0 == anchor { print replacement; found++; next }
    { print }
    END { if (found != 1) exit 1 }
  ' "$target" >"$target.mut" || fail 'MUT-001/rewrite-failed'
  mv "$target.mut" "$target"
  [[ "$(grep -Fxc "$anchor" "$target")" == 0 ]] || fail 'MUT-001/mutation-not-applied'
  [[ "$(grep -Fxc "$replacement" "$target")" == 1 ]] || fail 'MUT-001/mutation-not-applied'

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

  # The mutant must reproduce the exact pre-AUR-449 refusal: requiring a
  # provider even for the deterministic security-only pass.
  [[ "$rc" -eq 1 ]] || fail "MUT-001/mutation-survived:exit:$rc"
  if [[ -n "$out" ]]; then fail 'MUT-001/mutation-survived:stdout-not-empty'; fi
  if grep -Fq "$sec_header" <<<"$out"; then fail 'MUT-001/mutation-survived:security-section-present'; fi
  grep -Fq "$noprovider_text" <<<"$err" || fail 'MUT-001/unexpected-shape'
  if grep -Fq "$skip_text" <<<"$err"; then fail 'MUT-001/mutation-survived:skip-note-present'; fi

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-449.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur449_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR449UnitBridge(t *testing.T) { TestAUR449(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR449UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR449:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR449:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-449.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur449_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR449IntegrationBridge(t *testing.T) { IntegrationAUR449(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR449IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR449:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR449:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-449.sh
  chmod -R u+w -- "$root"
  # Reuse the already-built binary and the warm shared GOCACHE instead of a
  # cold nested build (see build_shared).
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-449.sh E2EAUR449) || fail e2e-failed
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
  TestAUR449) unit_case ;;
  IntegrationAUR449) integration_case ;;
  E2EAUR449) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
