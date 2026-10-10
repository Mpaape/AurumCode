#!/usr/bin/env bash
# AUR-608 acceptance: the model decides over the deterministic evidence.
# gate.triage absent is model; an explicit none opts a source out; a dispute
# demotes only with a justification (without one it weighs as needs_context
# and keeps counting); the triage acts only for a declared gate; without the
# model's answer, with evidence and a declared gate, stderr and the parecer
# say the triage did not happen and the block was kept; a model that did not
# answer cleanly (a degraded profile or batch included) never demotes; under
# a central policy nothing changes.
#
# Selectors:
#   all      AC-001..AC-006, MUT-001..MUT-003
#   AC-001   no triage key, declared gate, justified dispute: the evidence
#            stops counting and the parecer names the triaged source
#   AC-002   an explicit none keeps the evidence counting
#   AC-003   a dispute with an empty justification keeps counting
#   AC-004   without the model, or with a degraded answer, nothing is demoted,
#            the block is kept and a line says the triage did not happen
#   AC-005   without a declared gate nothing is triaged or announced
#   AC-006   under a central policy the result is the one before
#   MUT-001  the default back to none turns AC-001 red
#   MUT-002  a dispute accepted without a justification turns AC-003 red
#   MUT-003  a degraded answer allowed to demote turns AC-004 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-608'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 AC-006 MUT-001 MUT-002 MUT-003'
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

readonly pkgs=(./internal/config ./internal/review ./cmd/aurumcode)

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
  ac AC-001 '^(TestAUR608DefaultTriageDemotesJustifiedDispute|TestAUR608TriageLinesReachTheParecer|TestGateTriageParse|TestAUR608GateTriageByModelForScannerDefault)$' \
    TestAUR608DefaultTriageDemotesJustifiedDispute TestAUR608TriageLinesReachTheParecer TestGateTriageParse TestAUR608GateTriageByModelForScannerDefault
}
ac002() {
  ac AC-002 '^(TestAUR608ExplicitNoneKeepsEvidenceCounting|TestAUR608GateTriageExplicitNoneOptsOut|TestAUR579PolicyFloorAndRepositoryTriage)$' \
    TestAUR608ExplicitNoneKeepsEvidenceCounting TestAUR608GateTriageExplicitNoneOptsOut TestAUR579PolicyFloorAndRepositoryTriage
}
ac003() {
  ac AC-003 '^(TestAUR608DisputeWithoutJustificationStillCounts|TestAUR608DisputeCountsRequiresJustification)$' \
    TestAUR608DisputeWithoutJustificationStillCounts TestAUR608DisputeCountsRequiresJustification
}
ac004() {
  ac AC-004 '^(TestAUR608WithoutModelKeepsDeterministicBlock|TestAUR608TriageLinesReachTheParecer|TestAUR608DegradedModelNeverDemotes)$' \
    TestAUR608WithoutModelKeepsDeterministicBlock TestAUR608TriageLinesReachTheParecer TestAUR608DegradedModelNeverDemotes
}
ac005() {
  ac AC-005 '^TestAUR608TriageSilentWithoutDeclaredGate$' TestAUR608TriageSilentWithoutDeclaredGate
}
ac006() {
  ac AC-006 '^(TestAUR608CentralPolicyUnchanged|TestAUR579PolicyFloorAndRepositoryTriage)$' \
    TestAUR608CentralPolicyUnchanged TestAUR579PolicyFloorAndRepositoryTriage
}

mut001() {
  mutate MUT-001 internal/config/gate.go 'const triageDefault = TriageModel' \
    's/const triageDefault = TriageModel/const triageDefault = TriageNone/' '^TestAUR608DefaultTriageDemotesJustifiedDispute$' 'TestAUR608DefaultTriageDemotesJustifiedDispute'
}
mut002() {
  mutate MUT-002 internal/review/assessments_dispute.go 'return strings.TrimSpace(a.Justification) != ""' \
    's/return strings.TrimSpace(a.Justification) != ""/return strings.TrimSpace(a.Justification) != "mutant"/' '^TestAUR608DisputeWithoutJustificationStillCounts$' 'TestAUR608DisputeWithoutJustificationStillCounts'
}
mut003() {
  mutate MUT-003 cmd/aurumcode/review_gate.go 'if s.modelReason() != "" {' \
    's/if s.modelReason() != "" {/if false {/' '^TestAUR608DegradedModelNeverDemotes$' 'TestAUR608DegradedModelNeverDemotes'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 ac005 ac006 mut001 mut002 mut003; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi
