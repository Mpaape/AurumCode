#!/usr/bin/env bash
# E2E check for AUR-473: build (or reuse) the real aurumcode binary and run
# it, as a user would, against tests/fixtures/repos/git-demo.
#
# Proves the two-sided contract of `--seguranca` + provider outcomes:
#
#   * `--modelo <nome>` the gateway cannot serve is a USAGE error: exit 1,
#     the model named on stderr, stdout empty, and neither the security
#     section nor the coverage note printed -- the deterministic pass never
#     ran, so nothing was computed and nothing could be retained.
#   * A provider that was CONFIGURED and fails at RUNTIME (AUR-458) still
#     prints its already-computed security findings and the coverage note,
#     with exit 1. The fix fails EARLY; it never hides findings it computed.
#
# See docs/specs/AUR-473.md.
set -euo pipefail
export LC_ALL=C

ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB

readonly card=AUR-473
selector="${1:-E2EAUR473}"
[[ "$selector" == "E2EAUR473" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

demo_repo="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$demo_repo" || infra missing-git-demo-fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a473.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR

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

header='Security findings (standards/security-review):'
coverage_prefix='aurumcode review: security pass applied 4 of 8 security rules ('

noprov_env() {
  env -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL -u LLM_MODEL "$@"
}

# 1. AC-001: an unavailable named model is a usage error, before any work.
set +e
out_modelo="$(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca --modelo does-not-exist 2>"$run_dir/modelo.err")"
rc=$?
set -e
err_modelo="$(cat "$run_dir/modelo.err")"
[[ "$rc" -eq 1 ]] || fail "unavailable-modelo-must-exit-1:$rc"
grep -Fq 'model "does-not-exist" is unavailable' <<<"$err_modelo" || fail unavailable-modelo-error-missing
if grep -Fq "$header" <<<"$out_modelo"; then fail unavailable-modelo-printed-security-section; fi
if grep -Fq "$coverage_prefix" <<<"$err_modelo"; then fail unavailable-modelo-printed-coverage-note; fi
[[ -z "${out_modelo//[[:space:]]/}" ]] || fail unavailable-modelo-computed-a-review

# 2. The usage error outranks the --fail-on gate: exit 1, never 3.
set +e
(cd "$demo_repo" && noprov_env "$bin" review --base HEAD~1 --seguranca --modelo does-not-exist --fail-on high) >/dev/null 2>&1
rc=$?
set -e
[[ "$rc" -eq 1 ]] || fail "modelo-usage-error-must-outrank-gate:$rc"

# 3. AC-002: a configured provider that fails at RUNTIME keeps AUR-458 --
#    the deterministic findings were computed and must still print.
set +e
out_fail="$(cd "$demo_repo" && noprov_env env LLM_API_KEY=k LLM_BASE_URL=http://127.0.0.1:9/v1 "$bin" review --base HEAD~1 --seguranca 2>"$run_dir/fail.err")"
rc=$?
set -e
err_fail="$(cat "$run_dir/fail.err")"
[[ "$rc" -eq 1 ]] || fail "runtime-provider-failure-must-exit-1:$rc"
grep -Fq "$header" <<<"$out_fail" || fail runtime-provider-failure-lost-findings
grep -Fq "$coverage_prefix" <<<"$err_fail" || fail runtime-provider-failure-lost-coverage-note
for line in 4 5 6; do
  grep -Fq "config/demo-tokens.txt:${line}: [error]" <<<"$out_fail" || fail "runtime-failure-lost-finding-line:$line"
done

printf '%s/AC-001/e2e-ok\n' "$card"
