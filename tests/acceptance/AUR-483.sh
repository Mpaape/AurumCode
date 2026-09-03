#!/usr/bin/env bash
#
# Acceptance program for card AUR-483: "A documentacao gerada e do produto,
# nao dos testes dele."
#
# WHAT THIS PROVES
#
#   Measured on this repository: of the 613 code files the generator scans,
#   489 (79%) are test or fixture. internal/pipeline's shouldSkipPath
#   (extractor_pipeline.go) skipped node_modules, .git, .github, vendor,
#   target, dist, build, _site, .taskmaster and .aurumcode -- never tests/,
#   testdata/ or fixtures/. This program proves the fix: by default, a run
#   excludes tests/, testdata/, fixtures/ directory components and *_test
#   file stems (AC-001), declares how many files it excluded by scope
#   (AC-001), still documents them when explicitly configured to via
#   AURUMCODE_INCLUDE_TEST_DOCS (AC-002), and never swallows product code
#   whose path merely CONTAINS the substring "test" -- internal/attestation,
#   cmd/latest, a file named contest.go -- because matching is by whole path
#   component / whole filename stem, never by substring (AC-003).
#
# WHY NO GIT, NO PYTHON3
#
#   The sealed acceptance image (bootstrap-readonly-v1) carries only go and
#   bash. This card's fixture (tests/fixtures/docs/AUR-483) is a plain,
#   committed directory tree -- no bare git repo needed, unlike sibling
#   cards whose acceptance exercises `aurumcode review`'s git integration.
#   The two mutations below edit a staged, writable copy of
#   internal/pipeline/extractor_pipeline.go via awk/ENVIRON substring
#   replace (tests/acceptance/AUR-450.sh's technique), never sed/python3.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure: never valid red evidence, never a pass.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-483'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR483|IntegrationAUR483|E2EAUR483|AC-001-MUT-001|AC-003-MUT-002) ;;
  *) printf '%s/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

