#!/usr/bin/env bash
#
# Acceptance program for card AUR-462, scenario AC-001.
#
# WHAT THIS PROVES
#
#   The 2026-08-14 measurement found the deterministic security pass
#   ("aurumcode review --base HEAD~1 --seguranca") caught a Node
#   repository's planted secret and SQL injection but missed
#   `exec("ping -c 1 " + host)` (child_process.exec with concatenation) and
#   `innerHTML = userInput` (a direct, unescaped write): the causes were
#   security/command-injection's pattern requiring an `l`/`v` right after
#   `exec` (matching only C/Python's execl/execv/execve, never Node's
#   canonical exec/execSync/spawn-with-shell) and security/xss carrying no
#   pattern at all. This program proves the fix
#   (internal/review/rules/security.yml): Node's exec(), execSync(), and
#   spawn(..., {shell: true}) are now matched by command-injection, and a
#   bare-identifier write into innerHTML/outerHTML/document.write/
#   dangerouslySetInnerHTML is now matched by xss (AC-001); the three named
#   false-positive shapes -- an argv-form exec call, an innerHTML literal,
#   and "exec" merely mentioned in a comment -- are never matched (AC-002);
#   and the Python fixture that already produced a finding still produces
#   exactly that finding, unchanged (AC-003).
#
# WHY THIS RUNS AGAINST AN EPHEMERAL REPOSITORY, NOT A COMMITTED FIXTURE
#
#   This card's `paths` own internal/review/rules and its own
#   unit/integration/e2e/acceptance programs -- no fixtures directory
#   (unlike AUR-442, which owned tests/fixtures/review/vuln/hardcoded-secret
#   in its own `paths`). So nothing Node-shaped is committed to the
#   repository tree: nominal_case, mutation_case and e2e_case each
#   materialize the same 27-line Node source (via `git init`/`git commit`
#   in a throwaway directory) that tests/unit/AUR-462.go's
#   aur462NodeFixtureLines also embeds as a synthetic diff, so all three
#   layers agree on line numbers.
#
# WHY NO LLM FIXTURE IS NEEDED
#
#   `--seguranca` without --modelo and with no LLM_API_KEY/LLM_BASE_URL/
#   AURUMCODE_LLM_FIXTURE configured takes the engine's own AUR-449 path:
#   quality review is skipped (stderr says so) and only the deterministic,
#   model-free security pass runs, exit 0. This program unsets all three so
#   every run takes that path regardless of the ambient environment.
#
# SIBLING REGRESSIONS THIS CARD'S COVERAGE CHANGE CAUSED, AND THEIR FIX
#
#   Giving security/xss a pattern moved it out of
#   tests/unit/AUR-442.go::testAUR442DeclaredOnlyRulesStayPatternless's
#   declaredOnly list and into a new positive assertion beside it -- the
#   guard against an undelivered pattern still applies to the four rules
#   that remain patternless, unchanged. The coverage line moving from "3
#   of 8" to "4 of 8" (the real count, derived from
#   internal/review/rules/*.yml, not assumed) is now the literal
#   tests/unit/AUR-450.go, tests/integration/AUR-450.go,
#   tests/e2e/AUR-450.sh and tests/acceptance/AUR-450.sh pin. Both are now
#   in this card's `paths` and both are fixed here, not merely disclosed
#   -- see docs/specs/AUR-462.md's own section of this name for detail,
#   including the unrelated pre-existing AUR-458 conflict
#   tests/acceptance/AUR-450.sh AC-001 still hits (out of this card's
#   contract; reported there, not patched).
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving MUT-001/MUT-002 mutant)
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure: an input this card does not own was
#        never materialized, a required tool is missing. Never valid red
#        evidence, never a pass.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card='AUR-462'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR462|IntegrationAUR462|E2EAUR462|AC-001-MUT-001|AC-001-MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
command -v git >/dev/null 2>&1 || infra missing_git
command -v python3 >/dev/null 2>&1 || infra missing_python3

