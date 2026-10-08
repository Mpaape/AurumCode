#!/usr/bin/env bash
# E2E check for AUR-448: build (or reuse) the real aurumcode binary and run
# it, as a user would, against the tests/fixtures/repos/git-demo bare
# repository, proving both halves of this card's outcome: the no-provider
# message shows the complete fixture shape (rule_id included, with a real
# catalog id as the example), and a discard the rule gate (AUR-434) makes
# is named on stderr -- never silent, never on stdout. See
# docs/specs/AUR-448.md.
#
# This card's TDD proof section names the e2e selector E2EAUR435; that name
# is reserved by tests/e2e/AUR-435.sh (a different card). This file answers
# to E2EAUR448 instead -- see docs/specs/AUR-448.md's "A note on this
# card's own TDD-proof identifiers".
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-448
selector="${1:-E2EAUR448}"
[[ "$selector" == "E2EAUR448" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
known_problem_fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
test -d "$repo_dir" || infra missing_git_demo_fixture
test -s "$known_problem_fixture" || infra missing_known_problem_fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a448.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: never let a removal error
# override an already-decided result.
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-448.sh's e2e_case does).
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  bin="$AURUMCODE_BIN"
  test -x "$bin" || infra missing_prebuilt_binary
else
  bin="$run_dir/aurumcode"
  build_log="$run_dir/build.log"
  if ! (cd "$repo_root" && GOFLAGS=-mod=mod go build -buildvcs=false -o "$bin" ./cmd/aurumcode) >"$build_log" 2>&1; then
    cat "$build_log" >&2
    fail build_failed
  fi
fi

# run_bin runs the binary as a user would, from the given directory, with a
# clean environment (no inherited provider variables, no canary, no shared
# review cache -- a cache hit skips GenerateReview entirely and would hide
# a real discard) plus any extra KEY=VALUE pairs given anywhere among the
# remaining arguments. Raw exit code lands in rc; stdout/stderr land in
# separate files -- they travel through independent writers in
# cmd/aurumcode/main.go, so nothing here may assume their interleaving in a
# combined capture.
run_bin() {
  local dir="$1"; shift
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

# --- 1. No provider configured: the message shows the complete shape. ---

# AUR-542: AUR-490 (done, integrated on main before this card) made
# `review --base` without a provider run deterministic analysis and exit
# 0 instead of 1 unconditionally (opt into the old exit 1 with
# --exigir-qualidade, AUR-458); stdout also now always carries the
# summary/diagram (AUR-490 AC-002) instead of staying empty. Measured on
# main before this card, that change also silently dropped the complete
# fixture-shape teaching text this scenario exists to check (AUR-490's
# short skip note on its own names none of rule_id/severity/the catalog
# example/the fixture pointer) -- restored in
# cmd/aurumcode/main.go's qualitySkipped branch by also printing
# errNoProviderConfigured's own text, additively, alongside the short
# note AUR-490 added.
run_bin "$repo_dir" review --base HEAD~1
[[ "$rc" -eq 0 ]] || fail "no_provider_wrong_exit:$rc"
grep -Fq 'no LLM provider configured' "$run_dir/out.stderr" || fail no_provider_message_missing
grep -Fq '"rule_id"' "$run_dir/out.stderr" || fail no_provider_missing_rule_id_field
grep -Fq 'security/hardcoded-secret' "$run_dir/out.stderr" || fail no_provider_missing_real_rule_id_example
grep -Fq '"severity"' "$run_dir/out.stderr" || fail no_provider_missing_fixture_shape
grep -Fq 'tests/fixtures/review/known-problem-response.json' "$run_dir/out.stderr" || fail no_provider_missing_fixture_pointer
grep -Fq 'discarded' "$run_dir/out.stderr" || fail no_provider_missing_discard_explanation

# --- 2. Happy path: zero discards, the finding still prints, EMPTY stderr. ---

# AUR-542: AUR-490 (done, integrated before this card) made `review --base`
# always prepend the summary/mermaid-diagram block (AC-002), the same way
# --pr already did -- so stdout is no longer byte-identical to just the
# finding line; the finding line is still the exact, unchanged tail.
run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$known_problem_fixture"
[[ "$rc" -eq 0 ]] || fail "happy_path_wrong_exit:$rc"
want_happy_tail='config/demo-tokens.txt:4: [error] A credential-shaped value was committed in plain text (DEMO_API_TOKEN). (rule security/hardcoded-secret: Hardcoded Secrets)'
# AUR-514 (AC-002: os mesmos campos permanecem presentes no terminal)
# made the terminal print the finding's labeled detail lines ("  - Impact:
# ...", "  - Evidence: ...") right under it (cmd/aurumcode/finding_fields.go).
# The finding line is still the exact last line once those indented detail
# lines are set aside -- so any other line after it still fails this.
[[ "$(grep -v '^  - ' "$run_dir/out.stdout" | tail -n1)" == "$want_happy_tail" ]] || fail happy_path_stdout_regressed
grep -Fq '```mermaid' "$run_dir/out.stdout" || fail happy_path_missing_summary_block
[[ ! -s "$run_dir/out.stderr" ]] || fail "happy_path_stderr_not_empty:$(cat "$run_dir/out.stderr")"
# A reviewer found that tail -n1 alone lets an EXTRA, undetected finding
# line leak earlier in stdout (a second planted finding before the real
# one keeps this check green). Exactly one finding went in, so exactly
# one line shaped like a finding ("<file>:<line>: [<severity>] ...") may
# appear anywhere in stdout.
finding_lines="$(grep -Ec '^[^ ]+:[0-9]+: \[' "$run_dir/out.stdout")"
[[ "$finding_lines" -eq 1 ]] || fail "happy_path_wrong_finding_line_count:$finding_lines"

# --- 3. Mixed discard: stdout hides ungrounded findings, stderr names how many and why. ---

# All three entries carry evidence/impact/verification (AUR-541), the two
# ungrounded ones included, so the scope/evidence gate cannot be what
# removes them -- only the rule-citation gate (AUR-434) can, matching the
# exact want_mixed_stderr wording asserted below.
mixed_fixture="$run_dir/mixed.json"
cat >"$mixed_fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "grounded",
      "evidence": "a linha adicionada grava um valor de credencial como literal",
      "impact": "qualquer leitor do repositorio pode reaproveitar esse valor",
      "verification": "remover o literal e confirmar que o achado desaparece"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all",
      "evidence": "a linha citada nao traz nenhum identificador de regra",
      "impact": "um achado sem regra nao pode ser rastreado ao padrao de revisao",
      "verification": "confirmar que o gate de citacao de regra rejeita achados sem rule_id"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "unknown rule_id",
      "evidence": "a linha citada aponta para um identificador de regra que nao existe no catalogo",
      "impact": "um achado com regra inventada nao pode ser verificado contra o padrao publicado",
      "verification": "confirmar que o gate de citacao de regra rejeita achados com rule_id desconhecido"
    }
  ],
  "summary": "Mixed fixture for AUR-448 e2e."
}
EOF

