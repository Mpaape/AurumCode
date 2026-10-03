#!/usr/bin/env bash
# AUR-569 acceptance: a deterministic finding of the --seguranca pass at or above
# gate.fail_on fails the gate in every gate.inconclusive mode (no provider needed);
# without such a finding, warn still only warns.
#
# Selectors:
#   all              AC-001..AC-003, then the mutation
#   AC-001           versioned demo case (run.sh, expected/, out/) records the
#                    reproduction fixed: warn and block exit 3, origin security;
#                    the spec records the reproduction; run.sh --check passes
#   AC-002           Go proofs: warn and block with a security finding exit 3,
#                    gate line cites rule and origin
#   AC-003           Go proofs: no finding -> warn exits 0 (block still 1);
#                    no --seguranca -> no security finding
#   AC-002-MUT-001   letting warn swallow the security finding turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-569'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg demo/tutoriais/gate docs/specs/AUR-569.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for f in internal/gate/contributors.go cmd/aurumcode/aur569_test.go; do
  [[ -f "$repo_root/$f" ]] || infra "missing-source:$f"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a569.XXXXXX")" || infra mktemp
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
  grep -q '^caso_achado_deterministico()' "$demo/run.sh" || fail 'case-missing-in-run.sh'
  grep -q 'achado-deterministico' "$demo/run.sh" || fail 'case-not-declared'
  local log="$demo/out/achado-deterministico.log"
  [[ -s "$log" && -s "$demo/expected/achado-deterministico.txt" ]] || fail 'case-out-or-expected-missing'
  grep -q 'seguranca' "$log" || fail 'out-without-seguranca-run'
  grep -q 'security/hardcoded-secret' "$log" || fail 'out-without-finding'
  grep -q 'origem security' "$log" || fail 'out-without-origin'
  [[ "$(grep -c '^exit_code=3$' "$log")" == "2" ]] || fail 'out-warn-and-block-must-exit-3'
  grep -q '^exit_code=0$' "$log" || fail 'out-clean-case-must-exit-0'
  grep -q 'REPRODU' "$repo_root/docs/specs/AUR-569.md" || fail 'spec-without-reproduction'
  (cd "$repo_root" && bash demo/tutoriais/gate/run.sh --check >"$run_dir/check.log" 2>&1) || { cat "$run_dir/check.log" >&2; fail 'demo-check-failed'; }
}
run_ac002() {
  local log="$run_dir/ac002.log"
  run_go_test ./cmd/aurumcode/ '^TestAUR569SecurityFindingFailsGateInEveryInconclusiveMode$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR569SecurityFindingFailsGateInEveryInconclusiveMode
}
run_ac003() {
  local log="$run_dir/ac003.log"
  run_go_test ./cmd/aurumcode/ '^(TestAUR569WithoutFindingWarnStillOnlyWarns|TestAUR569NoSecurityPassNoSecurityFinding)$' "$log" || fail 'go-test-failed'
  need_pass "$log" TestAUR569WithoutFindingWarnStillOnlyWarns
  need_pass "$log" TestAUR569NoSecurityPassNoSecurityFinding
}
run_mutation() {
  local target="$run_dir/root/internal/gate/contributors.go"
  local anchor='func (SecurityPassContributor) Apply(_ context.Context, run *Run, res *Result) error {'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra mutation-anchor
  sed -i "/^func (SecurityPassContributor) Apply/a\\	if m, _ := run.Cfg.Gate.InconclusiveMode(); m == \"warn\" { return nil } // MUT-001: warn swallows the finding" "$target"
  grep -Fq 'MUT-001: warn swallows' "$target" || infra mutation-not-applied
  local log="$run_dir/mutation.log"
  run_go_test ./cmd/aurumcode/ '^TestAUR569SecurityFindingFailsGateInEveryInconclusiveMode$' "$log" || true
  grep -Eq 'build failed|undefined:|syntax error|declared and not used' "$log" && fail 'mutation-build-failure-not-behavioral'
  grep -q '^--- FAIL: TestAUR569SecurityFindingFailsGateInEveryInconclusiveMode' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-002-MUT-001) run_mutation ;;
  all) run_ac001; run_ac002; run_ac003; run_mutation ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