# Input preflight. Deliverables this card owns fail behavioral (their
# absence IS the missing behavior); everything else is an environment gap.
owned_inputs=(
  internal/review/rules/security.yml
  tests/unit/AUR-462.go
  tests/integration/AUR-462.go
  tests/e2e/AUR-462.sh
)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
required_inputs=(
  go.mod
  go.sum
  cmd/aurumcode
  internal/analyzer
  internal/llm
  internal/prompt
  internal/review
  internal/security/redaction
  pkg/types
  tests/fixtures/review/vuln/repo.git
)
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a462.XXXXXX")" || infra mktemp
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

# No LLM provider may be configured: --seguranca alone must take the
# deterministic no-provider path (AUR-449) on every run this program makes.
unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly what `go build ./cmd/aurumcode` needs:
# this card's owned paths plus the read-only packages the engine imports,
# and the Python fixture AC-003 regresses against.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode internal/analyzer internal/prompt internal/review internal/security
  copy "$root" internal/git internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome internal/pipeline
  copy "$root" cmd/regenerate-docs
  copy "$root" pkg/types internal/llm
  copy "$root" tests/fixtures/review/vuln
  chmod -R u+w -- "$root"
}

# make_node_repo materializes the ephemeral Node fixture under $1/node-repo
# and echoes its path: the exact 27-line source tests/unit/AUR-462.go's
# aur462NodeFixtureLines and tests/integration/AUR-462.go's mirror embed,
# so line numbers agree everywhere.
make_node_repo() {
  local base="$1"
  local repo="$base/node-repo"
  mkdir -p "$repo/src"
  (
    export GIT_AUTHOR_NAME='Aurum Test' GIT_AUTHOR_EMAIL='aurum-test@aurum.invalid'
    export GIT_COMMITTER_NAME='Aurum Test' GIT_COMMITTER_EMAIL='aurum-test@aurum.invalid'
    git -C "$repo" init -q -b main
    git -C "$repo" config user.email aurum-test@aurum.invalid
    git -C "$repo" config user.name 'Aurum Test'
    printf 'Ephemeral AUR-462 Node fixture. Nothing here is a real application.\n' >"$repo/README.md"
    git -C "$repo" add README.md
    git -C "$repo" commit -q -m 'seed: add the fixture skeleton'
    cat >"$repo/src/app.js" <<'NODEJS'
"use strict";
const { exec, execSync, spawn } = require("child_process");

function pingConcat(host) {
  exec("ping -c 1 " + host);
}

function pingSyncConcat(host) {
  execSync("ping -c 1 " + host);
}

function pingArgv(host) {
  exec(["ping", host]);
}

function pingShell(host) {
  spawn("ping -c 1 " + host, { shell: true });
}

// exec is dangerous when combined with string concatenation.
function renderUser(userInput) {
  document.getElementById("out").innerHTML = userInput;
}

function renderStatic() {
  document.getElementById("out").innerHTML = "<b>Static content</b>";
}

function renderSanitized(userInput) {
  document.getElementById("out").innerHTML = DOMPurify.sanitize(userInput);
}

function renderTrustedTemplate() {
  document.getElementById("out").innerHTML = TRUSTED_TEMPLATE;
}

function renderEscaped(x) {
  document.getElementById("out").innerHTML = escapeHtml(x);
}
NODEJS
    git -C "$repo" add src/app.js
    git -C "$repo" commit -q -m 'add: node source with planted command-injection and xss shapes'
  ) >"$base/git.log" 2>&1 || { cat "$base/git.log" >&2; infra git_fixture_failed; }
  printf '%s' "$repo"
}

readonly header='Security findings (standards/security-review):'
readonly cmd_citation='(rule security/command-injection: Command Injection)'
readonly xss_citation='(rule security/xss: Cross-Site Scripting (XSS))'

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

