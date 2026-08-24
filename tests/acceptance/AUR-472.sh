#!/usr/bin/env bash
#
# Acceptance program for card AUR-472, scenario AC-001.
#
# WHAT THIS PROVES
#
#   A 2026-08-14 adversarial review of AUR-465 found that
#   internal/documentation/welcome/sanitize.go's pinnedTagPattern treated
#   ANY ref shaped like a semver tag (v2, v10, v1.99) as already-compliant,
#   never checking whether that tag actually exists. Not exploitable while
#   only v1/v1.0.0/v1.0.1 are published, but a `v2` branch opened for the
#   next major -- common practice -- would pass through unrewritten and
#   publish exactly the mutable-branch anti-pattern AUR-465 exists to kill.
#   This program proves the fix: SanitizeActionRefTags now accepts a ref
#   only when it is a full commit SHA or an EXACT match against the
#   declared, repository-local set of tags that actually exist; a
#   semver-shaped ref that is not in that set is rewritten and the rewrite
#   is announced; and with no tag list available, every non-SHA ref is
#   rewritten (safe default), also announced.
#
# WHY THE TAG LIST NEEDS NO NEW read_paths INPUT
#
#   The sealed acceptance image has no `git` binary, and the sandbox
#   materializes files by `git ls-files` so `.git` itself never reaches it
#   (also `forbidden_paths`). Consulting the network is a Non-goal. The fix
#   declares `publishedTags` as a Go source-level literal inside
#   sanitize.go itself -- "configuracao declarada" per the card's
#   Non-goals -- so the list travels with the one file already in `paths`
#   and needs nothing new in `read_paths`.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/440/445/465)
#
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test` needs a
#   writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp and bridge the exported selector functions
#   into `_test.go` files there.
#
# MUT-001 (AC-001: shape accepted as value)
#   Reinstating the old pinnedTagPattern-based accept check in a SCRATCH
#   copy must make the unit lane and the e2e lane fail, each reporting
#   AUR-472/AC-001/MUT-001. The tracked sanitize.go is never touched.
#
# MUT-002 (AC-003: no-list lets a ref pass)
#   Neutering the no-list branch so it lets a ref through untouched instead
#   of rewriting it must make the unit lane and the e2e lane fail, each
#   reporting AUR-472/AC-001/MUT-002.
#
# EXIT CODES:
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
#   64 = unknown scenario selector
#   69 = inconclusive / infrastructure
set -euo pipefail
export LC_ALL=C
readonly card=AUR-472 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR472|IntegrationAUR472|E2EAUR472|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

inputs=(
  go.mod go.sum
  internal/documentation/welcome/generator.go
  internal/documentation/welcome/sanitize.go
  internal/documentation/welcome/templates/welcome-page.md
  tests/unit/AUR-472.go
  tests/integration/AUR-472.go
  tests/e2e/AUR-472.sh
  docs/specs/AUR-472.md
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a472.XXXXXX")" || infra mktemp
cleanup() {
  chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true
  rm -rf -- "$run_dir" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  local p
  for p in "${inputs[@]}"; do
    mkdir -p "$dest/$(dirname "$p")"
    cp "$repo_root/$p" "$dest/$p"
  done
  mkdir -p "$dest/internal/llm"
  cp "$repo_root"/internal/llm/*.go "$dest/internal/llm/" 2>/dev/null || true
  mkdir -p "$dest/internal/llm/provider"
  cp -r "$repo_root"/internal/llm/provider/. "$dest/internal/llm/provider/" 2>/dev/null || true
  mkdir -p "$dest/internal/llm/cost"
  cp -r "$repo_root"/internal/llm/cost/. "$dest/internal/llm/cost/" 2>/dev/null || true
}

stage "$root"

printf 'package unit\nimport "testing"\nfunc TestAUR472Bridge(t *testing.T){ TestAUR472(t) }\n' \
  >"$root/tests/unit/aur472_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR472Bridge(t *testing.T){ IntegrationAUR472(t) }\n' \
  >"$root/tests/integration/aur472_bridge_test.go"

go_lane() {
  local pkg="$1" root_override="${2:-$root}" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root_override" && HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR472Bridge$' 2>&1)"
  rc=$?
  set -e
  aur472_last_output="$out"
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    return "$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || { fail "zero-tests:$pkg"; }
  ! grep -Fq '[no test files]' <<<"$out" || { fail "no-test-files:$pkg"; }
  grep -Fq -- '--- PASS: TestAUR472Bridge' <<<"$out" || { fail "selector-did-not-run:$pkg"; }
  return 0
}

e2e_case() {
  local sel="${1:-E2EAUR472}" out rc
  set +e
  out="$(bash "$repo_root/tests/e2e/AUR-472.sh" "$sel" 2>&1)"
  rc=$?
  set -e
  aur472_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

spec_case() {
  [[ -s "$repo_root/docs/specs/AUR-472.md" ]] || fail 'behavior-missing:docs/specs/AUR-472.md absent or empty'
  grep -Fq 'SanitizeActionRefTags' "$repo_root/docs/specs/AUR-472.md" \
    || fail 'behavior-missing:spec never names SanitizeActionRefTags'
  grep -Fq 'AC-003' "$repo_root/docs/specs/AUR-472.md" \
    || fail 'behavior-missing:spec never records AC-003'
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit "$root" || fail 'selector-exit:unit'
    go_lane ./tests/integration "$root" || fail 'selector-exit:integration'
    rc=0; e2e_case E2EAUR472 || rc=$?
    if (( rc != 0 )); then
      (( rc == 69 || rc == 64 )) && exit "$rc"
      fail "selector-exit:e2e:$rc"
    fi
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","checks":["AC-001","AC-002","AC-003"]}\n' "$card" "$scenario"
    ;;
  TestAUR472) go_lane ./tests/unit "$root" ;;
  IntegrationAUR472) go_lane ./tests/integration "$root" ;;
  E2EAUR472) e2e_case E2EAUR472 ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    sed -i.bak \
      's/if !noList \&\& known\[ref\] {/if !noList \&\& (known[ref] || pinnedTagPattern.MatchString(ref)) {/' \
      "$mut_root/internal/documentation/welcome/sanitize.go"
    rm -f "$mut_root/internal/documentation/welcome/sanitize.go.bak"
    printf 'package unit\nimport "testing"\nfunc TestAUR472Bridge(t *testing.T){ TestAUR472(t) }\n' \
      >"$mut_root/tests/unit/aur472_bridge_test.go"

    unit_rc=0; go_lane ./tests/unit "$mut_root" || unit_rc=$?
    (( unit_rc != 0 )) || fail 'MUT-001:unit lane passed on the reverted shape-only guard, expected failure'
    grep -Fq 'v2' <<<"$aur472_last_output" \
      || fail 'MUT-001:unit lane failed but not on the v2 assertion'

    e2e_rc=0; e2e_case MUT-001 || e2e_rc=$?
    (( e2e_rc == 0 )) || fail "MUT-001:e2e lane's own mutation-detection run failed:$e2e_rc"
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur472_last_output" \
      || fail 'MUT-001:e2e lane never reported the MUT-001 label'
    grep -Fq '"result":"detected"' <<<"$aur472_last_output" \
      || fail 'MUT-001:e2e lane never confirmed detection'

    printf '%s/%s/MUT-001 confirmed: unit_rc=%d e2e_rc=%d, tracked sanitize.go untouched\n' "$card" "$scenario" "$unit_rc" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
  MUT-002)
    mut_root="$run_dir/mut-root2"
    stage "$mut_root"
    sed -i.bak \
      's/if !noList \&\& known\[ref\] {/if known[ref] || noList {/' \
      "$mut_root/internal/documentation/welcome/sanitize.go"
    rm -f "$mut_root/internal/documentation/welcome/sanitize.go.bak"
    printf 'package unit\nimport "testing"\nfunc TestAUR472Bridge(t *testing.T){ TestAUR472(t) }\n' \
      >"$mut_root/tests/unit/aur472_bridge_test.go"

    unit_rc=0; go_lane ./tests/unit "$mut_root" || unit_rc=$?
    (( unit_rc != 0 )) || fail 'MUT-002:unit lane passed on the neutered no-list guard, expected failure'
    grep -Fq 'AUR-472/AC-003' <<<"$aur472_last_output" \
      || fail 'MUT-002:unit lane failed but not on the AC-003 no-list assertion'

    e2e_rc=0; e2e_case MUT-002 || e2e_rc=$?
    (( e2e_rc == 0 )) || fail "MUT-002:e2e lane's own mutation-detection run failed:$e2e_rc"
    grep -Fq "$card/$scenario/MUT-002" <<<"$aur472_last_output" \
      || fail 'MUT-002:e2e lane never reported the MUT-002 label'
    grep -Fq '"result":"detected"' <<<"$aur472_last_output" \
      || fail 'MUT-002:e2e lane never confirmed detection'

    printf '%s/%s/MUT-002 confirmed: unit_rc=%d e2e_rc=%d, tracked sanitize.go untouched\n' "$card" "$scenario" "$unit_rc" "$e2e_rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-002","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
