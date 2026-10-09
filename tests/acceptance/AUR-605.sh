#!/usr/bin/env bash
# AUR-605 acceptance: when the LLM provider fails, the operator's fallback
# providers (LLM_FALLBACK_<n>_*) answer the same request, in order, each with
# its own URL, key and model; every switch is announced; when all fail the
# review fails closed naming each one; a half-configured slot fails before
# any request; the chain is bounded by one timeout per provider.
#
# Selectors:
#   all      AC-001..AC-009, MUT-001..MUT-003
#   AC-001   the primary fails, the fallback answers and the switch is announced
#   AC-002   all fail: error naming each provider
#   AC-003   no slot set: the primary alone, unchanged
#   AC-004   tool rounds only go to fallbacks that call tools
#   AC-005   one timeout per provider leaves the fallback time to answer
#   AC-006   slots resolve in order through the catalog, with their own key and model
#   AC-007   a slot set but unusable fails naming the slot's own variables
#   AC-008   workflow inputs/secrets and docs describe the fallback
#   AC-009   an answer from a fallback is never cached as the primary's, and the
#            cache key keeps the primary's URL
#   MUT-001  a single timeout for the chain turns AC-005 red
#   MUT-002  the primary's key reaching a slot turns AC-007 red
#   MUT-003  a silent switch turns AC-001 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-605'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 AC-006 AC-007 AC-008 AC-009 MUT-001 MUT-002 MUT-003'
if [[ " $known " != *" $selector "* ]]; then
  printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
  exit 64
fi

fail() {
  printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2
  exit 1
}
infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2
  exit 79
}

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
: "${GOCACHE:=$(mktemp -d)}"
export GOCACHE

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

readonly pkgs=(./internal/llm ./internal/llm/provider/profiles ./cmd/aurumcode)

# stage copies the module (whole cmd, internal, pkg) into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  chmod -R u+w -- "$root"
}

# run_test runs the named tests of the three packages in root.
run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}") >"$log" 2>&1
}

base="$work/base"
stage "$base"

# ac id pattern tests...: every named test must pass.
ac() {
  local id="$1" pattern="$2" log="$work/$1.log" name
  shift 2
  if ! run_test "$base" "$pattern" "$log"; then
    tail -n 30 "$log" >&2
    fail "test-failed"
  fi
  for name in "$@"; do
    grep -Eq -- "^--- PASS: $name " "$log" || { tail -n 30 "$log" >&2; fail "$id-missing-pass:$name"; }
  done
  printf '%s/%s/pass\n' "$card" "$id"
}

mutate() {
  local id="$1" file="$2" anchor="$3" expr="$4" pattern="$5" assertion="$6"
  local root="$work/$id" log="$work/$id.log"
  stage "$root"
  grep -Fq -- "$anchor" "$root/$file" || infra "$id-anchor-missing"
  sed -i "$expr" "$root/$file"
  if grep -Fq -- "$anchor" "$root/$file"; then
    infra "$id-not-applied"
  fi
  if run_test "$root" "$pattern" "$log"; then
    fail "$id-survived"
  fi
  if grep -Eq 'build failed|setup failed' "$log"; then
    tail -n 20 "$log" >&2
    infra "$id-did-not-compile"
  fi
  grep -Eq -- "^--- FAIL: $assertion" "$log" || {
    tail -n 20 "$log" >&2
    infra "$id-unexpected-failure"
  }
  rm -rf -- "$root"
  printf '%s/%s/rejected (%s)\n' "$card" "$id" "$assertion"
}

