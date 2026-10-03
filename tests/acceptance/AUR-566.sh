#!/usr/bin/env bash
# AUR-566 acceptance: `aurumcode fix` prints a standard unified diff with three
# lines of real file context, applicable with plain `git apply` and `patch -p1`.
#
# Selectors:
#   all             AC-001, AC-002, AC-003, then the mutation
#   AC-001          the fix output passes `git apply --check` and `git apply`
#                   with no flag in a sample repository, and the file holds
#                   the correction (needs git in the environment)
#   AC-002          hunk with three context lines and correct headers; new file
#                   and removal patches apply (package tests plus git apply)
#   AC-003          the revisao tutorial: run.sh --check passes, the text no
#                   longer mentions --unidiff-zero, getting-started shows the
#                   plain `git apply`
#   AC-001-MUT-001  going back to zero context turns AC-001 red
# Unknown selectors exit 64; infrastructure failures exit 79; failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-566'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

for f in internal/apply/apply.go internal/apply/hunk.go internal/apply/applycheck/applycheck.go cmd/aurumcode/aur566_fix_test.go \
  docs/tutorials/revisao.md docs/getting-started.md demo/tutoriais/revisao/run.sh; do
  [[ -f "$repo_root/$f" ]] || infra "missing:$f"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a566.XXXXXX")" || infra mktemp
trap 'chmod -R u+rwX "${run_dir:?}" >/dev/null 2>&1 || true; rm -rf "${run_dir:?}"' EXIT INT TERM HUP

ac003() {
  bash "$repo_root/demo/tutoriais/revisao/run.sh" --check >"$run_dir/check.out" 2>&1 || { cat "$run_dir/check.out" >&2; fail 'check-falhou'; }
  grep -q '^CHECK OK$' "$run_dir/check.out" || fail 'check-sem-ok'
  if grep -rq -- '--unidiff-zero' "$repo_root/docs/tutorials/revisao.md" "$repo_root/docs/getting-started.md" \
    "$repo_root/demo/tutoriais/revisao/run.sh" "$repo_root/demo/tutoriais/revisao/expected" "$repo_root/demo/tutoriais/revisao/out"; then
    fail 'unidiff-zero-ainda-citado'
  fi
  grep -q '^git apply --check fix.patch$' "$repo_root/docs/tutorials/revisao.md" || fail 'tutorial-sem-git-apply'
  grep -q '^git apply fix.patch$' "$repo_root/docs/getting-started.md" || fail 'getting-started-sem-git-apply'
  grep -q '^git apply --check: o patch aplica$' "$repo_root/demo/tutoriais/revisao/out/fix.log" || fail 'out-sem-prova-do-git-apply'
  grep -q '^@@ -3,6 +3,6 @@$' "$repo_root/demo/tutoriais/revisao/out/fix.log" || fail 'out-sem-contexto'
  printf '%s/AC-003/pass (tutorial --check verde, sem --unidiff-zero)\n' "$card"
}

if [[ "$selector" == AC-003 ]]; then ac003; exit 0; fi

command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
seed_root() {
  rm -rf "$run_dir/root"; mkdir -p "$run_dir/root"
  for s in go.mod go.sum cmd internal pkg; do cp -R "$repo_root/$s" "$run_dir/root/$s"; done
  chmod -R u+w -- "$run_dir/root"
}
seed_root
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1' GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

run_go_test() { # pkg pattern log
  local status
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v "$1" -run "$2") >"$3" 2>&1
  status=$?
  set -e
  cat "$3" >&2
  return $status
}

# The strict applier (internal/apply/applycheck) runs everywhere; the real
# `git apply` test runs, and must pass, wherever a git binary exists.
ac001_name='TestAUR566FixOutputAppliesStrictlyWithoutGit'
ac001_git='TestAUR566FixAppliesWithPlainGitApply'
ac001() {
  local how='aplicador estrito'
  run_go_test ./cmd/aurumcode "^($ac001_name|$ac001_git)\$" "$run_dir/ac001.log" || fail 'go-test-falhou'
  grep -q "^--- PASS: $ac001_name " "$run_dir/ac001.log" || fail 'sem-pass-do-aplicador-estrito'
  if command -v git >/dev/null 2>&1; then
    grep -q "^--- PASS: $ac001_git " "$run_dir/ac001.log" || fail 'sem-pass-do-git-apply'
    how='git apply --check e git apply sem flag'
  fi
  run_go_test ./internal/apply/... '^(TestPatchAppliesWithPlainGitStrictness|TestRejects.*)$' "$run_dir/ac001b.log" || fail 'apply-estrito-falhou'
  printf '%s/AC-001/pass (%s; a correcao esta no arquivo)\n' "$card" "$how"
}

ac002() {
  run_go_test ./internal/apply/... '.' "$run_dir/ac002a.log" || fail 'apply-falhou'
  for n in TestHunkHasThreeLinesOfContext TestNewFileAndRemovalPatches TestMissingNewlineAtEndOfFile; do
    grep -q "^--- PASS: $n " "$run_dir/ac002a.log" || fail "sem-pass:$n"
  done
  grep -q '^--- PASS: TestPatchAppliesWithPlainGitStrictness ' "$run_dir/ac002a.log" || fail 'sem-pass:strict'
  if command -v git >/dev/null 2>&1; then
    run_go_test ./cmd/aurumcode '^(TestAUR566PatchShapeAndOtherTools|TestAUR566NewFileAndRemovalApply)$' "$run_dir/ac002b.log" || fail 'cmd-falhou'
    for n in TestAUR566PatchShapeAndOtherTools TestAUR566NewFileAndRemovalApply; do
      grep -q "^--- PASS: $n " "$run_dir/ac002b.log" || fail "sem-pass:$n"
    done
  fi
  printf '%s/AC-002/pass (contexto de 3, cabecalhos, arquivo novo e remocao aplicam)\n' "$card"
}

mut001() {
  local f="$run_dir/root/internal/apply/apply.go"
  grep -q '^const contextLines = 3$' "$f" || infra mutation-anchor-missing
  sed -i 's/^const contextLines = 3$/const contextLines = 0/' "$f"
  grep -q '^const contextLines = 0$' "$f" || infra mutation-not-applied
  run_go_test ./cmd/aurumcode "^$ac001_name\$" "$run_dir/mut.log" && fail 'mutation-survived'
  grep -q "^--- FAIL: $ac001_name" "$run_dir/mut.log" || fail 'mutation-not-red-by-ac001'
  if grep -Eq 'build failed|undefined:|syntax error' "$run_dir/mut.log"; then fail 'mutation-build-failure-not-behavioral'; fi
  printf '%s/AC-001-MUT-001/pass (contexto zero reprova AC-001)\n' "$card"
}

case "$selector" in
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-001-MUT-001) mut001 ;;
  all) ac001; ac002; ac003; seed_root; mut001 ;;
esac
