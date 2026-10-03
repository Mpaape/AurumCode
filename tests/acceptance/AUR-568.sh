#!/usr/bin/env bash
# AUR-568 -- a requested --auditoria/--sarif that cannot be written never
# ends as success, on --base and --pr.
#
# Selectors:
#   AC-001          --auditoria in an unwritable path (parent is a regular
#                   file; missing directory): exit != 0, message names the
#                   path; with gate block, failure with the reason
#   AC-002          the same for --sarif
#   AC-003          writable paths behave as before (AUR-568 regression and
#                   AUR-521's own end-to-end tests)
#   AC-004          the tutorial auditoria-sarif records the failure case and
#                   the demo's --check holds
#   AC-001-MUT-001  in a copy, the audit write error is ignored again:
#                   AC-001's tests go RED
#   all             AC-001..AC-004 and the mutation
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = holds, 1 = behavioral RED, 64 = unknown selector, 79 = infrastructure
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-568'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
real_go="$(command -v go 2>/dev/null)" || infra missing_go

for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur568_test.go cmd/aurumcode/compliance_artifacts.go \
  docs/tutorials/auditoria-sarif.md demo/tutoriais/auditoria-sarif/run.sh docs/specs/AUR-568.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a568.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1' GOMEMLIMIT=2GiB GOMAXPROCS=1

go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  set +e
  (cd "$root" && "$real_go" test -mod=mod -p 1 -count=1 -timeout 300s -v -run "$pattern" "$@") >"$log" 2>&1
  local status=$?
  set -e
  return $status
}

require_pass() {
  local log="$1"; shift
  local name
  for name in "$@"; do
    grep -q "^--- PASS: $name " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

# unwritable <flag-element>: the base and --pr tests of one artifact flag.
unwritable() {
  local log="$run_dir/unwritable-$1.log"
  go_test "$repo_root" "$log" "^(TestAUR568BasePathUnwritableNeverSucceeds|TestAUR568PRPathUnwritableNeverSucceeds)\$/$1" ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR568BasePathUnwritableNeverSucceeds TestAUR568PRPathUnwritableNeverSucceeds
}

ac001() { unwritable auditoria; }
ac002() { unwritable sarif; }

ac003() {
  local log="$run_dir/ac003.log"
  go_test "$repo_root" "$log" '^(TestAUR568BaseWritablePathsUnchanged|TestAUR568BaseWritableSiblingAgreesWithDecision|TestAUR521.*)$' ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR568BaseWritablePathsUnchanged TestAUR568BaseWritableSiblingAgreesWithDecision TestAUR521AuditAndSARIFOnGateBreach TestAUR521PRPathWritesComplianceArtifacts
}

ac004() {
  local doc="$repo_root/docs/tutorials/auditoria-sarif.md" demo="$repo_root/demo/tutoriais/auditoria-sarif"
  grep -q '^<!-- saida: falha-caminho-invalido -->' "$doc" || fail tutorial-sem-caso-de-falha
  grep -q 'audit_write_failed' "$doc" || fail tutorial-sem-motivo
  grep -q 'sarif_write_failed' "$doc" || fail tutorial-sem-motivo-sarif
  grep -q '^exit_code=1' "$demo/out/falha-caminho-invalido.log" || fail out-sem-exit-1
  if grep -q 'o exit continua 0' "$doc" "$demo/run.sh" "$demo/expected/falha-caminho-invalido.txt"; then fail achado-antigo-ainda-presente; fi
  bash "$demo/run.sh" --check >"$run_dir/check.log" 2>&1 || { cat "$run_dir/check.log" >&2; fail demo-check; }
}

ac001_mut001() {
  local root="$run_dir/mut1" anchor='return render.WriteAuditRecord(in.auditoriaPath, rec, filter)'
  mkdir -p "$root"
  cp -R "$repo_root/go.mod" "$repo_root/go.sum" "$repo_root/cmd" "$repo_root/internal" "$repo_root/pkg" "$root/"
  chmod -R u+w "$root"
  local file="$root/cmd/aurumcode/compliance_artifacts.go"
  [[ "$(grep -cF -- "$anchor" "$file")" == 1 ]] || infra mutation-anchor-missing
  sed -i "s|$anchor|_ = render.WriteAuditRecord(in.auditoriaPath, rec, filter)\n\treturn nil|" "$file"
  grep -q '_ = render.WriteAuditRecord' "$file" || infra mutation-not-applied
  local log="$run_dir/mut1.log" survived=0
  go_test "$root" "$log" '^(TestAUR568BasePathUnwritableNeverSucceeds|TestAUR568PRPathUnwritableNeverSucceeds)$/auditoria' ./cmd/aurumcode/ && survived=1
  (( survived == 0 )) || { cat "$log" >&2; fail mutation-survived:ignore-audit-write-error; }
  grep -Eq -- '^--- FAIL: TestAUR568(Base|PR)PathUnwritableNeverSucceeds' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  rm -rf "$root"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-004) ac004 ;;
  AC-001-MUT-001) ac001_mut001 ;;
  all) ac001; ac002; ac003; ac004; ac001_mut001 ;;
esac
printf '%s/%s/ok\n' "$card" "$selector"
