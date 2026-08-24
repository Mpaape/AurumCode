#!/usr/bin/env bash
#
# Acceptance program for card AUR-458, scenario AC-001.
#
# WHAT THIS PROVES
#
#   Exit 0 must mean REVIEWED AND FOUND NOTHING. Before this card the
#   command reported 0 for a run in which the quality review never
#   happened -- with no provider configured, `review --base X --seguranca`
#   ran the deterministic pass alone and exited 0, so a CI job read "green"
#   as "reviewed" when only half the work was done. Worse, when a provider
#   WAS configured and failed (network, 4xx, unparseable answer, --limite
#   refusal), the deterministic security pass -- which calls no model at
#   all -- died with it, so the user lost real, already-computed findings.
#
#   This program proves both halves of the fix: the security pass survives
#   every quality-review failure and still reports its findings, and the
#   exit code says the quality half did not happen. It also proves the
#   published path this card must NOT break: `review --base X --seguranca`
#   with no credential keeps exit 0 and byte-identical stdout, because it
#   reviewed exactly what it was asked to.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-458'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR458|IntegrationAUR458|E2EAUR458|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  tests/unit/AUR-458.go
  tests/integration/AUR-458.go
  tests/e2e/AUR-458.sh
  cmd/aurumcode/notreviewed_test.go
  docs/specs/AUR-458.md
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod go.sum cmd/aurumcode internal/analyzer internal/config internal/llm internal/prompt
  internal/review internal/security/redaction pkg/types
  tests/fixtures/repos/git-demo/repo.git
  tests/fixtures/review/known-problem-response.json
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a458.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir" GOMAXPROCS=1

copy() { local root="$1"; shift; local p; for p in "$@"; do mkdir -p "$root/$(dirname "$p")"; cp -R "$repo_root/$p" "$root/$p"; done; }

stage_source() {
  local root="$1"; mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode cmd/regenerate-docs
  copy "$root" internal/analyzer internal/prompt internal/review internal/security internal/git internal/llm internal/pipeline
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  chmod -R u+w -- "$root"
}

shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2; infra build_failed
  fi
  shared_built=1
}

readonly sec_header='Security findings (standards/security-review):'
readonly clean_line='No issues found.'
# The exact sha256 AUR-449 pins for `--seguranca` stdout WITH the fixture
# provider configured. This card must not move it by one byte.
readonly expected_with_provider_sha256='63c649af1c90e38b473e1bd45b4152b1f96ecad17d5d9c05c17bb94df7b8240f'

noprov_env() { env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL "$@"; }

