#!/usr/bin/env bash
# AUR-446 AC-001: cada spec de card entregue descreve o comportamento
# entregue, nao o estado do dia em que foi escrito.
#
# SCOPE, PER THE CARD'S PRECONDITIONS
#   This acceptance runs inside `bootstrap-readonly-v1`: no network, no model
#   call. What THIS card accepts is STATIC verification -- literal `grep`
#   (whitespace-normalized) over the five corrected specs, plus two Go lanes
#   that prove the same anchors against real content and corroborate the
#   underlying facts against real read_paths sources
#   (cmd/aurumcode, internal/git/githubclient,
#   .github/workflows/examples/code-review.yml).
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428/440)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test` needs a
#   writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp and bridge the exported selector functions
#   into `_test.go` files there.
#
# NAMING NOTE
#   `.board/cards/ready/AUR-446.md`'s "TDD proof" clause cites the selectors
#   `TestAUR428`/`IntegrationAUR428`/`E2EAUR428` -- a copy-paste residue from
#   the AUR-428 template (see docs/specs/AUR-446.md, "Nota de nomenclatura").
#   This script accepts both the card's literal names and the repo-wide
#   convention (`TestAUR446`/`IntegrationAUR446`/`E2EAUR446`) as aliases of
#   the same selector, so either invocation runs the real check.
#
# MUT-001
#   Reintroducing "No `aurumcode docs` subcommand exists:" into
#   docs/specs/AUR-425.md or docs/specs/AUR-429.md makes every lane below
#   fail non-zero, surfacing AUR-446/AC-001/MUT-001 on stderr; restoring the
#   correction reproduces the exact GREEN.
#
# KNOWN GAP, REPORTED RATHER THAN WORKED AROUND
#   docs/specs/AUR-428.md also needed a correction (the auditoria's fifth
#   finding) but is in neither this card's `paths` nor `read_paths`
#   (.board/cards/ready/AUR-446.md); see docs/specs/AUR-446.md, "Gap
#   medido". Nothing in this script reads or asserts on it.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-446 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

case "$selector" in
  AC-001|TestAUR446|IntegrationAUR446|E2EAUR446|TestAUR428|IntegrationAUR428|E2EAUR428) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
# tests/e2e/AUR-446.sh's anchor checks pipe every spec file through `tr` to
# normalize whitespace; checked here too so its absence is reported as this
# script's own infrastructure gap, not swallowed as a false RED inside
# e2e_case's `set -euo pipefail`.
command -v tr >/dev/null 2>&1 || infra missing_tr

# Files this card's own paths declare. Their absence is this card's own
# behaviour failing to exist, never infrastructure.
own_inputs=(
  go.mod go.sum
  docs/specs/AUR-424.md
  docs/specs/AUR-425.md
  docs/specs/AUR-429.md
  docs/specs/AUR-437.md
  docs/specs/AUR-440.md
  docs/specs/AUR-446.md
  tests/unit/AUR-446.go
  tests/integration/AUR-446.go
  tests/e2e/AUR-446.sh
)
for p in "${own_inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

# read_paths this card depends on but does not own: their absence is an
# environment/materialization gap, per EXIT_CODE_CONVENTION.md.
read_dirs=(cmd/aurumcode internal/git/githubclient)
for d in "${read_dirs[@]}"; do
  [[ -d "$repo_root/$d" ]] || infra "read_path_not_materialized:$d"
done
[[ -f "$repo_root/.github/workflows/examples/code-review.yml" ]] || infra "read_path_not_materialized:.github/workflows/examples/code-review.yml"

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a446.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

for p in "${own_inputs[@]}"; do
  mkdir -p "$root/$(dirname "$p")"
  cp "$repo_root/$p" "$root/$p"
done
for d in "${read_dirs[@]}"; do
  mkdir -p "$root/$d"
  cp -R "$repo_root/$d/." "$root/$d/"
done
mkdir -p "$root/.github/workflows/examples"
cp "$repo_root/.github/workflows/examples/code-review.yml" \
  "$root/.github/workflows/examples/code-review.yml"

printf 'package unit\nimport "testing"\nfunc TestAUR446Bridge(t *testing.T){ TestAUR446(t) }\n' \
  >"$root/tests/unit/aur446_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR446Bridge(t *testing.T){ IntegrationAUR446(t) }\n' \
  >"$root/tests/integration/aur446_bridge_test.go"

go_lane() {
  # One offline, bounded invocation compiles and executes the requested
  # package's real assertions against the staged artifacts.
  local pkg="$1" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root" && AURUMCODE_ROOT="$root" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR446Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:.-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    fail "selector-exit:$pkg:$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || fail "zero-tests:$pkg"
  ! grep -Fq '[no test files]' <<<"$out" || fail "no-test-files:$pkg"
  grep -Fq -- '--- PASS: TestAUR446Bridge' <<<"$out" || fail "selector-did-not-run:$pkg"
}

e2e_case() {
  # tests/e2e/AUR-446.sh runs directly against repo_root (the real files,
  # not the scratch copy): the "grep de verificacao estatica" AC-001
  # promises must run against the genuine deliverables, not a staged
  # simulacrum. Its own exit codes are propagated: 79/64 must keep reading
  # as infrastructure/usage, never as a failed behaviour.
  local rc
  set +e
  (cd "$repo_root" && bash tests/e2e/AUR-446.sh E2EAUR446)
  rc=$?
  set -e
  (( rc == 0 )) && return 0
  (( rc == 79 || rc == 64 )) && exit "$rc"
  fail "selector:E2EAUR446:$rc"
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit
    go_lane ./tests/integration
    e2e_case
    printf '{"card":"%s","scenario":"%s","result":"pass","specs_checked":5,"anchors_verified":9}\n' "$card" "$scenario"
    ;;
  TestAUR446|TestAUR428) go_lane ./tests/unit ;;
  IntegrationAUR446|IntegrationAUR428) go_lane ./tests/integration ;;
  E2EAUR446|E2EAUR428) e2e_case ;;
esac
