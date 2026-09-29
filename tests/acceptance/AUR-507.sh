#!/usr/bin/env bash
# AUR-507: exercise the real Action entrypoint with contract stubs only.
# This proves wrapper arguments, diagnostics, outputs and exits; it does not
# run a model, GitHub Actions or the real Go CLI.
set -euo pipefail

# The test re-executes itself through temporary jq/CLI symlinks. No jq or Go
# binary is needed in the bootstrap Bash/BusyBox container.
case "${0##*/}" in
  jq)
    [[ $# == 3 && "$1" == -er && "$3" == "$AUR_TEST_EVENT" ]] || exit 2
    printf '%s\n' "$2" >> "$AUR_TEST_DIR/jq-calls"
    case "$2" in
      '.number // empty') printf '7\n' ;;
      '.pull_request.head.sha | select(type == "string" and length > 0)') printf 'headsha\n' ;;
      '.pull_request.base.sha | select(type == "string" and length > 0)') printf 'basesha\n' ;;
      *) exit 2 ;;
    esac
    exit 0
    ;;
  aurumcode-contract)
    printf '%s\n' "$@" > "$AUR_TEST_DIR/args"
    printf '%s\n' "$PWD" "${GITHUB_SHA:-}" "${AURUMCODE_BASE_SHA:-}" "${AURUMCODE_PR_PERMISSION_MODE:-}" > "$AUR_TEST_DIR/environment"
    if [[ "${AUR_TEST_OUTPUT:-}" == 1 ]]; then
      [[ -n "${AURUMCODE_OUTPUT_FILE:-}" ]] || exit 2
      printf '%s\n' \
        'version=1.2.3' \
        'changelog_bump=minor' \
        'changelog<<AURUMCODE_CHANGELOG_EOF' \
        '## 1.2.3' \
        'AURUMCODE_CHANGELOG_EOF' > "$AURUMCODE_OUTPUT_FILE"
    fi
    if [[ -n "${AUR_TEST_DIAGNOSTIC:-}" ]]; then
      printf '%s\n' "$AUR_TEST_DIAGNOSTIC" >&2
    fi
    exit "${AUR_TEST_EXIT:-0}"
    ;;
esac

selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003) ;;
  *) printf 'AUR-507/%s/unknown-selector\n' "$selector" >&2; exit 64 ;;
esac

fail() { printf 'AUR-507/%s/%s\n' "$scenario" "$1" >&2; exit 1; }
scenario="$selector"
script_dir="${BASH_SOURCE[0]%/*}"
[[ "$script_dir" != "${BASH_SOURCE[0]}" ]] || script_dir=.
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || fail missing-root
entrypoint="$repo_root/scripts/action-entrypoint.sh"
[[ -f "$entrypoint" ]] || fail missing-entrypoint

scratch="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a507.XXXXXX")" || fail mktemp
trap 'rm -rf -- "$scratch"' EXIT
mkdir -p "$scratch/bin" "$scratch/workspace"
ln -s "$repo_root/tests/acceptance/AUR-507.sh" "$scratch/bin/jq"
ln -s "$repo_root/tests/acceptance/AUR-507.sh" "$scratch/bin/aurumcode-contract"

export PATH="$scratch/bin:$PATH"
export AURUMCODE_CLI="$scratch/bin/aurumcode-contract"
export AUR_TEST_DIR="$scratch"
export AUR_TEST_EVENT="$scratch/event.json"
export GITHUB_WORKSPACE="$scratch/workspace"
export GITHUB_EVENT_NAME=pull_request
export GITHUB_EVENT_PATH="$AUR_TEST_EVENT"
export GITHUB_REPOSITORY=team/project
export GITHUB_OUTPUT="$scratch/github-output"
export RUNNER_TEMP="$scratch"
printf '%s\n' '{"number":7,"pull_request":{"head":{"sha":"headsha"},"base":{"sha":"basesha"}}}' > "$AUR_TEST_EVENT"

