#!/usr/bin/env bash
# AUR-425 AC-001: when a run of cmd/regenerate-docs produces zero
# documentation pages, the command states the reason and exits 1 instead of
# the historical silent exit 0.
#
# LANES
#   unit         tests/unit/AUR-425.go::TestAUR425 - the smallest observable
#                seam: unsupported-only project, exit 1, stated reason.
#   integration  tests/integration/AUR-425.go::IntegrationAUR425 - the config
#                boundary: an all-excluding AURUMCODE_LANGUAGES filter fails
#                with the filter named; the same project without the filter
#                still documents and exits 0.
#   e2e          tests/e2e/AUR-425.sh::E2EAUR425 - the declared command run
#                twice: exit 1, reason, determinism, canary, positive control.
#   regression   go test ./cmd/regenerate-docs - the package this card
#                changed, including its pre-existing verdict/exit-code suite.
#
# WHY EVERY LANE NEEDS THE FULL COMPILE CLOSURE
#   The promised behaviour is a raw exit status of the real binary, and the
#   card's MUT-001 (going back to exit 0 with zero pages) is only observable
#   by executing that binary; a static grep would pass under the mutation.
#   Compiling cmd/regenerate-docs requires its local import closure
#   (internal/documentation/..., internal/llm/..., internal/pipeline) plus
#   yaml.v3 from an offline module cache. Absent inputs are reported as
#   infrastructure (exit 69, blocked/inconclusive per the card's Review
#   clause), never as a pass and never as a behavioural failure.
#
# The unit/integration lanes are staged into a scratch tree, as established by
# tests/acceptance/AUR-424.sh: tests/unit and tests/integration in the full
# repository mix other cards' files (including package main sources), so the
# lane compiles exactly this card's selector plus a generated bridge.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-425 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR425|IntegrationAUR425|E2EAUR425) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# This card's own artifacts.
own_inputs=(
  cmd/regenerate-docs/main.go
  cmd/regenerate-docs/main_test.go
  tests/unit/AUR-425.go
  tests/integration/AUR-425.go
  tests/e2e/AUR-425.sh
)
for p in "${own_inputs[@]}"; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

# The compile closure of cmd/regenerate-docs (go list -deps, local packages).
# These are read inputs, not this card's write set; when the runner did not
# materialize them the accept is blocked, not failed and not passed.
closure=(
  go.mod go.sum
  internal/documentation/extractors
  internal/documentation/extractors/bash
  internal/documentation/extractors/cpp
  internal/documentation/extractors/csharp
  internal/documentation/extractors/go
  internal/documentation/extractors/javascript
  internal/documentation/extractors/powershell
  internal/documentation/extractors/python
  internal/documentation/extractors/rust
  internal/documentation/incremental
  internal/documentation/normalizer
  internal/documentation/site internal/documentation/review
  internal/documentation/welcome internal/documentation/review
  internal/llm
  internal/llm/cost
  internal/llm/httpbase
  internal/llm/provider/litellm
  internal/llm/provider/openai
  internal/pipeline
)
for p in "${closure[@]}"; do
  [[ -e "$repo_root/$p" ]] || infra "closure_not_materialized:$p"
done

# Offline module cache holding yaml.v3; resolved BEFORE HOME is redirected,
# since the default location derives from HOME.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a425.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

# One warm build cache for every lane, so the closure compiles once.
export GOCACHE="$run_dir/cache"

# stage_lane <package-dir> <selector-file> <selector-func>
# Copies the module skeleton plus exactly this card's selector into a scratch
# tree and generates the bridge _test file that makes the selector runnable.
stage_lane() {
  local pkg_dir="$1" selector_file="$2" selector_func="$3"
  local root="$run_dir/stage-${pkg_dir##*/}"
  mkdir -p "$root/$pkg_dir"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$root/"
  cp "$repo_root/$selector_file" "$root/$selector_file"
  printf 'package %s\nimport "testing"\nfunc TestAUR425Bridge(t *testing.T){ %s(t) }\n' \
    "${pkg_dir##*/}" "$selector_func" >"$root/$pkg_dir/aur425_bridge_test.go"
  printf '%s' "$root"
}

