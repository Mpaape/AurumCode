#!/usr/bin/env bash
# AUR-575 acceptance: the gate fails closed by construction. Every config
# section is validated at load (quality_gates.sast included), an unknown key in
# the repository's config.yml is a load error, an absent gate.inconclusive
# resolves to block whenever a gate or a scanner is configured, the
# "inconclusive -> Fail by mode" rule lives once in the gate pipeline, and an
# unknown severity on a deterministic finding counts as error.
#
# Selectors:
#   all        AC-001..AC-005, then MUT-001..MUT-003
#   AC-001     sast engine / rule_packs refused at load, key named, no review
#   AC-002     unknown key refused with the key named; every shipped config
#              (demos, tutorials, fixtures) still parses strictly
#   AC-003     sast enabled + scanner missing + no gate.inconclusive: exit != 0,
#              status failure "inconclusiva (bloqueio)"; written warn alerts;
#              the gate tutorial case is versioned and run.sh --check passes
#   AC-004     a plain contributor error and the missing SAST decide the same;
#              no gate source other than the pipeline resolves the mode
#   AC-005     unknown severity counts as error in SAST and deterministic loop
#   MUT-001    removing SastConfig.Validate from Parse turns AC-001 RED
#   MUT-002    restoring `case "": return "", nil` turns AC-003 RED
#   MUT-003    restoring `continue` for unknown severity turns AC-005 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-575'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg demo docs/specs/AUR-575.md demo/tutoriais/gate/run.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a575.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp" "$run_dir/root"
for source in go.mod go.sum cmd internal pkg demo docs; do cp -R "$repo_root/$source" "$run_dir/root/$source"; done
for source in tests/fixtures tests/e2e; do
  if [[ -d "$repo_root/$source" ]]; then mkdir -p "$run_dir/root/tests"; cp -R "$repo_root/$source" "$run_dir/root/$source"; fi
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# run_go_test root pkg pattern log: runs the named tests, keeps the log.
run_go_test() {
  set +e
  (cd "$1" && go test -buildvcs=false -mod=mod -p 1 -count=1 -timeout 400s -v "$2" -run "$3") >"$4" 2>&1
  local status=$?
  set -e
  return $status
}
need_pass() { grep -q "^--- PASS: $2 " "$1" || { cat "$1" >&2; fail "missing-pass:$2"; }; }

readonly ac001_cfg='^(TestAUR575SastSectionValidatedInParse)$'
readonly ac001_cmd='^(TestAUR575InvalidConfigRefusedBeforeModel)$'
readonly ac003_cfg='^(TestAUR575InconclusiveDefaultBlocksWhenGated)$'
readonly ac003_gate='^(TestAUR575SASTMissingBlocksWithoutInconclusive)$'
readonly ac003_cmd='^(TestAUR575SASTMissingWithoutInconclusiveBlocks|TestAUR575SASTMissingWithWrittenWarnAlerts|TestAUR575SASTMissingPublishesBlockedStatus|TestAUR519EvaluateGateFailOnWithoutInconclusiveBlocks)$'
readonly ac005_gate='^(TestAUR575UnknownSeverityCountsAsError)$'

root="$run_dir/root"

