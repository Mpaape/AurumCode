#!/usr/bin/env bash
# AUR-440 AC-001: the user copies .github/workflows/examples/code-review.yml
# into their repository, and from then on every pull request is reviewed
# automatically and receives the AurumCode comment, with nobody running a
# command.
#
# SCOPE, PER THE CARD'S PRECONDITIONS
#   This acceptance runs inside `bootstrap-readonly-v1`: no network, so it can
#   neither open a real pull request nor watch a live comment land. What THIS
#   card accepts is STATIC verification of the copied manifest, via real YAML
#   parses (yaml.v3 node trees, never grep):
#     - the workflow triggers natively on pull_request -- GitHub Actions'own
#       event, authenticated by the Action's own token; no self-hosted
#       webhook server and no HMAC exist in this architecture;
#     - the review binary is built from <owner/repo per go.mod> at a
#       publishable semver tag (`vN[.N[.N]]`), never a local commit SHA and
#       never a mutable branch, and no step delegates to the root composite
#       action (action.yml proves it is the documentation generator);
#     - the publishing job's permissions manifest declares
#       `pull-requests: write` (and stays `contents: read` everywhere else).
#   Publishing the real `v1` tag is a human action documented in
#   docs/specs/AUR-440.md; this acceptance never pretends to prove a live
#   publication.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY (same technique as AUR-422/424/428)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. `go test` needs a
#   writable build cache, so the Go lanes copy their exact inputs into a
#   private module under /tmp and bridge the exported selector functions into
#   `_test.go` files there.
#
# MUT-001
#   Remove the publishing job's `pull-requests: write` permission and every
#   lane below fails non-zero surfacing the AUR-440/AC-001/MUT-001 label on
#   stderr; restoring the workflow reproduces the exact GREEN.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-440 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR440|IntegrationAUR440|E2EAUR440) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Inputs the lanes cannot run without. The example workflow and
# docs/specs/AUR-440.md are NOT in this list on purpose: their absence or
# their pre-fix content is the card's RED (a behavior gap the lanes and the
# checks below report as behavior-missing), never a missing entrypoint.
inputs=(
  go.mod go.sum
  action.yml
  tests/unit/AUR-440.go
  tests/integration/AUR-440.go
  tests/e2e/AUR-440.sh
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a440.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"
for p in "${inputs[@]}"; do
  mkdir -p "$root/$(dirname "$p")"
  cp "$repo_root/$p" "$root/$p"
done
# Behavior-checked artifact: staged when present so the lanes judge it;
# absent means the lanes report the RED label instead of cp aborting here.
if [[ -f "$repo_root/.github/workflows/examples/code-review.yml" ]]; then
  mkdir -p "$root/.github/workflows/examples"
  cp "$repo_root/.github/workflows/examples/code-review.yml" \
    "$root/.github/workflows/examples/code-review.yml"
fi

printf 'package unit\nimport "testing"\nfunc TestAUR440Bridge(t *testing.T){ TestAUR440(t) }\n' \
  >"$root/tests/unit/aur440_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR440Bridge(t *testing.T){ IntegrationAUR440(t) }\n' \
  >"$root/tests/integration/aur440_bridge_test.go"

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
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR440Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_:-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    fail "selector-exit:$pkg:$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || fail "zero-tests:$pkg"
  ! grep -Fq '[no test files]' <<<"$out" || fail "no-test-files:$pkg"
  grep -Fq -- '--- PASS: TestAUR440Bridge' <<<"$out" || fail "selector-did-not-run:$pkg"
}

e2e_case() {
  # The E2E copies the example into a scratch user repository and validates
  # THE COPY; it runs against the repository itself, which materializes every
  # file it touches. Its own exit codes are propagated: 69/64 must keep
  # reading as infrastructure/usage, never as a failed behavior.
  local rc
  set +e
  (cd "$repo_root" && AURUM_A440_GOCACHE="$run_dir/cache" bash tests/e2e/AUR-440.sh E2EAUR440)
  rc=$?
  set -e
  (( rc == 0 )) && return 0
  (( rc == 69 || rc == 64 )) && exit "$rc"
  fail "selector:E2EAUR440:$rc"
}

spec_case() {
  # The card's Documentation clause: the spec that records the copy, the
  # permissions, the exit codes and -- critically -- that publishing the real
  # `v1` tag is a human action outside this hermetic acceptance.
  [[ -s "$repo_root/docs/specs/AUR-440.md" ]] || fail 'behavior-missing:docs/specs/AUR-440.md absent or empty'
  grep -Fq 'v1' "$repo_root/docs/specs/AUR-440.md" || fail 'behavior-missing:spec never names the v1 tag'
  grep -Fq 'pull-requests: write' "$repo_root/docs/specs/AUR-440.md" || fail 'behavior-missing:spec never documents the pull-requests: write permission'
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit
    go_lane ./tests/integration
    e2e_case
    spec_case
    printf '{"card":"%s","scenario":"%s","result":"pass","verification":"static-manifest","live_publication":"human-action-out-of-scope"}\n' "$card" "$scenario"
    ;;
  TestAUR440) go_lane ./tests/unit ;;
  IntegrationAUR440) go_lane ./tests/integration ;;
  E2EAUR440) e2e_case ;;
esac
