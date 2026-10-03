#!/usr/bin/env bash
#
# Acceptance program for card AUR-585: retired legacy with preserved evidence.
#
# Selectors:
#   all             AC-001..AC-004, then MUT-001 and MUT-002
#   AC-001          go mod tidy leaves go.mod/go.sum untouched, build and vet are
#                   green, and no `package main` lives under internal/
#   AC-002          tests/legacy packages pass and the done-card acceptances that
#                   use them (AUR-003..006, 471, 566) are green on the new paths
#   AC-003          no acceptance of a cancelled card remains; AUR-309.sh exits 69
#                   with the retirement message
#   AC-004          review/cache has its own unit test (key, invalidation, refusal)
#   MUT-001         the broken legacy/pipeline import comes back: AC-001 goes red
#   MUT-002         AUR-309.sh made to exit 0: AC-003 goes red
#   MUT-003         the cache key ignores the file content: AC-004 goes red
#
# Exit codes: 0 holds, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-585'
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

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a585.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOFLAGS='-mod=mod -buildvcs=false -p=1' GOWORK=off
export TMPDIR="$run_dir"

# Acceptances of cards that were cancelled; none may remain in tests/acceptance.
cancelled_ids=(
  AUR-010 AUR-203 AUR-312 AUR-313 AUR-333 AUR-334 AUR-335 AUR-336 AUR-337 AUR-338
  AUR-339 AUR-340 AUR-351 AUR-352 AUR-353 AUR-354 AUR-355 AUR-356 AUR-357 AUR-358
  AUR-365 AUR-366 AUR-367 AUR-368 AUR-369 AUR-370 AUR-371 AUR-372 AUR-373 AUR-374
  AUR-375 AUR-376 AUR-377 AUR-378 AUR-379 AUR-380 AUR-381 AUR-382 AUR-383 AUR-384
  AUR-385 AUR-386 AUR-387 AUR-388 AUR-389 AUR-390 AUR-391 AUR-423
)
((${#cancelled_ids[@]} == 48)) || infra cancelled_list_corrupt

# Acceptances of done cards whose packages moved to tests/legacy (or stay, as 566).
migrated_acceptances=(AUR-003 AUR-004 AUR-005 AUR-006 AUR-471 AUR-566)

seed_root() { # seed_root DIR
  rm -rf "$1"; mkdir -p "$1"
  local s
  for s in go.mod go.sum cmd internal pkg tests docs demo scripts .board; do
    [[ -e "$repo_root/$s" ]] && cp -R "$repo_root/$s" "$1/$s"
  done
  chmod -R u+w -- "$1"
  [[ -f "$1/go.mod" ]] || infra seed_failed
}

# --- AC-001 ------------------------------------------------------------------
check_tidy() { # check_tidy ROOT: prints a reason and returns 1 when red
  local root="$1" mains
  cp "$root/go.mod" "$run_dir/go.mod.before"; cp "$root/go.sum" "$run_dir/go.sum.before"
  if ! (cd "$root" && go mod tidy) >"$run_dir/tidy.log" 2>&1; then
    printf 'go mod tidy failed: %s\n' "$(sed -n '1,4p' "$run_dir/tidy.log" | tr '\n' ' ')"; return 1
  fi
  cmp -s "$run_dir/go.mod.before" "$root/go.mod" || { echo 'go mod tidy changed go.mod'; return 1; }
  cmp -s "$run_dir/go.sum.before" "$root/go.sum" || { echo 'go mod tidy changed go.sum'; return 1; }
  (cd "$root" && go build ./... && go vet ./...) >"$run_dir/build.log" 2>&1 || { echo "build/vet red: $(sed -n '1,3p' "$run_dir/build.log" | tr '\n' ' ')"; return 1; }
  mains="$(grep -rlE '^package main$' --include='*.go' "$root/internal" 2>/dev/null | sed -n '1,3p' | tr '\n' ' ' || true)"
  [[ -z "$mains" ]] || { echo "package main under internal/: $mains"; return 1; }
}

# --- AC-002 ------------------------------------------------------------------
check_migrated() { # check_migrated ROOT
  local root="$1" id out
  (cd "$root" && go test ./tests/legacy/... ./tests/contracts/sandbox-profile/...) >"$run_dir/legacy.log" 2>&1 \
    || { echo "tests/legacy red: $(tail -n3 "$run_dir/legacy.log" | tr '\n' ' ')"; return 1; }
  for id in "${migrated_acceptances[@]}"; do
    out="$run_dir/acc-$id.log"
    (cd "$root" && bash "tests/acceptance/$id.sh") >"$out" 2>&1 || { echo "$id red: $(tail -n1 "$out" | cut -c1-160)"; return 1; }
  done
}

# --- AC-003 ------------------------------------------------------------------
check_retired() { # check_retired ROOT
  local root="$1" id code
  for id in "${cancelled_ids[@]}"; do
    [[ ! -e "$root/tests/acceptance/$id.sh" ]] || { echo "cancelled acceptance remains: $id"; return 1; }
  done
  code=0; bash "$root/tests/acceptance/AUR-309.sh" >"$run_dir/309.out" 2>&1 || code=$?
  [[ "$code" == 69 ]] || { echo "AUR-309.sh exited $code, want 69"; return 1; }
  grep -q 'retired' "$run_dir/309.out" || { echo 'AUR-309.sh gave no retirement message'; return 1; }
}

# --- AC-004 ------------------------------------------------------------------
check_cache() { # check_cache ROOT
  (cd "$1" && go test -count=1 ./tests/unit/reviewcache) >"$run_dir/cache.log" 2>&1 \
    || { echo "review/cache unit test red: $(grep -m1 -E '^\s*cache_test|FAIL' "$run_dir/cache.log" | cut -c1-160)"; return 1; }
}

expect_green() { # expect_green LABEL FUNC ROOT
  local reason
  reason="$("$2" "$3")" || fail "$1: $reason"
  printf '%s/%s/ok\n' "$card" "$1"
}
expect_red() { # expect_red LABEL FUNC ROOT
  local reason
  if reason="$("$2" "$3")"; then fail "$1: the mutation was not detected"; fi
  printf '%s/%s/ok (%s)\n' "$card" "$1" "$reason"
}

root="$run_dir/root"
run_selector() {
  case "$1" in
    AC-001) seed_root "$root"; expect_green AC-001 check_tidy "$root" ;;
    AC-002) seed_root "$root"; expect_green AC-002 check_migrated "$root" ;;
    AC-003) seed_root "$root"; expect_green AC-003 check_retired "$root" ;;
    AC-004) seed_root "$root"; expect_green AC-004 check_cache "$root" ;;
    MUT-001)
      seed_root "$root"
      mkdir -p "$root/tests/contracts/revived"
      printf 'package revived\n\nimport _ "github.com/Mpaape/AurumCode/tests/characterization/legacy/pipeline"\n' >"$root/tests/contracts/revived/revived.go"
      expect_red MUT-001 check_tidy "$root" ;;
    MUT-002)
      seed_root "$root"
      sed -i 's/^exit 69$/exit 0/' "$root/tests/acceptance/AUR-309.sh"
      expect_red MUT-002 check_retired "$root" ;;
    MUT-003)
      seed_root "$root"
      sed -i 's/^\th.Write(body)$/\t_ = body/' "$root/internal/review/cache/cache.go"
      expect_red MUT-003 check_cache "$root" ;;
  esac
}

if [[ "$selector" == all ]]; then
  for s in AC-001 AC-002 AC-003 AC-004 MUT-001 MUT-002 MUT-003; do selector="$s"; run_selector "$s"; done
  printf '%s/all/pass\n' "$card"
else
  run_selector "$selector"
fi
