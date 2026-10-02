#!/usr/bin/env bash
# AUR-536 acceptance: closes AUR-515's own review follow-up. A verified
# repository and HEAD (AUR-515) are not enough -- the local checkout's
# working tree must also match its committed content, or the codebase
# context pass must omit context and declare why. This also proves the
# three "unverifiable" branches codebaseContextMismatch already carried
# (no local git metadata, no configured "origin", a failed pull request
# metadata fetch) but AUR-515 never actually drove through the real --pr
# path.
#
# Selectors:
#   all             run every AUR-536 behavior test below
#   AC-001          an untracked file on an otherwise verified, clean
#                   checkout never reaches the provider prompt as context;
#                   the omission is published
#   AC-002          three checkouts whose identity cannot be confirmed at
#                   all (no ".git", no "origin" remote, a failed PR
#                   metadata fetch) all omit codebase context with the
#                   published, reason-specific "unverifiable" wording
#   AC-002-MUT-001  treat codebaseContextReasonUnverifiable as verified
#                   (the const collapses to ""); all three AC-002 cases
#                   must fail (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-536'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
# A git binary is NOT required: verifiedCleanCheckoutReason (aur536.go)
# falls back to a pure-Go loose-object check, the same dual-path shape
# AUR-515's own identity check (aur515.go, via internal/analyzer) already
# uses in a sealed, network-denied, git-less profile.

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/pr.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur515.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur536.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur536_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a536.XXXXXX")" || infra mktemp
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

# AC-002-MUT-001: "treat inverificavel as verified" -- codebaseContextMismatch
# (aur515.go) returns the codebaseContextReasonUnverifiable constant from all
# three of its "cannot tell" branches, and verifiedCleanCheckoutReason
# (aur536.go) returns the same constant from its own "cannot tell" branch.
# Collapsing the constant's VALUE to "" turns every one of those returns
# into "verified, no mismatch" at once, without touching any call site, so
# this is a single, stable, token-split anchor on the declaration line
# itself (split so this script's own source cannot match its own edit).
apply_mutation() {
  local target="$run_dir/root/cmd/aurumcode/aur515.go"
  local anchor='const codebaseContextReasonUnverifiable = "unverif'
  anchor="${anchor}iable\""
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor}|const codebaseContextReasonUnverifiable = \"\"|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
pkgs='./cmd/aurumcode/...'
case "$selector" in
  all)            test_pattern='^TestAUR536' ;;
  AC-001)         test_pattern='^TestAUR536UntrackedFileOmitsContextFromPrompt$' ;;
  AC-002)         test_pattern='^TestAUR536NoGitMetadataOmitsContext$|^TestAUR536NoOriginRemoteOmitsContext$|^TestAUR536PullRequestMetadataFailureOmitsContext$' ;;
  AC-002-MUT-001) test_pattern='^TestAUR536NoGitMetadataOmitsContext$|^TestAUR536NoOriginRemoteOmitsContext$|^TestAUR536PullRequestMetadataFailureOmitsContext$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  for name in TestAUR536NoGitMetadataOmitsContext TestAUR536NoOriginRemoteOmitsContext TestAUR536PullRequestMetadataFailureOmitsContext; do
    grep -Eq -- "^--- FAIL: ${name} " "$log" || fail "mutation-survived:${name}"
  done
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR536' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in UntrackedFileOmitsContextFromPrompt NoGitMetadataOmitsContext NoOriginRemoteOmitsContext PullRequestMetadataFailureOmitsContext; do
    grep -q "^--- PASS: TestAUR536$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
