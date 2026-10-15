#!/usr/bin/env bash
# AUR-623 acceptance: in a private repository without GitHub Code Security
# the scheduled plugin's findings become issues of that repository. The
# sync opens one issue per finding (hidden marker with the fingerprint,
# label, readable pt-BR body, secret redacted), keeps without duplicating,
# closes what disappeared from a conclusive scan and reopens what came
# back; a public repository or an unreadable visibility is refused before
# any write; an inconclusive scan makes no request at all. The workflow
# runs `plugin issues` on the issues destination, without upload-sarif;
# the tutorial is executable.
#
# Selectors:
#   all      AC-001..AC-005, MUT-001, MUT-002
#   AC-001   2 findings, no issues: 2 issues with marker, label, body;
#            secret redacted; label dropped by the API fails
#   AC-002   keep without duplicating, close with comment, open the new,
#            reopen the returning one; listing crosses pages; foreign
#            markers untouched
#   AC-003   public or unreadable visibility: exit 1 and no write;
#            inconclusive scan: no request
#   AC-004   workflow: plugin, ingest and plugin issues on the issues
#            destination, no upload-sarif, issues: write documented
#   AC-005   tutorial plugin-agendado has the issues and public-refusal
#            cases; --check green
#   MUT-001  opening issues on a public repository turns AC-003 red
#   MUT-002  closing issues on an inconclusive scan turns AC-003 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-623'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 MUT-001 MUT-002'
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
for tool in sed grep awk diff; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_$tool"
done

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

readonly pkgs=(./internal/plugin)
readonly t_open='^(TestAUR623OpensIssuesWithMarker|TestAUR623DroppedLabelFails|TestAUR623OversizeFindingFits|TestAUR623RateLimitRetried)$'
readonly t_sync='^(TestAUR623SyncKeepsClosesReopens|TestAUR623ForeignIssuesUntouchedAndZeroCloses|TestAUR623HumanCloseStands)$'
readonly t_refuse='^(TestAUR623RefusesPublicAndInconclusive|TestAUR623UsageBeforeAnyRequest)$'
readonly t_workflow='^(TestAUR623WorkflowIssuesDestination|TestAUR622WorkflowPluginsJob)$'

need_go() {
  command -v go >/dev/null 2>&1 || infra missing_go
  : "${GOCACHE:=$(mktemp -d)}"
  export GOCACHE
}

# stage copies the module and the workflow into a fresh root, so a mutation
# never touches the repository.
stage() {
  local root="$1" item
  mkdir -p "$root/.github"
  for item in go.mod go.sum internal pkg .github/workflows; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/$item"
  done
  chmod -R u+w -- "$root"
}

run_test() {
  local root="$1" pattern="$2" log="$3"
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}") >"$log" 2>&1
}

base_ready=''
base() {
  [[ -n "$base_ready" ]] && return
  need_go
  stage "$work/base"
  base_ready=1
}

ac() {
  local id="$1" pattern="$2" log="$work/$1.log"
  base
  if ! run_test "$work/base" "$pattern" "$log"; then
    tail -n 40 "$log" >&2
    fail "test-failed"
  fi
  grep -q -- '^--- PASS: TestAUR623' "$log" || fail "no-aur623-test-ran"
  if grep -q -- '^--- SKIP' "$log"; then fail "test-skipped"; fi
  printf '%s/%s/pass\n' "$card" "$id"
}

mutant() {
  local id="$1" root="$2" pattern="$3" log="$work/$1.log"
  if run_test "$root" "$pattern" "$log"; then
    fail "mutation-survived"
  fi
  grep -q -- '^--- FAIL: TestAUR623' "$log" || { tail -n 40 "$log" >&2; fail "mutation-did-not-run-the-test"; }
  printf '%s/%s/killed\n' "$card" "$id"
}

run_ac_005() {
  local t="$repo_root/demo/tutoriais/plugin-agendado" casos
  [[ -f "$t/run.sh" ]] || fail "tutorial-ausente"
  casos="$(sed -n 's/^CASOS=(\(.*\))$/\1/p' "$t/run.sh")"
  for c in issues issues-publico; do
    [[ " $casos " == *" $c "* ]] || fail "caso-ausente:$c"
    [[ -f "$t/expected/$c.txt" ]] || fail "expected-ausente:$c"
  done
  grep -qF 'abertas 2, mantidas 0, fechadas 0, reabertas 0' "$t/expected/issues.txt" || fail "issues-sem-abertura"
  grep -qF 'abertas 0, mantidas 1, fechadas 1, reabertas 0' "$t/expected/issues.txt" || fail "issues-sem-fechamento"
  grep -qF 'reabertas 1' "$t/expected/issues.txt" || fail "issues-sem-reabertura"
  grep -qF 'escritas no GitHub falso: 0' "$t/expected/issues-publico.txt" || fail "publico-com-escrita"
  grep -qF 'requisicoes ao GitHub falso: 0' "$t/expected/issues-publico.txt" || fail "inconclusivo-com-chamada"
  bash "$t/run.sh" --check >"$work/check.log" 2>&1 || { cat "$work/check.log" >&2; fail "check-diverge"; }
  printf '%s/AC-005/pass\n' "$card"
}

# MUT-001: the visibility check accepts any repository.
run_mut_001() {
  local root="$work/mut1" file
  need_go
  stage "$root"
  file="$root/internal/plugin/issues_run.go"
  grep -q '^	return CheckDestination(visibility, DestinationIssues) == nil && private$' "$file" || infra "mutation-anchor-missing"
  sed -i 's/^\treturn CheckDestination(visibility, DestinationIssues) == nil \&\& private$/\treturn true/' "$file"
  mutant MUT-001 "$root" "$t_refuse"
}

# MUT-002: an inconclusive scan is synced as a conclusive zero (closes all).
run_mut_002() {
  local root="$work/mut2" file
  need_go
  stage "$root"
  file="$root/internal/plugin/issues_run.go"
  grep -q '^	if !conclusive {$' "$file" || infra "mutation-anchor-missing"
  sed -i 's/^\tif !conclusive {$/\tif !conclusive \&\& false {/' "$file"
  mutant MUT-002 "$root" "$t_refuse"
}

case "$selector" in
  all)
    ac AC-001 "$t_open"
    ac AC-002 "$t_sync"
    ac AC-003 "$t_refuse"
    ac AC-004 "$t_workflow"
    run_ac_005
    run_mut_001
    run_mut_002
    printf '%s/all/pass\n' "$card"
    ;;
  AC-001) ac AC-001 "$t_open" ;;
  AC-002) ac AC-002 "$t_sync" ;;
  AC-003) ac AC-003 "$t_refuse" ;;
  AC-004) ac AC-004 "$t_workflow" ;;
  AC-005) run_ac_005 ;;
  MUT-001) run_mut_001 ;;
  MUT-002) run_mut_002 ;;
esac
