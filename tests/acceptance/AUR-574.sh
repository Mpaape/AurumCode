#!/usr/bin/env bash
#
# Acceptance program for card AUR-574: a generic text file (.txt, which the
# runtime assigns the vimdoc grammar by extension alone) is reviewed as text
# by the model and never dropped from the review as "documentation".
#
# Selectors:
#   all             AC-001, AC-002, AC-003, then AC-001-MUT-001
#   AC-001          a sealed copy of the repo is built; `review --base HEAD~1`
#                   over the git-demo fixture must put config/demo-tokens.txt
#                   and NOTES.txt under "## Code Changes" with "Code files in
#                   this diff" >= 2; tests/e2e/AUR-441.sh and
#                   tests/acceptance/AUR-541.sh all pass unedited
#   AC-002          .md stays documentation, .go stays code, .txt and an
#                   extensionless file are not documentation (Go tests)
#   AC-003          the qualquer-linguagem tutorial carries the .txt case and
#                   `run.sh --check` is green
#   AC-001-MUT-001  in the copy only, vimdoc is put back under documentation:
#                   the AC-001 measurement must go red
#
# Exit codes: 0 holds, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-574'
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

catalog=internal/analyzer/language_catalog.yml
for input in go.mod go.sum cmd internal pkg tests/e2e tests/acceptance tests/fixtures/repos/git-demo/repo.git \
  "$catalog" docs/tutorials/qualquer-linguagem.md demo/tutoriais/qualquer-linguagem/run.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a574.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1' GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

seed_root() {
  rm -rf "$run_dir/root"; mkdir -p "$run_dir/root"
  local s
  for s in go.mod go.sum cmd internal pkg tests; do cp -R "$repo_root/$s" "$run_dir/root/$s"; done
  chmod -R u+w -- "$run_dir/root"
}

build_bin() { # build_bin OUT
  (cd "$run_dir/root" && go build -buildvcs=false -o "$1" ./cmd/aurumcode) >"$run_dir/build.log" 2>&1 || { cat "$run_dir/build.log" >&2; infra build_failed; }
}

# measure BIN: prints the captured prompt's coverage on stderr and returns 0
# only when both text files are reviewed as code.
measure() {
  local bin="$1" repo="$run_dir/demo-repo" cap="$run_dir/capture.txt" fx="$run_dir/fixture.json"
  rm -rf "$repo" "$run_dir/cache" "$cap"
  cp -R "$run_dir/root/tests/fixtures/repos/git-demo/repo.git" "$repo"; chmod -R u+w -- "$repo"
  printf '{"issues":[],"summary":"ok"}\n' >"$fx"
  (cd "$repo" && AURUMCODE_LLM_FIXTURE="$fx" AURUMCODE_CACHE_DIR="$run_dir/cache" AURUMCODE_PROMPT_CAPTURE="$cap" "$bin" review --base HEAD~1 >/dev/null 2>"$run_dir/review.err") || true
  [[ -s "$cap" ]] || infra no-prompt-captured
  local n section
  n="$(sed -n 's/^- Code files in this diff: \([0-9]*\)$/\1/p' "$cap")"
  section="$(sed -n '/^## Code Changes$/,/^## Codebase context/p' "$cap")"
  grep -E '^- Code files in this diff|^## Code Changes' "$cap" >&2 || true
  [[ -n "$n" ]] || infra no-coverage-line
  (( n >= 2 )) || { echo "code files in this diff: $n" >&2; return 1; }
  grep -Fq '### File: config/demo-tokens.txt' <<<"$section" || { echo 'config/demo-tokens.txt not in Code Changes' >&2; return 1; }
  grep -Fq '### File: NOTES.txt' <<<"$section" || { echo 'NOTES.txt not in Code Changes' >&2; return 1; }
  return 0
}

ac001() {
  seed_root
  build_bin "$run_dir/aurumcode"
  measure "$run_dir/aurumcode" || fail 'text-files-not-reviewed'
  local log="$run_dir/e2e441.log"
  (cd "$run_dir/root" && bash tests/e2e/AUR-441.sh) >"$log" 2>&1 || { cat "$log" >&2; fail 'e2e-AUR-441-red'; }
  log="$run_dir/a541.log"
  (cd "$run_dir/root" && bash tests/acceptance/AUR-541.sh all) >"$log" 2>&1 || { local rc=$?; cat "$log" >&2; (( rc == 79 )) && infra 'AUR-541-inconclusive'; fail 'AUR-541-red'; }
}

ac002() {
  seed_root
  local log="$run_dir/ac002.log"
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 400s -v -run 'AUR574|Language' ./internal/analyzer/) >"$log" 2>&1 || { cat "$log" >&2; fail 'go-test-red'; }
  local t
  for t in TestAUR574_CategoryOfPath TestAUR574_VimdocIsNeverDocumentation TestAUR574_ExtensionlessCode; do
    grep -q "^--- PASS: $t " "$log" || fail "missing-pass:$t"
  done
}

ac003() {
  grep -q '^<!-- saida: txt-com-segredo -->$' "$repo_root/docs/tutorials/qualquer-linguagem.md" || fail 'tutorial-sem-o-caso'
  grep -q '^caso_txt_com_segredo()' "$repo_root/demo/tutoriais/qualquer-linguagem/run.sh" || fail 'demo-sem-o-caso'
  bash "$repo_root/demo/tutoriais/qualquer-linguagem/run.sh" --check >"$run_dir/check.out" 2>&1 || { cat "$run_dir/check.out" >&2; fail 'check-falhou'; }
  grep -q 'txt-com-segredo: ok' "$run_dir/check.out" || fail 'check-sem-o-caso'
}

mut001() {
  seed_root
  local cat="$run_dir/root/$catalog"
  grep -Fq 'other: [vimdoc]' "$cat" || infra mutation-anchor-missing
  sed -i 's/^  other: \[vimdoc\]$//; s/^\(  documentation: \[.*\)\]$/\1, vimdoc]/' "$cat"
  grep -Eq '^  documentation: \[.*vimdoc\]$' "$cat" || infra mutation-not-applied
  build_bin "$run_dir/aurumcode-mut"
  if measure "$run_dir/aurumcode-mut"; then fail 'mutation-survived'; fi
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
  AC-001-MUT-001) mut001 ;;
  all) ac001; ac002; ac003; mut001 ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