# go_lane <staged-root> <package-dir>
# Compiles and executes the staged selector. AURUMCODE_ROOT points the test
# at the real repository so it builds the real binary under test.
go_lane() {
  local root="$1" pkg_dir="$2" out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$root" && AURUMCODE_ROOT="$repo_root" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$GOCACHE" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
    go test -timeout 300s -v -vet=off -p 1 -count=1 "./$pkg_dir" -run '^TestAUR425Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    # Every distinct card token the lane emitted is surfaced on stderr, so a
    # mutation run registers its AUR-425/AC-001/MUT-001 marker alongside the
    # behaviour token instead of hiding it behind the first match.
    detail="$(grep -o "$card/$scenario/[A-Za-z0-9/_:.-]*" <<<"$out" | sort -u | head -n 4 || true)"
    [[ -z "$detail" ]] || printf '%s\n' "$detail" >&2
    grep -q "$card/$scenario/infrastructure/" <<<"$out" && infra "selector:$pkg_dir"
    fail "selector-exit:$pkg_dir:$rc"
  fi
  # A lane that compiled or selected nothing proves nothing.
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == 1 )) || fail "zero-tests:$pkg_dir"
  ! grep -Fq '[no test files]' <<<"$out" || fail "no-test-files:$pkg_dir"
  grep -Fq -- '--- PASS: TestAUR425Bridge' <<<"$out" || fail "selector-did-not-run:$pkg_dir"
}

unit_lane() {
  local root; root="$(stage_lane tests/unit tests/unit/AUR-425.go TestAUR425)"
  go_lane "$root" tests/unit
}

integration_lane() {
  local root; root="$(stage_lane tests/integration tests/integration/AUR-425.go IntegrationAUR425)"
  go_lane "$root" tests/integration
}

# The E2E's own exit status is propagated rather than flattened: 69 means its
# inputs were not materialized (blocked/inconclusive), 64 a selector misuse;
# collapsing either into exit 1 would report missing infrastructure as a
# failed behaviour, the opposite verdict.
e2e_case() {
  local rc
  set +e
  (cd "$repo_root" && GOCACHE="$GOCACHE" bash tests/e2e/AUR-425.sh E2EAUR425)
  rc=$?
  set -e
  (( rc == 0 )) && return 0
  (( rc == 69 || rc == 64 )) && exit "$rc"
  fail "selector:E2EAUR425:$rc"
}

# The package this card changed, with its pre-existing suite: the verdict
# table, the raw-exit subprocess test, budget and repo-code-execution guards.
regression_lane() {
  local out rc
  set +e
  out="$( ulimit -v 8388608
    cd "$repo_root" && \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$GOCACHE" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache" \
    go test -timeout 300s -count=1 -p 1 ./cmd/regenerate-docs 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "regression:$rc"
  grep -q '^ok ' <<<"$out" || fail 'regression:zero-packages'
}

case "$selector" in
  AC-001)
    # Behaviour lanes run before the regression lane on purpose: under the
    # card's MUT-001 both would fail, but only the behaviour lanes emit the
    # AUR-425/AC-001/MUT-001 marker the card requires the accept to register.
    unit_lane
    integration_lane
    e2e_case
    regression_lane
    printf '{"card":"%s","scenario":"%s","result":"pass","command":"cmd/regenerate-docs over a project with no supported source","empty_run_exit":1,"lanes":"unit,integration,e2e,regression"}\n' \
      "$card" "$scenario"
    ;;
  TestAUR425) unit_lane ;;
  IntegrationAUR425) integration_lane ;;
  E2EAUR425) e2e_case ;;
esac
