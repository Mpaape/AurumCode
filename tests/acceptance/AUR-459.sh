#!/usr/bin/env bash
#
# Acceptance program for card AUR-459, scenario AC-001.
#
# WHAT THIS PROVES
#
#   A finding the model reported reaches the user, whichever of the two
#   findings vocabularies it used. Before this card the prompt taught
#   "line_comments" first and the parser read only "issues", so a model
#   that answered with line_comments -- which is what the real gateway
#   does -- produced "No issues found." with exit 0 over a diff carrying a
#   planted secret. Three properties, all required:
#
#     1. A response whose findings live only in "line_comments" and cite a
#        rule of the embedded catalog prints those findings on stdout.
#     2. The same response without a rule_id is NOT silence: the AUR-434
#        rule gate still discards it, and AUR-448's warning names how many
#        and why on stderr.
#     3. The prompt teaches exactly one findings schema, the one the parser
#        consumes (internal/prompt's TestReviewTemplateMatchesParser reads
#        the template's own bytes; it is run here, not merely cited).
#
# WHY THE LLM CALL IS A FIXTURE
#
#   The sealed profile (bootstrap-readonly-v1) denies network.
#   AURUMCODE_LLM_FIXTURE points the binary's provider selection at canned,
#   deterministic response files; internal/review.FakeProvider implements
#   the same llm.Provider interface a vendor provider would.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-459'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR459|IntegrationAUR459|E2EAUR459|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/llm
  internal/prompt
  internal/prompt/templates/review.md
  internal/review
  internal/security
  internal/git
  internal/documentation/extractors
  internal/documentation/incremental
  internal/documentation/normalizer
  internal/documentation/site
  internal/documentation/welcome
  internal/pipeline
  internal/config
  cmd/regenerate-docs
  pkg/types
  tests/fixtures/repos/git-demo/repo.git
  tests/unit/AUR-459.go
  tests/integration/AUR-459.go
  tests/e2e/AUR-459.sh
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a459.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 for every go invocation in this file.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export TMPDIR="$run_dir"

run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMEMLIMIT=2GiB go "$@" )
}

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode cmd/regenerate-docs
  # This list is enumerated by hand and rots: when cmd/aurumcode gains an
  # import under internal/, the build fails here under GOPROXY=off with a
  # confusing third-party message, because the resolver reaches for the whole
  # module graph before it can name the local package that is missing. Add the
  # package here IN THE SAME CHANGE that introduces the import, never before it
  # exists -- naming a package that is not tracked yet breaks this acceptance
  # the other way, at copy()'s own existence check.
  # Derive the set with: go list -deps ./cmd/aurumcode
  copy "$root" internal/analyzer internal/prompt internal/review internal/security internal/llm internal/git internal/pipeline internal/config
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" pkg/types
  copy "$root" tests/fixtures/repos/git-demo tests/fixtures/review
  chmod -R u+w -- "$root"
}

# write_fixtures plants this card's deterministic model responses: the
# shape the REAL gateway answers with (findings only under
# "line_comments"), once citing a rule of the embedded catalog and once
# citing none.
write_fixtures() {
  local dir="$1"
  mkdir -p "$dir"
  cat >"$dir/line-comments-grounded.json" <<'EOF'
{
  "line_comments": [
    {
      "path": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "body": "reported only under line_comments"
    }
  ],
  "summary": "one finding"
}
EOF
  cat >"$dir/line-comments-ungrounded.json" <<'EOF'
{
  "line_comments": [
    {
      "path": "config/demo-tokens.txt",
      "line": 4,
      "body": "reported only under line_comments, citing no rule"
    }
  ],
  "summary": "one finding"
}
EOF
}

shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! run_go "$shared_root" build -o "$shared_bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    infra build_failed
  fi
  shared_built=1
}

