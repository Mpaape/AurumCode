#!/usr/bin/env bash
#
# Acceptance program for card AUR-493.
#
# WHAT THIS PROVES
#
#   AC-001: .github/workflows/review.yml no longer pins `ref: main` on the
#   checkout that builds the AurumCode image. Instead it derives the ref to
#   build from github.workflow_ref (everything after the last "@"), falling
#   back to github.sha when github.workflow_ref is empty (the local-path
#   invocation case). This program extracts that exact shell logic from the
#   YAML file (between the AUR-493-EXTRACT-BEGIN/-END markers), substitutes
#   the two GitHub Actions expressions it references with shell variables
#   this test supplies (a "shim" standing in for the Actions runner's own
#   expression evaluator), and executes the extracted script under `bash -c`
#   with concrete inputs -- so the assertion runs the CANDIDATE's own
#   committed logic, not a reimplementation of it.
#
#   AC-002: none of the three example/consumer workflow files pins @main;
#   the two consumer-facing ones pin @v2; the two published copies of the
#   example are byte-identical.
#
#   AC-003: the repository's own self-review workflow still calls the
#   reusable workflow by local path (the one case where the ref used is the
#   PR's own, not a version pin), and this program proves that in that
#   scenario github.workflow_ref comes back empty and the derivation falls
#   back to github.sha -- exactly the fallback branch documented at
#   review.yml's own extraction site.
#
#   MUT-001/MUT-002: each mutation is applied to a throwaway STAGED COPY of
#   the four workflow files (never the real, committed ones), and the exact
#   same check function AC-001/AC-002 runs is re-invoked, by re-executing
#   this very script (AURUM_A493_REPO_ROOT redirected at the staged copy),
#   against that mutant. A surviving mutant (an unexpected exit 0) is
#   itself this program's failure. Restoration is proven by a fresh
#   invocation against the real, unmutated repository still exiting 0
#   afterward.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
#   64 = unknown scenario selector
#
# Every path this program reads is this card's own declared deliverable per
# its `paths:` list, so absence or mismatch is behavioral RED (exit 1),
# never an infrastructure gap -- there is no `infra()` in this file (see
# EXIT_CODE_CONVENTION.md, "Not yet converted").
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-493'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|AC-002|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
default_repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || fail repo_root
# AURUM_A493_REPO_ROOT lets the MUT-* cases below re-run the very checks
# this file defines against a throwaway staged/mutated copy of the four
# workflow files instead of the real repository -- see "MUT-001/MUT-002"
# above.
repo_root="${AURUM_A493_REPO_ROOT:-$default_repo_root}"

review_yml="$repo_root/.github/workflows/review.yml"
self_review_yml="$repo_root/.github/workflows/code-review.yml"
example_yml="$repo_root/.github/workflows/examples/code-review.yml"
site_yml="$repo_root/docs/site/workflow.yml"

for p in "$review_yml" "$self_review_yml" "$example_yml" "$site_yml"; do
  [[ -f "$p" ]] || fail "deliverable-missing:${p#"$repo_root"/}"
done

# extract_ref_logic pulls the exact shell text committed between the
# AUR-493-EXTRACT-BEGIN/-END markers in review.yml, and rewrites its two
# GitHub Actions expressions (${{ github.workflow_ref }}, ${{ github.sha }})
# into references to shell variables this test controls. This is the "shim"
# the card allows in place of a literal copy: the body executed is read
# fresh from the candidate file every run, only the two runner-supplied
# expressions are substituted.
extract_ref_logic() {
  local body
  body="$(sed -n '/# AUR-493-EXTRACT-BEGIN/,/# AUR-493-EXTRACT-END/p' "$review_yml")"
  [[ -n "$body" ]] || fail 'extraction-markers-absent'
  body="$(printf '%s\n' "$body" | sed '/# AUR-493-EXTRACT-BEGIN/d; /# AUR-493-EXTRACT-END/d')"
  [[ -n "$(printf '%s' "$body" | tr -d '[:space:]')" ]] || fail 'extraction-body-empty'
  grep -Fq '${{ github.workflow_ref }}' <<<"$body" || fail 'extraction-missing-workflow_ref-expr'
  grep -Fq '${{ github.sha }}' <<<"$body" || fail 'extraction-missing-sha-expr'
  body="${body//\$\{\{ github.workflow_ref \}\}/\$AUR493_TEST_WORKFLOW_REF}"
  body="${body//\$\{\{ github.sha \}\}/\$AUR493_TEST_SHA}"
  printf '%s\n' "$body"
}

