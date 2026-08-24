#!/usr/bin/env bash
#
# Acceptance program for card AUR-466, scenario AC-001 (+ AC-002 + AC-003
# in the same nominal run, mirroring tests/acceptance/AUR-462.sh's shape).
#
# WHAT THIS PROVES
#
#   The 2026-08-14 measurement on a real, 55-commit Node repository found
#   15 `error` findings and every sampled one false: security/hardcoded-secret
#   matching a documentation placeholder (README.md's `export
#   GEMINI_API_KEY=sua-chave`), a help-text string that teaches the export
#   (lib/setup-snippets.mjs), and two test-fixture assignments whose values
#   are synthetic labels (`ttm-smoke-key`, `sk-body-ULTRA-SECRET`); and
#   security/sql-injection matching a query built entirely from string
#   CONSTANTS (lib/db.mjs), whose real values are bound later through `?`
#   placeholders. The root cause was one shared shape: both rules matched
#   on the FORM of the text, never the VALUE or the CONTEXT. This program
#   proves the fix (internal/review/rules/security.yml): none of the five
#   measured false-positive shapes are matched (AC-001); a real
#   digit-bearing secret, a SQL query concatenating a VARIABLE, and a shell
#   command concatenating a VARIABLE are still all three matched (AC-002);
#   and the pre-existing Python, hardcoded-secret, and Node
#   command-injection/xss regression fixtures still produce exactly the
#   same findings, unchanged (AC-003).
#
# WHY THIS READS A COMMITTED FIXTURE, NOT AN EPHEMERAL REPOSITORY
#
#   Same reasoning tests/acceptance/AUR-462.sh's own header states, and
#   the same builder: the sealed acceptance profile (bootstrap-readonly-v1)
#   carries no `git` binary, so
#   tests/fixtures/review/vuln/node-placeholder-vs-secret is a bare,
#   loose-object repository built by
#   tests/fixtures/repos/git-demo/build-fixture.sh from
#   tests/fixtures/review/vuln/node-placeholder-vs-secret/history.spec.
#   Its HEAD~1..HEAD diff adds the identical two files
#   tests/unit/AUR-466.go's aur466FixtureLines embeds as a synthetic diff,
#   so every selector agrees on line numbers.
#
# WHY NO LLM FIXTURE IS NEEDED
#
#   Same AUR-449 no-provider path AUR-462 relies on: `--seguranca` alone,
#   with LLM_API_KEY/LLM_BASE_URL/AURUMCODE_LLM_FIXTURE unset, skips
#   quality review and runs only the deterministic, model-free security
#   pass. This program unsets all three so every run takes that path.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001/MUT-002 mutant)
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

readonly card='AUR-466'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR466|IntegrationAUR466|E2EAUR466|AC-001-MUT-001|AC-001-MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(
  internal/review/rules/security.yml
  tests/unit/AUR-466.go
  tests/integration/AUR-466.go
  tests/e2e/AUR-466.sh
  tests/fixtures/review/vuln/node-placeholder-vs-secret/repo.git
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/config
  internal/prompt
  internal/review
  internal/security
  internal/git
  internal/documentation
  internal/pipeline
  pkg/types
  internal/llm
  tests/fixtures/review/vuln/repo.git
  tests/fixtures/review/vuln/hardcoded-secret/repo.git
  tests/fixtures/review/vuln/node-xss-command-injection/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a466.XXXXXX")" || infra mktemp
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

unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/prompt internal/review internal/security
  copy "$root" internal/git internal/documentation internal/pipeline
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/review/vuln
  chmod -R u+w -- "$root"
}

readonly node_repo="$repo_root/tests/fixtures/review/vuln/node-placeholder-vs-secret/repo.git"
readonly header='Security findings (standards/security-review):'
readonly secret_citation='(rule security/hardcoded-secret: Hardcoded Secrets)'
readonly sql_citation='(rule security/sql-injection: SQL Injection Vulnerability)'
readonly cmd_citation='(rule security/command-injection: Command Injection)'

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

