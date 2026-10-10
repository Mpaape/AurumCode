#!/usr/bin/env bash
# AUR-613 acceptance: a second central layer, the engineering standards,
# below the security policy and above the reviewed repository. Declared with
# --padroes (or AURUMCODE_STANDARDS) and by standards_repository in the
# reusable workflow, it contributes skills and analysts only; a config.yml
# with any section is refused; a directory inside the reviewed tree is
# refused; the review names the layer on its findings; the audit records it
# only when it exists; without it nothing changes.
#
# Selectors:
#   all      AC-001..AC-008, MUT-001..MUT-003
#   AC-001   a standards skill yields a finding on --base, --pr and the agent
#            server, named after the layer and counted in the gate as skills
#   AC-002   analysts of the standards profiles.yml run when selected
#   AC-003   policy > standards > repository by skill name, with warnings
#   AC-004   config.yml with gate/ignore/exceptions/rules/quality_gates/
#            publication is refused
#   AC-005   a standards directory inside the reviewed tree is refused
#   AC-006   review.yml checks standards_repository out into
#            .aurumcode-standards (persist-credentials false) and passes
#            --padroes only with the input
#   AC-007   the audit records the layer (origin, digest) only when declared
#   AC-008   without --padroes and AURUMCODE_STANDARDS the output is the same
#   MUT-001  accepting gate in the layer's config.yml turns AC-004 red
#   MUT-002  inverting the policy/standards order turns AC-003 red
#   MUT-003  accepting a directory inside the reviewed tree turns AC-005 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-613'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 AC-006 AC-007 AC-008 MUT-001 MUT-002 MUT-003'
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

readonly pkgs=(./internal/config ./cmd/aurumcode)

# stage copies the module (go.mod, go.sum, cmd, internal, pkg) and the
# reusable workflow (read by the AC-006 test at ../../.github/workflows)
# into a fresh root.
stage() {
  local root="$1"
  mkdir -p "$root/.github/workflows"
  for item in go.mod go.sum cmd internal pkg; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/"
  done
  [[ -f "$repo_root/.github/workflows/review.yml" ]] || infra missing-review.yml
  cp "$repo_root/.github/workflows/review.yml" "$root/.github/workflows/review.yml"
  chmod -R u+w -- "$root"
}

# run_test runs the named tests of the packages in root.
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
    tail -n 40 "$log" >&2
    fail "test-failed"
  fi
  for name in "$@"; do
    grep -Eq -- "^--- PASS: $name " "$log" || { tail -n 30 "$log" >&2; fail "$id-missing-pass:$name"; }
  done
  rm -f -- "$log"
  printf '%s/%s/pass\n' "$card" "$id"
}

# mutate id file anchor sed-expr pattern assertion: with the defect back the
# named test must fail (compiling, for the right reason); the staged copy
# is removed afterwards so the sealed /tmp never holds two modules at once.
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
  chmod -R u+w -- "$root" 2>/dev/null || true
  rm -rf -- "$root" "$log"
  go clean -testcache >/dev/null 2>&1 || true
  printf '%s/%s/rejected (%s)\n' "$card" "$id" "$assertion"
}

ac001() {
  ac AC-001 '^TestAUR613(StandardsSkillFindingOnBase|EnvDeclaresLayerAndEmptyFlagIsUsageError|StandardsSkillFindingOnPR|StandardsSkillThroughAgentServer|StandardsFindingCountsInTheGate)$' \
    TestAUR613StandardsSkillFindingOnBase TestAUR613EnvDeclaresLayerAndEmptyFlagIsUsageError \
    TestAUR613StandardsSkillFindingOnPR TestAUR613StandardsSkillThroughAgentServer TestAUR613StandardsFindingCountsInTheGate
}
ac002() {
  ac AC-002 '^TestAUR613(StandardsAnalystRunsWhenSelected|StandardsAnalystRunsOnPR|StandardsAnalystShadowsRepositoryAnalyst)$' \
    TestAUR613StandardsAnalystRunsWhenSelected TestAUR613StandardsAnalystRunsOnPR TestAUR613StandardsAnalystShadowsRepositoryAnalyst
}
ac003() {
  ac AC-003 '^TestAUR613(ThreeLayersOrderAndAttribution|StandardsWinOverRepositorySkill)$' \
    TestAUR613ThreeLayersOrderAndAttribution TestAUR613StandardsWinOverRepositorySkill
}
ac004() {
  ac AC-004 '^TestAUR613StandardsConfigWithSectionIsRefused$' TestAUR613StandardsConfigWithSectionIsRefused
}
ac005() {
  ac AC-005 '^TestAUR613StandardsInsideReviewedTreeIsRefused$' TestAUR613StandardsInsideReviewedTreeIsRefused
}

