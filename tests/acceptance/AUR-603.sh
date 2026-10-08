#!/usr/bin/env bash
# AUR-603 acceptance: every model finding that would block the gate goes
# through an adversarial verification against the reviewed code. Only a
# refutation whose quote exists literally in the shown code demotes it (to a
# marked, audited, non-blocking comment); everything else keeps it blocking.
#
# Selectors:
#   all        AC-001..AC-005, MUT-001, MUT-002
#   AC-001     refuted with a literal quote: demoted, marked, audited, out of the gate
#   AC-002     refuted with a quote that is not in the revision: still blocks
#   AC-003     confirmed, uncertain, invalid answer, provider error, ceiling: still blocks
#   AC-004     a deterministic finding is never sent to the verifier
#   AC-005     enabled: false sends nothing; max_calls bounds the calls
#   MUT-001    accepting a refutation without checking the quote turns AC-002 RED
#   MUT-002    demoting when the verifier fails turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-603'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/review/verify/ ./internal/config/ ./cmd/aurumcode/)
readonly ac1='^(TestAC001RefutedWithLiteralQuoteIsDemotedAndRecorded|TestVerificationDemotesRefutedModelFindingBeforeTheGate)$'
readonly ac2='^(TestAC002RefutedWithQuoteNotInRevisionKeepsBlocking|TestAC002TrailingSpacesAreTheOnlyForgivenDifference|TestVerificationKeepsBlockingWhenTheQuoteIsNotTheCode)$'
readonly ac3='^(TestAC003EveryOtherOutcomeKeepsBlocking)$'
readonly ac4='^(TestAC004DeterministicFindingsAreNeverSent|TestVerificationDemotesRefutedModelFindingBeforeTheGate)$'
readonly ac5='^(TestAC005NonBlockingFindingsAndTheCeiling|TestVerificationDisabledSendsNothing|TestReviewVerificationDefaultsAndValidation)$'
readonly quote_check='if strings.Contains(normalize(content), q) {'
readonly provider_error='return v.kept(rec, OutcomeProviderError, err.Error())'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a603.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"
  shift 3
  local targets=("$@")
  [[ "${#targets[@]}" -gt 0 ]] || targets=("${pkgs[@]}")
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${targets[@]}" ) >"$log" 2>&1
}

run_ac() {
  local name="$1" pattern="$2" want="$3" root="$run_dir/root" log="$run_dir/$1.log" passes
  command -v go >/dev/null 2>&1 || infra missing_go
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$pattern" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  passes="$(grep -Ec -- '^--- PASS: ' "$log" || true)"
  [[ "$passes" == "$want" ]] || { cat "$log" >&2; fail "want-$want-passes-got-$passes"; }
  printf '%s/%s/pass\n' "$card" "$name"
}

expect_red() {
  local root="$1" log="$2" pattern="$3"
  if go_test "$root" "$log" "$pattern" ./internal/review/verify/; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

run_mut001() {
  local root="$run_dir/root-mut1" file
  stage "$root"
  file="$root/internal/review/verify/quote.go"
  [[ "$(grep -Fc -- "$quote_check" "$file")" == 1 ]] || infra quote-check-missing
  # Any non-empty quote is taken as found: the refutation is never checked.
  sed -i 's/if strings.Contains(normalize(content), q) {/if strings.Contains(normalize(content), q) || q != "" {/' "$file"
  ! grep -Fq -- "$quote_check" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$ac2"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" file
  stage "$root"
  file="$root/internal/review/verify/verifier.go"
  [[ "$(grep -Fc -- "$provider_error" "$file")" == 2 ]] || infra provider-error-path-missing
  # A failed verifier call demotes the finding instead of keeping it.
  sed -i 's/return v.kept(rec, OutcomeProviderError, err.Error())/rec.Outcome, rec.Demoted = OutcomeRefuted, true; return rec/' "$file"
  ! grep -Fq -- "$provider_error" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut2.log" "$ac3"
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac1" 2 ;;
  AC-002) run_ac AC-002 "$ac2" 3 ;;
  AC-003) run_ac AC-003 "$ac3" 1 ;;
  AC-004) run_ac AC-004 "$ac4" 2 ;;
  AC-005) run_ac AC-005 "$ac5" 3 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "$ac1" 2
    run_ac AC-002 "$ac2" 3
    run_ac AC-003 "$ac3" 1
    run_ac AC-004 "$ac4" 2
    run_ac AC-005 "$ac5" 3
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
