#!/usr/bin/env bash
# AUR-567 acceptance: the gate line carries the typed origin (skills, analysis, sast,
# security, dtrack) the audit records, for every source; the gate line and the report show
# the same message; the formal review follows the declared gate.
#
# Selectors:
#   all              AC-001..AC-003, then the mutation
#   AC-001           Go proofs: one finding of each source (skills, analysis, sast with a
#                    fake semgrep, security) in one --base run, each gate line says
#                    `origem <origin of the audit>`; --pr security-pass finding exits 3
#   AC-002           Go proof: the gate line shows the report's message after redaction
#   AC-003           tutorial and demo coherent (run.sh --check), configuration doc states
#                    the behavior, formal review follows the gate, spec registered
#   AC-001-MUT-001   the SAST line saying `origem policy` again turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-567'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg demo/tutoriais/gate docs/specs/AUR-567.md docs/configuration.md docs/tutorials/gate.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for f in internal/gate/sast.go internal/gate/sources.go cmd/aurumcode/aur567_test.go; do
  [[ -f "$repo_root/$f" ]] || infra "missing-source:$f"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a567.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/root"
for source in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$source" "$run_dir/root/$source"; done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() { # pkg pattern log
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 400s -v "$1" -run "$2") >"$3" 2>&1
  local status=$?
  set -e
  cat "$3" >&2
  return $status
}
need_pass() { grep -q "^--- PASS: $2 " "$1" || fail "missing-pass:$2"; }

demo="$repo_root/demo/tutoriais/gate"

run_ac001() {
  local log="$run_dir/ac001.log"
  run_go_test ./cmd/aurumcode/ '^(TestAUR567GateLineCarriesTypedOriginForEverySource|TestAUR567PRSecurityPassFindingFailsGate)$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR567GateLineCarriesTypedOriginForEverySource
  need_pass "$log" TestAUR567PRSecurityPassFindingFailsGate
}
run_ac002() {
  local log="$run_dir/ac002.log"
  run_go_test ./cmd/aurumcode/ '^TestAUR567GateLineAndReportShowTheSameMessage$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR567GateLineAndReportShowTheSameMessage
}
run_ac003() {
  local log="$run_dir/ac003.log"
  run_go_test ./cmd/aurumcode/ '^(TestAUR567FormalReviewFollowsDeclaredGate|TestAUR567GateAlignedReviewEvent|TestAUR568PRPathUnwritableNeverSucceeds)$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR567FormalReviewFollowsDeclaredGate
  need_pass "$log" TestAUR567GateAlignedReviewEvent
  need_pass "$log" TestAUR568PRPathUnwritableNeverSucceeds
  local f="$demo/out/fontes.log"
  [[ -s "$f" ]] || fail 'fontes-out-missing'
  for o in skills analysis sast; do grep -q "policy gate: .*origem $o" "$f" || fail "fontes-without-origem-$o"; done
  grep -q 'origem policy' "$f" && fail 'fontes-still-says-origem-policy'
  grep -q 'REDACTED\] secret or credential' "$f" && fail 'fontes-redacted-title'
  grep -q 'Origem em toda linha de gate (AUR-567)' "$repo_root/docs/configuration.md" || fail 'config-doc-without-origin'
  grep -q 'Review formal e gate (AUR-567)' "$repo_root/docs/configuration.md" || fail 'config-doc-without-formal-review'
  grep -q 'Origem uniforme (AUR-567)' "$repo_root/docs/tutorials/gate.md" || fail 'tutorial-without-origin'
  grep -q 'origem sast, secao policy' "$repo_root/docs/tutorials/gate.md" || fail 'tutorial-block-stale'
  (cd "$repo_root" && bash demo/tutoriais/gate/run.sh --check >"$run_dir/check.log" 2>&1) || { cat "$run_dir/check.log" >&2; fail 'demo-check-failed'; }
}
run_mutation() {
  local target="$run_dir/root/internal/gate/sast.go"
  local from='name, OriginSAST+", secao "+origin)'
  [[ "$(grep -Fc "$from" "$target")" == "1" ]] || infra mutation-anchor
  sed -i 's/name, OriginSAST+", secao "+origin)/name, origin) \/\/ MUT-001: origem policy/' "$target"
  grep -Fq 'MUT-001: origem policy' "$target" || infra mutation-not-applied
  local log="$run_dir/mutation.log"
  run_go_test ./cmd/aurumcode/ '^TestAUR567GateLineCarriesTypedOriginForEverySource$' "$log" || true
  grep -Eq 'build failed|undefined:|syntax error|declared and not used' "$log" && fail 'mutation-build-failure-not-behavioral'
  grep -q '^--- FAIL: TestAUR567GateLineCarriesTypedOriginForEverySource' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-001-MUT-001) run_mutation ;;
  all) run_ac001; run_ac002; run_ac003; run_mutation ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