last_status=0
run_entry() {
  last_status=0
  bash "$entrypoint" "$@" > "$scratch/stdout" 2> "$scratch/stderr" || last_status=$?
}
expect_status() {
  [[ "$last_status" == "$1" ]] || fail "exit:$last_status:want:$1"
}
expect_args() {
  [[ -f "$scratch/args" ]] || fail cli-not-called
  local -a actual=( ) expected=( "$@" )
  mapfile -t actual < "$scratch/args"
  [[ "${#actual[@]}" == "${#expected[@]}" ]] || fail "argument-count:${#actual[@]}:want:${#expected[@]}"
  local i
  for ((i=0; i<${#expected[@]}; i++)); do
    [[ "${actual[i]}" == "${expected[i]}" ]] || fail "argument:$i:${actual[i]}:want:${expected[i]}"
  done
}
expect_action_environment() {
  local -a actual=( )
  mapfile -t actual < "$scratch/environment"
  [[ "${actual[0]}" == "$scratch/workspace" ]] || fail workspace-not-selected
  [[ "${actual[1]}" == headsha && "${actual[2]}" == basesha ]] || fail wrong-pr-shas
  [[ "${actual[3]}" == endpoint ]] || fail wrong-permission-mode
  local -a calls=( )
  mapfile -t calls < "$scratch/jq-calls"
  [[ "${#calls[@]}" == 3 ]] || fail "jq-calls:${#calls[@]}:want:3"
}

ac001() {
  scenario=AC-001
  export AUR_TEST_EXIT=1 AUR_TEST_DIAGNOSTIC='quality review inconclusive' AUR_TEST_OUTPUT=0
  : > "$scratch/jq-calls"
  run_entry action config false true false none default false
  expect_status 1
  expect_args review --pr 7 --repo team/project --publicar --exigir-qualidade --seguranca
  expect_action_environment
  [[ "$(< "$scratch/stderr")" == *"$AUR_TEST_DIAGNOSTIC"* ]] || fail diagnostic-lost
  [[ ! -s "$GITHUB_OUTPUT" ]] || fail unexpected-output
}

ac002() {
  scenario=AC-002
  export AUR_TEST_EXIT=0 AUR_TEST_DIAGNOSTIC='' AUR_TEST_OUTPUT=0
  : > "$scratch/jq-calls"
  run_entry action config false true false none default
  expect_status 0
  expect_args review --pr 7 --repo team/project --publicar --exigir-qualidade --seguranca
  expect_action_environment
  [[ ! -s "$GITHUB_OUTPUT" ]] || fail unexpected-default-output

  export AUR_TEST_EXIT=3 AUR_TEST_OUTPUT=1
  : > "$scratch/jq-calls"
  run_entry action review true false true warning special true
  expect_status 3
  expect_args review --pr 7 --repo team/project --publicar --exigir-qualidade \
    --modo-publicacao review --na-linha --check --fail-on warning --modelo special --changelog
  expect_action_environment
  local expected_output
  expected_output=$'version=1.2.3\nchangelog_bump=minor\nchangelog<<AURUMCODE_CHANGELOG_EOF\n## 1.2.3\nAURUMCODE_CHANGELOG_EOF'
  [[ "$(< "$GITHUB_OUTPUT")" == "$expected_output" ]] || fail changelog-output-lost

  run_entry action invalid false true false none default false
  expect_status 64
}

ac003() {
  scenario=AC-003
  export AUR_TEST_EXIT=3 AUR_TEST_DIAGNOSTIC='' AUR_TEST_OUTPUT=0
  : > "$scratch/jq-calls"
  run_entry review --base HEAD~1 --fail-on error
  expect_status 3
  expect_args review --base HEAD~1 --fail-on error
  [[ ! -s "$scratch/jq-calls" ]] || fail local-called-jq
  local -a actual=( )
  mapfile -t actual < "$scratch/environment"
  [[ "${actual[0]}" == "$PWD" ]] || fail local-changed-directory

  export AUR_TEST_EXIT=0
  run_entry --version
  expect_status 0
  expect_args --version
}

case "$selector" in
  all) ac001; ac002; ac003 ;;
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-003) ac003 ;;
esac
printf 'AUR-507/%s/pass (entrypoint contract stubs, not E2E)\n' "$selector"
