#!/usr/bin/env bash
#
# Acceptance program for card AUR-451, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The 2026-08-14 product-review measurement: cmd/aurumcode/pr.go never
#   called SecurityScan, and of the flags only countAtOrAbove was used
#   internally for --check. On the --pr path -- this product's main use
#   case -- there was no security scan, --fail-on did not gate, --limite
#   did not cap spend, and --modelo did not choose the model; the whole
#   engine only ever worked on --base. This program proves the fix: `review
#   --pr <n> --repo <o>/<r> --publicar --na-linha --seguranca --fail-on
#   high` now runs the deterministic security pass over the pull request's
#   diff and gates on its findings, reusing the exact functions the --base
#   path already calls (review.SecurityScanWithCoverage,
#   severityRank/countAtOrAbove/exitFindings, the AUR-433 cost gate,
#   selectProviderForModel -- see cmd/aurumcode/pr.go's package doc) rather
#   than a second implementation. See docs/specs/AUR-451.md.
#
# WHY THIS PROGRAM SUPPLIES ITS OWN DIFF
#
#   tests/fixtures/scm/github/pr-42.diff (a read_path, not writable by this
#   card) carries nothing a security-category rule of the embedded catalog
#   would ever match. tests/e2e/AUR-451.sh -- this program's nominal_case
#   and e2e_case -- builds its own PR diff instead, planting
#   `API_KEY=AURUM-FAKE-KEY-9000-2222`, the exact synthetic value
#   tests/unit/AUR-442.go's TestAUR442 already proves matches
#   security/hardcoded-secret.
#
# WHY EVERYTHING IS OFFLINE
#
#   The sealed profile (bootstrap-readonly-v1) denies network. Every proof
#   runs against a loopback-only fake GitHub and a deterministic offline
#   model response the proof programs write themselves.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure -- never valid red evidence
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-451'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR451|IntegrationAUR451|E2EAUR451|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a451.XXXXXX")" || infra mktemp
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
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# Input preflight. Deliverables this card owns fail behavioral (their
# absence IS the missing behavior); everything else is an environment gap.
owned_inputs=(
  cmd/aurumcode/pr.go
  cmd/aurumcode/main.go
  tests/unit/AUR-451.go
  tests/integration/AUR-451.go
  tests/e2e/AUR-451.sh
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done

# stage_source materializes exactly what `go build ./cmd/aurumcode` and the
# PR review path need: this card's owned cmd/aurumcode plus every
# read-only package it imports and the fixtures this card's own proofs
# read (tests/fixtures/scm/github for the GitHub-shaped JSON fixtures --
# the diff body itself is synthesized by tests/e2e/AUR-451.sh, not read
# from a fixture file).
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/analyzer internal/config internal/prompt internal/review internal/security internal/llm internal/git pkg/types
  copy "$root" tests/fixtures/scm/github tests/fixtures/repos/git-demo tests/fixtures/review

  # cmd/aurumcode is one package: if another integrated card added a file
  # there that imports the documentation pipeline, `go build ./cmd/aurumcode`
  # needs those packages too (same detection tests/acceptance/AUR-438.sh
  # and tests/acceptance/AUR-450.sh already use).
  if grep -Elq 'AurumCode/internal/(documentation|pipeline)"' "$root"/cmd/aurumcode/*.go 2>/dev/null; then
    copy "$root" internal/documentation/extractors internal/documentation/incremental \
      internal/documentation/normalizer internal/documentation/site \
      internal/documentation/welcome internal/pipeline cmd/regenerate-docs
  fi

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
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# run_e2e runs tests/e2e/AUR-451.sh against the given binary from the given
# staged root. Its own 79 (infrastructure) is re-emitted as infra here,
# never collapsed into behavioral RED.
run_e2e() {
  local root="$1" bin="$2"
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$bin" bash tests/e2e/AUR-451.sh E2EAUR451) \
    >"$run_dir/e2e.stdout" 2>"$run_dir/e2e.stderr"
  rc=$?
  set -e
  printf '%s\n' "$rc"
}

nominal_case() {
  build_shared
  local root="$run_dir/root-nominal"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-451.sh
  chmod -R u+w -- "$root"
  local rc
  rc="$(run_e2e "$root" "$shared_bin")"
  ((rc != 79)) || infra "nominal-inconclusive:$rc"
  ((rc == 0)) || fail "behavior-missing:exit:$rc"
  cleanup_root "$root"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-451.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur451_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR451UnitBridge(t *testing.T) { TestAUR451(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR451UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR451:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR451:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-451.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur451_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR451IntegrationBridge(t *testing.T) { IntegrationAUR451(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR451IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR451:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR451:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-451.sh
  chmod -R u+w -- "$root"
  local rc
  rc="$(run_e2e "$root" "$shared_bin")"
  ((rc != 79)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# mutation_case is MUT-001: "ignorar --seguranca no caminho de PR faz o
# aceite falhar porque a vulnerabilidade plantada nao e reportada." It
# edits a writable staged copy of cmd/aurumcode/pr.go, silencing exactly
# the line that folds the security pass's findings into the issues
# published as pull request comments -- `result.Issues = combined`,
# reached only when --seguranca actually matched something -- while
# leaving --seguranca's own validation, the coverage note, and every other
# behavior untouched. Under the mutant, --seguranca is accepted as a flag
# but its findings never reach a comment: exactly the measured defect this
# card exists to close. The committed source is never touched: the
# mutation exists only in this case's own staged copy.
mutation_case() {
  build_shared # warm GOCACHE; the rebuild recompiles one package.

  local root="$run_dir/root-mut"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-451.sh
  chmod -R u+w -- "$root"

  local target="$root/cmd/aurumcode/pr.go"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor='result.Issues = combined'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
  local replacement='_ = combined // MUT-001: suppress --seguranca findings on the PR path'
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
  grep -Fq 'MUT-001: suppress --seguranca findings on the PR path' "$target" || fail 'MUT-001/mutation-not-applied'
  [[ "$(grep -Fc "$anchor" "$target")" == 0 ]] || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  # Under the mutant, the planted vulnerability is silently dropped from
  # the publish list: tests/e2e/AUR-451.sh's own ac001_* assertions
  # (the finding line, the rule citation, or the inline-publish marker)
  # must be exactly what fails -- never some unrelated, coincidental
  # failure (a wrong PR number, a build error, a --limite regression).
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_BIN="$bin" bash tests/e2e/AUR-451.sh E2EAUR451 2>&1)"
  rc=$?
  set -e
  if ((rc == 0)); then
    fail 'MUT-001/not-rejected'
  fi
  if ((rc == 79)); then
    infra "MUT-001/inconclusive:$out"
  fi
  # The mutant drops every security finding, so issues is empty and
  # tests/e2e/AUR-451.sh's scenario 1 hits its own early "No issues found."
  # / exit-0 branch instead of ever reaching the --fail-on gate --
  # ac001_wrong_exit (rc 0, not the expected 3) is therefore the primary
  # signal; the other three are accepted too in case a future edit to
  # either file changes exactly where the missing finding first trips an
  # assertion.
  grep -Eq 'ac001_(wrong_exit|missing_finding_line|missing_rule_citation|missing_inline_marker|wrong_post_count)' <<<"$out" \
    || fail "MUT-001/wrong-failure-mode:$out"

  # Restoration: the unmutated shared binary still passes the same proof --
  # the GREEN reproduces exactly.
  local root2="$run_dir/root-mut-restore"
  stage_source "$root2"
  copy "$root2" tests/e2e/AUR-451.sh
  chmod -R u+w -- "$root2"
  local rc2
  rc2="$(run_e2e "$root2" "$shared_bin")"
  ((rc2 == 0)) || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  cleanup_root "$root2"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mutation_case
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR451) unit_case ;;
  IntegrationAUR451) integration_case ;;
  E2EAUR451) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