# nominal_case is AC-001's + AC-002's + AC-003's core behavioral proof: run
# the built binary exactly as a user would, against the ephemeral Node
# fixture and against the project's own already-committed Python
# regression fixture.
nominal_case() {
  build_shared
  local node_repo; node_repo="$(make_node_repo "$run_dir/nominal")"

  local out_sec
  out_sec="$(cd "$node_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
  grep -Fq "$header" <<<"$out_sec" || fail behavior-missing
  grep -Fq 'src/app.js:5: [error]' <<<"$out_sec" || fail behavior-missing:exec-concat
  grep -Fq 'src/app.js:9: [error]' <<<"$out_sec" || fail behavior-missing:execsync-concat
  grep -Fq 'src/app.js:17: [error]' <<<"$out_sec" || fail behavior-missing:spawn-shell-true
  grep -Fq 'src/app.js:22: [error]' <<<"$out_sec" || fail behavior-missing:innerhtml-direct-write

  # AC-002: the three named false-positive shapes never appear.
  if grep -Fq 'src/app.js:13: [error]' <<<"$out_sec"; then fail false-positive:argv-exec; fi
  if grep -Fq 'src/app.js:20: [error]' <<<"$out_sec"; then fail false-positive:comment-exec; fi
  if grep -Fq 'src/app.js:26: [error]' <<<"$out_sec"; then fail false-positive:innerhtml-literal; fi

  # Adversarial-review proof (2026-08-23): a sanitizer call, an uppercase
  # module constant, and an escaping helper call must never appear either.
  if grep -Fq 'src/app.js:30: [error]' <<<"$out_sec"; then fail false-positive:sanitizer-call; fi
  if grep -Fq 'src/app.js:34: [error]' <<<"$out_sec"; then fail false-positive:uppercase-constant; fi
  if grep -Fq 'src/app.js:38: [error]' <<<"$out_sec"; then fail false-positive:escape-helper-call; fi

  local after_header cmd_count xss_count
  after_header="${out_sec#*"$header"}"
  cmd_count="$(grep -Fo "$cmd_citation" <<<"$after_header" | wc -l)"
  xss_count="$(grep -Fo "$xss_citation" <<<"$after_header" | wc -l)"
  [[ "$cmd_count" -eq 3 ]] || fail "unexpected-command-injection-count:$cmd_count"
  [[ "$xss_count" -eq 1 ]] || fail "unexpected-xss-count:$xss_count"

  # Determinism.
  local out_again
  out_again="$(cd "$node_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
  [[ "$out_sec" == "$out_again" ]] || fail non-deterministic

  # AC-003: the Python fixture's pre-existing finding survives unchanged.
  local py_repo="$shared_root/tests/fixtures/review/vuln/repo.git"
  local out_py
  out_py="$(cd "$py_repo" && "$shared_bin" review --base HEAD~1 --seguranca)" || fail behavior-missing:python-regression
  grep -Fq 'src/db.py:8: [error]' <<<"$out_py" || fail regression:python-sql-injection-missing
  grep -Fq '(rule security/sql-injection: SQL Injection Vulnerability)' <<<"$out_py" || fail regression:python-citation-missing
  [[ "$(grep -Fo '[error]' <<<"$out_py" | wc -l)" -eq 1 ]] || fail regression:python-finding-count-changed

  # Without --seguranca: no security section leaks in.
  local out_base
  out_base="$(cd "$node_repo" && "$shared_bin" review --base HEAD~1 2>/dev/null || true)"
  if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi
}