ac001() {
  ac AC-001 '^TestAUR605(PrimaryFailsFallbackAnswers|HealthyPrimaryAloneIsCalled|ProviderFromEnvFallsBackToTheSlot)$' \
    TestAUR605PrimaryFailsFallbackAnswers TestAUR605HealthyPrimaryAloneIsCalled TestAUR605ProviderFromEnvFallsBackToTheSlot
}
ac002() {
  ac AC-002 '^TestAUR605AllFailNamesEveryProvider$' TestAUR605AllFailNamesEveryProvider
}
ac003() {
  ac AC-003 '^TestAUR605(NoFallbackIsThePrimaryItself|NoSlotNoFallback)$' \
    TestAUR605NoFallbackIsThePrimaryItself TestAUR605NoSlotNoFallback
}
ac004() {
  ac AC-004 '^TestAUR605ToolRoundsSkipMembersWithoutTools$' TestAUR605ToolRoundsSkipMembersWithoutTools
}
ac005() {
  ac AC-005 '^TestAUR605ChainTimeoutLeavesTimeForTheFallback$' TestAUR605ChainTimeoutLeavesTimeForTheFallback
}
ac006() {
  ac AC-006 '^TestAUR605SlotsResolveInOrderWithTheirOwnKey$' TestAUR605SlotsResolveInOrderWithTheirOwnKey
}
ac007() {
  ac AC-007 '^TestAUR605(UnusableSlotFailsNamingIt|UnusableSlotFailsTheSelection|NativeKeyNeverReachesASlot)$' \
    TestAUR605UnusableSlotFailsNamingIt TestAUR605UnusableSlotFailsTheSelection TestAUR605NativeKeyNeverReachesASlot
}
ac009() {
  ac AC-009 '^TestAUR605(FallbackAnswerIsMarkedAndURLIsThePrimarys|CacheKeyKeepsThePrimaryURL)$' \
    TestAUR605FallbackAnswerIsMarkedAndURLIsThePrimarys TestAUR605CacheKeyKeepsThePrimaryURL
}

# need <file> <literal> fails unless the file carries the literal.
need() {
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  grep -Fq -- "$2" "$repo_root/$1" || fail "AC-008/$1/lacks:$2"
}

ac008() {
  local n
  for n in 1 2; do
    need .github/workflows/review.yml "fallback_${n}_provider:"
    need .github/workflows/review.yml "fallback_${n}_model:"
    need .github/workflows/review.yml "LLM_FALLBACK_${n}_API_KEY: \${{ secrets.LLM_FALLBACK_${n}_API_KEY }}"
    need .github/workflows/review.yml "LLM_FALLBACK_${n}_BASE_URL: \${{ secrets.LLM_FALLBACK_${n}_BASE_URL }}"
    need .github/workflows/review.yml "-e LLM_FALLBACK_${n}_API_KEY"
  done
  need docs/provedores.md '## Provedor reserva (fallback)'
  need docs/provedores.md 'LLM_FALLBACK_<n>_PROVIDER'
  need docs/provedores.md 'trying fallback'
  need docs/tutorials/provedores.md '<!-- saida: reserva-assume -->'
  need docs/tutorials/provedores.md '<!-- saida: falha-todas-caem -->'
  printf '%s/AC-008/pass\n' "$card"
}

mut001() {
  mutate MUT-001 internal/llm/fallback.go 'return each * time.Duration(len(f.providers))' \
    's/return each \* time.Duration(len(f.providers))/return each/' '^TestAUR605ChainTimeoutLeavesTimeForTheFallback$' 'TestAUR605ChainTimeoutLeavesTimeForTheFallback'
}
mut002() {
  mutate MUT-002 internal/llm/provider/profiles/fallback.go 'return getenv(slotVar(n, slotAPIKey))' \
    's/return getenv(slotVar(n, slotAPIKey))/return getenv(name)/' '^TestAUR605UnusableSlotFailsNamingIt$' 'TestAUR605UnusableSlotFailsNamingIt'
}
mut003() {
  mutate MUT-003 internal/llm/fallback.go 'if i+1 < len(f.providers) && f.notice != nil {' \
    's/if i+1 < len(f.providers) \&\& f.notice != nil {/if false \&\& f.notice != nil {/' '^TestAUR605PrimaryFailsFallbackAnswers$' 'TestAUR605PrimaryFailsFallbackAnswers'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 ac005 ac006 ac007 ac008 ac009 mut001 mut002 mut003; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi
