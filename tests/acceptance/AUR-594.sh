#!/usr/bin/env bash
# AUR-594 acceptance: a large pull request is reviewed. When the API refuses
# the diff for its size (406 too_large) the same base...head range is read
# from the verified checkout; a diff above one prompt is reviewed in batches
# by directory with one consolidated result and one gate; at a batch ceiling
# the files left out are named and the approval is withheld.
#
# Selectors:
#   all        AC-001..AC-004, then MUT-001..MUT-003
#   AC-001     406 too_large: the verified checkout's diff is reviewed; an
#              unverified checkout fails with nothing sent or published;
#              only the size refusal is typed (another 406 stays an error);
#              without git the range comes from the object database, and
#              with git it matches `git diff base...head` hunk for hunk
#   AC-002     a change above one prompt: two batches, coverage complete,
#              gate pass, the audit lists the batches and their files
#   AC-003     max_batches / max_prompt_tokens reached: partial_coverage,
#              exit 1 under block, the files left out named in the notice
#              and in the audit
#   AC-004     the review tutorial carries the large-PR case (fake GitHub
#              answering 406, local diff, batches) recorded with the
#              product image; the spec records PR #88 reviewed completely
#   MUT-001    treating the 406 as an empty diff turns AC-001 RED
#   MUT-002    not declaring the files a ceiling left out (approval with
#              batches missing) turns AC-003 RED
#   MUT-003    a ceiling that admits no batch consolidating an empty result
#              (the model never called, approved by default) turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-594'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg cmd/aurumcode/aur594_test.go internal/review/batches.go internal/git/githubclient/diff_too_large.go; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a594.XXXXXX")" || infra mktemp
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
    if [[ -e "$repo_root/$source" ]]; then cp -R "$repo_root/$source" "$root/$source"; fi
  done
  if [[ -d "$repo_root/tests/fixtures" ]]; then mkdir -p "$root/tests"; cp -R "$repo_root/tests/fixtures" "$root/tests/fixtures"; fi
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@" ) >"$log" 2>&1
}

replace_once() {
  local file="$1" anchor="$2" replacement="$3" count
  count="$(grep -Fc -- "$anchor" "$file")" || infra "anchor-missing:${file##*/}"
  [[ "$count" == 1 ]] || infra "anchor-not-unique:${file##*/}"
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { a = ENVIRON["ANCHOR"]; r = ENVIRON["REPL"] }
    { i = index($0, a); if (i > 0) { print substr($0, 1, i - 1) r substr($0, i + length(a)) } else { print } }
  ' "$file" >"$file.mut" && mv "$file.mut" "$file"
  grep -Fq -- "$replacement" "$file" || infra "mutation-not-applied:${file##*/}"
}

readonly ac001_cmd=(TestTooLargeDiffIsReadFromObjectCheckout TestTooLargeDiffWithUnverifiedObjectCheckoutFails)
readonly ac001_git_cmd=(TestTooLargeDiffIsReadFromTheVerifiedCheckout TestTooLargeDiffWithUnverifiedCheckoutFails)
readonly ac001_client=(TestDiffRefusedAsTooLargeIsTyped TestParseHunkHeaderWithoutCount)
readonly ac002_tests=(TestDiffAboveTheBudgetIsReviewedInBatches)
readonly ac003_tests=(TestBatchCeilingLeavesFilesOutAndWithholdsApproval TestNoBatchAdmittedIsNeverApproved)

pattern_of() { local IFS='|'; printf '^(%s)$' "$*"; }

run_tests() {
  local name="$1" pkg="$2"; shift 2
  local root="$run_dir/root-$name" log="$run_dir/$name-${pkg//\//_}.log"
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$(pattern_of "$@")" "$pkg" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  local t
  for t in "$@"; do grep -Eq -- "^--- PASS: ${t} " "$log" || { cat "$log" >&2; fail "missing-pass:$t"; }; done
}

expect_red() {
  local root="$1" log="$2" pkg="$3"; shift 3
  if go_test "$root" "$log" "$(pattern_of "$@")" "$pkg"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,3p' >&2
}

run_ac001() {
  run_tests AC-001 ./cmd/aurumcode/ "${ac001_cmd[@]}"
  run_tests AC-001 ./internal/git/githubclient/ "${ac001_client[@]}"
  # With a git binary (the product image has one) the same range also goes
  # through `git diff base...head`, and the object-database reader must
  # match it hunk for hunk. Without git these tests would only skip.
  if command -v git >/dev/null 2>&1; then
    run_tests AC-001 ./cmd/aurumcode/ "${ac001_git_cmd[@]}"
    run_tests AC-001 ./internal/analyzer/ TestRangeDiffFromObjectsMatchesGit
    printf '%s/AC-001/pass (git and object database)\n' "$card"
  else
    printf '%s/AC-001/pass (object database; no git binary here)\n' "$card"
  fi
}

run_ac002() {
  run_tests AC-002 ./cmd/aurumcode/ "${ac002_tests[@]}"
  printf '%s/AC-002/pass\n' "$card"
}

run_ac003() {
  run_tests AC-003 ./cmd/aurumcode/ "${ac003_tests[@]}"
  printf '%s/AC-003/pass\n' "$card"
}

run_ac004() {
  local expected="$repo_root/demo/tutoriais/revisao/expected/pr-workflow.txt"
  local spec="$repo_root/docs/specs/AUR-594.md"
  [[ -f "$expected" ]] || fail no-large-pr-tutorial-case
  grep -Fq 'the API refused the pull request diff as too large; reviewing the same range computed from the verified checkout' "$expected" || fail tutorial-without-local-diff
  grep -Eq 'reviewed in [0-9]+ batches by directory' "$expected" || fail tutorial-without-batches
  [[ -f "$spec" ]] || fail no-spec
  grep -Fq 'PR #88' "$spec" || fail spec-without-pr88
  grep -Fq '"complete": true' "$spec" || fail spec-without-complete-coverage
  printf '%s/AC-004/pass\n' "$card"
}

run_mut001() {
  local root="$run_dir/root-mut1"
  stage "$root"
  replace_once "$root/internal/git/githubclient/client.go" \
    'return nil, fmt.Errorf("HTTP %d: %s: %w", resp.StatusCode, string(body), ErrDiffTooLarge)' \
    'return &Diff{}, nil /* MUT-001 */'
  expect_red "$root" "$run_dir/mut1.log" ./cmd/aurumcode/ TestTooLargeDiffIsReadFromObjectCheckout
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2"
  stage "$root"
  replace_once "$root/internal/review/batch_merge.go" \
    'if len(outside) == 0 {' \
    'if len(outside) >= 0 /* MUT-002 */ {'
  expect_red "$root" "$run_dir/mut2.log" ./cmd/aurumcode/ "${ac003_tests[@]}"
  printf '%s/MUT-002/rejected\n' "$card"
}

run_mut003() {
  local root="$run_dir/root-mut3"
  stage "$root"
  replace_once "$root/internal/review/batches.go" \
    'if len(plan.batches) == 0 {' \
    'if len(plan.batches) < 0 /* MUT-003 */ {'
  expect_red "$root" "$run_dir/mut3.log" ./cmd/aurumcode/ TestNoBatchAdmittedIsNeverApproved
  printf '%s/MUT-003/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac001 ;;
  AC-002) run_ac002 ;;
  AC-003) run_ac003 ;;
  AC-004) run_ac004 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  MUT-003) run_mut003 ;;
  all)
    run_ac001
    run_ac002
    run_ac003
    run_ac004
    run_mut001
    run_mut002
    run_mut003
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
