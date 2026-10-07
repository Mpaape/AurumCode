#!/usr/bin/env bash
# AUR-595 acceptance: the self review concludes on a real pull request.
# The deliberation ceiling bounds only what the tool rounds add beyond the
# base prompt; on --pr gitleaks scans the pull request's base..head (the
# verified checkout's HEAD), never the merge commit GITHUB_SHA names; a
# failed engine states its summarized, redacted detail in the gate line,
# the audit and the published review; the repository ignores the recorded
# tutorial outputs. The real pull request run (AC-004 on GitHub) is
# recorded in docs/specs/AUR-595.md.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001..MUT-002
#   AC-001     base prompt above max_cost_tokens, no tool: concludes; tool
#              rounds beyond the base still exceed the ceiling
#   AC-002     GITHUB_SHA = merge commit absent from the checkout: scans
#              base..head and concludes; absent base stays inconclusive
#   AC-003     engine failure detail, redacted, in gate line, audit, review
#   AC-004     the repository config ignores recorded tutorial outputs and
#              the docs carry the recommendation; AC-001..AC-003 together
#   MUT-001    counting the base prompt in the ceiling turns AC-001 RED
#   MUT-002    using GITHUB_SHA (the merge commit) turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-595'
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
for input in go.mod go.sum cmd internal pkg .aurumcode/config.yml docs/configuration.md docs/getting-started.md cmd/aurumcode/aur595_test.go cmd/aurumcode/pr_scan_range.go internal/deliberation/transcript.go internal/deliberation/cost_test.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a595.XXXXXX")" || infra mktemp
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
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
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

readonly ac001_unit=(TestAUR595LargeBasePromptWithoutToolsIsNotALimit TestAUR595ToolRoundsBeyondTheBaseStillExceedTheCeiling TestAUR580MaxCostTokensIsALimitError)
readonly ac001_cmd=(TestAUR595BasePromptAboveTheCeilingConcludes TestAUR580CostExceededIsInconclusive)
readonly ac002_cmd=(TestAUR595GitleaksScansThePullRequestHeadNotTheMergeCommit TestAUR595AbsentBaseIsInconclusiveWithItsDetail)
readonly ac003_cmd=(TestAUR595AbsentBaseIsInconclusiveWithItsDetail TestAUR595EngineFailureDetailIsRedacted)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

# run_tests name pkg tests...: every named test must PASS (a SKIP fails).
run_tests() {
  local name="$1" pkg="$2"; shift 2
  local root="$run_dir/root-$name" log="$run_dir/$name-${pkg//\//_}.log"
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  if grep -Eq -- '^--- SKIP: ' "$log"; then cat "$log" >&2; fail "test-skipped:$name"; fi
  local t
  for t in "$@"; do grep -Eq -- "^--- PASS: ${t} " "$log" || { cat "$log" >&2; fail "missing-pass:$t"; }; done
}

# expect_red root log pkg tests...: the mutated copy compiles and turns a
# named test RED by its assertion, never by a build error.
expect_red() {
  local root="$1" log="$2" pkg="$3"; shift 3
  if go_test "$root" "$log" "$(pattern_of "$@")" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

run_ac001() {
  run_tests AC-001 ./internal/deliberation/ "${ac001_unit[@]}"
  run_tests AC-001 ./cmd/aurumcode/ "${ac001_cmd[@]}"
  printf '%s/AC-001/pass\n' "$card"
}

run_ac002() {
  run_tests AC-002 ./cmd/aurumcode/ "${ac002_cmd[@]}"
  printf '%s/AC-002/pass\n' "$card"
}

run_ac003() {
  run_tests AC-003 ./cmd/aurumcode/ "${ac003_cmd[@]}"
  printf '%s/AC-003/pass\n' "$card"
}

run_ac004() {
  local cfg="$repo_root/.aurumcode/config.yml"
  grep -Fxq '  - "demo/tutoriais/*/out/**"' "$cfg" || fail recorded-logs-not-ignored
  grep -Fxq '  - "demo/tutoriais/*/expected/**"' "$cfg" || fail recorded-expected-not-ignored
  grep -Fq 'Recomendação para artefatos gerados' "$repo_root/docs/configuration.md" || fail no-generated-artifacts-recommendation
  grep -Fq 'deliberation.cost_tokens' "$repo_root/docs/configuration.md" || fail ceiling-not-documented
  run_tests AC-004 ./cmd/aurumcode/ "${ac001_cmd[@]}" "${ac002_cmd[@]}" TestAUR595EngineFailureDetailIsRedacted
  printf '%s/AC-004/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/deliberation/transcript.go" \
    't.BaseTokens = resp.TokensIn' \
    't.BaseTokens, t.CostTokens = resp.TokensIn, t.CostTokens+resp.TokensIn /* MUT-001 */'
  expect_red "$root" "$run_dir/mut1.log" ./internal/deliberation/ TestAUR595LargeBasePromptWithoutToolsIsNotALimit
  expect_red "$root" "$run_dir/mut1-cmd.log" ./cmd/aurumcode/ TestAUR595BasePromptAboveTheCeilingConcludes
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/cmd/aurumcode/pr_scan_range.go" \
    'r.Head = strings.ToLower(strings.TrimSpace(head))' \
    'r.Head = p.env().githubSHA /* MUT-002 */; _ = head'
  expect_red "$root" "$run_dir/mut2.log" ./cmd/aurumcode/ TestAUR595GitleaksScansThePullRequestHeadNotTheMergeCommit
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_ac004
    run_mut001
    run_mut002
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