fail() { printf '%s/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Input preflight. Deliverables this card owns fail behavioral (their
# absence IS the missing behavior); everything else is an environment gap.
owned_inputs=(
  tests/unit/AUR-483.go
  tests/integration/AUR-483.go
  tests/e2e/AUR-483.sh
  tests/fixtures/docs/AUR-483/internal/attestation/report.go
  tests/fixtures/docs/AUR-483/cmd/latest/main.go
  tests/fixtures/docs/AUR-483/internal/core/service.go
  tests/fixtures/docs/AUR-483/internal/core/contest.go
  tests/fixtures/docs/AUR-483/internal/core/service_test.go
  tests/fixtures/docs/AUR-483/tests/sample.go
  tests/fixtures/docs/AUR-483/testdata/gen.go
  tests/fixtures/docs/AUR-483/fixtures/data.go
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done

required_inputs=(
  go.mod go.sum
  cmd/aurumcode
  cmd/regenerate-docs
  internal/pipeline
  internal/documentation/extractors
  internal/documentation/incremental
  internal/documentation/normalizer
  internal/documentation/review
  internal/documentation/site
  internal/documentation/welcome
  internal/analyzer
  internal/config
  internal/git/githubclient
  internal/llm
  internal/prompt
  internal/review
  internal/security/redaction
  pkg/types
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a483.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"
export GOMAXPROCS=1

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs:
# this card's own paths plus the read-only packages the binary imports.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode cmd/regenerate-docs internal/pipeline internal/documentation/extractors
  copy "$root" internal/documentation/incremental internal/documentation/normalizer internal/documentation/review internal/documentation/site internal/documentation/welcome
  copy "$root" internal/analyzer internal/config internal/git/githubclient internal/llm internal/prompt internal/review internal/security
  copy "$root" pkg/types
  copy "$root" tests/fixtures/docs
  chmod -R u+w -- "$root"
}

# build_shared builds the binary exactly once per acceptance run and reuses
# it for the nominal, unit-bridge, integration-bridge and e2e cases.
# cmd/regenerate-docs, not cmd/aurumcode's own `docs` subcommand: the latter
# discards internal/pipeline's log output (log.SetOutput(io.Discard) in
# cmd/aurumcode/docs.go) and reports its own curated summary instead, which
# would hide AC-001's scope declaration. cmd/regenerate-docs -- the binary
# the publish workflow (action.yml) actually invokes -- lets that log line
# reach stderr untouched.
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/regenerate-docs"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/regenerate-docs) >"$log" 2>&1; then
    cat "$log" >&2
    infra build_failed
  fi
  shared_built=1
}

run_regen() {
  local bin="$1" fixture="$2" outdir="$3"; shift 3
  env -u LLM_API_KEY -u LLM_BASE_URL -u OPENAI_API_KEY \
    AURUMCODE_SOURCE_DIR="$fixture" \
    AURUMCODE_OUTPUT_DIR="$outdir" \
    AURUMCODE_DOCS_DIR="$outdir" \
    AURUMCODE_VALIDATE_JEKYLL=false \
    AURUMCODE_DEPLOY_GH_PAGES=false \
    AURUMCODE_DOCS_REVIEW=off \
    "$@" "$bin"
}

nominal_case() {
  build_shared
  local fixture="$shared_root/tests/fixtures/docs/AUR-483"
  local out_default out_optin
  out_default="$run_dir/nominal-default"
  out_optin="$run_dir/nominal-optin"

  run_regen "$shared_bin" "$fixture" "$out_default" env -u AURUMCODE_INCLUDE_TEST_DOCS \
    >"$run_dir/nominal-default.out" 2>"$run_dir/nominal-default.err" || fail nominal-default-run-failed

  local page
  for page in internal__attestation.md cmd__latest.md internal__core.md; do
    [[ -f "$out_default/go/$page" ]] || fail "nominal-default-missing-product-page:$page"
  done
  for page in tests.md testdata.md fixtures.md; do
    [[ -f "$out_default/go/$page" ]] && fail "nominal-default-must-exclude:$page"
  done
  local count
  count="$(find "$out_default" -name '*.md' ! -name 'index.md' | wc -l | tr -d ' ')"
  [[ "$count" -eq 3 ]] || fail "nominal-default-page-count:$count"

  # AC-001: the run DECLARES how many files it excluded by scope.
  grep -Eq 'scope: excluded [0-9]+ file\(s\) as test or fixture scope' "$run_dir/nominal-default.err" \
    || fail nominal-scope-declaration-missing

  run_regen "$shared_bin" "$fixture" "$out_optin" env AURUMCODE_INCLUDE_TEST_DOCS=true \
    >"$run_dir/nominal-optin.out" 2>"$run_dir/nominal-optin.err" || fail nominal-optin-run-failed
  for page in tests.md fixtures.md; do
    [[ -f "$out_optin/go/$page" ]] || fail "nominal-optin-missing:$page"
  done
  grep -Fq 'scope: test/fixture exclusion disabled by config' "$run_dir/nominal-optin.err" \
    || fail nominal-optin-scope-declaration-missing
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-483.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur483_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR483UnitBridge(t *testing.T) { TestAUR483(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR483UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR483:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR483:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-483.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur483_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR483IntegrationBridge(t *testing.T) { IntegrationAUR483(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR483IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR483:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR483:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-483.sh
  chmod -R u+w -- "$root"
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-483.sh E2EAUR483) || fail e2e-failed
  cleanup_root "$root"
}

# mut1_case is MUT-001: "remover a exclusao de tests do filtro padrao".
# Neutralizes the scope check in the full-extraction walk so it never fires,
# rebuilds, and proves the default run goes back to documenting test/fixture
# pages -- the exact pre-AUR-483 regression this card exists to prevent.
mut1_case() {
  build_shared

  local root="$run_dir/root-mut1"
  stage_source "$root"
  local target="$root/internal/pipeline/extractor_pipeline.go"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor='if !p.IncludeTests() && !p.config.Incremental {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  local replacement='if false && !p.config.Incremental { // MUT-001: default exclusion neutralized'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    {
      idx = index($0, anchor)
      if (idx > 0) {
        print substr($0, 1, idx - 1) repl substr($0, idx + length(anchor))
      } else {
        print $0
      }
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target"
  grep -Fq 'MUT-001: default exclusion neutralized' "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/regenerate-docs-mut1"
  local log="$root/build-mut1.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/regenerate-docs) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local fixture="$root/tests/fixtures/docs/AUR-483"
  local out="$run_dir/mut1-out"
  run_regen "$bin" "$fixture" "$out" >"$run_dir/mut1.out" 2>"$run_dir/mut1.err" || fail 'MUT-001/run-failed'

  # The mutant must survive being caught: with the default exclusion
  # neutralized, tests.md reappears in the DEFAULT run. If it does not, the
  # mutation did not exercise the code path this card's proof depends on.
  if [[ ! -f "$out/go/tests.md" ]]; then
    fail 'MUT-001/mutation-did-not-restore-test-page'
  fi

  cleanup_root "$root"
  printf '%s/AC-001/MUT-001/rejected\n' "$card"
}

# mut2_case is MUT-002: "fazer o filtro casar por substring e engolir
# internal/attestation". Rewrites IsTestScopePath to short-circuit on any
# path merely CONTAINING "test", rebuilds, and proves internal/attestation
# and cmd/latest -- both containing the substring "test" but neither a
# tests/testdata/fixtures path component -- vanish from a default run: the
# exact AC-003 trap this card's own component matching must reject.
mut2_case() {
  build_shared

  local root="$run_dir/root-mut2"
  stage_source "$root"
  local target="$root/internal/pipeline/extractor_pipeline.go"
  [[ -f "$target" ]] || fail 'MUT-002/target-missing'
  local anchor='func IsTestScopePath(path string) bool {'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-002/anchor-not-unique'
  local replacement='func IsTestScopePath(path string) bool { if strings.Contains(strings.ToLower(path), "test") { return true } // MUT-002: substring match'
  ANCHOR="$anchor" REPL="$replacement" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    {
      idx = index($0, anchor)
      if (idx > 0) {
        print substr($0, 1, idx - 1) repl substr($0, idx + length(anchor))
      } else {
        print $0
      }
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target"
  grep -Fq 'MUT-002: substring match' "$target" || fail 'MUT-002/mutation-not-applied'

  local bin="$run_dir/regenerate-docs-mut2"
  local log="$root/build-mut2.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/regenerate-docs) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-002/build-failed'
  fi

  local fixture="$root/tests/fixtures/docs/AUR-483"
  local out="$run_dir/mut2-out"
  run_regen "$bin" "$fixture" "$out" >"$run_dir/mut2.out" 2>"$run_dir/mut2.err" || fail 'MUT-002/run-failed'

  # The mutant must swallow product code: internal/attestation and
  # cmd/latest, each containing the substring "test", must vanish under
  # the substring-matching mutant.
  if [[ -f "$out/go/internal__attestation.md" ]]; then
    fail 'MUT-002/mutation-did-not-swallow-attestation'
  fi
  if [[ -f "$out/go/cmd__latest.md" ]]; then
    fail 'MUT-002/mutation-did-not-swallow-latest'
  fi

  cleanup_root "$root"
  printf '%s/AC-003/MUT-002/rejected\n' "$card"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mut1_case
  mut2_case
  cleanup_root "$shared_root"
  printf '%s/AC-001/ok\n' "$card"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR483) unit_case ;;
  IntegrationAUR483) integration_case ;;
  E2EAUR483) e2e_case ;;
  AC-001-MUT-001) mut1_case ;;
  AC-003-MUT-002) mut2_case ;;
esac
