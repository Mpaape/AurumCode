#!/usr/bin/env bash
# AUR-513 acceptance: the per-file review cache key must change whenever
# anything that can change the model's answer changes -- the selected
# profile(s), the content of every configured skill/doc (repo and central
# policy), the dynamic rule/skill-section catalog, and (already true before
# this card) the review language, codebase context and memory notes (AC-001,
# AC-002); a per-file cache hit must never drop the cross-file evidence a
# changed sibling file still needs (AC-003); and a corrupted or unreadable
# cache must degrade to a fresh review, never a silent approval or omitted
# coverage (AC-004). See docs/review-cache.md for the full account.
#
# Selectors:
#   all             run every behavior test below
#   AC-001          a different profile selection on the same diff forces a
#                   fresh model call; the identical selection may reuse it
#   AC-002          editing a configured doc/skill file invalidates the
#                   cache; the key itself never carries that text
#   AC-003          a cache hit for one file still carries its symbols into
#                   a changed sibling file's prompt via the codebase-context
#                   pack
#   AC-004          a corrupted cache entry, or a cache directory that
#                   cannot be opened at all, degrades to a fresh review
#   AC-001-MUT-001  drop the profile identity from the cache key; AC-001
#                   must FAIL (RED) -- this is MUT-001: a different profile
#                   selection must never be read as the same cache identity
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-513'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/review_cache.go" ]] || fail missing-source
[[ -f "$repo_root/cmd/aurumcode/main.go" ]] || fail missing-source
[[ -f "$repo_root/cmd/aurumcode/aur513_test.go" ]] || fail missing-behavior-test
[[ -f "$repo_root/docs/review-cache.md" ]] || fail missing-doc

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a513.XXXXXX")" || infra mktemp
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

# MUT-001: drop the profile identity from the review cache key by forcing
# every selected profile's own signature (cmd/aurumcode/main.go) to the
# same constant empty string, regardless of which profile(s) were
# actually selected. A single-profile selection then joins to the exact
# same empty identity as the zero-profile case, so a review under one
# profile and a review of the identical diff under no profile collide on
# the same cache entry -- exactly the bypass AC-001 exists to refuse: a
# different profile selection must never be read as the same cache
# identity. Anchored on the stable append call so this file cannot match
# its own edit, and a missing anchor is infrastructure, never a silent
# no-op.
apply_mutation() {
  local target="$run_dir/root/cmd/aurumcode/main.go"
  local anchor='profileSigs = append(profileSigs, p.Signature())'
  local count
  count="$(grep -Fc -- "$anchor" "$target")" || infra mutation-anchor-missing
  (( count == 1 )) || infra mutation-anchor-not-unique
  local replacement='profileSigs = append(profileSigs, p.Name[:0])'
  # Fixed-string sed via a bracket-expression-free substitution: grep -F
  # already proved the anchor is a literal, unique match, and the anchor
  # has no "|" byte, so "|" is a safe sed delimiter here.
  sed -i "s|${anchor}|${replacement}|" "$target"
  grep -Fq -- "$anchor" "$target" && infra mutation-not-applied
  grep -Fq -- "$replacement" "$target" || infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
pkgs='./cmd/aurumcode/...'
case "$selector" in
  all)            test_pattern='^TestAUR513' ;;
  AC-001)         test_pattern='^TestAUR513AC001ProfileSelectionChangeForcesFreshReview$' ;;
  AC-002)         test_pattern='^TestAUR513AC002SkillContentChangeForcesFreshReview$' ;;
  AC-003)         test_pattern='^TestAUR513AC003CrossFileEvidenceSurvivesPartialHit$' ;;
  AC-004)         test_pattern='^(TestAUR513AC004CorruptCacheEntryDegradesToFreshReview|TestAUR513AC004UnreadableCacheDirDegradesToFreshReview)$' ;;
  AC-001-MUT-001) test_pattern='^TestAUR513AC001ProfileSelectionChangeForcesFreshReview$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR513' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR513' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in \
      AC001ProfileSelectionChangeForcesFreshReview AC002SkillContentChangeForcesFreshReview \
      AC003CrossFileEvidenceSurvivesPartialHit AC004CorruptCacheEntryDegradesToFreshReview \
      AC004UnreadableCacheDirDegradesToFreshReview; do
    grep -q "^--- PASS: TestAUR513$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
