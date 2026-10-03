#!/usr/bin/env bash
# AUR-572 acceptance: complete help per subcommand and a report that never says
# "No issues found." while a source is inconclusive.
#
# Selectors:
#   all             AC-001, AC-002, AC-003, then the mutation
#   AC-001          `aurumcode --help` lists review, fix, sbom, sign, xbom and
#                   each `<sub> --help` prints every flag its FlagSet declares
#                   (compared with FlagSet.VisitAll) plus an example
#   AC-002          inconclusive SAST -> no "No issues found.", a sentence naming
#                   the source; conclusive clean -> "No issues found."
#   AC-003          the AC-003 of AUR-561..564 (commands and flags of the
#                   tutorials against --help) stay green
#   AC-002-MUT-001  printing "No issues found." again with an inconclusive source
#                   turns AC-002 red
# Unknown selectors exit 64; infrastructure failures exit 79; failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-572'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

for f in cmd/aurumcode/subcommands.go cmd/aurumcode/aur572_test.go internal/render/no_findings.go \
  tests/acceptance/AUR-561.sh tests/acceptance/AUR-562.sh tests/acceptance/AUR-563.sh tests/acceptance/AUR-564.sh; do
  [[ -f "$repo_root/$f" ]] || infra "missing:$f"
done

ac003() {
  local n
  for n in 561 562 563 564; do
    bash "$repo_root/tests/acceptance/AUR-$n.sh" AC-003 >&2 || fail "AUR-$n-AC-003-vermelho"
  done
  printf '%s/AC-003/pass (AC-003 de AUR-561..564 verdes)\n' "$card"
}

if [[ "$selector" == AC-003 ]]; then ac003; exit 0; fi

command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a572.XXXXXX")" || infra mktemp
trap 'chmod -R u+rwX "${run_dir:?}" >/dev/null 2>&1 || true; rm -rf "${run_dir:?}"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
seed_root() {
  rm -rf "$run_dir/root"; mkdir -p "$run_dir/root"
  for s in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$s" "$run_dir/root/$s"; done
  chmod -R u+w -- "$run_dir/root"
}
seed_root
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false' GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() { # pattern log
  local status
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./cmd/aurumcode -run "$1") >"$2" 2>&1
  status=$?
  set -e
  cat "$2" >&2
  return $status
}

ac001_tests=(TestAUR572TopLevelHelpListsEverySubcommand TestAUR572SubcommandHelpListsEveryDeclaredFlag)
ac002_tests=(TestAUR572PrintFindingsNeverClaimsCleanWhenInconclusive TestAUR572InconclusiveSASTReportNeverSaysNoIssuesFound TestAUR572ConclusiveCleanReportKeepsNoIssuesFound)
pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

ac001() {
  local n
  run_go_test "$(pattern_of "${ac001_tests[@]}")" "$run_dir/ac001.log" || fail 'go-test-falhou'
  for n in "${ac001_tests[@]}"; do grep -q "^--- PASS: $n " "$run_dir/ac001.log" || fail "sem-pass:$n"; done
  printf '%s/AC-001/pass (ajuda geral lista os cinco; cada --help traz toda flag do FlagSet)\n' "$card"
}

ac002() {
  local n
  run_go_test "$(pattern_of "${ac002_tests[@]}")" "$run_dir/ac002.log" || fail 'go-test-falhou'
  for n in "${ac002_tests[@]}"; do grep -q "^--- PASS: $n " "$run_dir/ac002.log" || fail "sem-pass:$n"; done
  printf '%s/AC-002/pass (inconclusivo nomeado; caso limpo mantem No issues found.)\n' "$card"
}

mut001() {
  local f="$run_dir/root/cmd/aurumcode/main.go"
  grep -Fq 'fmt.Fprintln(stdout, render.NoFindingsLine(inconclusiveReason))' "$f" || infra mutation-anchor-missing
  sed -i 's/fmt.Fprintln(stdout, render.NoFindingsLine(inconclusiveReason))/fmt.Fprintln(stdout, render.NoIssuesLine)/' "$f"
  grep -Fq 'fmt.Fprintln(stdout, render.NoIssuesLine)' "$f" || infra mutation-not-applied
  # inconclusiveReason is now unused by the mutated line only as an argument; keep the build alive
  sed -i 's/^func printFindings(stdout io.Writer, result \*types.ReviewResult, inconclusiveReason string) {/&\n\t_ = inconclusiveReason/' "$f"
  run_go_test "$(pattern_of "${ac002_tests[@]}")" "$run_dir/mut.log" && fail 'mutation-survived'
  grep -Eq '^--- FAIL: TestAUR572' "$run_dir/mut.log" || fail 'mutation-not-red-by-ac002'
  if grep -Eq 'build failed|undefined:|syntax error|cannot use' "$run_dir/mut.log"; then fail 'mutation-build-failure-not-behavioral'; fi
  printf '%s/AC-002-MUT-001/pass (No issues found. com inconclusivo reprova AC-002)\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-002-MUT-001) mut001 ;;
  all) ac001; ac002; ac003; seed_root; mut001 ;;
esac
