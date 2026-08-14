#!/usr/bin/env bash
# AUR-424 AC-001: running the standard-library Go documentation extractor over
# a real Go project produces a real page per package, with the project's real
# functions, types and doc comments, and never depends on an external tool.
#
# WHY THIS SCRIPT STAGES A SCRATCH COPY INSTEAD OF RUNNING IN PLACE
#   `.board/bin/oci-run` executes this file inside a container whose rootfs is
#   read-only and whose only writable storage is a bounded tmpfs at /tmp; only
#   this card's `paths`/`read_paths` are materialized under the workspace
#   root. `go test`/`go build` need a writable module cache and build cache,
#   so every input this script needs is copied into a private, writable
#   directory under /tmp before any Go command runs. This mirrors the
#   technique already established by tests/acceptance/AUR-422.sh.
#
# WHY THIS SCRIPT DOES NOT RUN `go run ./cmd/regenerate-docs` DIRECTLY
#   AUR-424's `read_paths` materializes cmd/regenerate-docs itself plus four
#   specific sibling files this card's fix depends on. It does not, and was
#   never asked to, materialize cmd/regenerate-docs's own further transitive
#   local-package dependencies (internal/pipeline, internal/llm and its
#   provider packages, and the sibling language extractors main.go also
#   registers) -- those belong to other offices' cards. Compiling the full
#   `cmd/regenerate-docs` binary is therefore infrastructure this card's
#   allowlist cannot provide, regardless of this card's own fix; see
#   docs/specs/AUR-424.md for the measured, reproducible detail. What this
#   card owns, and what actually contains the whole fix per its own
#   Acceptance note, is internal/documentation/extractors/go: this script
#   proves that package's public contract directly, through the same
#   extractors.Registry wiring cmd/regenerate-docs/main.go uses to reach it,
#   against the card's own checked-in fixture.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-424 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR424|IntegrationAUR424|E2EAUR424) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

inputs=(
  go.mod go.sum
  internal/documentation/extractors/types.go
  internal/documentation/extractors/errors.go
  internal/documentation/extractors/registry.go
  internal/documentation/extractors/go/extractor.go
  internal/documentation/extractors/go/extractor_test.go
  tests/fixtures/docs/goproject/go.mod
  tests/fixtures/docs/goproject/doc.go
  tests/fixtures/docs/goproject/greeting.go
  tests/fixtures/docs/goproject/mathutil.go
  tests/unit/AUR-424.go
  tests/integration/AUR-424.go
  tests/e2e/AUR-424.sh
  cmd/regenerate-docs/main.go
)
for p in "${inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

# The regression lane compiles cmd/regenerate-docs and its dependencies, which
# need gopkg.in/yaml.v3 from an already-populated module cache: GOPROXY stays
# off because this acceptance is offline. Resolved before HOME is redirected,
# since HOME is what the default location derives from.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a424.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"
for p in "${inputs[@]}"; do
  mkdir -p "$root/$(dirname "$p")"
  cp "$repo_root/$p" "$root/$p"
done
# tests/fixtures/docs/goproject carries its own go.mod (a separate module) so
# it is never swept into this repository's own `go build ./...`/`go test
# ./...`; that also means it needs no further staging here.

printf 'package unit\nimport "testing"\nfunc TestAUR424Bridge(t *testing.T){ TestAUR424(t) }\n' \
  >"$root/tests/unit/aur424_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR424Bridge(t *testing.T){ IntegrationAUR424(t) }\n' \
  >"$root/tests/integration/aur424_bridge_test.go"

