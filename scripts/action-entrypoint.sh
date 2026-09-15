#!/usr/bin/env bash
set -euo pipefail

# This image has one product surface: code review. Direct container usage
# forwards CLI arguments unchanged. GitHub Action usage passes the `action`
# sentinel followed by presentation options; the review itself still comes
# from the binary, the repository context and the configured model prompt.

cli="${AURUMCODE_CLI:-/app/aurumcode}"
workspace="${GITHUB_WORKSPACE:-$PWD}"

if [[ "${1:-}" != "action" ]]; then
    exec "$cli" "$@"
fi

if (( $# != 8 )); then
    echo "AurumCode action: invalid internal arguments" >&2
    exit 64
fi

publication="$2"
inline_comments="$3"
security="$4"
check="$5"
fail_on="$6"
model="$7"
changelog="$8"

case "$publication" in
    config|comments|review) ;;
    *) echo "AurumCode action: publication must be config, comments or review" >&2; exit 64 ;;
esac
case "$inline_comments" in
    true|false) ;;
    *) echo "AurumCode action: inline-comments must be true or false" >&2; exit 64 ;;
esac
case "$security" in
    true|false) ;;
    *) echo "AurumCode action: security must be true or false" >&2; exit 64 ;;
esac
case "$check" in
    true|false) ;;
    *) echo "AurumCode action: check must be true or false" >&2; exit 64 ;;
esac
case "$fail_on" in
    none|error|warning|info) ;;
    *) echo "AurumCode action: fail-on must be none, error, warning or info" >&2; exit 64 ;;
esac
case "$changelog" in
    true|false) ;;
    *) echo "AurumCode action: changelog must be true or false" >&2; exit 64 ;;
esac

if [[ ! -d "$workspace" ]]; then
    echo "AurumCode action: workspace '$workspace' does not exist" >&2
    exit 3
fi
cd "$workspace"

if [[ "${GITHUB_EVENT_NAME:-}" != "pull_request" && "${GITHUB_EVENT_NAME:-}" != "pull_request_target" ]]; then
    echo "AurumCode action: run this action from a pull_request workflow" >&2
    exit 64
fi

event_path="${GITHUB_EVENT_PATH:-}"
if [[ -z "$event_path" || ! -f "$event_path" ]]; then
    echo "AurumCode action: GITHUB_EVENT_PATH is unavailable" >&2
    exit 3
fi

pr_number="$(jq -er '.number // empty' "$event_path")"
GITHUB_SHA="$(jq -er '.pull_request.head.sha | select(type == "string" and length > 0)' "$event_path")"
AURUMCODE_BASE_SHA="$(jq -er '.pull_request.base.sha | select(type == "string" and length > 0)' "$event_path")"
AURUMCODE_PR_PERMISSION_MODE=endpoint
export GITHUB_SHA AURUMCODE_BASE_SHA AURUMCODE_PR_PERMISSION_MODE
repo="${GITHUB_REPOSITORY:-}"
if [[ -z "$repo" ]]; then
    echo "AurumCode action: GITHUB_REPOSITORY is unavailable" >&2
    exit 3
fi

args=(review --pr "$pr_number" --repo "$repo" --publicar)
if [[ "$publication" != config ]]; then
    args+=(--modo-publicacao "$publication")
fi
if [[ "$inline_comments" == true ]]; then
    args+=(--na-linha)
fi
if [[ "$security" == true ]]; then
    args+=(--seguranca)
fi
if [[ "$check" == true ]]; then
    args+=(--check)
fi
if [[ "$fail_on" != none ]]; then
    args+=(--fail-on "$fail_on")
fi
if [[ "$model" != default ]]; then
    args+=(--modelo "$model")
fi

output_file=""
if [[ "$changelog" == true ]]; then
    args+=(--changelog)
    output_dir="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
    output_file="$(mktemp "${output_dir%/}/aurumcode-output.XXXXXX")"
    export AURUMCODE_OUTPUT_FILE="$output_file"
fi

set +e
"$cli" "${args[@]}"
status=$?
set -e

# AUR-499: expose the suggested version and changelog entry to the consumer
# workflow through the modern $GITHUB_OUTPUT file (the deprecated output
# command is not used). The file's contents are already redacted by the CLI.
if [[ -n "$output_file" && -s "$output_file" && -n "${GITHUB_OUTPUT:-}" ]]; then
    cat "$output_file" >> "$GITHUB_OUTPUT"
fi
rm -f "$output_file" 2>/dev/null || true
exit "$status"
