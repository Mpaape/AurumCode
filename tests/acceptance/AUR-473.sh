#!/usr/bin/env bash
#
# Acceptance program for card AUR-473.
#
# WHAT THIS PROVES
#
#   `--modelo <nome>` naming a model the gateway does not serve is a USAGE
#   error, the same class as a mistyped flag -- not a runtime failure of a
#   provider that was configured and broke. It must fail BEFORE any work is
#   computed: exit 1, the model named on stderr, stdout empty, and neither
#   the `--seguranca` section nor its coverage note printed, because the
#   deterministic pass never ran. Nothing computed means nothing retained,
#   which is why this does not contradict AUR-458: that card forbids
#   RETAINING already-computed findings, not failing early.
#
#   The sibling contract stays intact (AC-002): a provider that WAS
#   configured and fails at RUNTIME with `--seguranca` still prints its
#   deterministic security findings and the coverage note and exits 1. The
#   fix fails early; it never hides a finding it already computed.
#
#   And the original AUR-450 acceptance is green again (AC-003).
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
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

readonly card='AUR-473'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|AC-002|AC-003|all|TestAUR473|IntegrationAUR473|E2EAUR473|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  tests/unit/AUR-473.go
  tests/integration/AUR-473.go
  tests/e2e/AUR-473.sh
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/git/githubclient
  internal/llm
  internal/prompt
  internal/review
  internal/security/redaction
  pkg/types
  tests/fixtures/repos/git-demo
  tests/fixtures/review
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a473.XXXXXX")" || infra mktemp
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

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs on
# this base (670c7f6 removed internal/documentation/*, internal/pipeline and
# cmd/regenerate-docs, so staging any of those aborts before an assertion).
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/analysis internal/analyzer internal/apply internal/config internal/context internal/git internal/llm internal/memory internal/prompt internal/render internal/review internal/security internal/testgen
  copy "$root" pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  chmod -R u+w -- "$root"
}

readonly sec_header='Security findings (standards/security-review):'
readonly coverage_prefix='aurumcode review: security pass applied 4 of 8 security rules ('

noprov_env() {
  env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL "$@"
}

shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! (cd "$shared_root" && go build -o "$shared_bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    infra build_failed
  fi
  shared_built=1
}

# nominal_case is AC-001: the unavailable named model is a usage error,
# before any work -- no security section, no coverage note, empty stdout.
nominal_case() {
  build_shared
  local demo_repo="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local out err rc
  set +e
  out="$(cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --modelo local 2>"$run_dir/modelo.err")"
  rc=$?
  set -e
  err="$(cat "$run_dir/modelo.err")"

  [[ "$rc" -eq 1 ]] || fail "unavailable-modelo-must-exit-1:$rc"
  grep -Fq 'model "local" is unavailable' <<<"$err" || fail unavailable-modelo-error-missing
  if grep -Fq "$sec_header" <<<"$out"; then fail unavailable-modelo-printed-security-section; fi
  if grep -Fq "$coverage_prefix" <<<"$err"; then fail unavailable-modelo-printed-coverage-note; fi
  [[ -z "${out//[[:space:]]/}" ]] || fail unavailable-modelo-computed-a-review

  # The usage error outranks --fail-on: 1, never 3.
  set +e
  (cd "$demo_repo" && noprov_env "$shared_bin" review --base HEAD~1 --seguranca --modelo local --fail-on high) >/dev/null 2>&1
  rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail "modelo-usage-error-must-outrank-gate:$rc"
}

# ac002_case is AC-002: AUR-458 stays intact. A configured provider that
# fails at runtime with --seguranca still prints its already-computed
# deterministic findings and exits 1.
ac002_case() {
  build_shared
  local demo_repo="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  local out err rc
  set +e
  out="$(cd "$demo_repo" && noprov_env env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$shared_bin" review --base HEAD~1 --seguranca 2>"$run_dir/fail.err")"
  rc=$?
  set -e
  err="$(cat "$run_dir/fail.err")"

  [[ "$rc" -eq 1 ]] || fail "runtime-provider-failure-must-exit-1:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail runtime-provider-failure-lost-findings
  grep -Fq "$coverage_prefix" <<<"$err" || fail runtime-provider-failure-lost-coverage-note
  local line
  for line in 4 5 6; do
    grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out" || fail "runtime-failure-lost-finding:$line"
  done
}

