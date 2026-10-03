#!/usr/bin/env bash
# AUR-583 acceptance: architecture guards cover internal/. The import
# direction is a table in docs/architecture.md read by a test; the central
# policy's authority over every Config field is a declarative table checked
# by reflection; no function of cmd, internal or pkg passes 150 lines; the
# refactor (gate facts in pkg/types, contributors returning partial results,
# one token heuristic, yaml.v3 front matter, a slot failure that is an error)
# leaves the audit, the SARIF and the reports byte for byte as they were.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001..MUT-005
#   AC-001     the layer table is read and every production import obeys it
#   AC-002     every Config/ReviewConfig/QualityGatesConfig field is classified
#   AC-003     no function over 150 lines, no production file named by a card
#   AC-004     audit, SARIF and report tests plus the recorded tutorials
#   MUT-001    internal/gate importing internal/render turns AC-001 RED
#   MUT-002    a new Config field without a classification turns AC-002 RED
#   MUT-003    ApplyCentralPolicy grown past 150 lines turns AC-003 RED
#   MUT-004    a review source returning the breach code as a literal is RED
#   MUT-005    a map keyed by a named engine constant in internal/gate is RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-583'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003|MUT-004|MUT-005) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg docs/architecture.md cmd/aurumcode/structure_test.go internal/config/governance.go internal/config/governance_test.go pkg/types/gatefacts.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a583.XXXXXX")" || infra mktemp
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
  for source in go.mod go.sum cmd internal pkg docs demo; do
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  for source in tests/fixtures tests/e2e; do
    if [[ -d "$repo_root/$source" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/$source" "$root/$source"; fi
  done
  chmod -R u+w -- "$root"
}

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

# require_pass log names... fails unless every named test passed.
require_pass() {
  local log="$1" name; shift
  for name in "$@"; do
    grep -Eq -- "^--- PASS: ${name} " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

# replace_once file anchor replacement: a literal, unique edit.
replace_once() {
  local file="$1" anchor="$2" replacement="$3" count
  count="$(grep -Fc -- "$anchor" "$file")" || infra "anchor-missing:${file##*/}"
  [[ "$count" == 1 ]] || infra "anchor-not-unique:${file##*/}"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { a = ENVIRON["ANCHOR"]; r = ENVIRON["REPL"] }
    { i = index($0, a); if (i > 0) { print substr($0, 1, i - 1) r substr($0, i + length(a)) } else { print } }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  grep -Fq -- "$replacement" "$file" || infra "mutation-not-applied:${file##*/}"
}

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

readonly ac001_tests=(TestImportsFollowLayerTable TestAUR558ArchitectureDocCitesEveryInternalPackage)
readonly ac002_tests=(TestEveryConfigFieldHasCentralPolicyAuthority TestAUR518ApplyCentralPolicyNilIsUnchanged TestAUR518ApplyCentralPolicyRuleOverrideIgnored TestAUR518ApplyCentralPolicyIgnoreOverrideDropped TestAUR518ApplyCentralPolicyKeepsRepoContextAndLanguageFallback TestAUR518ApplyCentralPolicyLanguageAndPublicationOverride TestAUR580PolicyDeliberationDecidesAlone)
readonly ac003_tests=(TestNoFunctionExceedsLineLimitAnywhere TestNoProductionFileNamedByCardAnywhere TestAUR557NoFunctionExceedsLineLimit TestReviewSourcesReturnNoGateExitCode TestNoEngineBranchInGateOrCmdByAnyName)
readonly ac004_tests=(TestAUR521AuditAndSARIFOnGateBreach TestAUR521AuditFingerprintStableAcrossTwoRuns TestAUR521PRPathWritesComplianceArtifacts TestAUR521ExceptedFindingSuppressedNotBlocking TestAUR521AuditAndSARIFAgreeOnInconclusive TestAUR521PolicyDigestChangesWhenPolicyChanges TestAUR568BaseWritablePathsUnchanged TestAUR577FakeEngineReachesEverySinkByOrigin TestPipelineAppliesContributorsInDeclaredOrder TestPipelineContributorErrorIsInconclusiveNeverApproved TestResultMergeJoinsReasonsAndIgnoresInactive TestSlotFailureIsAnErrorNeverAPanic TestFrontMatterAcceptsTheShapesSkillsUse)
readonly pkgs=(./cmd/aurumcode/ ./internal/config/ ./internal/gate/ ./internal/render/ ./internal/prompt/ ./internal/context/skills/)
# Tutorials whose recorded output carries an audit record, a SARIF document
# or a gate report; their expected/ was recorded before this refactor.
readonly tutorials=(auditoria-sarif gate excecoes dados-de-analise politica-central sbom-dependency-track segredos skills revisao)

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
  printf '%s/%s/pass\n' "$card" "$name"
}

run_ac004() {
  run_ac AC-004 "${ac004_tests[@]}"
  local t
  for t in "${tutorials[@]}"; do
    [[ -f "$repo_root/demo/tutoriais/$t/run.sh" ]] || infra "missing-tutorial:$t"
    (cd "$repo_root" && bash "demo/tutoriais/$t/run.sh" --check >"$run_dir/tut-$t.log" 2>&1) || { cat "$run_dir/tut-$t.log" >&2; fail "tutorial-check-failed:$t"; }
  done
  printf '%s/AC-004/tutorials/pass\n' "$card"
}

# expect_red root log tests...: the mutated copy must compile and turn at
# least one named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2"; shift 2
  if go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used|imported and not used|import cycle' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:|^    ' "$log" | sed -n '1,4p' >&2
}

