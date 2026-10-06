#!/usr/bin/env bash
# AUR-590 acceptance: measured repairs.
#
# Selectors:
#   all      AC-001, AC-002, AC-004, AC-005, MUT-001, MUT-002, AC-003, AC-006
#   AC-001   git fixtures isolated from ambient git configuration: the gittest
#            proofs plus the two packed-repository tests (PASS where git
#            exists; the sealed image has no git and prints skipped:no-git)
#   AC-002   no panic( in production files of internal/prompt; the embedded
#            defaults' failure is returned and tested
#   AC-003   `go list -m all` with GOPROXY=off over the module
#   AC-004   tut_check counts every expected line and orders RESULTADO lines
#   AC-005   govet: GOWORK=off proved by behavior; touched cgo package is
#            inconclusive
#   AC-006   AUR-491, AUR-448 and AUR-458 acceptances green
#   MUT-001  one of two `exit_code=3` turned into `exit_code=1` turns AC-004 RED
#   MUT-002  GOWORK=off removed from the govet environment turns AC-005 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-590'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

current="$selector"
fail() { printf '%s/%s/%s\n' "$card" "$current" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$current" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg demo/tutoriais/_lib/tutorial.sh demo/tutoriais/qualquer-linguagem/expected/falha-extensao-desconhecida.txt demo/tutoriais/qualquer-linguagem/out/falha-extensao-desconhecida.log; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a590.XXXXXX")" || infra mktemp
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

# go_test root log pattern pkgs... runs the named tests; rc is go's.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

# require_pass log names... fails unless every named test passed.
require_pass() {
  local log="$1" name; shift
  for name in "$@"; do
    grep -Eq -- "^[[:space:]]*--- PASS: $name \\(" "$log" || { cat "$log" >&2; fail "not-passed:$name"; }
  done
}

# require_pass_or_skip log names...: PASS, or SKIP when git is absent.
require_pass_or_skip() {
  local log="$1" name; shift
  for name in "$@"; do
    if command -v git >/dev/null 2>&1; then
      require_pass "$log" "$name"
    else
      grep -Eq -- "^[[:space:]]*--- (PASS|SKIP): $name \\(" "$log" || { cat "$log" >&2; fail "not-run:$name"; }
    fi
  done
}

module=''
staged() { [[ -n "$module" ]] || { module="$run_dir/module"; stage "$module"; }; }

ac_001() {
  current=AC-001; staged
  local log="$run_dir/ac001.log"
  go_test "$module" "$log" 'TestAmbientHomeConfigBreaksAPlainFixture|TestHermeticEnvIgnoresHostileHome|TestHermeticEnvIgnoresHostileGlobalFile|TestOpenRepo_GitBinary_PackedObjects|TestOpenRepo_GitBinary_LinkedWorktree|TestAUR536PackedRepositoryWithoutGitIsUnverifiable' \
    ./internal/gittest ./internal/analyzer ./cmd/aurumcode || { cat "$log" >&2; fail go-test; }
  require_pass_or_skip "$log" TestAmbientHomeConfigBreaksAPlainFixture TestHermeticEnvIgnoresHostileHome \
    TestHermeticEnvIgnoresHostileGlobalFile TestOpenRepo_GitBinary_PackedObjects TestAUR536PackedRepositoryWithoutGitIsUnverifiable
  command -v git >/dev/null 2>&1 || printf '%s/AC-001/skipped:no-git\n' "$card"
  printf '%s/AC-001/pass\n' "$card"
}

ac_002() {
  current=AC-002; staged
  local hits log="$run_dir/ac002.log"
  hits="$(find "$module/internal/prompt" -type f -name '*.go' ! -name '*_test.go' -exec grep -nF 'panic(' {} + || true)"
  [[ -z "$hits" ]] || { printf '%s\n' "$hits" >&2; fail panic-in-internal-prompt; }
  go_test "$module" "$log" 'TestEmbeddedDefaultsLoad|TestLoadSlotLimitsReturnsErrors|TestCatalogIDsReturnsLoaderError|TestBuilderRefusesAssemblyWhenDefaultsFailed' ./internal/prompt ||
    { cat "$log" >&2; fail go-test; }
  require_pass "$log" TestEmbeddedDefaultsLoad TestLoadSlotLimitsReturnsErrors TestCatalogIDsReturnsLoaderError TestBuilderRefusesAssemblyWhenDefaultsFailed
  printf '%s/AC-002/pass\n' "$card"
}

# AC-003: the sealed image's module cache resolves the whole module graph
# offline (repinned from the current go.mod/go.sum, docs/specs/AUR-590.md).
ac_003() {
  current=AC-003; staged
  ( cd "$module" && GOFLAGS='-mod=mod' go list -m all ) >"$run_dir/ac003.log" 2>&1 ||
    { cat "$run_dir/ac003.log" >&2; fail go-list-offline-failed; }
  printf '%s/AC-003/pass\n' "$card"
}

# tutorial_copy dir: the qualquer-linguagem tutorial and _lib, writable.
tutorial_copy() {
  local dir="$1"
  mkdir -p "$dir/demo/tutoriais"
  cp -R "$repo_root/demo/tutoriais/_lib" "$repo_root/demo/tutoriais/qualquer-linguagem" "$dir/demo/tutoriais/"
  chmod -R u+w -- "$dir"
}

