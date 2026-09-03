#!/usr/bin/env bash
#
# E2E program for card AUR-483, selector E2EAUR483.
#
# WHAT THIS PROVES
#
#   Runs the real cmd/regenerate-docs CLI -- the binary the publish workflow
#   (action.yml) actually invokes to build https://mpaape.github.io/AurumCode/
#   -- against the committed fixture tree tests/fixtures/docs/AUR-483: four
#   product Go packages (internal/attestation, cmd/latest, internal/core
#   with both service.go and contest.go) and test/fixture scope
#   (tests/sample.go, testdata/gen.go, fixtures/data.go,
#   internal/core/service_test.go). It inspects both the markdown pages the
#   binary writes to disk and its own stderr log, end to end through the
#   real CLI entrypoint rather than internal/pipeline's Go API
#   (tests/integration/AUR-483.go's job).
#
#   cmd/regenerate-docs is env-var driven (AURUMCODE_-prefixed, no flags of
#   its own for this card's knob). AURUMCODE_INCLUDE_TEST_DOCS is read
#   directly by internal/pipeline itself, not wired through this binary's
#   own flag/env parsing (cmd/regenerate-docs is read_paths for this card,
#   not paths: it is not touched) -- proving the capability reaches this
#   entrypoint anyway.
#
#   cmd/aurumcode's own `docs` subcommand is deliberately NOT used here: it
#   discards internal/pipeline's log output entirely
#   (log.SetOutput(io.Discard) in cmd/aurumcode/docs.go) and reports its own
#   curated summary instead, which would hide AC-001's scope declaration.
#   cmd/regenerate-docs lets that log line reach stderr untouched.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure
set -Eeuo pipefail
export LC_ALL=C

selector="${1:-E2EAUR483}"
case "$selector" in
  E2EAUR483) ;;
  *) printf 'AUR-483/E2E/unknown-selector\n' >&2; exit 64 ;;
esac

fail() { printf 'AUR-483/E2E/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-483/E2E/infrastructure/%s\n' "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

fixture="$repo_root/tests/fixtures/docs/AUR-483"
[[ -d "$fixture" ]] || fail 'fixture-missing'
[[ -d "$fixture/internal/attestation" ]] || fail 'fixture-missing:internal/attestation'
[[ -d "$fixture/cmd/latest" ]] || fail 'fixture-missing:cmd/latest'

bin="${AURUMCODE_BIN:-}"
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-483.XXXXXX")" || infra mktemp
cleanup() { chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM HUP

if [[ -z "$bin" ]]; then
  command -v go >/dev/null 2>&1 || infra missing_go
  bin="$run_dir/regenerate-docs"
  ( cd "$repo_root" && go build -o "$bin" ./cmd/regenerate-docs ) >"$run_dir/build.log" 2>&1 || {
    cat "$run_dir/build.log" >&2
    infra build_failed
  }
fi
[[ -x "$bin" ]] || infra "binary-missing:$bin"

default_out="$run_dir/out-default"
opt_in_out="$run_dir/out-optin"

run_regen() {
  local outdir="$1"; shift
  env -u LLM_API_KEY -u LLM_BASE_URL -u OPENAI_API_KEY \
    AURUMCODE_SOURCE_DIR="$fixture" \
    AURUMCODE_OUTPUT_DIR="$outdir" \
    AURUMCODE_DOCS_DIR="$outdir" \
    AURUMCODE_VALIDATE_JEKYLL=false \
    AURUMCODE_DEPLOY_GH_PAGES=false \
    AURUMCODE_DOCS_REVIEW=off \
    "$@" "$bin"
}

# Default run: test/fixture scope excluded.
run_regen "$default_out" env -u AURUMCODE_INCLUDE_TEST_DOCS \
  >"$run_dir/default.out" 2>"$run_dir/default.err" || fail 'default-run-failed'

for page in internal__attestation.md cmd__latest.md internal__core.md; do
  [[ -f "$default_out/go/$page" ]] || fail "default-run-missing-product-page:$page"
done
for page in tests.md testdata.md fixtures.md; do
  [[ -f "$default_out/go/$page" ]] && fail "default-run-must-exclude-test-page:$page"
done
# index.md and reference.md are scaffold pages, not content extracted from
# source: reference.md carries the API enumeration AUR-484 moved out of
# index.md's body, and appears only with two or more pages. Counting either
# would measure the scaffold, not the scope filter this asserts.
page_count="$(find "$default_out" -name '*.md' ! -name 'index.md' ! -name 'reference.md' | wc -l | tr -d ' ')"
[[ "$page_count" -eq 3 ]] || fail "default-run-page-count:$page_count"

# AC-001: the run DECLARES how many files it excluded by scope, on stderr.
grep -Eq 'scope: excluded [0-9]+ file\(s\) as test or fixture scope' "$run_dir/default.err" \
  || fail 'scope-declaration-missing'

# AC-003 content check: the product page for the substring trap must carry
# the product symbol, proving it was really documented, not merely present
# as an empty stub.
grep -q 'NewReport' "$default_out/go/internal__attestation.md" 2>/dev/null || fail 'attestation-content-missing'
grep -q 'Version' "$default_out/go/cmd__latest.md" 2>/dev/null || fail 'latest-content-missing'

# Opt-in via the AURUMCODE_-prefixed environment override: capability must
# not have been removed. testdata.md is deliberately not asserted here --
# see tests/integration/AUR-483.go's restorablePages comment: the Go
# extractor's own, pre-existing, unrelated skip list always excludes a
# "testdata" directory regardless of this card's flag.
run_regen "$opt_in_out" env AURUMCODE_INCLUDE_TEST_DOCS=true \
  >"$run_dir/optin.out" 2>"$run_dir/optin.err" || fail 'opt-in-run-failed'

for page in internal__attestation.md cmd__latest.md internal__core.md tests.md fixtures.md; do
  [[ -f "$opt_in_out/go/$page" ]] || fail "opt-in-run-missing-page:$page"
done
grep -Fq 'scope: test/fixture exclusion disabled by config' "$run_dir/optin.err" \
  || fail 'opt-in-scope-declaration-missing'

printf 'AUR-483/E2E/ok\n'