# run_extraction executes the shimmed body with the given
# workflow_ref/sha inputs and prints the resulting tool_ref. The extracted
# body (between the markers) only assigns the shell variable tool_ref -- the
# real workflow's own `printf 'ref=%s\n' "$tool_ref" >> "$GITHUB_OUTPUT"`
# line lives OUTSIDE the markers on purpose, since $GITHUB_OUTPUT is an
# Actions-runner mechanism this test does not shim -- so this helper appends
# one extra statement that prints the same variable to stdout instead.
run_extraction() {
  local workflow_ref="$1" sha="$2" logic="$3"
  AUR493_TEST_WORKFLOW_REF="$workflow_ref" AUR493_TEST_SHA="$sha" \
    bash -c "${logic}"$'\n''printf '\''%s\n'\'' "$tool_ref"' \
    || fail 'extraction-script-failed'
}

case_ac001() {
  grep -Fq 'ref: main' "$review_yml" && fail 'ref-main-still-present'

  grep -Fq 'ref: ${{ steps.resolve-tool-ref.outputs.ref }}' "$review_yml" \
    || fail 'checkout-does-not-use-derived-ref'

  local logic
  logic="$(extract_ref_logic)"

  local got
  got="$(run_extraction 'owner/repo/.github/workflows/review.yml@refs/tags/v2' 'deadbeef' "$logic")"
  [[ "$got" == 'refs/tags/v2' ]] || fail "tag-ref-extraction-wrong:got=$got"

  got="$(run_extraction 'owner/repo/.github/workflows/review.yml@refs/heads/main' 'deadbeef' "$logic")"
  [[ "$got" == 'refs/heads/main' ]] || fail "branch-ref-extraction-wrong:got=$got"

  got="$(run_extraction '' 'cafef00dcafef00dcafef00dcafef00dcafef00d' "$logic")"
  [[ "$got" == 'cafef00dcafef00dcafef00dcafef00dcafef00d' ]] || fail "empty-fallback-wrong:got=$got"

  printf '%s/%s/ok\n' "$card" "$selector"
}

case_ac002() {
  for f in "$example_yml" "$site_yml" "$self_review_yml"; do
    grep -Fq '@main' "$f" && fail "at-main-present:${f#"$repo_root"/}"
  done
  for f in "$example_yml" "$site_yml"; do
    grep -Fq 'uses: Mpaape/AurumCode/.github/workflows/review.yml@v2' "$f" \
      || fail "at-v2-missing:${f#"$repo_root"/}"
  done
  cmp -s "$example_yml" "$site_yml" || fail 'example-and-site-not-identical'
  printf '%s/%s/ok\n' "$card" "$selector"
}

case_ac003() {
  grep -Fq 'uses: ./.github/workflows/review.yml' "$self_review_yml" \
    || fail 'self-review-not-local-path'

  local logic got
  logic="$(extract_ref_logic)"
  # The local-invocation scenario AC-003 documents: github.workflow_ref is
  # empty (this workflow was not reached through an "@<ref>" pin), so the
  # derivation must fall back to github.sha -- proven with a sha distinct
  # from AC-001's, so this is a fresh execution of the candidate logic, not
  # a cached AC-001 result.
  got="$(run_extraction '' '1111111111111111111111111111111111111111' "$logic")"
  [[ "$got" == '1111111111111111111111111111111111111111' ]] \
    || fail "local-invocation-fallback-wrong:got=$got"
  printf '%s/%s/ok\n' "$card" "$selector"
}

