#!/usr/bin/env bash
# AUR-592 acceptance: Aurum for developers who code with an AI agent. The
# local MCP server (`aurumcode mcp`, stdio) is a thin adapter over the same
# review session the CLI and CI run; a skill and an optional pre-commit hook
# make the agent ask the gate before committing.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001..MUT-002
#   AC-001     a stdio MCP client lists the four tools and calls aurum_gate
#              on a fixture repository: same exit, report, blocking findings
#              and decision as `review --base` on the same diff; protocol
#              conformance (JSON-RPC only on stdout, errors, notifications)
#   AC-002     under a central policy aurum_rules lists policy and
#              repository skills; no tool declares a parameter that turns a
#              rule off or names another policy
#   AC-003     no MCP answer carries the diff's secret; an invalid argument
#              is refused before any review runs; a failed or missing
#              provider, or an empty change, is inconclusive, never a pass
#   AC-004     tutorial `agente` (--check), the site page with the setup of
#              Claude Code, Codex and Cursor in the nav, the agent skill
#   MUT-001    a server with its own gate path (not the session) turns AC-001 RED
#   MUT-002    answers without the redaction choke point turn AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-592'
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
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/mcp_server_test.go cmd/aurumcode/mcp_gateway.go internal/mcpserver/server.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a592.XXXXXX")" || infra mktemp
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

readonly ac001_cmd=(TestMCPGateIsTheReviewSessionGate)
readonly ac001_server=(TestInitializeListAndNotifications TestProtocolErrors)
readonly ac002_cmd=(TestMCPRulesUnderCentralPolicy)
readonly ac003_cmd=(TestMCPAnswersAreRedactedAndFailClosed TestMCPEmptyChangeIsNeverAPass)
readonly ac003_server=(TestProtocolErrorsAreRedacted TestInvalidArgumentsRefusedBeforeRunning TestDecisionNeverPassesWhenInconclusive TestTimeoutIsInconclusive TestEveryAnswerIsRedacted)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_tests() {
  local name="$1" pkg="$2"; shift 2
  local root="$run_dir/root-$name" log="$run_dir/$name-${pkg//\//_}.log"
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  local t
  for t in "$@"; do grep -Eq -- "^--- PASS: ${t} " "$log" || { cat "$log" >&2; fail "missing-pass:$t"; }; done
}

expect_red() {
  local root="$1" log="$2" pkg="$3"; shift 3
  if go_test "$root" "$log" "$(pattern_of "$@")" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,3p' >&2
}

run_ac001() {
  run_tests AC-001 ./cmd/aurumcode/ "${ac001_cmd[@]}"
  run_tests AC-001 ./internal/mcpserver/ "${ac001_server[@]}"
  printf '%s/AC-001/pass\n' "$card"
}

run_ac002() {
  run_tests AC-002 ./cmd/aurumcode/ "${ac002_cmd[@]}"
  printf '%s/AC-002/pass\n' "$card"
}

run_ac003() {
  run_tests AC-003 ./cmd/aurumcode/ "${ac003_cmd[@]}"
  run_tests AC-003 ./internal/mcpserver/ "${ac003_server[@]}"
  printf '%s/AC-003/pass\n' "$card"
}

run_ac004() {
  local skill="$repo_root/.agents/skills/aurum-review/SKILL.md" page="$repo_root/docs/agentes.md"
  local tut="$repo_root/demo/tutoriais/agente" doc="$repo_root/docs/tutorials/agente.md"
  for input in "$skill" "$page" "$tut/run.sh" "$doc" "$repo_root/mkdocs.yml" "$repo_root/docs/tutorials/README.md"; do
    [[ -e "$input" ]] || fail "missing:${input#"$repo_root"/}"
  done
  sed -n '1p' "$skill" | grep -Fxq -- '---' || fail skill-without-frontmatter
  awk 'NR>1 && /^---$/ {exit} NR>1' "$skill" | grep -Eq '^name: aurum-review$' || fail skill-without-name
  awk 'NR>1 && /^---$/ {exit} NR>1' "$skill" | grep -Eq '^description: .{20,}' || fail skill-without-description
  grep -Fq 'aurum_gate' "$skill" || fail skill-does-not-call-the-gate
  local agent
  for agent in 'claude mcp add' '.mcp.json' 'Codex' 'Cursor' 'aurumcode mcp'; do
    grep -Fq -- "$agent" "$page" || fail "page-lacks:$agent"
  done
  grep -Eq '^ +- .*agentes\.md' "$repo_root/mkdocs.yml" || fail page-not-in-nav
  # docs/tutorials/*.md enter the nav through scripts/docs/hooks.py; the index lists each one.
  grep -Fq '](agente.md)' "$repo_root/docs/tutorials/README.md" || fail tutorial-not-indexed
  local case_name
  for case_name in gate-consultado gate-inconclusivo skill-do-repo; do
    grep -Eq "^caso_${case_name//-/_}\(\)" "$tut/run.sh" || fail "tutorial-lacks-case:$case_name"
    [[ -s "$tut/expected/$case_name.txt" ]] || fail "tutorial-lacks-expected:$case_name"
  done
  if ! bash "$tut/run.sh" --check >"$run_dir/tut-check.log" 2>&1; then
    # The card's paths materialize cmd/aurumcode only; without the rest of
    # cmd/ the tree identity cannot match out/.imagem. Then every case must
    # still match expected/, and the tree is checked where cmd/ is whole.
    local whole=1
    [[ -d "$repo_root/cmd/analysis-data" ]] || whole=0
    if [[ "$whole" == 1 ]] || [[ "$(grep -c '^caso .*: ok$' "$run_dir/tut-check.log")" != 4 ]] ||
       ! grep -q '^DIVERGENCIA: out/ foi gravado por uma imagem de outra arvore' "$run_dir/tut-check.log"; then
      cat "$run_dir/tut-check.log" >&2; fail tutorial-check-failed
    fi
    printf '%s/AC-004/tree-not-materialized (cases match expected/; tree checked by AUR-561..564 and CI)\n' "$card" >&2
  fi
  printf '%s/AC-004/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/cmd/aurumcode/mcp_gateway.go" \
    'exit = session.Run(b)' \
    'exit = func() int { for _, ph := range []session.Phase{session.PhaseResolve, session.PhaseEvidence, session.PhaseModel} { if e, d := b.Step(ph)(); d { return e } }; if countAtOrAbove(b.result.Issues, severityRank("error")) > 0 { return 3 }; return 0 }() /* MUT-001 */'
  expect_red "$root" "$run_dir/mut1.log" ./cmd/aurumcode/ "${ac001_cmd[@]}"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/mcpserver/server.go" \
    'return toolResult(redactValue(answer, s.opts.Redactor, redactor)), nil' \
    '_ = redactor; return toolResult(answer), nil /* MUT-002 */'
  expect_red "$root" "$run_dir/mut2.log" ./internal/mcpserver/ "${ac003_server[@]}"
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