# mutation_case_1 is MUT-001: neutralizing the Node command-injection
# addition (reverting security/command-injection's pattern to its
# pre-AUR-462 form) must make the three Node exec findings vanish -- the
# same behavior a real regression would show, and exactly what nominal_case
# would have to catch to keep AC-001 honest.
mutation_case_1() {
  build_shared
  local root="$run_dir/root-mut1"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-001/target-missing'
  local anchor="pattern: '(?i)\\b(system|popen|exec[lv]p?e?|subprocess\\.(run|call|Popen))\\s*\\(.*[\"'']\\s*\\+|\\bexec(Sync)?\\s*\\(.*[\"'']\\s*\\+|\\bspawn\\s*\\(.*shell\\s*:\\s*true\\b'"
  local replacement="pattern: '(?i)\\b(system|popen|exec[lv]p?e?|subprocess\\.(run|call|Popen))\\s*\\(.*[\"'']\\s*\\+'"
  grep -Fq "$anchor" "$target" || fail 'MUT-001/anchor-not-found'
  python3 - "$target" "$anchor" "$replacement" <<'PYEOF' || fail 'MUT-001/rewrite-failed'
import sys
path, anchor, replacement = sys.argv[1:4]
s = open(path).read()
assert s.count(anchor) == 1
open(path, "w").write(s.replace(anchor, replacement))
PYEOF
  grep -Fq "$anchor" "$target" && fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut1"
  local log="$root/build-mut1.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  local node_repo; node_repo="$(make_node_repo "$run_dir/mut1")"
  local out
  out="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-001/mutation-run-failed'
  grep -Fq "$header" <<<"$out" || fail 'MUT-001/pass-did-not-run'
  # The mutant must lose the three exec findings -- nominal_case's presence
  # assertions for lines 5, 9, 17 would fail under this mutant.
  if grep -Fq 'src/app.js:5: [error]' <<<"$out"; then fail 'MUT-001/mutation-survived:line-5'; fi
  if grep -Fq 'src/app.js:9: [error]' <<<"$out"; then fail 'MUT-001/mutation-survived:line-9'; fi
  if grep -Fq 'src/app.js:17: [error]' <<<"$out"; then fail 'MUT-001/mutation-survived:line-17'; fi

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# mutation_case_2 is MUT-002: giving security/xss a pattern that matches
# ANY innerHTML assignment, including a literal constant, must make the
# AC-002 false-positive line (src/app.js:26, a literal) start appearing --
# exactly the false positive AC-002 forbids, and exactly what nominal_case
# would have to catch to keep AC-002 honest.
mutation_case_2() {
  build_shared
  local root="$run_dir/root-mut2"
  stage_source "$root"

  local target="$root/internal/review/rules/security.yml"
  [[ -f "$target" ]] || fail 'MUT-002/target-missing'
  local anchor="pattern: '\\b(?i:innerHTML|outerHTML)\\s*=\\s*[a-z_\$][\\w\$.]*\\s*(?:;|\$)|(?i:document\\.write)\\s*\\(\\s*[a-z_\$][\\w\$.]*\\s*\\)|(?i:dangerouslySetInnerHTML)\\s*=\\s*\\{\\{\\s*__html\\s*:\\s*[a-z_\$][\\w\$.]*\\s*\\}\\}'"
  local replacement="pattern: '(?i)\\b(innerHTML|outerHTML)\\s*='"
  grep -Fq "$anchor" "$target" || fail 'MUT-002/anchor-not-found'
  python3 - "$target" "$anchor" "$replacement" <<'PYEOF' || fail 'MUT-002/rewrite-failed'
import sys
path, anchor, replacement = sys.argv[1:4]
s = open(path).read()
assert s.count(anchor) == 1
open(path, "w").write(s.replace(anchor, replacement))
PYEOF
  grep -Fq "$anchor" "$target" && fail 'MUT-002/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut2"
  local log="$root/build-mut2.log"
  if ! (cd "$root" && go build -o "$bin" ./cmd/aurumcode) >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-002/build-failed'
  fi

  local node_repo; node_repo="$(make_node_repo "$run_dir/mut2")"
  local out
  out="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail 'MUT-002/mutation-run-failed'
  grep -Fq "$header" <<<"$out" || fail 'MUT-002/pass-did-not-run'
  # The mutant must gain the forbidden false positive on the literal line.
  grep -Fq 'src/app.js:26: [error]' <<<"$out" || fail 'MUT-002/mutation-did-not-surface-false-positive'

  cleanup_root "$root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-462.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/unit/aur462_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR462UnitBridge(t *testing.T) { TestAUR462(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR462UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR462:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR462:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-462.go
  chmod -R u+w -- "$root"
  cat >"$root/tests/integration/aur462_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR462IntegrationBridge(t *testing.T) { IntegrationAUR462(t) }
EOF
  local out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" go test -v -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR462IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR462:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR462:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-462.sh
  chmod -R u+w -- "$root"
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-462.sh E2EAUR462) || fail e2e-failed
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
  TestAUR462) unit_case ;;
  IntegrationAUR462) integration_case ;;
  E2EAUR462) e2e_case ;;
  AC-001-MUT-001) mutation_case_1 ;;
  AC-001-MUT-002) mutation_case_2 ;;
esac