# nominal_case is AC-001's + AC-002's + AC-003's core behavioral proof.
nominal_case() {
  build_shared

  local out_sec
  out_sec="$(cd "$node_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
  grep -Fq "$header" <<<"$out_sec" || fail behavior-missing

  # AC-001: none of the five measured false-positive shapes ever appear.
  # README.md's doc placeholder (export ... =sua-chave).
  if grep -Fq 'README.md:' <<<"$out_sec"; then fail false-positive:doc-placeholder; fi
  # src/app.js: help text (line 6), two fixture assignments (lines 10-11),
  # constant SQL concat (line 16).
  for ln in 6 10 11 16; do
    if grep -Fq "src/app.js:$ln: [error]" <<<"$out_sec"; then fail "false-positive:line-$ln"; fi
  done

  # AC-002: the real secret, real SQL variable concat, and real shell
  # variable concat are all three still found.
  grep -Fq 'src/app.js:21: [error]' <<<"$out_sec" || fail behavior-missing:real-secret
  grep -Fq "$secret_citation" <<<"$out_sec" || fail behavior-missing:secret-citation
  grep -Fq 'src/app.js:24: [error]' <<<"$out_sec" || fail behavior-missing:sql-variable-concat
  grep -Fq "$sql_citation" <<<"$out_sec" || fail behavior-missing:sql-citation
  grep -Fq 'src/app.js:29: [error]' <<<"$out_sec" || fail behavior-missing:shell-variable-concat
  grep -Fq "$cmd_citation" <<<"$out_sec" || fail behavior-missing:cmd-citation

  local after_header count
  after_header="${out_sec#*"$header"}"
  count="$(grep -Fo '[error]' <<<"$after_header" | wc -l)"
  [[ "$count" -eq 3 ]] || fail "unexpected-finding-count:$count"

  # Determinism.
  local out_again
  out_again="$(cd "$node_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic

  # AC-003: the pre-existing Python SQL-injection regression fixture is
  # unaffected -- same finding, same count.
  local py_repo="$shared_root/tests/fixtures/review/vuln/repo.git"
  local out_py
  out_py="$(cd "$py_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:python-regression
  grep -Fq 'src/db.py:8: [error]' <<<"$out_py" || fail regression:python-sql-injection-missing
  grep -Fq "$sql_citation" <<<"$out_py" || fail regression:python-citation-missing
  [[ "$(grep -Fo '[error]' <<<"$out_py" | wc -l)" -eq 1 ]] || fail regression:python-finding-count-changed

  # AC-003: the pre-existing hardcoded-secret regression fixture is
  # unaffected -- two findings, same lines, same citation.
  local secret_repo="$shared_root/tests/fixtures/review/vuln/hardcoded-secret/repo.git"
  local out_secret
  out_secret="$(cd "$secret_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:secret-regression
  grep -Fq "$secret_citation" <<<"$out_secret" || fail regression:secret-citation-missing
  [[ "$(grep -Fo "$secret_citation" <<<"$out_secret" | wc -l)" -eq 2 ]] || fail regression:secret-finding-count-changed
  if grep -Fq 'AURUM-FAKE-KEY-9000-2222' <<<"$out_secret"; then fail regression:secret-value-leaked; fi
  if grep -Fq 'AURUM-FAKE-PASSWORD-9000-1111' <<<"$out_secret"; then fail regression:secret-value-leaked; fi

  # AC-003: the AUR-462 Node command-injection/xss regression fixture is
  # unaffected -- same finding counts, unchanged.
  local node462_repo="$shared_root/tests/fixtures/review/vuln/node-xss-command-injection/repo.git"
  local out_462
  out_462="$(cd "$node462_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:node462-regression
  local c462 x462
  local after462="${out_462#*"$header"}"
  c462="$(grep -Fo "$cmd_citation" <<<"$after462" | wc -l)"
  x462="$(grep -Fo '(rule security/xss: Cross-Site Scripting (XSS))' <<<"$after462" | wc -l)"
  [[ "$c462" -eq 3 ]] || fail "regression:node462-command-injection-count:$c462"
  [[ "$x462" -eq 1 ]] || fail "regression:node462-xss-count:$x462"

  # Without --seguranca: no security section leaks in.
  local out_base
  out_base="$(cd "$node_repo" && "$shared_bin" review --base HEAD~1 2>/dev/null || true)"
  if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi
}

