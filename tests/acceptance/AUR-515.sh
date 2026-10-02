#!/usr/bin/env bash
# AUR-515 acceptance: the --pr path's codebase-context pass only reads the
# local checkout when it is verified -- read-only, via git -- as the exact
# repository named by --repo, at the exact commit the GitHub API reports as
# the pull request's head. A mismatch (different repository, divergent
# HEAD, or simply being unable to tell) omits the codebase context and
# declares the omission as a published review limitation; the remote diff
# review itself keeps running unaffected.
#
# Selectors:
#   all             run every behavior test below
#   AC-001          a checkout of a different repository never reaches the
#                   provider prompt as context
#   AC-002          a checkout matching repo+HEAD uses context; a checkout
#                   at a divergent HEAD omits it
#   AC-003          the remote diff review (and the omission limitation)
#                   still reach the published review when context is omitted
#   AC-001-MUT-001  skip the target verification (always trust the local
#                   checkout); AC-001 must fail (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-515'
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
# A git binary is NOT required: the checkout-identity read
# (cmd/aurumcode/aur515.go) walks the .git layout and HEAD itself (via
# internal/analyzer, the same dual-path reader the rest of this codebase
# already relies on in a sealed, network-denied, git-less profile).

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/pr.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur515.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur515_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a515.XXXXXX")" || infra mktemp
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

# MUT-001: "always trust the local checkout" -- the target verification
# call's result is discarded and replaced with the empty string (no
# mismatch, ever), so codebaseContextMismatch's actual answer never gates
# resolveCodebaseContext. AC-001 (a different repository's checkout) must
# then fail: its marker reaches the provider prompt. Anchored on the stable
# call-assignment line; the token is split so this file cannot match its
# own edit, and a missing anchor is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/cmd/aurumcode/pr.go"
  local anchor='mismatch := codebaseContextMismatch(ctx, client, owner, '"repoName, prNumber)"
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor}|mismatch := \"\"|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
pkgs='./cmd/aurumcode/...'
case "$selector" in
  all)            test_pattern='^TestAUR515' ;;
  AC-001)         test_pattern='^TestAUR515DifferentRepoOmitsContextFromPrompt$' ;;
  AC-002)         test_pattern='^TestAUR515MatchingHeadUsesContext$|^TestAUR515DivergentHeadOmitsContextButKeepsDiffReview$' ;;
  AC-003)         test_pattern='^TestAUR515DivergentHeadOmitsContextButKeepsDiffReview$' ;;
  AC-001-MUT-001) test_pattern='^TestAUR515DifferentRepoOmitsContextFromPrompt$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR515' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR515' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in DifferentRepoOmitsContextFromPrompt MatchingHeadUsesContext DivergentHeadOmitsContextButKeepsDiffReview; do
    grep -q "^--- PASS: TestAUR515$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
