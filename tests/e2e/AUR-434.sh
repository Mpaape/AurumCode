#!/usr/bin/env bash
# E2E check for AUR-434: build the real aurumcode binary and run it, as a
# user would, against the tests/fixtures/repos/git-demo bare repository,
# with the LLM call pinned to deterministic offline fixtures. Proves that
# every printed problem cites the sustaining rule from the embedded project
# review standard and that an ungrounded finding never reaches the user.
# See docs/specs/AUR-434.md for the command reference.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-434
selector="${1:-E2EAUR434}"
[[ "$selector" == "E2EAUR434" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
known_fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
test -d "$repo_dir" || infra missing_fixture_repo
test -s "$known_fixture" || infra missing_fixture_response

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a434.XXXXXX")" || infra mktemp
# Never let a removal error override an already-decided result (see
# tests/acceptance/AUR-430.sh's cleanup_root for why the chmod).
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-434.sh's e2e_case does).
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS=-mod=mod go build -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# Fixtures this script plants itself: one grounded finding plus two
# ungrounded ones (no rule id / nonexistent rule id), and one response with
# only an ungrounded finding.
mixed_fixture="$run_dir/mixed.json"
cat >"$mixed_fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "UNGROUNDED-BAD-RULE planted finding citing a nonexistent rule."
    }
  ],
  "summary": "Mixed fixture for AUR-434."
}
EOF

ungrounded_fixture="$run_dir/ungrounded.json"
cat >"$ungrounded_fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    }
  ],
  "summary": "Only ungrounded findings."
}
EOF

run_once() {
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$1" "$bin" review --base HEAD~1)
}

# The AUR-430 known-problem fixture already cites a catalog rule: its
# finding must now print with the rule citation appended.
out_known="$(run_once "$known_fixture")" || fail run_failed
grep -Fq 'config/demo-tokens.txt' <<<"$out_known" || fail missing_expected_finding
grep -Fq '[error]' <<<"$out_known" || fail missing_expected_severity
grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' <<<"$out_known" || fail missing_rule_citation

# Mixed response: the grounded finding survives with its citation; the
# ungrounded findings never reach the user.
out1="$(run_once "$mixed_fixture")" || fail run_failed
grep -Fq 'config/demo-tokens.txt:3' <<<"$out1" || fail missing_grounded_finding
grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' <<<"$out1" || fail missing_rule_citation
if grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out1"; then fail ungrounded_no_rule_reached_user; fi
if grep -Fq 'UNGROUNDED-BAD-RULE' <<<"$out1"; then fail ungrounded_bad_rule_reached_user; fi

out2="$(run_once "$mixed_fixture")" || fail rerun_failed
[[ "$out1" == "$out2" ]] || fail non_deterministic

# Every finding ungrounded: the unchanged AUR-430 no-findings output.
out3="$(run_once "$ungrounded_fixture")" || fail run_failed
grep -Fq 'No issues found.' <<<"$out3" || fail missing_no_issues_output
if grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out3"; then fail ungrounded_no_rule_reached_user; fi

printf '%s/AC-001/E2EAUR434/ok\n' "$card"
