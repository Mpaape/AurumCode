#!/usr/bin/env bash
# E2E check for AUR-430: build the real aurumcode binary and run it, as a
# user would, against the tests/fixtures/repos/git-demo bare repository,
# with the LLM call pinned to a deterministic offline fixture. See
# docs/specs/AUR-430.md for the command reference.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-430
selector="${1:-E2EAUR430}"
[[ "$selector" == "E2EAUR430" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
test -d "$repo_dir" || fail missing_fixture_repo
test -s "$fixture" || fail missing_fixture_response

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a430.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: never let a removal error
# override an already-decided result.
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-430.sh's e2e_case does, so the
# whole acceptance run shares one Go build cache instead of each layer
# cold-compiling net/http+crypto/tls from scratch under the profile's tight
# 256MB memory ceiling). Standalone invocation falls back to building its
# own, isolated copy.
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

run_once() {
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$fixture" "$bin" review --base HEAD~1)
}

out1="$(run_once)" || fail run_failed
grep -Fq 'config/demo-tokens.txt' <<<"$out1" || fail missing_expected_finding
grep -Fq '[error]' <<<"$out1" || fail missing_expected_severity

out2="$(run_once)" || fail rerun_failed
[[ "$out1" == "$out2" ]] || fail non_deterministic

# --base is required: omitting it must fail closed, not silently succeed.
if (cd "$repo_dir" && "$bin" review >/dev/null 2>&1); then
  fail missing_base_should_fail
fi

printf '%s/AC-001/E2EAUR430/ok\n' "$card"
