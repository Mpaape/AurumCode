#!/usr/bin/env bash
# AUR-517 acceptance: the published review summary never re-presents, as a
# current defect, an accusation the scope/evidence gate or the rule gate
# already discarded from the model's findings, a surviving finding keeps its
# evidence/impact and a matching verdict with its eligible summary still
# visible, and the local report, a plain PR comment and a formal PR review
# all state the same decision.
#
# Selectors:
#   all             run every behavior test below
#   AC-001 .. AC-003
#   AC-001-MUT-001  restore unrestricted summary publication; AC-001 must
#                   fail (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-517'
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

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/aur517_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a517.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/root" "$run_dir/cache" "$run_dir/gotmp"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# MUT-001: restore the unrestricted summary publication this card removed --
# the call always behaves as though nothing was discarded, so a withheld
# accusation is republished again. Anchored on the stable call site; the
# token is split so this file cannot match its own edit, and a missing
# anchor is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/internal/review/reviewer.go"
  local anchor='withholdSummaryWhenFiltered(result.Summary, discarded'"ByPipeline)"
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor}|withholdSummaryWhenFiltered(result.Summary, 0)|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
case "$selector" in
  all)            test_pattern='^TestAUR517' ;;
  AC-001)         test_pattern='^TestAUR517SummaryWithheldWhenAccusationOutOfScope$' ;;
  AC-002)         test_pattern='^TestAUR517ValidFindingKeepsEvidenceAndVerdict$' ;;
  AC-003)         test_pattern='^TestAUR517SameDecisionAcrossSinks$' ;;
  AC-001-MUT-001) test_pattern='^TestAUR517SummaryWithheldWhenAccusationOutOfScope$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./cmd/aurumcode/... ./internal/review/... -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR517' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR517' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in SummaryWithheldWhenAccusationOutOfScope ValidFindingKeepsEvidenceAndVerdict SameDecisionAcrossSinks; do
    grep -q "^--- PASS: TestAUR517$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