go_lane() {
  # One offline invocation compiles and executes the requested package's real
  # assertion against the materialized documents.
  local pkg="$1" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root" && AURUMCODE_ROOT="$root" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR424Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    detail="$(grep -om1 "$card/$scenario/[A-Za-z0-9/_-]*" <<<"$out" | head -n1 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    fail "selector-exit:$pkg:$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || fail "zero-tests:$pkg"
  ! grep -Fq '\[no test files\]' <<<"$out" || fail "no-test-files:$pkg"
  grep -Fq -- '--- PASS: TestAUR424Bridge' <<<"$out" || fail "selector-did-not-run:$pkg"
}

e2e_case() {
  # Run against the repository itself, not the staged copy: since this card's
  # E2E executes `go run ./cmd/regenerate-docs` for real, it needs the whole
  # binary, not the handful of files staged above.
  #
  # The E2E's own exit status is propagated rather than flattened. It exits 69
  # when the binary's dependencies are not materialized, and the card's Review
  # clause requires that to read as blocked/inconclusive; collapsing it into
  # this script's `fail` (exit 1) would report missing inputs as a failed
  # behaviour, which is the opposite verdict.
  local rc
  set +e
  (cd "$repo_root" && bash tests/e2e/AUR-424.sh E2EAUR424)
  rc=$?
  set -e
  (( rc == 0 )) && return 0
  (( rc == 69 || rc == 64 )) && exit "$rc"
  fail "selector:E2EAUR424:$rc"
}

# regression_lane is the answer to how the first version of this file reported
# {"result":"pass"} over a repository whose test suite was red.
#
# This card's fix changes a contract that pre-existing tests outside its own
# `paths` assert: the Go extractor's Validate no longer reports a missing tool,
# it writes its own output instead of a subprocess doing it, and the smoke test
# that pinned "no tool on PATH means no documentation" no longer describes Go.
# Five such tests broke, in three packages the two lanes above never compile.
# An acceptance that only runs the card's own selectors cannot see any of it.
#
# SCOPE, AND WHY IT IS NOT `./internal/...`
#   `go test ./internal/...` cannot be used as written: `internal/evidence`'s
#   test imports tests/integration, which holds `package main` files
#   (tests/integration/AUR-001.go, AUR-002.go, AUR-004.go) alongside `package
#   integration` ones, so that package fails to build. This is pre-existing and
#   unrelated to AUR-424 -- it reproduces identically at ec0495c, this card's
#   parent commit -- and is recorded in docs/specs/AUR-424.md rather than
#   silently swallowed here. The package list below is where every changed
#   contract lives: internal/documentation holds the extractors, internal/
#   pipeline owns the skip classification this card changed the meaning of
#   (SkipToolUnavailable, "produced no documentation"), cmd/regenerate-docs is
#   the call site that constructs the Go extractor, and tests/e2e plus
#   tests/characterization hold the pre-existing contracts it broke.
regression_lane() {
  # The lane's inputs are the whole tree, which is far more than this card's
  # paths/read_paths materialize; when they are absent the honest answer is a
  # missing input, never a pass.
  local probe
  for probe in \
    internal/documentation/extractors/tool_unavailable_test.go \
    internal/documentation/extractors/tool_failure_test.go \
    internal/documentation/extractors/output_confirmed_test.go \
    tests/e2e/smoke_test.go \
    tests/characterization/legacy/documentation/characterization.go
  do
    [[ -f "$repo_root/$probe" ]] || infra "regression_lane_not_materialized:$probe"
  done

  local out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$repo_root" && \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
    go test -timeout 300s -count=1 -p 1 \
      ./internal/documentation/... ./internal/pipeline/... ./cmd/... \
      ./tests/e2e/... ./tests/characterization/... 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "regression:$rc"

  # A lane that compiled nothing proves nothing.
  grep -q '^ok ' <<<"$out" || fail 'regression:zero-packages'
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit
    go_lane ./tests/integration
    e2e_case
    regression_lane
    printf '{"card":"%s","scenario":"%s","result":"pass","external_tool_calls":0,"regression_lane":"internal/documentation,internal/pipeline,cmd,tests/e2e,tests/characterization"}\n' "$card" "$scenario"
    ;;
  TestAUR424) go_lane ./tests/unit ;;
  IntegrationAUR424) go_lane ./tests/integration ;;
  E2EAUR424) e2e_case ;;
esac
