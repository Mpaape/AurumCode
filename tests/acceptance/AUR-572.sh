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

for f in cmd/aurumcode/subcommands.go cmd/aurumcode/aur572_test.go internal/render/no_findings.go docs/tutorials/README.md; do
  [[ -f "$repo_root/$f" ]] || infra "missing:$f"
done

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

# AC-003. The sealed profile sees only the card's paths and read_paths, so the
# AC-003 of AUR-561..564 may be absent there; they run when present. In every
# case the same property is checked here against the real binary: each
# `aurumcode <sub> --flag` of the tutorials, of getting-started and of the demo
# scripts names a command the help lists and a flag its --help declares.
ac003() {
  local n bin="$run_dir/aurumcode" f line sub tok name checked=0 ran=0
  for n in 561 562 563 564; do
    if [[ -f "$repo_root/tests/acceptance/AUR-$n.sh" ]]; then
      bash "$repo_root/tests/acceptance/AUR-$n.sh" AC-003 >&2 || fail "AUR-$n-AC-003-vermelho"
      ran=$((ran + 1))
    fi
  done
  (cd "$run_dir/root" && go build -mod=mod -o "$bin" ./cmd/aurumcode) >"$run_dir/build.log" 2>&1 || { cat "$run_dir/build.log" >&2; infra go-build; }
  "$bin" --help >"$run_dir/help.txt" 2>&1 || fail help-geral-falhou
  : >"$run_dir/cmds.txt"
  for f in "$repo_root"/docs/tutorials/*.md "$repo_root/docs/getting-started.md"; do
    awk '/^```/ { if (open) { open = 0 } else { open = ($0 == "```bash") } next } open && /^(\$ )?aurumcode / { sub(/^\$ /, ""); print }' "$f" >>"$run_dir/cmds.txt"
  done
  for f in "$repo_root"/demo/tutoriais/*/run.sh; do
    sed -n 's/^[[:space:]]*aurum \(review\|fix\|sbom\|sign\|xbom\) /aurumcode \1 /p' "$f" >>"$run_dir/cmds.txt"
    sed -n 's/.*aurum_raw -- \(review\|fix\|sbom\|sign\|xbom\) /aurumcode \1 /p' "$f" >>"$run_dir/cmds.txt"
  done
  [[ -s "$run_dir/cmds.txt" ]] || fail nenhum-comando-extraido
  while IFS= read -r line; do
    line="${line%%#*}"; line="${line%%>*}"
    # shellcheck disable=SC2206
    local toks=($line)
    sub="${toks[1]:-}"
    [[ -n "$sub" && "$sub" != -* ]] || continue
    grep -qE "^  $sub[[:space:]]" "$run_dir/help.txt" || fail "comando-fora-do-help:$sub ($line)"
    "$bin" "$sub" --help >"$run_dir/help-$sub.txt" 2>&1 || fail "help-falhou:$sub"
    for tok in "${toks[@]:2}"; do
      case "$tok" in
        --*)
          name="${tok#--}"; name="${name%%=*}"
          [[ "$name" == help ]] && continue
          grep -qE "^  -$name( |\$)" "$run_dir/help-$sub.txt" || fail "flag-fora-do-help:$sub --$name ($line)"
          checked=$((checked + 1)) ;;
      esac
    done
  done < <(sort -u "$run_dir/cmds.txt")
  (( checked >= 10 )) || fail "poucas-flags-conferidas:$checked"
  printf '%s/AC-003/pass (%d flags conferidas contra --help; %d scripts AC-003 de 561..564 presentes e verdes)\n' "$card" "$checked" "$ran"
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
  AC-003) ac003 ;;
  AC-002-MUT-001) mut001 ;;
  all) ac001; ac002; ac003; seed_root; mut001 ;;
esac
