#!/usr/bin/env bash
# E2E check for AUR-436: build (or reuse) the real aurumcode binary and use
# `review --base HEAD~1 --modelo local` as a user would, against the
# tests/fixtures/repos/git-demo bare repository, with the LLM call pinned to
# a deterministic offline fixture this script writes itself. Also proves the
# card's core promise the other way around: a model nothing is configured to
# serve fails loudly with exit 1 and an actionable message, never an empty
# review with exit 0. See docs/specs/AUR-436.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-436
selector="${1:-E2EAUR436}"
[[ "$selector" == "E2EAUR436" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$repo_dir" || infra missing_fixture_repo

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a436.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: never let a removal error
# override an already-decided result.
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-436.sh's e2e_case does).
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
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN)."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF

# run_modelo runs the binary as a user would with the offline provider
# configured; run_noprov runs it with every provider-selection variable
# explicitly emptied, so nothing can serve the chosen model. Raw exit code
# lands in rc, stdout/stderr in $run_dir/out.{stdout,stderr}.
run_modelo() {
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}
run_noprov() {
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE= LLM_API_KEY= LLM_BASE_URL= LLM_MODEL= "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# The user chooses the local model: exit 0, the finding prints, and the
# selection note on stderr names the chosen model.
run_modelo --base HEAD~1 --modelo local
[[ "$rc" -eq 0 ]] || fail "modelo_local_failed:exit:$rc"
grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail missing_expected_finding
grep -Fq 'reviewing with model "local"' "$run_dir/out.stderr" || fail missing_selection_note
grep -Fq 'reviewing with model' "$run_dir/out.stdout" && fail selection_note_on_stdout
modelo_stdout="$(cat "$run_dir/out.stdout")"

# Determinism: same input, same output, same exit.
run_modelo --base HEAD~1 --modelo local
[[ "$rc" -eq 0 ]] || fail non_deterministic_exit
[[ "$(cat "$run_dir/out.stdout")" == "$modelo_stdout" ]] || fail non_deterministic_output

# The flag commands the selection: another name is echoed back.
run_modelo --base HEAD~1 --modelo qwen2.5-coder
[[ "$rc" -eq 0 ]] || fail "modelo_named_failed:exit:$rc"
grep -Fq 'reviewing with model "qwen2.5-coder"' "$run_dir/out.stderr" || fail selection_note_ignores_flag

# The pre-existing no-flag contract (AUR-430/431) is untouched: exit 0,
# byte-identical stdout, no selection note.
run_modelo --base HEAD~1
[[ "$rc" -eq 0 ]] || fail "no_flag_contract_broken:exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$modelo_stdout" ]] || fail no_flag_stdout_changed
grep -Fq 'reviewing with model' "$run_dir/out.stderr" && fail no_flag_gained_note

# An unavailable model fails loudly: exit 1, the error names the model and
# says how to configure it -- never an empty review with exit 0 (MUT-001).
run_noprov --base HEAD~1 --modelo local
[[ "$rc" -eq 1 ]] || fail "unavailable_not_exit_1:exit:$rc"
grep -Fq 'No issues found.' "$run_dir/out.stdout" && fail unavailable_reported_empty_review
grep -Fq 'model "local" is unavailable' "$run_dir/out.stderr" || fail unavailable_error_missing_model
grep -Fq 'AURUMCODE_LLM_FIXTURE' "$run_dir/out.stderr" || fail unavailable_error_missing_offline_hint
grep -Fq 'LLM_BASE_URL' "$run_dir/out.stderr" || fail unavailable_error_missing_endpoint_hint

# An explicitly empty model name is a usage error, exit 2.
run_modelo --base HEAD~1 --modelo=
[[ "$rc" -eq 2 ]] || fail "empty_modelo_not_rejected:exit:$rc"

printf '%s/AC-001/E2EAUR436/ok\n' "$card"