# mutation_case_1 is MUT-001: restore the digit-or-hyphen placeholder
# pattern (revert security/hardcoded-secret's value class from `[0-9]`
# back to `[0-9\-]`) and prove the AC-001 false positives return.
mutation_case_1() {
  build_shared
  local root="$run_dir/root-mut1"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  # The anchor is read verbatim from the live file rather than hand-typed
  # here, so this program can never drift from the actual YAML escaping
  # (single quotes doubled per YAML's own single-quoted-string rule) the
  # security/hardcoded-secret rule's pattern: line carries.
  local anchor
  anchor="$(awk '/  - id: security\/hardcoded-secret/{f=1} f && /^    pattern:/{print; exit}' "$target")"
  [[ -n "$anchor" ]] || fail 'MUT-001/anchor-not-found'
  case "$anchor" in
    *'[0-9]'*) ;;
    *) fail 'MUT-001/anchor-missing-digit-class' ;;
  esac
  local replacement="${anchor//\[0-9\]/[0-9\\-]}"
  grep -Fq "$anchor" "$target" || fail 'MUT-001/anchor-not-found'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || fail 'MUT-001/anchor-not-unique'
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
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail 'MUT-001/rewrite-failed'
  grep -Fq "$anchor" "$target" && fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut1"
  local log="$root/build-mut1.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local out
  out="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-001/mutation-run-failed'
  grep -Fq "$header" <<<"$out" || fail 'MUT-001/pass-did-not-run'
  # The mutant must regain at least one of the suppressed false positives.
  if ! grep -Fq 'src/app.js:10: [error]' <<<"$out"; then fail 'MUT-001/mutation-did-not-restore-false-positive'; fi

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# mutation_case_2 is MUT-002: widen security/hardcoded-secret until the
# AC-002 real secret (which DOES carry a digit) is also silenced --
# a candidate that zeros AC-001 by zeroing AC-002 too must be rejected.
mutation_case_2() {
  build_shared
  local root="$run_dir/root-mut2"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-002/target-missing'
  local id_anchor='  - id: security/hardcoded-secret'
  grep -Fq "$id_anchor" "$target" || fail 'MUT-002/id-not-found'
  # Comment the rule's pattern line out entirely (the maximal, honest
  # "silence everything" mutant): no pattern means the matcher never runs.
  local pattern_line
  pattern_line="$(awk '/  - id: security\/hardcoded-secret/{f=1} f && /^    pattern:/{print; exit}' "$target")"
  [[ -n "$pattern_line" ]] || fail 'MUT-002/pattern-line-not-found'
  ANCHOR="$pattern_line" awk '
    BEGIN { anchor = ENVIRON["ANCHOR"] }
    {
      if ($0 == anchor) { next }
      print $0
    }
  ' "$target" >"$target.mut" && mv "$target.mut" "$target" || fail 'MUT-002/rewrite-failed'
  grep -Fq "$pattern_line" "$target" && fail 'MUT-002/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut2"
  local log="$root/build-mut2.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-002/build-failed'
  fi

  local out
  out="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-002/mutation-run-failed'
  grep -Fq "$header" <<<"$out" || fail 'MUT-002/pass-did-not-run'
  # The mutant must lose the AC-002 real-secret finding: exactly the
  # "zeroing AC-001 by zeroing AC-002 too" shape this card forbids.
  if grep -Fq 'src/app.js:21: [error]' <<<"$out"; then fail 'MUT-002/mutation-survived'; fi

  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-466.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur466_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR466UnitBridge(t *testing.T) { TestAUR466(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR466UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR466:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR466:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-466.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur466_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR466IntegrationBridge(t *testing.T) { IntegrationAUR466(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR466IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR466:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR466:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-466.sh
  chmod -R u+w -- "$root"
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-466.sh E2EAUR466) || fail e2e-failed
  cleanup_root "$root"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  mutation_case_1
  mutation_case_2
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR466) unit_case ;;
  IntegrationAUR466) integration_case ;;
  E2EAUR466) e2e_case ;;
  AC-001-MUT-001) mutation_case_1 ;;
  AC-001-MUT-002) mutation_case_2 ;;
esac
