#!/usr/bin/env bash
# E2E check for AUR-433: build (or reuse) the real aurumcode binary and use
# `review --base HEAD~1 --limite <usd>` as a user would, against the
# tests/fixtures/repos/git-demo bare repository, with the LLM call pinned
# to a deterministic offline fixture this script writes itself. Proves the
# card's core promise both ways: a run under the limit reports the
# estimated AND real cost and prints the finding; a run over the limit
# refuses -- calling the model zero times, provable via
# AURUMCODE_PROMPT_CAPTURE (see tests/acceptance/AUR-433.sh's header note)
# -- and spends nothing. See docs/specs/AUR-433.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-433
selector="${1:-E2EAUR433}"
[[ "$selector" == "E2EAUR433" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$repo_dir" || infra missing_fixture_repo

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a433.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: never let a removal error
# override an already-decided result.
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-433.sh's e2e_case does).
# Standalone invocation falls back to building its own, isolated copy.
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

# Deterministic offline model response, written by this script so it
# depends on no response fixture it does not fully control.
fixture="$run_dir/response.json"
cat >"$fixture" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "warning",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN)."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF

# run_review runs the binary as a user would with the offline provider
# configured. run_review_captured additionally points AURUMCODE_PROMPT_
# CAPTURE at a fresh path first, so the file's existence afterward is the
# call counter this card's MUT-001 needs (see tests/acceptance/AUR-433.sh's
# header note). Raw exit code lands in rc, stdout/stderr in
# $run_dir/out.{stdout,stderr}.
run_review() {
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}
run_review_captured() {
  local capture_path="$1"; shift
  rm -f -- "$capture_path"
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" AURUMCODE_PROMPT_CAPTURE="$capture_path" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# Baseline: no --limite, exit 0, the finding prints.
run_review --base HEAD~1
[[ "$rc" -eq 0 ]] || fail "baseline_failed:exit:$rc"
grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail missing_expected_finding
baseline_stdout="$(cat "$run_dir/out.stdout")"

# --limite well above the cost: proceeds, both cost lines print, the model
# was called (capture file exists), stdout stays byte-identical.
run_review_captured "$run_dir/capture-allow.txt" --base HEAD~1 --limite 0.50
[[ "$rc" -eq 0 ]] || fail "allowed_failed:exit:$rc"
grep -Eq 'estimated cost \$[0-9]+\.[0-9]{4}, diff-only pre-flight \(--limite \$0\.5000\)' "$run_dir/out.stderr" || fail missing_estimated_cost
grep -Eq 'actual cost \$[0-9]+\.[0-9]{4} \(--limite \$0\.5000\)' "$run_dir/out.stderr" || fail missing_actual_cost
[[ -e "$run_dir/capture-allow.txt" ]] || fail call_counter_zero_when_allowed
[[ "$(cat "$run_dir/out.stdout")" == "$baseline_stdout" ]] || fail allowed_stdout_changed

# --limite far below the cost: refuses, exit 1, the model is never called
# (capture file absent -- MUT-001's target), nothing spent, no finding.
run_review_captured "$run_dir/capture-refuse.txt" --base HEAD~1 --limite 0.0001
[[ "$rc" -eq 1 ]] || fail "over_limit_wrong_exit:$rc"
[[ -e "$run_dir/capture-refuse.txt" ]] && fail call_counter_nonzero_when_refused
grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" && fail over_limit_spent_anyway
grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail missing_refusal_message
grep -Fq 'actual cost' "$run_dir/out.stderr" && fail reported_spend_that_never_happened

# Without --limite the pre-existing contract is untouched: exit 0,
# byte-identical stdout, no cost line at all.
run_review --base HEAD~1
[[ "$rc" -eq 0 ]] || fail "no_flag_contract_broken:exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$baseline_stdout" ]] || fail no_flag_stdout_changed
grep -Fq 'cost' "$run_dir/out.stderr" && fail no_flag_gained_cost_line

# An explicitly empty --limite is a usage error, exit 2.
run_review --base HEAD~1 --limite=
[[ "$rc" -eq 2 ]] || fail "empty_limite_not_rejected:exit:$rc"

printf '%s/AC-001/E2EAUR433/ok\n' "$card"