# stage_workflows copies the four real workflow files into a fresh
# directory tree rooted at $1, preserving their relative paths, so a
# mutation can be applied to the copy while the real, committed files are
# never touched.
stage_workflows() {
  local root="$1"
  mkdir -p "$root/.github/workflows/examples" "$root/docs/site"
  cp "$review_yml" "$root/.github/workflows/review.yml"
  cp "$self_review_yml" "$root/.github/workflows/code-review.yml"
  cp "$example_yml" "$root/.github/workflows/examples/code-review.yml"
  cp "$site_yml" "$root/docs/site/workflow.yml"
}

# rerun_selector re-executes THIS SAME SCRIPT with $1 as the selector,
# redirected at repository root $2, and returns its exit code (never lets
# `set -e` abort the caller on a nonzero code, since a nonzero code is
# exactly what a correctly-rejected mutant must produce).
rerun_selector() {
  local sel="$1" root="$2" rc=0
  AURUM_A493_REPO_ROOT="$root" bash "$repo_root_script" "$sel" >/dev/null 2>&1 || rc=$?
  printf '%s\n' "$rc"
}
readonly repo_root_script="$default_repo_root/tests/acceptance/AUR-493.sh"

# case_mut001 is MUT-001 from the card: revert the checkout's ref back to
# the literal `ref: main` in a staged copy, and prove AC-001 (re-run
# against that staged copy) now fails -- then prove AC-001 against the
# real, unmutated repository still passes, so the mutation never leaked.
case_mut001() {
  local root="$run_dir/mut1"
  mkdir -p "$root"
  stage_workflows "$root"
  local target="$root/.github/workflows/review.yml"
  local anchor='ref: ${{ steps.resolve-tool-ref.outputs.ref }}'
  grep -Fq "$anchor" "$target" || fail 'MUT-001/anchor-absent'
  sed -i "s#${anchor//./\\.}#ref: main#" "$target"
  grep -Fq 'ref: main' "$target" || fail 'MUT-001/mutation-not-applied'

  local rc
  rc="$(rerun_selector AC-001 "$root")"
  [[ "$rc" -ne 0 ]] || fail 'MUT-001/mutant-not-detected'

  local restore_rc
  restore_rc="$(rerun_selector AC-001 "$default_repo_root")"
  [[ "$restore_rc" -eq 0 ]] || fail 'MUT-001/restoration-broken'

  printf '%s/%s/rejected\n' "$card" "$selector"
}

# case_mut002 is MUT-002 from the card: revert the published site example's
# pin from @v2 back to @main in a staged copy, and prove AC-002 (re-run
# against that staged copy) now fails -- then prove AC-002 against the
# real, unmutated repository still passes.
case_mut002() {
  local root="$run_dir/mut2"
  mkdir -p "$root"
  stage_workflows "$root"
  local target="$root/docs/site/workflow.yml"
  sed -i 's#@v2#@main#' "$target"
  grep -Fq '@main' "$target" || fail 'MUT-002/mutation-not-applied'

  local rc
  rc="$(rerun_selector AC-002 "$root")"
  [[ "$rc" -ne 0 ]] || fail 'MUT-002/mutant-not-detected'

  local restore_rc
  restore_rc="$(rerun_selector AC-002 "$default_repo_root")"
  [[ "$restore_rc" -eq 0 ]] || fail 'MUT-002/restoration-broken'

  printf '%s/%s/rejected\n' "$card" "$selector"
}

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a493.XXXXXX")" || fail mktemp
cleanup() { rm -rf -- "$run_dir"; }
trap cleanup EXIT INT TERM HUP

case "$selector" in
  AC-001) case_ac001 ;;
  AC-002) case_ac002 ;;
  AC-003) case_ac003 ;;
  MUT-001) case_mut001 ;;
  MUT-002) case_mut002 ;;
esac