run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$mixed_fixture"
[[ "$rc" -eq 0 ]] || fail "mixed_wrong_exit:$rc"
grep -Fq 'config/demo-tokens.txt:3' "$run_dir/out.stdout" || fail mixed_missing_grounded_finding
grep -Fq '(rule security/hardcoded-secret: Hardcoded Secrets)' "$run_dir/out.stdout" || fail mixed_missing_rule_citation
if grep -Fq 'no rule_id at all' "$run_dir/out.stdout"; then fail mixed_ungrounded_reached_stdout; fi
if grep -Fq 'unknown rule_id' "$run_dir/out.stdout"; then fail mixed_ungrounded_reached_stdout; fi
want_mixed_stderr='aurumcode review: 2 finding(s) discarded: 1 with no rule_id, 1 citing an unknown rule_id (security/definitely-not-a-rule)'
[[ "$(cat "$run_dir/out.stderr")" == "$want_mixed_stderr" ]] || fail mixed_stderr_mismatch

mixed_stdout_1="$(cat "$run_dir/out.stdout")"
mixed_stderr_1="$(cat "$run_dir/out.stderr")"
run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$mixed_fixture"
[[ "$rc" -eq 0 ]] || fail "mixed_rerun_wrong_exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$mixed_stdout_1" ]] || fail mixed_stdout_non_deterministic
[[ "$(cat "$run_dir/out.stderr")" == "$mixed_stderr_1" ]] || fail mixed_stderr_non_deterministic

# --- 4. Every finding discarded: the exact defect this card fixes. ---
#
# Before AUR-448 this produced "No issues found." with NOTHING on stderr --
# a confidently wrong "your code is clean" for a fixture that planted a
# real finding.

# This single finding carries evidence/impact/verification (AUR-541) so
# only the rule-citation gate removes it, matching
# want_all_discarded_stderr's exact wording below.
all_discarded_fixture="$run_dir/all-discarded.json"
cat >"$all_discarded_fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all",
      "evidence": "a linha citada nao traz nenhum identificador de regra",
      "impact": "um achado sem regra nao pode ser rastreado ao padrao de revisao",
      "verification": "confirmar que o gate de citacao de regra rejeita achados sem rule_id"
    }
  ],
  "summary": "All-discarded fixture for AUR-448 e2e."
}
EOF

run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$all_discarded_fixture"
[[ "$rc" -eq 0 ]] || fail "all_discarded_wrong_exit:$rc"
# AUR-542: AUR-490 prepends the summary/diagram block here too (see the
# "happy path" comment above); "No issues found." is still the exact tail.
[[ "$(tail -n1 "$run_dir/out.stdout")" == "No issues found." ]] || fail all_discarded_stdout_regressed
# A reviewer found that tail -n1 alone lets the discarded finding itself
# leak onto an earlier stdout line undetected. Every finding here was
# discarded, so stdout must carry zero finding-shaped lines AND must not
# name the discarded finding's own text (this card's own exact leak
# mutation: printing "config/demo-tokens.txt:4: [error] no rule_id at
# all" before "No issues found." must fail this check).
if grep -Eq '^[^ ]+:[0-9]+: \[' "$run_dir/out.stdout"; then
  fail all_discarded_stdout_leaked_finding
fi
if grep -Fq 'no rule_id at all' "$run_dir/out.stdout"; then
  fail all_discarded_stdout_leaked_finding
fi
want_all_discarded_stderr='aurumcode review: 1 finding(s) discarded: 1 with no rule_id'
[[ "$(cat "$run_dir/out.stderr")" == "$want_all_discarded_stderr" ]] || fail all_discarded_stderr_missing

printf '%s/AC-001/E2EAUR448/ok\n' "$card"
