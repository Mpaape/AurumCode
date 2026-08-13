#!/usr/bin/env bash
# E2E check for AUR-443: build (or reuse) the real aurumcode binary and run
# it the way a user who has never read the source would -- `--help`,
# `--version`, `review --help`, a first review with no provider configured,
# a ref that does not resolve, and outside any git repository -- then
# confirm review --base's and docs's published output are untouched. See
# docs/specs/AUR-443.md.
set -euo pipefail
export LC_ALL=C

readonly card=AUR-443
selector="${1:-E2EAUR443}"
[[ "$selector" == "E2EAUR443" ]] || { printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64; }

fail() { printf '%s/AC-001/%s\n' "$card" "$1" >&2; exit 1; }
infra() { printf '%s/AC-001/infrastructure/%s\n' "$card" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

command -v go >/dev/null 2>&1 || infra missing_go

repo_dir="$repo_root/tests/fixtures/repos/git-demo/repo.git"
test -d "$repo_dir" || infra missing_git_demo_fixture
known_problem_fixture="$repo_root/tests/fixtures/review/known-problem-response.json"
test -f "$known_problem_fixture" || infra missing_known_problem_fixture

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e2e-a443.XXXXXX")" || infra mktemp
# See tests/acceptance/AUR-430.sh's cleanup_root: never let a removal error
# override an already-decided result.
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
# Reuse an already-warm build cache and an already-built binary when a
# caller provides them (tests/acceptance/AUR-443.sh's e2e_case does).
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

# run_bin runs the binary as a user would, from the given directory, with a
# clean environment (no inherited LLM provider variables, no canary) plus
# any extra KEY=VALUE pairs given anywhere among the remaining arguments
# (this card's own calls below put them last, after the subcommand and its
# flags, so the scan below is order-independent rather than leading-only).
# Raw exit code lands in rc; stdout/stderr in $run_dir/out.{stdout,stderr}.
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
  (cd "$dir" && env -u AURUM_SECRET_CANARY -u AURUMCODE_LLM_FIXTURE -u LLM_API_KEY -u LLM_BASE_URL \
    "${extra_env[@]}" "$bin" "${bin_args[@]}") \
    >"$run_dir/out.stdout" 2>"$run_dir/out.stderr"
  rc=$?
  set -e
}

# --- 1. Top-level --help / -h / help: discoverability without reading source. ---

run_bin "$repo_root" --help
[[ "$rc" -eq 0 ]] || fail "help_failed:exit:$rc"
[[ -s "$run_dir/out.stderr" ]] && fail help_wrote_stderr
grep -Fq 'review' "$run_dir/out.stdout" || fail help_missing_review
grep -Fq 'docs' "$run_dir/out.stdout" || fail help_missing_docs
grep -Fq 'aurumcode review --base HEAD~1' "$run_dir/out.stdout" || fail help_missing_example
help_stdout="$(cat "$run_dir/out.stdout")"

for arg in -h help; do
  run_bin "$repo_root" "$arg"
  [[ "$rc" -eq 0 ]] || fail "${arg}_failed:exit:$rc"
  [[ "$(cat "$run_dir/out.stdout")" == "$help_stdout" ]] || fail "${arg}_differs_from_--help"
done

# An unrelated unknown token is still a usage error -- --help did not
# accidentally widen "unknown command" to swallow everything.
run_bin "$repo_root" --totally-bogus-token
[[ "$rc" -eq 2 ]] || fail "bogus_token_wrong_exit:$rc"
grep -Fq 'unknown command' "$run_dir/out.stderr" || fail bogus_token_missing_message

# --- 2. --version / version. ---

for arg in --version version; do
  run_bin "$repo_root" "$arg"
  [[ "$rc" -eq 0 ]] || fail "${arg}_failed:exit:$rc"
  grep -Fq 'aurumcode ' "$run_dir/out.stdout" || fail "${arg}_missing_prefix"
done

# --- 3. One --help convention across review and docs. ---