# need <file> <literal> fails unless the file carries the literal.
need() {
  [[ -f "$repo_root/$1" ]] || infra "missing-$1"
  grep -Fq -- "$2" "$repo_root/$1" || fail "AC-006/$1/lacks:$2"
}

ac006() {
  need .github/workflows/review.yml 'standards_repository:'
  need .github/workflows/review.yml 'standards_ref:'
  need .github/workflows/review.yml 'AURUMCODE_STANDARDS_TOKEN:'
  need .github/workflows/review.yml 'path: .aurumcode-standards'
  need .github/workflows/review.yml 'token: ${{ secrets.AURUMCODE_STANDARDS_TOKEN || github.token }}'
  need .github/workflows/review.yml 'args+=(--padroes /github/standards)'
  need .github/workflows/review.yml '.aurumcode-standards:/github/standards:ro'
  # The checkout step persists no credentials: the only persist-credentials
  # between the step's name and the next step must be false.
  awk '/name: Checkout engineering standards/{f=1} f&&/persist-credentials/{print; exit}' "$repo_root/.github/workflows/review.yml" |
    grep -Fq 'persist-credentials: false' || fail 'AC-006/review.yml/standards-checkout-persists-credentials'
  need docs/configuration.md '## Camada de padrões de engenharia'
  need docs/configuration.md 'standards_repository'
  need docs/gate-corporativo.md 'standards_repository'
  ac AC-006 '^TestAUR613WorkflowChecksOutStandardsLayer$' TestAUR613WorkflowChecksOutStandardsLayer
}
ac007() {
  ac AC-007 '^TestAUR613AuditRecordsStandardsLayerOnlyWhenDeclared$' TestAUR613AuditRecordsStandardsLayerOnlyWhenDeclared
}
ac008() {
  ac AC-008 '^TestAUR613WithoutTheLayerNothingChanges$' TestAUR613WithoutTheLayerNothingChanges
}

mut001() {
  mutate MUT-001 internal/config/layer.go 'Analysts: true, Sections: NoSection}' \
    's/Analysts: true, Sections: NoSection}/Analysts: true, Sections: AnySection}/' \
    '^TestAUR613StandardsConfigWithSectionIsRefused$' 'TestAUR613StandardsConfigWithSectionIsRefused'
}
mut002() {
  mutate MUT-002 internal/config/layer.go 'LayerName{LayerPolicy, LayerStandards, LayerRepository}' \
    's/LayerName{LayerPolicy, LayerStandards, LayerRepository}/LayerName{LayerStandards, LayerPolicy, LayerRepository}/' \
    '^TestAUR613ThreeLayersOrderAndAttribution$' 'TestAUR613ThreeLayersOrderAndAttribution'
}
mut003() {
  mutate MUT-003 cmd/aurumcode/review_layers.go 'if err := config.ValidateLayerOutsideReviewedTree(layer, dir, reviewedRoot, language); err != nil {' \
    's/if err := config.ValidateLayerOutsideReviewedTree(layer, dir, reviewedRoot, language); err != nil {/if err := error(nil); err != nil {/' \
    '^TestAUR613StandardsInsideReviewedTreeIsRefused$' 'TestAUR613StandardsInsideReviewedTreeIsRefused'
}

# One function per selector; all runs every one in order.
if [[ "$selector" == 'all' ]]; then
  for step in ac001 ac002 ac003 ac004 ac005 ac006 ac007 ac008 mut001 mut002 mut003; do
    "$step"
  done
  printf '%s/all/pass\n' "$card"
else
  step="${selector//-/}"
  step="${step,,}"
  "$step"
fi
