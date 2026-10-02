#!/usr/bin/env bash
# E2E check for AUR-443: build (or reuse) the real aurumcode binary and run
# it the way a user who has never read the source would -- `--help`,
# `--version`, `review --help`, a first review with no provider configured,
# a ref that does not resolve, and outside any git repository -- then
# confirm review --base's published output is untouched. See
# docs/specs/AUR-443.md.
#
# AUR-542: the `docs` subcommand and cmd/regenerate-docs were removed by
# commit 670c7f6 ("Focus AurumCode on code review and publish interactive
# documentation"), well before this card, as a deliberate product pivot
# (AUR-490's done-card record treats that removal as already settled
# fact). Every assertion below that named `docs` tested a subcommand that
# no longer exists; those assertions are removed here as outdated, not
# retired wholesale, because the rest of this script's coverage (--help,
# --version, no-provider message, bad ref, not-a-repo) is still the
# product's real contract. See docs/specs/AUR-542.md.
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

# --- 3. review --help's convention: stdout, exit 0. ---

run_bin "$repo_root" review --help
[[ "$rc" -eq 0 ]] || fail "review_help_wrong_exit:$rc"
[[ -s "$run_dir/out.stderr" ]] && fail review_help_wrote_stderr
grep -Fq -- '-base' "$run_dir/out.stdout" || fail review_help_missing_flags

# A genuine usage error is still stderr + exit 2.
run_bin "$repo_root" review --this-flag-does-not-exist
[[ "$rc" -eq 2 ]] || fail "review_usage_error_wrong_exit:$rc"
[[ -s "$run_dir/out.stdout" ]] && fail review_usage_error_wrote_stdout

# --- 4. First run, no provider configured: the message shows the fixture
# shape. AUR-542: AUR-490 (done, integrated before this card) made a bare
# `review --base` without a provider exit 0 (deterministic analysis only)
# instead of 1 unconditionally; see that card's done-record and
# cmd/aurumcode/main.go's AUR-490 comment. The fixture-shape teaching
# text this scenario checks is restored in that same function's
# qualitySkipped branch (see its AUR-542 comment) -- AUR-490's own short
# skip note did not carry it. ---

run_bin "$repo_dir" review --base HEAD~1
[[ "$rc" -eq 0 ]] || fail "no_provider_wrong_exit:$rc"
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

# --- 7. review --base's published contract: zero findings still ends in
# "No issues found." as the exact last line. AUR-542: AUR-490 (done,
# integrated before this card) made this prepend a summary/diagram block
# UNCONDITIONALLY (AC-002), with or without a provider, so stdout is no
# longer the byte-identical lone string this used to check -- measured
# directly, not inferred, by running this exact scenario on main before
# this card. ---

clean_fixture="$run_dir/response-clean.json"
printf '{"issues":[],"summary":"Nothing to report."}' >"$clean_fixture"
run_bin "$repo_dir" review --base HEAD~1 "AURUMCODE_LLM_FIXTURE=$clean_fixture"
[[ "$rc" -eq 0 ]] || fail "review_regression:exit:$rc"
[[ "$(tail -n1 "$run_dir/out.stdout")" == "No issues found." ]] || fail review_contract_changed
# A reviewer found that tail -n1 alone lets a discarded finding leak
# earlier in stdout undetected (a planted finding line before "No issues
# found." keeps this check green). Zero findings means ZERO lines shaped
# like a finding ("<file>:<line>: [<severity>] ...") anywhere in stdout,
# not just that the last line is the right string.
if grep -Eq '^[^ ]+:[0-9]+: \[' "$run_dir/out.stdout"; then
  fail review_contract_leaked_finding
fi

printf '%s/AC-001/E2EAUR443/ok\n' "$card"
