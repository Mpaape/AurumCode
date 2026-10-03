#!/usr/bin/env bash
# AUR-576 acceptance: one review session (internal/review/session) serves
# --base and --pr. The phase order, the inconclusive ranking (gate.RankReason)
# and the exit decision (gate.ExitPolicy) exist once; each source declares
# what a model outcome means for it as data (session.LocalDiff,
# session.PullRequest); the verdict-reuse snapshot holds the same findings on
# both sources; the test seams are injected session dependencies.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001 and MUT-002
#   AC-001     go/ast: cmd/aurumcode declares no phase list and no source
#              returns its own exit code; gate.ExitPolicy has one caller;
#              session.Run follows session.Order
#   AC-002     same inputs, same reason and exit on both sources (table with
#              provider failure, degraded parse, missing scanner, artifact not
#              written, finding above the threshold, plus the declared
#              per-source rows); the ladder's precedence; both paths run the
#              same gate pipeline
#   AC-003     the raw snapshot and the gate's findings are the same set on
#              both sources, security findings included
#   AC-004     every existing test of cmd/aurumcode, internal/gate and
#              internal/review passes unchanged in expectation; the package
#              seams are gone and no new t.Setenv("PATH") was added
#   MUT-001    a second exit ladder in the --base publisher turns AC-001 RED;
#              reordering the ladder (fail-on above a missing artifact) turns
#              AC-002 RED
#   MUT-002    leaving the kept-apart security findings out of the snapshot
#              turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-576'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg internal/review/session/session.go cmd/aurumcode/review_session_test.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a576.XXXXXX")" || infra mktemp
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
  for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$root/$source"; done
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
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

readonly ac001_tests=(TestAUR576OnePhaseListAndOneExitDecision TestRunFollowsTheOneOrder)
readonly ac002_tests=(TestAUR576SameInputsSameExitOnBothSources TestExitPolicyPrecedence TestRankReasonOrder TestNotReviewedRules TestAUR557PathsShareOnePipeline)
readonly ac003_tests=(TestAUR576SnapshotHoldsTheSameFindingsOnBothSources)
readonly pkgs=(./cmd/aurumcode/ ./internal/review/session/ ./internal/gate/)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_ac() {
  local name="$1"; shift
  local root="$run_dir/root-$name" log="$run_dir/$name.log"
  stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}" || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" "$@"
}

run_ac004() {
  local root="$run_dir/root-ac004" log="$run_dir/ac004.log" seam
  stage "$root"
  for seam in 'var gatePipelineObserver' 'var newGatePipeline' 'var diffContentDigest' 'var evaluateGate' 'var resolveWithFilesHook' 'var newCacheDigestBuilder'; do
    if grep -Fq -- "$seam" "$root"/cmd/aurumcode/*.go; then fail "seam-still-a-package-variable:${seam#var }"; fi
  done
  local setenv_path
  setenv_path="$(cat "$root"/cmd/aurumcode/*_test.go | grep -Fc 't.Setenv("PATH"')" || true
  # Six existed before this card (AUR-548's fake semgrep executables).
  (( setenv_path <= 6 )) || fail "new-t.Setenv-PATH:$setenv_path"
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 ./cmd/aurumcode/... ./internal/gate/... ./internal/review/... ) >"$log" 2>&1 || { cat "$log" >&2; fail go-test-failed; }
  grep -Eq '^ok[[:space:]]+github.com/Mpaape/AurumCode/cmd/aurumcode' "$log" || fail 'cmd-not-run'
}

# expect_red root log tests... : the named tests must now fail.
expect_red() {
  local root="$1" log="$2"; shift 2
  if go_test "$root" "$log" "$(pattern_of "$@")" "${pkgs[@]}"; then cat "$log" >&2; fail mutation-survived; fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then cat "$log" >&2; infra mutation-did-not-compile; fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
}

run_mut001() {
  local root="$run_dir/root-mut1a"
  stage "$root"
  replace_once "$root/cmd/aurumcode/review_base_publish.go" \
    'return b.decideExit(publishOutcome{artifactsMissing: len(artifactFailures) > 0}), true' \
    'if b.findingsAtThreshold() > 0 { return exitFindings, true }; return b.decideExit(publishOutcome{artifactsMissing: len(artifactFailures) > 0}), true'
  expect_red "$root" "$run_dir/mut1a.log" "${ac001_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR576OnePhaseListAndOneExitDecision' "$run_dir/mut1a.log" || fail 'mut001a-wrong-test'
  root="$run_dir/root-mut1b"
  stage "$root"
  replace_once "$root/internal/gate/exit.go" \
    'return ExitDecision{ExitBehavioral, CauseArtifactMissing}, in.ArtifactsMissing' \
    'return ExitDecision{ExitBehavioral, CauseArtifactMissing}, in.ArtifactsMissing && in.FindingsAtThreshold == 0'
  expect_red "$root" "$run_dir/mut1b.log" "${ac002_tests[@]}"
  grep -Eq -- '^--- FAIL: TestAUR576SameInputsSameExitOnBothSources' "$run_dir/mut1b.log" || fail 'mut001b-wrong-test'
  printf '%s/%s/MUT-001/rejected\n' "$card" "$selector"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/cmd/aurumcode/review_evidence.go" \
    's.rawIssues = append(append([]types.ReviewIssue(nil), s.result.Issues...), s.securityApart()...)' \
    's.rawIssues = append([]types.ReviewIssue(nil), s.result.Issues...)'
  expect_red "$root" "$run_dir/mut2.log" "${ac003_tests[@]}"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$selector"
}

case "$selector" in
  AC-001) run_ac ac001 "${ac001_tests[@]}" ;;
  AC-002) run_ac ac002 "${ac002_tests[@]}" ;;
  AC-003) run_ac ac003 "${ac003_tests[@]}" ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac ac001 "${ac001_tests[@]}"
    run_ac ac002 "${ac002_tests[@]}"
    run_ac ac003 "${ac003_tests[@]}"
    run_ac004
    run_mut001
    run_mut002
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