run_bin "$repo_root" review --help
[[ "$rc" -eq 0 ]] || fail "review_help_wrong_exit:$rc"
[[ -s "$run_dir/out.stderr" ]] && fail review_help_wrote_stderr
grep -Fq -- '-base' "$run_dir/out.stdout" || fail review_help_missing_flags

run_bin "$repo_root" docs --help
[[ "$rc" -eq 0 ]] || fail "docs_help_wrong_exit:$rc"
[[ -s "$run_dir/out.stderr" ]] && fail docs_help_wrote_stderr
grep -Fq 'usage: aurumcode docs' "$run_dir/out.stdout" || fail docs_help_missing_usage

# A genuine usage error, for both subcommands, is still stderr + exit 2.
for sub in review docs; do
  run_bin "$repo_root" "$sub" --this-flag-does-not-exist
  [[ "$rc" -eq 2 ]] || fail "${sub}_usage_error_wrong_exit:$rc"
  [[ -s "$run_dir/out.stdout" ]] && fail "${sub}_usage_error_wrote_stdout"
done

# --- 4. First run, no provider configured: the message shows the fixture shape. ---

run_bin "$repo_dir" review --base HEAD~1
[[ "$rc" -eq 1 ]] || fail "no_provider_wrong_exit:$rc"
grep -Fq 'no LLM provider configured' "$run_dir/out.stderr" || fail no_provider_message_missing
grep -Fq 'tests/fixtures/review/known-problem-response.json' "$run_dir/out.stderr" || fail no_provider_missing_fixture_pointer
grep -Fq '"severity"' "$run_dir/out.stderr" || fail no_provider_missing_fixture_shape

# Following the pointer actually works offline.
run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$known_problem_fixture"
[[ "$rc" -eq 0 ]] || fail "pointed_fixture_failed:exit:$rc"
grep -Fq 'config/demo-tokens.txt' "$run_dir/out.stdout" || fail pointed_fixture_missing_finding

# --- 5. --limite's refusal exit code is unchanged (investigated, not fixed). ---

run_bin "$repo_dir" review --base HEAD~1 --limite 0.0001 "AURUMCODE_LLM_FIXTURE=$known_problem_fixture"
[[ "$rc" -eq 1 ]] || fail "limite_refusal_wrong_exit:$rc"
grep -Fq 'refusing to call the model' "$run_dir/out.stderr" || fail limite_refusal_message_missing

# --- 6. Git-repository and ref errors: one clean message. ---

run_bin "$run_dir" review --base HEAD~1
[[ "$rc" -eq 1 ]] || fail "not_a_repo_wrong_exit:$rc"
grep -Fq 'is not a git repository' "$run_dir/out.stderr" || fail not_a_repo_message_missing
[[ "$(grep -o 'not a git repository' "$run_dir/out.stderr" | wc -l)" -eq 1 ]] || fail not_a_repo_message_duplicated

run_bin "$repo_dir" review --base does-not-exist-e2e
[[ "$rc" -eq 1 ]] || fail "bad_ref_wrong_exit:$rc"
grep -Fq 'ref "does-not-exist-e2e" not found' "$run_dir/out.stderr" || fail bad_ref_message_missing
grep -Fq 'no such file or directory' "$run_dir/out.stderr" && fail bad_ref_leaked_filesystem_path

# --- 7. review --base's published contract, and docs's, are untouched. ---

clean_fixture="$run_dir/response-clean.json"
printf '{"issues":[],"summary":"Nothing to report."}' >"$clean_fixture"
run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$clean_fixture"
[[ "$rc" -eq 0 ]] || fail "review_regression:exit:$rc"
[[ "$(cat "$run_dir/out.stdout")" == "No issues found." ]] || fail review_contract_changed

goproject_dir="$repo_root/tests/fixtures/docs/goproject"
if [[ -d "$goproject_dir" ]]; then
  run_bin "$repo_root" docs --source "$goproject_dir" --output "$run_dir/site"
  [[ "$rc" -eq 0 ]] || fail "docs_regression:exit:$rc"
  grep -Fq 'Generated' "$run_dir/out.stdout" || fail docs_contract_changed
fi

printf '%s/AC-001/E2EAUR443/ok\n' "$card"