# MUT-001: the gate reaches back into the presentation package.
run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/gate/result.go" \
    '"github.com/Mpaape/AurumCode/pkg/types"' \
    '"github.com/Mpaape/AurumCode/pkg/types"
	"github.com/Mpaape/AurumCode/internal/render"'
  printf '\n// MUT-001: the domain reads a presentation helper.\nvar _ = render.GateSARIFFindings\n' >>"$root/internal/gate/result.go"
  expect_red "$root" "$run_dir/mut1.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestImportsFollowLayerTable' "$run_dir/mut1.log" || fail mut001-wrong-test
  grep -Fq 'internal/gate/result.go imports internal/render' "$run_dir/mut1.log" || fail mut001-not-named
  printf '%s/MUT-001/rejected\n' "$card"
}

# MUT-002: Config grows a section nobody classified.
run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/config/config.go" \
    'Deliberation *DeliberationConfig `yaml:"deliberation"`' \
    'Deliberation *DeliberationConfig `yaml:"deliberation"`
	// MUT-002: a section born without a central-policy classification.
	Telemetry string `yaml:"telemetry"`'
  expect_red "$root" "$run_dir/mut2.log" "${ac002_tests[@]}"
  grep -Eq -- '^--- FAIL: TestEveryConfigFieldHasCentralPolicyAuthority' "$run_dir/mut2.log" || fail mut002-wrong-test
  grep -Fq 'Config.telemetry has no central-policy classification' "$run_dir/mut2.log" || fail mut002-not-named
  printf '%s/MUT-002/rejected\n' "$card"
}

# MUT-003: ApplyCentralPolicy grows past the ceiling again.
run_mut003() {
  local root="$run_dir/root-mut3" pad i
  stage "$root"
  pad=''
  for i in $(seq 1 150); do pad+=$'\n\t// MUT-003 padding'; done
  replace_once "$root/internal/config/central.go" \
    'for _, section := range governedSections {' \
    "for _, section := range governedSections {${pad}"
  expect_red "$root" "$run_dir/mut3.log" "${ac003_tests[@]}"
  grep -Fq 'internal/config/central.go: ApplyCentralPolicy' "$run_dir/mut3.log" || fail mut003-not-named
  printf '%s/MUT-003/rejected\n' "$card"
}

# MUT-004: a review source returns the breach code as a bare literal.
run_mut004() {
  local root="$run_dir/root-mut4"
  stage "$root"
  replace_once "$root/cmd/aurumcode/review_pr_generate.go" \
    'return 1, true' \
    'return 3, true // MUT-004: a second exit ladder'
  expect_red "$root" "$run_dir/mut4.log" "${ac003_tests[@]}"
  grep -Eq -- '^--- FAIL: TestReviewSourcesReturnNoGateExitCode' "$run_dir/mut4.log" || fail mut004-wrong-test
  printf '%s/MUT-004/rejected\n' "$card"
}

# MUT-005: the gate keys a map on an engine name held by a named constant.
run_mut005() {
  local root="$run_dir/root-mut5"
  stage "$root"
  printf '\n// MUT-005: an engine branch spelled through a constant.\nconst mutEngine = "semgrep"\n\nvar mutEngineWeights = map[string]int{mutEngine: 1}\n' >>"$root/internal/gate/result.go"
  expect_red "$root" "$run_dir/mut5.log" "${ac003_tests[@]}"
  grep -Eq -- '^--- FAIL: TestNoEngineBranchInGateOrCmdByAnyName' "$run_dir/mut5.log" || fail mut005-wrong-test
  grep -Fq 'an engine name as a map key' "$run_dir/mut5.log" || fail mut005-not-named
  printf '%s/MUT-005/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "${ac001_tests[@]}" ;;
  AC-002) run_ac AC-002 "${ac002_tests[@]}" ;;
  AC-003) run_ac AC-003 "${ac003_tests[@]}" ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  MUT-004) run_mut004 ;;
  MUT-005) run_mut005 ;;
  all)
    run_ac AC-001 "${ac001_tests[@]}"
    run_ac AC-002 "${ac002_tests[@]}"
    run_ac AC-003 "${ac003_tests[@]}"
    run_ac004
    run_mut001
    run_mut002
    run_mut003
    run_mut004
    run_mut005
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
