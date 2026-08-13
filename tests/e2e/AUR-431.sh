#!/usr/bin/env bash
# E2E check for AUR-431: build (or reuse) the real aurumcode binary and use
# `review --base HEAD~1 --fail-on high` as a CI pipeline would, against the
# tests/fixtures/repos/git-demo bare repository, with the LLM call pinned to
# a deterministic offline fixture this script writes itself. See
# docs/specs/AUR-431.md for the command reference and exit codes.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-431
selector="${1:-E2EAUR431}"
[[ "$selector" == "E2EAUR431" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$repo_dir" || infra missing_fixture_repo

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a431.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: never let a removal error
# override an already-decided result.
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-431.sh's e2e_case does).
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

# Deterministic offline model responses, written by this script so it
# depends on no response fixture it does not fully control.
fixture_error="$run_dir/response-error.json"
fixture_info="$run_dir/response-info.json"
cat >"$fixture_error" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN)."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}
EOF
cat >"$fixture_info" <<'EOF'
{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "info",
      "message": "Consider documenting where this demo value comes from."
    }
  ],
  "summary": "Only informational notes."
}
EOF

# run_gate runs the binary as a CI job would and records the raw exit code
# in rc, stdout in $run_dir/out.stdout, stderr in $run_dir/out.stderr.
run_gate() {
  local fixture="$1"; shift
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review "$@") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# The gate fires: an error finding at --fail-on high exits 3 and still
# reports the finding.
run_gate "$fixture_error" --base HEAD~1 --fail-on high
[[ "$rc" -eq 3 ]] || fail "gate_did_not_fire:exit:$rc"
grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail missing_expected_finding
grep -Fq '[error]' "$run_dir/out.stdout" || fail missing_expected_severity
gated_stdout="$(cat "$run_dir/out.stdout")"

# Determinism: same input, same output, same exit.
run_gate "$fixture_error" --base HEAD~1 --fail-on high
[[ "$rc" -eq 3 ]] || fail non_deterministic_exit
[[ "$(cat "$run_dir/out.stdout")" == "$gated_stdout" ]] || fail non_deterministic_output

# Below the threshold the gate stays open: info-only findings exit 0.
run_gate "$fixture_info" --base HEAD~1 --fail-on high
[[ "$rc" -eq 0 ]] || fail "gate_false_trigger:exit:$rc"

# The pre-existing no-flag contract (AUR-430) is untouched: exit 0 and
# byte-identical stdout.
run_gate "$fixture_error" --base HEAD~1
[[ "$rc" -eq 0 ]] || fail "no_flag_contract_broken:exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "$gated_stdout" ]] || fail no_flag_stdout_changed

# An unknown level fails closed as a usage error, exit 2.
run_gate "$fixture_error" --base HEAD~1 --fail-on catastrophic
[[ "$rc" -eq 2 ]] || fail "invalid_level_not_rejected:exit:$rc"

printf '%s/AC-001/E2EAUR431/ok\n' "$card"
