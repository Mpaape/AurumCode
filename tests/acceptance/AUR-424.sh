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
  out="$(cd "$root" && AURUMCODE_ROOT="$root" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -v -vet=off -p 1 -count=1 "$pkg" -run '^TestAUR424Bridge$' 2>&1)"
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
  (cd "$root" && bash tests/e2e/AUR-424.sh E2EAUR424) || fail selector:E2EAUR424
}

case "$selector" in
  AC-001)
    go_lane ./tests/unit
    go_lane ./tests/integration
    e2e_case
    printf '{"card":"%s","scenario":"%s","result":"pass","external_tool_calls":0}\n' "$card" "$scenario"
    ;;
  TestAUR424) go_lane ./tests/unit ;;
  IntegrationAUR424) go_lane ./tests/integration ;;
  E2EAUR424) e2e_case ;;
esac
