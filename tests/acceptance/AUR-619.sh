#!/usr/bin/env bash
# AUR-619 acceptance: a publication GitHub refuses by permission (401/403)
# is diagnosed with the permission the token lacks (pull-requests: write for
# the parecer, inline comments and formal reviews; statuses: write for
# commit statuses), where to grant it and GitHub's own answer, in pt-BR and
# en; the consumer QA's fix fixture carries a changelog entry and the
# verifier names the checks that failed when a run after the fix is not a
# success.
#
# Selectors:
#   all      AC-001..AC-003, MUT-001, MUT-002
#   AC-001   parecer, inline comment and formal review refused: the stderr
#            names pull-requests: write and where to grant it, keeps the
#            HTTP 403 answer, in both languages; other errors unchanged
#   AC-002   commit statuses (review and policy gate) refused: statuses:
#            write named, answer kept; 422 is not a permission denial
#   AC-003   bug-sql-corrigido adds a changelog entry under Unreleased; the
#            verifier lists the failed checks after the fix
#   MUT-001  printing the raw error without the permission turns AC-001 red
#   MUT-002  removing the fixture's changelog entry turns AC-003 red
# Exit: 0 pass, 1 behavioral failure, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-619'
selector="${1:-all}"
known='all AC-001 AC-002 AC-003 MUT-001 MUT-002'
if [[ " $known " != *" $selector "* ]]; then
  printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
  exit 64
fi

fail() {
  printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2
  exit 1
}
infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2
  exit 79
}

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for tool in sed grep; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_$tool"
done
: "${GOCACHE:=$(mktemp -d)}"
export GOCACHE

work="$(mktemp -d)"
trap 'chmod -R u+w -- "$work" 2>/dev/null || true; rm -rf -- "$work"' EXIT

readonly product_pkgs=(./cmd/aurumcode ./internal/git/githubclient ./internal/i18n)
readonly consumer_pkgs=(./tests/consumer)
readonly comment_tests='^(TestAUR619PermissionDeniedNamesThePermission|TestAUR619PublishingDeniedReachesTheOperator|TestAUR619APIErrorKeepsTheHTTPLine|TestAUR619PermissionDeniedStatuses|TestEveryKeyExistsInEveryLocale)$'
readonly status_tests='^(TestAUR619PublishingDeniedReachesTheOperator|TestAUR619ClientAnswersAreTyped)$'
readonly consumer_tests='^(TestAUR619CorrectionFixtureCarriesAChangelogEntry|TestAUR619FailedChecksAreNamed)$'

# stage copies the module (cmd, internal, pkg and the consumer QA) into a
# fresh root, so a mutation never touches the repository.
stage() {
  local root="$1" item
  mkdir -p "$root/tests"
  for item in go.mod go.sum cmd internal pkg tests/consumer; do
    [[ -e "$repo_root/$item" ]] || infra "missing-$item"
    cp -R "$repo_root/$item" "$root/$item"
  done
  chmod -R u+w -- "$root"
}

# run_test runs the tests matching pattern in the packages given, in root.
run_test() {
  local root="$1" pattern="$2" log="$3"
  shift 3
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$@") >"$log" 2>&1
}

base="$work/base"
stage "$base"

# ac id pattern pkgs...: the tests must pass, and every named test must have
# run (a selector that matches nothing is not a proof).
ac() {
  local id="$1" pattern="$2" log="$work/$1.log"
  shift 2
  if ! run_test "$base" "$pattern" "$log" "$@"; then
    tail -n 40 "$log" >&2
    fail "test-failed"
  fi
  grep -q -- '^--- PASS: TestAUR619' "$log" || fail "no-aur619-test-ran"
  printf '%s/%s/pass\n' "$card" "$id"
}

# mutant id stage-dir pattern pkgs...: the mutated copy must fail by an
# assertion of a test that ran, never by a build that did not.
mutant() {
  local id="$1" root="$2" pattern="$3" log="$work/$1.log"
  shift 3
  if run_test "$root" "$pattern" "$log" "$@"; then
    fail "mutation-survived"
  fi
  grep -q -- '^--- FAIL: TestAUR619' "$log" || { tail -n 40 "$log" >&2; fail "mutation-did-not-run-the-test"; }
  printf '%s/%s/killed\n' "$card" "$id"
}

run_ac_001() { ac AC-001 "$comment_tests" "${product_pkgs[@]}"; }
run_ac_002() { ac AC-002 "$status_tests" "${product_pkgs[@]}"; }
run_ac_003() { ac AC-003 "$consumer_tests" "${consumer_pkgs[@]}"; }

run_mut_001() {
  local root="$work/mut1" file
  stage "$root"
  file="$root/cmd/aurumcode/publish_permission.go"
  grep -q 'if !githubclient.IsPermissionDenied(err) {' "$file" || infra "mutation-anchor-missing"
  sed -i 's/if !githubclient.IsPermissionDenied(err) {/if githubclient.IsPermissionDenied(err) || true {/' "$file"
  mutant MUT-001 "$root" "$comment_tests" "${product_pkgs[@]}"
}

run_mut_002() {
  local root="$work/mut2"
  stage "$root"
  cp "$root/tests/consumer/fixtures/base/CHANGELOG.md" "$root/tests/consumer/fixtures/bug-sql-corrigido/CHANGELOG.md"
  mutant MUT-002 "$root" "$consumer_tests" "${consumer_pkgs[@]}"
}

case "$selector" in
  all)
    run_ac_001
    run_ac_002
    run_ac_003
    run_mut_001
    run_mut_002
    printf '%s/all/pass\n' "$card"
    ;;
  AC-001) run_ac_001 ;;
  AC-002) run_ac_002 ;;
  AC-003) run_ac_003 ;;
  MUT-001) run_mut_001 ;;
  MUT-002) run_mut_002 ;;
esac