# check_copy dir: tut_check over the copy; prints its output (exit ignored:
# the copy's tree identity never matches out/.imagem, which is judged by the
# tutorials' own acceptances, not here).
check_copy() { bash "$1/demo/tutoriais/qualquer-linguagem/run.sh" --check 2>&1 || true; }

readonly tcase='falha-extensao-desconhecida'

ac_004() {
  current=AC-004
  local dir="$run_dir/ac004" out
  tutorial_copy "$dir"
  out="$(check_copy "$dir")"
  grep -Fxq "caso $tcase: ok" <<<"$out" || { printf '%s\n' "$out" >&2; fail control-case-not-ok; }
  # A RESULTADO line moved after the next one is out of order.
  local exp="$dir/demo/tutoriais/qualquer-linguagem/expected/$tcase.txt"
  awk '/^RESULTADO:/ && !moved { held = $0; moved = 1; next } { print } /^RESULTADO:/ && held != "" { print held; held = "" }' "$exp" >"$exp.new"
  mv "$exp.new" "$exp"
  out="$(check_copy "$dir")"
  grep -Fq "DIVERGENCIA caso=$tcase: RESULTADO fora de ordem" <<<"$out" || { printf '%s\n' "$out" >&2; fail order-not-checked; }
  printf '%s/AC-004/pass\n' "$card"
}

mut_001() {
  current=MUT-001
  local dir="$run_dir/mut001" out log
  tutorial_copy "$dir"
  log="$dir/demo/tutoriais/qualquer-linguagem/out/$tcase.log"
  [[ "$(grep -c '^exit_code=3$' "$log")" -eq 2 ]] || infra fixture-needs-two-exit-code-3
  awk '!done && $0 == "exit_code=3" { print "exit_code=1"; done = 1; next } { print }' "$log" >"$log.new"
  mv "$log.new" "$log"
  out="$(check_copy "$dir")"
  printf '%s\n' "$out" | grep -F "DIVERGENCIA caso=$tcase" >&2 || true
  grep -Fq "DIVERGENCIA caso=$tcase: trecho esperado 2 vez(es), encontrado 1: exit_code=3" <<<"$out" ||
    { printf '%s\n' "$out" >&2; fail mutation-survived; }
  printf '%s/MUT-001/red\n' "$card"
}

ac_005() {
  current=AC-005; staged
  local log="$run_dir/ac005.log"
  grep -Fq '"GOWORK=off"' "$module/internal/scanner/govet/engine.go" || fail gowork-not-fixed
  go_test "$module" "$log" 'TestParentWorkspaceDoesNotReachVet|TestGoChildHasWorkspaceOff|TestCgoFileInTouchedPackageIsInconclusive|TestCgoFileInUntouchedPackageIsIgnored' ./internal/scanner/govet ||
    { cat "$log" >&2; fail go-test; }
  require_pass "$log" TestParentWorkspaceDoesNotReachVet TestGoChildHasWorkspaceOff TestCgoFileInTouchedPackageIsInconclusive TestCgoFileInUntouchedPackageIsIgnored
  printf '%s/AC-005/pass\n' "$card"
}

mut_002() {
  current=MUT-002
  local root="$run_dir/mut002" log="$run_dir/mut002.log" engine
  stage "$root"
  engine="$root/internal/scanner/govet/engine.go"
  grep -Fq ', "GOWORK=off"}' "$engine" || infra mutation-anchor-missing
  sed -i 's/, "GOWORK=off"}/}/' "$engine"
  if go_test "$root" "$log" 'TestParentWorkspaceDoesNotReachVet' ./internal/scanner/govet; then
    cat "$log" >&2; fail mutation-survived
  fi
  grep -Fq -- '--- FAIL: TestParentWorkspaceDoesNotReachVet' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -F 'want only calc/calc.go:9' "$log" >&2 || true
  printf '%s/MUT-002/red\n' "$card"
}

ac_006() {
  current=AC-006
  local c sel
  for c in AUR-491 AUR-448 AUR-458; do
    sel=all; [[ "$c" == AUR-491 ]] || sel=AC-001
    ( cd "$repo_root" && bash "tests/acceptance/$c.sh" "$sel" ) >"$run_dir/$c.log" 2>&1 ||
      { tail -n 3 "$run_dir/$c.log" >&2; fail "blocked:$c:$(tail -n 1 "$run_dir/$c.log")"; }
  done
  printf '%s/AC-006/pass\n' "$card"
}

case "$selector" in
  AC-001) ac_001 ;;
  AC-002) ac_002 ;;
  AC-003) ac_003 ;;
  AC-004) ac_004 ;;
  AC-005) ac_005 ;;
  AC-006) ac_006 ;;
  MUT-001) mut_001 ;;
  MUT-002) mut_002 ;;
  all)
    ac_001; ac_002; ac_004; ac_005; mut_001; mut_002; ac_003; ac_006
    current=all
    printf '%s/all/pass\n' "$card"
    ;;
esac
