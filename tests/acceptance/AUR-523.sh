#!/usr/bin/env bash
#
# Acceptance program for card AUR-523 (AC-001..AC-004, MUT-001).
#
# It compiles the real aurumcode binary, runs it over the multi-language
# corpus in tests/benchmark/multilang with the central policy and the
# deterministic fake provider, and checks that verdict and findings come from
# the binary's own output. MUT-001 reads the findings from the fixture instead
# and must be detected (the provenance check goes red).
#
# Exit codes: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-523'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

bench="$repo_root/tests/benchmark"
for input in aur523.go aur523_report.go aur523_test.go multilang/manifest.json multilang/policy/.aurumcode/config.yml \
             out/multilang-report.json out/multilang-report.md; do
  [[ -e "$bench/$input" ]] || fail "behavior-missing:$input"
done
[[ -d "$bench/multilang/cases" ]] || fail 'behavior-missing:multilang/cases'
[[ -f "$repo_root/go.mod" ]] || infra missing-go-mod
[[ -f "$repo_root/go.sum" ]] || infra missing-go-sum

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a523.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-buildvcs=false -mod=mod'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
mkdir -p "$GOCACHE" "$GOTMPDIR"

# Shell-side assertions on the versioned evidence (each selector has its own).
want() { [[ "$selector" == all || "$selector" == "$1" ]]; }

if want AC-002; then
  langs="$(for f in "$bench"/multilang/cases/*/case.json; do
    sed -n 's/^[[:space:]]*"language":[[:space:]]*"\([^"]*\)".*/\1/p' "$f"
  done | sort -u | wc -l)"
  ((langs >= 6)) || fail "fewer-than-6-languages:$langs"
  grep -q '"corpus_sha256"' "$bench/multilang/manifest.json" || fail 'manifest-without-corpus-digest'
fi
if want AC-003; then
  for col in recall precision approved_with_defect recall_interval_95; do
    grep -q "\"$col\"" "$bench/out/multilang-report.json" || fail "report-missing-$col"
  done
fi
if want AC-004; then
  for d in corpus_sha256 policy_sha256; do
    grep -Eq "\"$d\": \"[0-9a-f]{64}\"" "$bench/out/multilang-report.json" || fail "report-header-missing-$d"
  done
fi

pattern=''
case "$selector" in
  all)                   pattern='^TestAUR523' ;;
  AC-001)                pattern='^TestAUR523RealBinaryOutputIsTheSource$' ;;
  AC-002)                pattern='^TestAUR523CorpusLabeledVersionedWithDigest$' ;;
  AC-003)                pattern='^TestAUR523ReportPerLanguage$' ;;
  AC-004)                pattern='^TestAUR523ReportReproducible$' ;;
  MUT-001|AC-001-MUT-001) pattern='^TestAUR523MutationFixtureReadIsRed$' ;;
esac

log="$run_dir/go-test.log"
set +e
(cd "$repo_root" && go test -count=1 -v -timeout 480s ./tests/benchmark -run "$pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2
((status == 0)) || fail "go-test-exit:$status"

grep -Eq -- '^--- PASS: TestAUR523' "$log" || fail 'no-test-executed'
grep -Eq '(^|[[:space:]])ok[[:space:]].*tests/benchmark' "$log" || fail 'package-not-ok'

printf '%s/%s/pass\n' "$card" "$selector"