# nominal_case is AC-001's core behavioral proof.
nominal_case() {
  build_shared
  local demo="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local fixture="$shared_root/tests/fixtures/review/known-problem-response.json"
  local bad="$run_dir/bad.json"; printf 'not json at all {{{' >"$bad"
  local out err rc

  # (1) A provider that FAILS must not take the security pass down, and the
  #     exit code must say the quality half never happened.
  set +e
  out="$(cd "$demo" && noprov_env env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/e1")"
  rc=$?; set -e
  [[ "$rc" -eq 1 ]] || fail "provider-failure-must-exit-1:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail provider-failure-lost-security-pass
  if grep -Fq "$clean_line" <<<"$out"; then fail provider-failure-claimed-clean; fi

  # (2) A response that does not parse: same guarantee.
  set +e
  out="$(cd "$demo" && noprov_env env AURUMCODE_LLM_FIXTURE="$bad" "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/e2")"
  rc=$?; set -e
  [[ "$rc" -eq 1 ]] || fail "unparseable-must-exit-1:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail unparseable-lost-security-pass

  # (3) --limite refusing before the call: same guarantee.
  set +e
  out="$(cd "$demo" && noprov_env env AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca --limite 0.0000001 2>"$run_dir/e3")"
  rc=$?; set -e
  [[ "$rc" -eq 1 ]] || fail "limite-refusal-must-exit-1:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail limite-refusal-lost-security-pass

  # (4) "Did not review" outranks the --fail-on gate: 1, never 3.
  set +e
  (cd "$demo" && noprov_env env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$shared_bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>&1
  rc=$?; set -e
  [[ "$rc" -eq 1 ]] || fail "gate-precedence-must-be-1:$rc"

  # (5) THE PUBLISHED PATH THIS CARD MUST NOT BREAK: no credential at all,
  #     --seguranca alone, exit 0 -- it reviewed exactly what was asked.
  set +e
  out="$(cd "$demo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/e5")"
  rc=$?; set -e
  [[ "$rc" -eq 0 ]] || fail "demo-path-broken-must-exit-0:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail demo-path-lost-findings

  # (6) ...and it becomes non-zero only when the caller opts in.
  set +e
  (cd "$demo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --exigir-qualidade) >/dev/null 2>&1
  rc=$?; set -e
  [[ "$rc" -eq 1 ]] || fail "exigir-qualidade-must-exit-1:$rc"

  # (7) A provider that WORKS keeps AUR-449's pinned stdout byte for byte.
  local sha
  sha="$(cd "$demo" && noprov_env env AURUMCODE_LLM_FIXTURE="$fixture" "$shared_bin" review --base HEAD~1 --seguranca 2>/dev/null | sha256sum | cut -d' ' -f1)"
  [[ "$sha" == "$expected_with_provider_sha256" ]] || fail "with-provider-stdout-changed:$sha"

  # (8) Without --seguranca there is nothing to fall through to: the
  #     published refusal is untouched (empty stdout, exit 1).
  set +e
  out="$(cd "$demo" && noprov_env env AURUMCODE_LLM_FIXTURE="$bad" "$shared_bin" review --base HEAD~1 2>/dev/null)"
  rc=$?; set -e
  [[ "$rc" -eq 1 ]] || fail "no-seguranca-refusal-changed:$rc"
  [[ -z "$out" ]] || fail no-seguranca-unexpected-stdout
}

# mutation_case reintroduces the defect -- "did not review" reports success
# -- and requires the nominal proof to fall.
mutation_case() {
  build_shared
  local mroot="$run_dir/root-mut"
  rm -rf "$mroot"; cp -R "$shared_root" "$mroot"; chmod -R u+w -- "$mroot"
  perl -0pi -e 's/\tif qualityFailed \{\n\t\treturn exitQualityNotReviewed\n\t\}/\tif qualityFailed {\n\t\treturn 0\n\t}/' "$mroot/cmd/aurumcode/main.go" || infra mutation_rewrite
  grep -A2 'if qualityFailed {' "$mroot/cmd/aurumcode/main.go" | grep -q 'return 0' || infra mutation_not_applied
  local mbin="$run_dir/aurumcode-mut"
  (cd "$mroot" && go build -o "$mbin" ./cmd/aurumcode) >"$mroot/build.log" 2>&1 || { cat "$mroot/build.log" >&2; infra mutant_build_failed; }

  local demo="$mroot/tests/fixtures/repos/git-demo/repo.git" rc
  set +e
  (cd "$demo" && noprov_env env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$mbin" review --base HEAD~1 --seguranca) >/dev/null 2>&1
  rc=$?; set -e
  # The mutant MUST report the defect (exit 0). If it still exits 1, the
  # assertion is not actually load-bearing.
  [[ "$rc" -eq 0 ]] || fail "MUT-001-did-not-reproduce-the-defect:$rc"
  printf '%s/%s/MUT-001/defect-reproduced\n' "$card" "$scenario"
}

case "$selector" in
  AC-001)
    nominal_case
    (cd "$repo_root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-458.sh E2EAUR458) >/dev/null || fail e2e-failed
    printf '%s/%s/ok\n' "$card" "$scenario"
    ;;
  TestAUR458)
    build_shared
    copy "$shared_root" tests/unit/AUR-458.go
    cat >"$shared_root/tests/unit/aur458_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR458UnitBridge(t *testing.T) { TestAUR458(t) }
EOF
    (cd "$shared_root" && AURUMCODE_ROOT="$shared_root" AURUMCODE_BIN="$shared_bin" go test -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR458UnitBridge$' -count=1) || fail unit-failed
    ;;
  IntegrationAUR458)
    build_shared
    copy "$shared_root" tests/integration/AUR-458.go
    cat >"$shared_root/tests/integration/aur458_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR458IntegrationBridge(t *testing.T) { IntegrationAUR458(t) }
EOF
    (cd "$shared_root" && AURUMCODE_ROOT="$shared_root" AURUMCODE_BIN="$shared_bin" go test -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR458IntegrationBridge$' -count=1) || fail integration-failed
    ;;
  E2EAUR458)
    build_shared
    (cd "$repo_root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-458.sh E2EAUR458) || fail e2e-failed
    ;;
  AC-001-MUT-001)
    mutation_case
    ;;
esac
exit 0