run_ac001() {
  run_go_test "$root" ./internal/config/ "$ac001_cfg" "$run_dir/ac001a.log" || { cat "$run_dir/ac001a.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac001a.log" TestAUR575SastSectionValidatedInParse
  run_go_test "$root" ./cmd/aurumcode/ "$ac001_cmd" "$run_dir/ac001b.log" || { cat "$run_dir/ac001b.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac001b.log" TestAUR575InvalidConfigRefusedBeforeModel
}
run_ac002() {
  run_go_test "$root" ./internal/config/ '^(TestAUR575UnknownKeyIsRefused|TestAUR575ExistingConfigsStayValid)$' "$run_dir/ac002.log" || { cat "$run_dir/ac002.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac002.log" TestAUR575UnknownKeyIsRefused
  need_pass "$run_dir/ac002.log" TestAUR575ExistingConfigsStayValid
  grep -Eq 'checked [1-9][0-9]* shipped config.yml files' "$run_dir/ac002.log" || fail 'no-shipped-config-checked'
}
run_ac003() {
  run_go_test "$root" ./internal/config/ "$ac003_cfg" "$run_dir/ac003a.log" || { cat "$run_dir/ac003a.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac003a.log" TestAUR575InconclusiveDefaultBlocksWhenGated
  run_go_test "$root" ./internal/gate/ "$ac003_gate" "$run_dir/ac003b.log" || { cat "$run_dir/ac003b.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac003b.log" TestAUR575SASTMissingBlocksWithoutInconclusive
  run_go_test "$root" ./cmd/aurumcode/ "$ac003_cmd" "$run_dir/ac003c.log" || { cat "$run_dir/ac003c.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac003c.log" TestAUR575SASTMissingWithoutInconclusiveBlocks
  need_pass "$run_dir/ac003c.log" TestAUR575SASTMissingWithWrittenWarnAlerts
  need_pass "$run_dir/ac003c.log" TestAUR575SASTMissingPublishesBlockedStatus
  local demo="$repo_root/demo/tutoriais/gate" log
  log="$demo/out/inconclusivo-sast.log"
  grep -q 'politica-sast-padrao' "$demo/run.sh" || fail 'tutorial-case-missing'
  grep -q 'inconclusive' "$demo/politica-sast-padrao/.aurumcode/config.yml" && fail 'tutorial-policy-declares-inconclusive'
  grep -q 'inconclusivo (sast_unavailable)' "$log" || fail 'out-without-sast-unavailable'
  grep -q 'RESULTADO: sem gate.inconclusive, scanner habilitado que nao rodou bloqueia' "$log" || fail 'out-without-block-result'
  grep -q 'politica-sast-padrao' "$repo_root/docs/tutorials/gate.md" || fail 'tutorial-doc-without-case'
  (cd "$repo_root" && bash demo/tutoriais/gate/run.sh --check >"$run_dir/check.log" 2>&1) || { cat "$run_dir/check.log" >&2; fail 'demo-check-failed'; }
}
run_ac004() {
  run_go_test "$root" ./internal/gate/ '^(TestAUR575OneRuleForEveryInconclusiveSource|TestPipelineContributorErrorIsInconclusiveNeverApproved)$' "$run_dir/ac004.log" || { cat "$run_dir/ac004.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac004.log" TestAUR575OneRuleForEveryInconclusiveSource
  need_pass "$run_dir/ac004.log" TestPipelineContributorErrorIsInconclusiveNeverApproved
  # Only the pipeline turns an inconclusive result into a failure; policy.go
  # reads the mode solely to skip grading a blocked run.
  local f deciders=''
  for f in "$repo_root"/internal/gate/*.go; do
    case "$f" in *_test.go) continue ;; esac
    if grep -Eq 'Fail = (blockOnInconclusive|inconclusiveMode|mode ==)|mode == "block"' "$f"; then deciders="$deciders ${f##*/}"; fi
  done
  [[ -z "$deciders" ]] || fail "source-decides-mode:${deciders# }"
  grep -q 'func ApplyInconclusiveMode(' "$repo_root/internal/gate/pipeline.go" || fail 'resolver-not-in-pipeline'
}
run_ac005() {
  run_go_test "$root" ./internal/gate/ "$ac005_gate" "$run_dir/ac005.log" || { cat "$run_dir/ac005.log" >&2; fail go-test-failed; }
  need_pass "$run_dir/ac005.log" TestAUR575UnknownSeverityCountsAsError
}

# replace_literal file old new: literal replacement (no regex), fails when the
# anchor is absent so a stale mutation never passes silently.
replace_literal() {
  local content
  content="$(cat -- "$1"; printf x)"; content="${content%x}"
  [[ "$content" == *"$2"* ]] || return 1
  printf '%s' "${content/"$2"/"$3"}" >"$1"
}
# mutant_root name file old new: applies one literal replacement to a fresh
# copy of the Go sources and prints the copy's root.
mutant_root() {
  local name="$1" file="$2" old="$3" new="$4" dir="$run_dir/mut-$1"
  rm -rf "$dir"; mkdir -p "$dir"
  for source in go.mod go.sum cmd internal pkg; do cp -R "$root/$source" "$dir/$source"; done
  replace_literal "$dir/$file" "$old" "$new" || infra "mutation-anchor:$name"
  printf '%s\n' "$dir"
}
# expect_red log test: the mutant compiled and the named test failed.
expect_red() {
  if grep -Eq '\[build failed\]|\[setup failed\]|^# github.com' "$1"; then cat "$1" >&2; fail 'mutant-did-not-compile'; fi
  grep -q "^--- FAIL: $2 " "$1" || { cat "$1" >&2; fail "mutant-survived:$2"; }
  printf '%s/%s/red: %s\n' "$card" "$selector" "$(grep -m1 "^--- FAIL: $2 " "$1")"
}

run_mut001() {
  local dir
  dir="$(mutant_root 001 internal/config/config.go '	if err := cfg.QualityGates.Sast.Validate(); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
' '')"
  run_go_test "$dir" ./internal/config/ "$ac001_cfg" "$run_dir/mut001.log" && fail 'mutant-passed'
  expect_red "$run_dir/mut001.log" TestAUR575SastSectionValidatedInParse
}
run_mut002() {
  local dir
  dir="$(mutant_root 002 internal/config/gate.go '	case "":
		return fallback, nil' '	case "":
		return "", nil')"
  run_go_test "$dir" ./internal/gate/ "$ac003_gate" "$run_dir/mut002.log" && fail 'mutant-passed'
  expect_red "$run_dir/mut002.log" TestAUR575SASTMissingBlocksWithoutInconclusive
}
run_mut003() {
  local dir
  dir="$(mutant_root 003 internal/gate/sast.go '		if GateRankOf(issue.Severity) < rank {' '		if issueRank, ok := SeverityRankOf(issue.Severity); !ok || issueRank < rank {')"
  replace_literal "$dir/internal/gate/sources.go" '		if GateRankOf(issue.Severity) < rank {' '		if issueRank, ok := SeverityRankOf(issue.Severity); !ok || issueRank < rank {' || infra 'mutation-anchor:003-sources'
  run_go_test "$dir" ./internal/gate/ "$ac005_gate" "$run_dir/mut003.log" && fail 'mutant-passed'
  expect_red "$run_dir/mut003.log" TestAUR575UnknownSeverityCountsAsError
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  AC-005) run_ac005 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  all)
    for s in AC-001 AC-002 AC-003 AC-004 AC-005; do selector="$s"; "run_$(tr 'A-Z-' 'a-z\n' <<<"$s" | tr -d '\n')"; printf '%s/%s/pass\n' "$card" "$s"; done
    for s in MUT-001 MUT-002 MUT-003; do selector="$s"; "run_$(tr 'A-Z-' 'a-z\n' <<<"$s" | tr -d '\n')"; printf '%s/%s/pass\n' "$card" "$s"; done
    selector=all ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
