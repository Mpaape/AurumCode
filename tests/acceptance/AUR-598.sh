#!/usr/bin/env bash
# AUR-598 acceptance: a model finding that cites the redaction marker is
# discarded, counted and named in the discard warning; a deterministic
# scanner finding with the same marker keeps blocking; the tutorial PR
# helper parses under semgrep.
#
# Selectors:
#   all        AC-001, AC-002, MUT-001, MUT-002, the static part of AC-003,
#              and the semgrep part of AC-003 when a semgrep binary and the
#              rule registry are reachable (else the line says so: the proof
#              runs in the product image, see docs/specs/AUR-598.md)
#   AC-001     model finding with the marker discarded, counted and warned
#   AC-002     scanner findings with the marker are kept
#   AC-003     pr.sh passes bash -n, has no read-write `<>` redirection, and
#              (with AURUM598_SEMGREP=on) semgrep reports 0 errors on it
#   MUT-001    removing the filter call turns AC-001 RED
#   MUT-002    applying the filter to scanner findings turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-598'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly helper='demo/tutoriais/_lib/pr.sh'
readonly pkg='./internal/review/'
readonly ac1_test='TestRedactionMarkerAC001ModelFindingDiscarded'
readonly ac2_test='TestRedactionMarkerAC002ScannerFindingKept'
readonly filter_call='result.Issues, echoed = discardRedactedModelFindings(result.Issues)'
readonly origin_guard='return issue.Origin == ""'

for input in go.mod go.sum cmd internal pkg "$helper"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a598.XXXXXX")" || infra mktemp
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
    cp -R "$repo_root/$source" "$root/$source"
  done
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

run_tests() {
  local name="$1" test="$2"
  local root="$run_dir/root" log="$run_dir/$name.log"
  command -v go >/dev/null 2>&1 || infra missing_go
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "^${test}\$" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  grep -Eq -- "^--- PASS: ${test} " "$log" || { cat "$log" >&2; fail "missing-pass:$test"; }
}

expect_red() {
  local root="$1" log="$2" test="$3"
  if go_test "$root" "$log" "^${test}\$" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,3p' >&2
}

run_ac001() {
  run_tests AC-001 "$ac1_test"
  printf '%s/AC-001/pass\n' "$card"
}

run_ac002() {
  run_tests AC-002 "$ac2_test"
  printf '%s/AC-002/pass\n' "$card"
}

# check_helper is the offline part of AC-003: the helper is valid bash and
# no longer uses the read-write redirection semgrep cannot parse.
check_helper() {
  local file="$1"
  bash -n "$file" || fail "bash-n:$helper"
  if grep -nE '<>' "$file" >&2; then fail "read-write-redirection:$helper"; fi
}

# semgrep_ready says whether a semgrep binary can reach the product's rule
# packs (registry); the sealed acceptance has neither.
semgrep_ready() {
  command -v semgrep >/dev/null 2>&1 || return 1
  [[ "${AURUM598_SEMGREP:-}" == on ]]
}

run_semgrep() {
  local root="$run_dir/root-ac3" report="$run_dir/semgrep.json"
  mkdir -p "$root/${helper%/*}"
  cp "$repo_root/$helper" "$root/$helper"
  ( cd "$root" && semgrep scan --json --quiet --metrics=off --disable-version-check \
      --config p/security-audit --config p/owasp-top-ten "$helper" ) >"$report" 2>"$run_dir/semgrep.err" \
    || { cat "$run_dir/semgrep.err" >&2; fail semgrep-failed; }
  python3 - "$report" <<'PY' || fail semgrep-errors
import json, sys
report = json.load(open(sys.argv[1]))
errors = report.get("errors", [])
scanned = report.get("paths", {}).get("scanned", [])
for e in errors:
    print("semgrep error:", e.get("type"), e.get("path", ""), e.get("message", "")[:200], file=sys.stderr)
if len(scanned) != 1:
    print("scanned paths:", scanned, file=sys.stderr)
    sys.exit(1)
sys.exit(1 if errors else 0)
PY
  printf '%s/AC-003/pass (semgrep, %s, 0 errors)\n' "$card" "$helper"
}

run_ac003() {
  check_helper "$repo_root/$helper"
  if [[ "$selector" == AC-003 ]]; then
    semgrep_ready || infra "semgrep-unavailable (set AURUM598_SEMGREP=on in the product image with network)"
  fi
  if semgrep_ready; then
    run_semgrep
  else
    printf '%s/AC-003/static-pass; semgrep not-run-here (proven in the product image, docs/specs/AUR-598.md)\n' "$card"
  fi
}

run_mut001() {
  local root="$run_dir/root-mut1" file
  stage "$root"
  file="$root/internal/review/reviewer.go"
  grep -Fq -- "$filter_call" "$file" || infra filter-call-missing
  grep -Fv -- "$filter_call" "$file" >"$file.mut" && mv "$file.mut" "$file"
  ! grep -Fq -- "$filter_call" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$ac1_test"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" file
  stage "$root"
  file="$root/internal/review/redacted_findings.go"
  grep -Fq -- "$origin_guard" "$file" || infra origin-guard-missing
  # The filter now treats every finding as the model's.
  sed -i 's/return issue.Origin == ""/return issue.Origin == issue.Origin/' "$file"
  ! grep -Fq -- "$origin_guard" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut2.log" "$ac2_test"
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