run_bin() {
  local bin="$1" dir="$2"; shift 2
  local extra_env=() bin_args=() a
  for a in "$@"; do
    if [[ "$a" =~ ^[A-Za-z_][A-Za-z0-9_]*=.*$ ]]; then
      extra_env+=("$a")
    else
      bin_args+=("$a")
    fi
  done
  set +e
  (cd "$dir" && env -u AURUM_SECRET_CANARY -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u AURUMCODE_CACHE_DIR \
    "${extra_env[@]}" "$bin" "${bin_args[@]}") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

nominal_case() {
  build_shared
  write_fixtures "$run_dir/fixtures"
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"

  # --- 1. line_comments citing a catalog rule reach stdout. ---
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/line-comments-grounded.json"
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  if grep -Fq 'No issues found.' "$run_dir/out.stdout"; then fail behavior-missing; fi
  grep -Fq 'config/demo-tokens.txt:4' "$run_dir/out.stdout" || fail behavior-missing
  grep -Fq 'reported only under line_comments' "$run_dir/out.stdout" || fail behavior-missing
  grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' "$run_dir/out.stdout" || fail behavior-missing

  # Determinism: same input, same stdout.
  local first; first="$(cat "$run_dir/out.stdout")"
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/line-comments-grounded.json"
  [[ "$rc" -eq 0 ]] || fail non-deterministic
  [[ "$first" == "$(cat "$run_dir/out.stdout")" ]] || fail non-deterministic

  # --- 2. Without a rule_id the finding is still not silence. ---
  # The AUR-434 gate discards it (stdout stays "No issues found."), and
  # AUR-448's warning says so on stderr. Trading the old false negative
  # for a NEW silent one is exactly what this check forbids.
  run_bin "$shared_bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/line-comments-ungrounded.json"
  [[ "$rc" -eq 0 ]] || fail behavior-missing
  [[ "$(cat "$run_dir/out.stdout")" == 'No issues found.' ]] || fail behavior-missing
  local want_stderr='aurumcode review: 1 finding(s) discarded: 1 with no rule_id'
  [[ "$(cat "$run_dir/out.stderr")" == "$want_stderr" ]] || fail behavior-missing

  # --- 3. The prompt teaches exactly one findings schema. ---
  if ! run_go "$shared_root" test ./internal/prompt/ -run 'TestReviewTemplateMatchesParser|TestAcceptedReviewFieldsAreConsumed|TestLineComments' -count=1 -timeout 120s >"$run_dir/unit.log" 2>&1; then
    cat "$run_dir/unit.log" >&2
    fail behavior-missing
  fi
}

# MUT-001: put the defect back -- the parser stops adopting line_comments
# -- and require the nominal proof to go red. A check that passes with the
# defect restored proves nothing.
mutation_case() {
  build_shared
  write_fixtures "$run_dir/fixtures"
  local root="$run_dir/root-mutant" bin="$run_dir/aurumcode-mutant"
  cp -R "$shared_root" "$root"
  chmod -R u+w -- "$root"
  rm -f "$root/build.log"

  local target="$root/internal/prompt/parser.go"
  local anchor='	p.adoptLineComments(jsonContent, &result)'
  grep -Fq "$anchor" "$target" || infra mutation_anchor_missing
  local tmp="$root/parser.mutated"
  awk -v anchor="$anchor" '{ if ($0 == anchor) { print "\t_ = p.adoptLineComments" } else { print } }' "$target" >"$tmp" || infra mutation_rewrite
  mv "$tmp" "$target"
  grep -Fq "$anchor" "$target" && infra mutation_not_applied

  if ! run_go "$root" build -o "$bin" ./cmd/aurumcode >"$root/build.log" 2>&1; then
    cat "$root/build.log" >&2
    infra mutant_build_failed
  fi

  local repo_dir="$root/tests/fixtures/repos/git-demo/repo.git"
  run_bin "$bin" "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$run_dir/fixtures/line-comments-grounded.json"
  # With the defect back, the finding the model reported disappears: this
  # is the confidently-wrong "No issues found." the card exists to kill.
  if ! grep -Fq 'No issues found.' "$run_dir/out.stdout"; then
    fail MUT-001
  fi
  if grep -Fq 'reported only under line_comments' "$run_dir/out.stdout"; then
    fail MUT-001
  fi
  # The internal/prompt contract tests must also go red on the mutant.
  if run_go "$root" test ./internal/prompt/ -run 'TestAcceptedReviewFieldsAreConsumed|TestLineComments' -count=1 -timeout 120s >"$run_dir/mutant-unit.log" 2>&1; then
    fail MUT-001
  fi
}

# unit_case runs the card's declared unit program. tests/unit/*.go are
# plain .go files, so `go test` alone would report "no test files" and pass
# vacuously; the bridge _test.go below is the repo's convention (see
# tests/acceptance/AUR-448.sh) for making the file actually execute, and
# the "ok" line is checked so a zero-test run cannot read as green.
unit_case() {
  build_shared
  local root="$shared_root" out rc
  copy "$root" tests/unit/AUR-459.go
  chmod -R u+w -- "$root/tests/unit"
  cat >"$root/tests/unit/aur459_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR459UnitBridge(t *testing.T) { TestAUR459(t) }
EOF
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR459UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  ((rc == 0)) || { printf '%s\n' "$out" >&2; fail "selector:TestAUR459:exit:$rc"; }
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR459:zero-tests
}

# integration_case runs the card's declared integration program, at the
# seam the unit program cannot see (parser + rule gate through the real
# GenerateReview). Same bridge convention as unit_case.
integration_case() {
  build_shared
  local root="$shared_root" out rc
  copy "$root" tests/integration/AUR-459.go
  chmod -R u+w -- "$root/tests/integration"
  cat >"$root/tests/integration/aur459_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR459IntegrationBridge(t *testing.T) { IntegrationAUR459(t) }
EOF
  set +e
  out="$(cd "$root" && ulimit -v 8388608 && AURUMCODE_ROOT="$root" GOMAXPROCS=1 GOMEMLIMIT=2GiB go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR459IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  ((rc == 0)) || { printf '%s\n' "$out" >&2; fail "selector:IntegrationAUR459:exit:$rc"; }
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR459:zero-tests
}

# e2e_case delegates to the card's own E2E program, which asserts the CI
# decision (--fail-on) rather than the review output AC-001 asserts.
e2e_case() {
  [[ -x "$repo_root/tests/e2e/AUR-459.sh" || -f "$repo_root/tests/e2e/AUR-459.sh" ]] || infra "missing-input:tests/e2e/AUR-459.sh"
  set +e
  bash "$repo_root/tests/e2e/AUR-459.sh" E2EAUR459
  local rc=$?
  set -e
  ((rc == 0)) || exit "$rc"
}

# Each declared selector runs its OWN program. A selector whose program
# does not exist must never fall through to AC-001's assertions: three
# greens harvested from one execution is the anti-pattern this board names
# explicitly, so an unknown or unimplemented selector exits 64 (absence
# reads as absence) rather than passing on someone else's work.
case "$selector" in
  AC-001-MUT-001) mutation_case ;;
  TestAUR459) unit_case ;;
  IntegrationAUR459) integration_case ;;
  E2EAUR459) e2e_case ;;
  AC-001) nominal_case ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
exit 0
