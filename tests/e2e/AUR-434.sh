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
# only an ungrounded finding. All four carry evidence/impact/verification
# (AUR-541) -- including the three ungrounded ones -- so the
# scope-and-evidence gate (internal/review/scope.go) cannot be what
# removes them; only the rule-citation gate this card is actually about
# (enforceRuleCitations, below) can, and its exact wording is asserted.
mixed_fixture="$run_dir/mixed.json"
cat >"$mixed_fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text.",
      "evidence": "The added line stores the value as a literal string assignment.",
      "impact": "A reader with repository access can extract and reuse the value.",
      "verification": "Replace the literal with an environment lookup and confirm the finding clears."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id.",
      "evidence": "A linha citada nao traz nenhum identificador de regra.",
      "impact": "Um achado sem regra nao pode ser rastreado ao padrao de revisao do projeto.",
      "verification": "Confirmar que o pipeline de citacao de regra rejeita achados sem rule_id."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "UNGROUNDED-BAD-RULE planted finding citing a nonexistent rule.",
      "evidence": "A linha citada aponta para um identificador de regra que nao existe no catalogo.",
      "impact": "Um achado com regra inventada nao pode ser verificado contra o padrao publicado.",
      "verification": "Confirmar que o pipeline de citacao de regra rejeita achados com rule_id desconhecido."
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
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id.",
      "evidence": "A linha citada nao traz nenhum identificador de regra.",
      "impact": "Um achado sem regra nao pode ser rastreado ao padrao de revisao do projeto.",
      "verification": "Confirmar que o pipeline de citacao de regra rejeita achados sem rule_id."
    }
  ],
  "summary": "Only ungrounded findings."
}
EOF

run_once() {
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$1" "$bin" review --base HEAD~1) 2>"$run_dir/stderr-last.txt"
}
last_stderr() { cat "$run_dir/stderr-last.txt"; }

# The AUR-430 known-problem fixture already cites a catalog rule: its
# finding must now print with the rule citation appended.
out_known="$(run_once "$known_fixture")" || fail run_failed
grep -Fq 'config/demo-tokens.txt' <<<"$out_known" || fail missing_expected_finding
grep -Fq '[error]' <<<"$out_known" || fail missing_expected_severity
grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' <<<"$out_known" || fail missing_rule_citation

# Mixed response: the grounded finding survives with its citation; the
# ungrounded findings never reach the user. Asserting the rule gate's own
# stderr wording (formatDiscardWarning, internal/review/reviewer.go) rules
# out the scope/evidence gate having removed these instead -- which would
# make this test pass even with enforceRuleCitations disabled.
out1="$(run_once "$mixed_fixture")" || fail run_failed
err1="$(last_stderr)"
grep -Fq 'config/demo-tokens.txt:3' <<<"$out1" || fail missing_grounded_finding
grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' <<<"$out1" || fail missing_rule_citation
if grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out1"; then fail ungrounded_no_rule_reached_user; fi
if grep -Fq 'UNGROUNDED-BAD-RULE' <<<"$out1"; then fail ungrounded_bad_rule_reached_user; fi
grep -Fq 'with no rule_id' <<<"$err1" || fail missing_rule_gate_no_rule_id_reason
grep -Fq 'citing an unknown rule_id (security/definitely-not-a-rule)' <<<"$err1" || fail missing_rule_gate_unknown_rule_id_reason
if grep -Fq 'descartado(s) pelo gate de escopo e evidencia' <<<"$err1"; then
  fail scope_gate_removed_instead_of_rule_gate
fi

out2="$(run_once "$mixed_fixture")" || fail rerun_failed
[[ "$out1" == "$out2" ]] || fail non_deterministic

# Every finding ungrounded: the unchanged AUR-430 no-findings output, with
# the rule gate's own reason on stderr (not the scope gate's).
out3="$(run_once "$ungrounded_fixture")" || fail run_failed
err3="$(last_stderr)"
grep -Fq 'No issues found.' <<<"$out3" || fail missing_no_issues_output
if grep -Fq 'UNGROUNDED-NO-RULE' <<<"$out3"; then fail ungrounded_no_rule_reached_user; fi
grep -Fq 'with no rule_id' <<<"$err3" || fail missing_rule_gate_reason
if grep -Fq 'descartado(s) pelo gate de escopo e evidencia' <<<"$err3"; then
  fail scope_gate_removed_instead_of_rule_gate
fi

printf '%s/AC-001/E2EAUR434/ok\n' "$card"
