#!/usr/bin/env bash
# E2E check for AUR-462: build (or reuse) the real aurumcode binary and run
# it, as a user would, against an EPHEMERAL Node git repository this script
# creates on the fly with `git init`/`git commit` (this card's `paths` own
# no fixtures directory, unlike AUR-442's committed hardcoded-secret
# fixture, so nothing Node-shaped is committed to the tree at all).
#
# WHAT THIS PROVES, DISTINCT FROM tests/unit/AUR-462.go (package-boundary,
# synthetic types.Diff) AND tests/integration/AUR-462.go (CLI-boundary,
# stdout content assertions):
#
#   The 2026-08-14 measurement found the security pass caught a Node repo's
#   planted secret and SQL injection but missed `exec("ping -c 1 " + host)`
#   (child_process.exec concatenation) and `innerHTML = userInput` (a
#   direct write), because security/command-injection's pattern required an
#   `l`/`v` right after `exec` (matching only execl/execv/execve) and
#   security/xss carried no pattern at all. This script proves, at the
#   full-process boundary: the four Node forms named in the card
#   (exec/execSync concatenation, spawn with shell:true, innerHTML direct
#   write) are found; the three AC-002 false-positive shapes (argv-form
#   exec, an innerHTML literal, "exec" inside a comment) are not; the
#   citation COUNTS per rule are exact (3 command-injection, 1 xss); the
#   run is deterministic; and, distinct from the integration program's own
#   check, that a Node repository carrying only these findings now closes
#   the `--fail-on high` gate with exit 3 -- it could not before this card,
#   because nothing in a Node repo's exec/innerHTML shapes ever matched.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-462
selector="${1:-E2EAUR462}"
[[ "$selector" == "E2EAUR462" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go
command -v git >/dev/null 2>&1 || infra missing_git

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a462.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

# No LLM provider may be configured for these runs: --seguranca alone must
# take the deterministic no-provider path (AUR-449), never a live model
# call, so this script's result depends on nothing outside the process it
# starts.
unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS='-mod=mod -p=1' go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# Materialize the ephemeral Node fixture: same 27-line content and line
# numbers as tests/unit/AUR-462.go's aur462NodeFixtureLines and
# tests/integration/AUR-462.go's aur462NodeFixtureLines, so all three
# selectors agree on what "line 5", "line 9", "line 17", "line 22" mean.
node_repo="$run_dir/node-repo"
mkdir -p "$node_repo/src"
export GIT_AUTHOR_NAME='Aurum Test' GIT_AUTHOR_EMAIL='aurum-test@aurum.invalid'
export GIT_COMMITTER_NAME='Aurum Test' GIT_COMMITTER_EMAIL='aurum-test@aurum.invalid'
git -C "$node_repo" init -q -b main || infra git_init_failed
git -C "$node_repo" config user.email aurum-test@aurum.invalid
git -C "$node_repo" config user.name 'Aurum Test'
printf 'Ephemeral AUR-462 Node fixture. Nothing here is a real application.\n' >"$node_repo/README.md"
git -C "$node_repo" add README.md
git -C "$node_repo" commit -q -m 'seed: add the fixture skeleton'

cat >"$node_repo/src/app.js" <<'NODEJS'
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
git -C "$node_repo" add src/app.js
git -C "$node_repo" commit -q -m 'add: node source with planted command-injection and xss shapes'

header='Security findings (standards/security-review):'
cmd_citation='(rule security/command-injection: Command Injection)'
xss_citation='(rule security/xss: Cross-Site Scripting (XSS))'

# AC-001: the four planted Node forms are found, each on its exact line.
out_sec="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail behavior-missing
grep -Fq "$header" <<<"$out_sec" || fail security-section-missing
grep -Fq 'src/app.js:5: [error]' <<<"$out_sec" || fail exec-concat-not-found
grep -Fq 'src/app.js:9: [error]' <<<"$out_sec" || fail execsync-concat-not-found
grep -Fq 'src/app.js:17: [error]' <<<"$out_sec" || fail spawn-shell-true-not-found
grep -Fq 'src/app.js:22: [error]' <<<"$out_sec" || fail innerhtml-direct-write-not-found

# AC-002: the three named false-positive shapes never appear -- an
# argv-form exec (line 13), an innerHTML literal (line 26), and "exec"
# inside a comment (line 20).
grep -Fq 'src/app.js:13: [error]' <<<"$out_sec" && fail argv-exec-false-positive
grep -Fq 'src/app.js:20: [error]' <<<"$out_sec" && fail comment-exec-false-positive
grep -Fq 'src/app.js:26: [error]' <<<"$out_sec" && fail innerhtml-literal-false-positive

# Adversarial-review proof (2026-08-23): the first cut of the xss pattern
# anchored only the START of the RHS, so it matched any call whose callee
# name begins with an identifier character -- including the canonical XSS
# mitigation itself. A sanitizer call (line 30, DOMPurify.sanitize), an
# uppercase module constant (line 34, TRUSTED_TEMPLATE), and an escaping
# helper call (line 38, escapeHtml) must never appear.
grep -Fq 'src/app.js:30: [error]' <<<"$out_sec" && fail sanitizer-call-false-positive
grep -Fq 'src/app.js:34: [error]' <<<"$out_sec" && fail uppercase-constant-false-positive
grep -Fq 'src/app.js:38: [error]' <<<"$out_sec" && fail escape-helper-call-false-positive

# Exact citation counts: three command-injection findings (lines 5, 9,
# 17), one xss finding (line 22). A pattern that over-matches would
# silently inflate one of these two counts even if the four required
# lines above are all present.
after_header="${out_sec#*"$header"}"
cmd_count="$(grep -Fo "$cmd_citation" <<<"$after_header" | wc -l)"
xss_count="$(grep -Fo "$xss_citation" <<<"$after_header" | wc -l)"
[[ "$cmd_count" -eq 3 ]] || fail "unexpected-command-injection-count:$cmd_count"
[[ "$xss_count" -eq 1 ]] || fail "unexpected-xss-count:$xss_count"

# Determinism: same input, same bytes.
out_again="$(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca)" || fail rerun-failed
[[ "$out_sec" == "$out_again" ]] || fail non-deterministic

# Distinct from the integration program's own check: composed with
# --fail-on, the Node repo's findings (all severity error) now close the
# gate. Before this card nothing in this repo's exec/innerHTML shapes ever
# matched, so this gate could not have closed on Node content like this.
set +e
(cd "$node_repo" && "$bin" review --base HEAD~1 --seguranca --fail-on high) >/dev/null 2>"$run_dir/failon.err"
rc=$?
set -e
[[ "$rc" -eq 3 ]] || fail "fail-on-gate-did-not-close:$rc"

# Without --seguranca: no security section leaks in regardless of exit
# code (see tests/integration/AUR-462.go for why exit code itself is out
# of scope here -- this card never touches that path).
out_base="$(cd "$node_repo" && "$bin" review --base HEAD~1 2>/dev/null || true)"
if grep -Fq "$header" <<<"$out_base"; then fail base-grew-a-security-section; fi

printf '%s/AC-001/e2e-ok\n' "$card"
