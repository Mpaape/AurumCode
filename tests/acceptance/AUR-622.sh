#!/usr/bin/env bash
# AUR-622 acceptance: an external plugin of the scheduled run. The
# configuration refuses malformed plugins and keeps a policy's required
# entry; the ingest writes a SARIF with only the main channel's true
# positives, redacted, and prints only counts; every inconclusive run (the
# harness's own SARIF and errors.jsonl included) exits 1 with no SARIF; a
# path outside the tree fails the ingest; the workflow's plugins jobs only
# publish to code scanning; the tutorial is executable and its blocks are
# the demo's files.
#
# Selectors:
#   all      AC-001..AC-006, MUT-001, MUT-002
#   AC-001   config: malformed plugin refused; required policy entry stays
#   AC-002   ingest: 2 TRUE_POSITIVE of 3 -> SARIF with 2 results, stable
#            rule/fingerprint/category, secret redacted, stdout only counts
#   AC-003   degraded, invalid JSON, schema 2.x, plugin exit != 0, missing
#            file, harness SARIF coverageDegraded/hollow/failed or missing,
#            analysis errors: exit 1 and no SARIF
#   AC-004   finding path absolute, "..", missing or through a symlink out
#            of the tree: the ingest fails
#   AC-005   workflow plugins jobs: triggers, refusal, no public channel,
#            redirected output, pinned upload-sarif with the plugin category
#   AC-006   tutorial plugin-agendado: >= 3 use cases + >= 1 failure,
#            --check green, doc blocks identical to the demo files
#   MUT-001  the ingest accepting degraded: true turns AC-003 red
#   MUT-002  the ingest printing the plugin's output turns AC-002 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-622'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 AC-004 AC-005 AC-006 MUT-001 MUT-002'
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

readonly pkgs=(./internal/config ./internal/plugin)
readonly t_config='^(TestAUR622ConfigRefusesMalformedPlugins|TestAUR622PolicyRequiredPluginStays|TestAUR622PolicyRefusesDockerfilePlugin)$'
readonly t_ingest='^(TestAUR622IngestWritesOnlyTruePositives|TestAUR622IngestCountsOtherChannelAndNoVerdict|TestAUR622GenericSARIFAdapter|TestAUR622UnverifiedDropBecomesWarning)$'
readonly t_inconclusive='^(TestAUR622InconclusiveWritesNoSARIF|TestAUR622NonAnalysisErrorIsConclusive)$'
readonly t_paths='^TestAUR622PathOutsideTreeFails$'
readonly t_workflow='^TestAUR622WorkflowPluginsJob$'

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
  grep -q -- '^--- PASS: TestAUR622' "$log" || fail "no-aur622-test-ran"
  if grep -q -- '^--- SKIP' "$log"; then fail "test-skipped"; fi
  printf '%s/%s/pass\n' "$card" "$id"
}

mutant() {
  local id="$1" root="$2" pattern="$3" log="$work/$1.log"
  if run_test "$root" "$pattern" "$log"; then
    fail "mutation-survived"
  fi
  grep -q -- '^--- FAIL: TestAUR622' "$log" || { tail -n 40 "$log" >&2; fail "mutation-did-not-run-the-test"; }
  printf '%s/%s/killed\n' "$card" "$id"
}

run_ac_006() {
  local t="$repo_root/demo/tutoriais/plugin-agendado" doc="$repo_root/docs/tutorials/plugin-agendado.md"
  local casos uso falha n=0 path
  [[ -f "$t/run.sh" && -f "$doc" ]] || fail "tutorial-ausente"
  casos="$(sed -n 's/^CASOS=(\(.*\))$/\1/p' "$t/run.sh")"
  [[ -n "$casos" ]] || fail "casos-ausentes"
  falha=0; uso=0
  for c in $casos; do
    [[ -f "$t/expected/$c.txt" ]] || fail "expected-ausente:$c"
    if grep -q 'RESULTADO: .*inconclusivo' "$t/expected/$c.txt"; then falha=$((falha + 1)); else uso=$((uso + 1)); fi
  done
  (( uso >= 3 && falha >= 1 )) || fail "casos-insuficientes:uso=$uso,falha=$falha"
  bash "$t/run.sh" --check >"$work/check.log" 2>&1 || { cat "$work/check.log" >&2; fail "check-diverge"; }
  while IFS= read -r path; do
    n=$((n + 1))
    [[ -f "$repo_root/$path" ]] || fail "arquivo-inexistente:$path"
    awk -v want="<!-- arquivo: $path -->" '
      $0 == want { armed = 1; next }
      armed && !open && /^```/ { open = 1; next }
      open && /^```/ { exit }
      open { print }
    ' "$doc" > "$work/bloco"
    diff -u "$repo_root/$path" "$work/bloco" >/dev/null || fail "bloco-diverge:$path"
  done < <(sed -n 's/^<!-- arquivo: \(.*\) -->$/\1/p' "$doc")
  (( n >= 3 )) || fail "blocos-insuficientes:$n"
  printf '%s/AC-006/pass (%d casos de uso, %d de falha, %d blocos identicos)\n' "$card" "$uso" "$falha" "$n"
}

run_mut_001() {
  local root="$work/mut1" file
  need_go
  stage "$root"
  file="$root/internal/plugin/adapter_vvah.go"
  grep -q 'if r.Degraded == nil || \*r.Degraded {' "$file" || infra "mutation-anchor-missing"
  sed -i 's/if r.Degraded == nil || \*r.Degraded {/if false {/' "$file"
  mutant MUT-001 "$root" "$t_inconclusive"
}

run_mut_002() {
  local root="$work/mut2" file
  need_go
  stage "$root"
  file="$root/internal/plugin/report.go"
  grep -q '^		fmt.Fprintln(stdout, SummaryText(language, o.Name, counts))$' "$file" || infra "mutation-anchor-missing"
  sed -i 's/^\t"io"$/\t"io"\n\t"os"/' "$file"
  sed -i 's/^\t\tfmt.Fprintln(stdout, SummaryText(language, o.Name, counts))$/\t\tfmt.Fprintln(stdout, SummaryText(language, o.Name, counts))\n\t\tif data, err := os.ReadFile(o.Input); err == nil {\n\t\t\tfmt.Fprintln(stdout, string(data))\n\t\t}/' "$file"
  mutant MUT-002 "$root" "$t_ingest"
}

case "$selector" in
  all)
    ac AC-001 "$t_config"
    ac AC-002 "$t_ingest"
    ac AC-003 "$t_inconclusive"
    ac AC-004 "$t_paths"
    ac AC-005 "$t_workflow"
    run_ac_006
    run_mut_001
    run_mut_002
    printf '%s/all/pass\n' "$card"
    ;;
  AC-001) ac AC-001 "$t_config" ;;
  AC-002) ac AC-002 "$t_ingest" ;;
  AC-003) ac AC-003 "$t_inconclusive" ;;
  AC-004) ac AC-004 "$t_paths" ;;
  AC-005) ac AC-005 "$t_workflow" ;;
  AC-006) run_ac_006 ;;
  MUT-001) run_mut_001 ;;
  MUT-002) run_mut_002 ;;
esac