# ac003_case is AC-003: the original AUR-450 acceptance is green again.
ac003_case() {
  (cd "$repo_root" && bash tests/acceptance/AUR-450.sh AC-001) || fail 'AC-003/AUR-450-regressed'
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-473.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur473_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR473UnitBridge(t *testing.T) { TestAUR473(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR473UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR473:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR473:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-473.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur473_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR473IntegrationBridge(t *testing.T) { IntegrationAUR473(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR473IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR473:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR473:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-473.sh
  chmod -R u+w -- "$root"
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-473.sh E2EAUR473) || fail e2e-failed
  cleanup_root "$root"
}

# apply_mutation rewrites one load-bearing anchor in a staged copy of
# cmd/aurumcode/main.go (never the committed source) and rebuilds. The
# anchor must appear exactly once, so a future refactor cannot silently
# mutate the wrong line.
apply_mutation() {
  local root="$1" anchor="$2" repl="$3" tag="$4"
  stage_source "$root"
  local target="$root/cmd/aurumcode/main.go"
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail "mutation/anchor-not-unique:$tag"
  ANCHOR="$anchor" REPL="$repl" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"]; repl = ENVIRON["REPL"] }
    {
      idx = index($0, anchor)
      if (idx > 0) { print substr($0, 1, idx - 1) repl substr($0, idx + length(anchor)) }
      else { print $0 }
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target"
  [[ "$(grep -Fc "$anchor" "$target")" == 0 ]] || fail "mutation/not-applied:$tag"
  local bin="$run_dir/aurumcode-$tag"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$root/build-mut.log" 2>&1; then
    cat "$root/build-mut.log" >&2
    fail "mutation/build-failed:$tag"
  fi
  printf '%s' "$bin"
}

# MUT-001: run the security pass before refusing the named model. The
# mutant must now print the security section for an unavailable --modelo,
# proving the AC-001 assertion is load-bearing. The mutant is rejected.
mutation_001() {
  build_shared
  local root="$run_dir/root-mut1"
  local bin demo_repo out rc
  bin="$(apply_mutation "$root" '!*seguranca || *modelo != ""' '!*seguranca' mut1)"
  demo_repo="$root/tests/fixtures/repos/git-demo/repo.git"
  set +e
  out="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca --modelo local 2>/dev/null)"
  rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail "MUT-001/mutation-changed-exit:$rc"
  grep -Fq "$sec_header" <<<"$out" || fail 'MUT-001/mutation-did-not-reproduce'
  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# MUT-002: make a runtime provider failure also skip the security pass, by
# disabling the AUR-458 diversion so a failed GenerateReview returns through
# the published refusal instead of delivering the deterministic findings.
# The mutant must now lose the findings AC-002 requires, proving the
# assertion is load-bearing. The mutant is rejected.
mutation_002() {
  build_shared
  local root="$run_dir/root-mut2"
  local bin demo_repo out rc
  bin="$(apply_mutation "$root" 'err != nil && *seguranca' 'err != nil && false' mut2)"
  demo_repo="$root/tests/fixtures/repos/git-demo/repo.git"
  set +e
  out="$(cd "$demo_repo" && noprov_env env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$bin" review --base HEAD~1 --seguranca 2>/dev/null)"
  rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail "MUT-002/mutation-changed-exit:$rc"
  if grep -Fq "$sec_header" <<<"$out"; then fail 'MUT-002/mutation-did-not-reproduce'; fi
  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

run_all() {
  nominal_case
  ac002_case
  ac003_case
  unit_case
  integration_case
  e2e_case
  mutation_001
  mutation_002
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) nominal_case; printf '%s/%s/ok\n' "$card" "$scenario" ;;
  AC-002) ac002_case; printf '%s/%s/ok\n' "$card" "$scenario" ;;
  AC-003) ac003_case; printf '%s/%s/ok\n' "$card" "$scenario" ;;
  TestAUR473) unit_case ;;
  IntegrationAUR473) integration_case ;;
  E2EAUR473) e2e_case ;;
  MUT-001) mutation_001 ;;
  MUT-002) mutation_002 ;;
  all) run_all ;;
esac
