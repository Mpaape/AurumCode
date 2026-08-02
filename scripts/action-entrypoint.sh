#!/usr/bin/env bash
# GitHub Action entrypoint script for AurumCode.
# Runs the pipeline selected by AURUMCODE_MODE.
#
# Fail-closed contract:
#   * A mode whose required binary or helper script is missing is an ERROR.
#     It is never reported as "skipped" with a zero exit status.
#   * Counters (issues_found, coverage_percentage) are emitted only when they
#     were actually measured. Otherwise they are reported as "unknown" so a
#     caller cannot mistake "not measured" for "measured zero".
#   * documentation_url is emitted only when a publishing step confirmed the
#     URL. Building a site locally is not a publication.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# Directory holding the AurumCode binaries inside the image.
AURUMCODE_BIN_DIR="${AURUMCODE_BIN_DIR:-/app}"

# CLI implementing the review and qa pipelines. The image built from this
# repository's Dockerfile does NOT ship it: there is no cmd/cli in the source
# tree, only cmd/regenerate-docs. Requesting those modes therefore fails with
# an explicit message instead of reporting a successful no-op.
AURUMCODE_CLI="${AURUMCODE_CLI:-${AURUMCODE_BIN_DIR}/cli}"

# Directory that build-docs-site.sh populates, relative to the workspace.
AURUMCODE_SITE_DIR="${AURUMCODE_SITE_DIR:-docs/public}"

# Optional machine-readable review report used to measure issues_found.
AURUMCODE_REVIEW_REPORT="${AURUMCODE_REVIEW_REPORT:-aurumcode-review.json}"

# Optional coverage summary (output of `go tool cover -func`) used to measure
# coverage_percentage.
AURUMCODE_COVERAGE_FILE="${AURUMCODE_COVERAGE_FILE:-coverage.txt}"

# Set by the publishing step (for example the page_url output of
# actions/deploy-pages). Empty means "nothing was published".
AURUMCODE_PAGES_URL="${AURUMCODE_PAGES_URL:-}"

SUPPORTED_MODES="review, documentation, qa, all"

# Sentinel for values that were never measured.
UNKNOWN="unknown"

die() {
    echo "ERROR: $*" >&2
    exit 1
}

# require_binary <path> <mode>
# Fails with a message naming both the missing binary and the mode that needs
# it, so a caller can tell which capability the image lacks.
require_binary() {
    local binary="$1"
    local mode="$2"

    if [ ! -x "$binary" ]; then
        die "mode '${mode}' requires the executable '${binary}', which is not present in this image. This image ships only '${AURUMCODE_BIN_DIR}/regenerate-docs' (built from cmd/regenerate-docs); there is no cmd/cli in the source tree. Build and install '${binary}', or select a mode this image supports (${SUPPORTED_MODES})."
    fi
}

# require_script <path> <mode>
require_script() {
    local script="$1"
    local mode="$2"

    if [ ! -f "$script" ]; then
        die "mode '${mode}' requires the helper script '${script}', which is missing from this image."
    fi
}

MODE="${AURUMCODE_MODE:-${1:-}}"

if [ -z "$MODE" ]; then
    die "AURUMCODE_MODE is not set. Set it to one of: ${SUPPORTED_MODES}."
fi

echo "AurumCode - Starting Pipeline"
echo "Mode: ${MODE}"
echo ""

# Set up environment.
GITHUB_WORKSPACE="${GITHUB_WORKSPACE:-/github/workspace}"
GITHUB_EVENT_PATH="${GITHUB_EVENT_PATH:-/github/workflow/event.json}"
export GITHUB_WORKSPACE GITHUB_EVENT_PATH

if [ ! -d "$GITHUB_WORKSPACE" ]; then
    die "GITHUB_WORKSPACE '${GITHUB_WORKSPACE}' does not exist or is not a directory."
fi

cd "$GITHUB_WORKSPACE"

# Initialize outputs. Counters start as "unknown", not 0: a literal zero would
# be indistinguishable from a real measurement of zero.
REVIEW_RESULT="not_run"
ISSUES_FOUND="$UNKNOWN"
COVERAGE_PERCENTAGE="$UNKNOWN"
DOCUMENTATION_URL=""

# Run the code review pipeline.
run_review() {
    echo "Running Code Review Pipeline..."
    require_binary "$AURUMCODE_CLI" "review"

    if "$AURUMCODE_CLI" review \
        --provider="${LLM_PROVIDER:-}" \
        --model="${LLM_MODEL:-}" \
        --api-key="${LLM_API_KEY:-}" \
        --github-token="${GITHUB_TOKEN:-}" \
        --post-comments="${POST_PR_COMMENTS:-false}"; then
        REVIEW_RESULT="passed"
    else
        REVIEW_RESULT="failed"
    fi

    # issues_found is a measurement, so it is only set when a report exists.
    if [ -f "$AURUMCODE_REVIEW_REPORT" ]; then
        ISSUES_FOUND="$(jq -r '(.issues // []) | length' "$AURUMCODE_REVIEW_REPORT")"
    else
        echo "NOTE: '${AURUMCODE_REVIEW_REPORT}' not produced; issues_found reported as '${UNKNOWN}'."
    fi

    if [ "$REVIEW_RESULT" = "failed" ]; then
        die "review pipeline reported failures."
    fi

    echo ""
}

# Run the documentation pipeline.
run_documentation() {
    echo "Running Documentation Pipeline..."

    local generate="${SCRIPT_DIR}/generate-enhanced-docs.sh"
    local build="${SCRIPT_DIR}/build-docs-site.sh"

    # Resolved from this script's own directory, not from the caller's
    # workspace: a consumer repository is not expected to vendor our scripts.
    require_script "$generate" "documentation"
    require_script "$build" "documentation"

    bash "$generate" "${DOCUMENTATION_MODE:-incremental}"
    bash "$build"

    # Verify the build actually produced a site rather than trusting exit 0.
    if [ ! -d "$AURUMCODE_SITE_DIR" ] || [ -z "$(find "$AURUMCODE_SITE_DIR" -type f -print -quit)" ]; then
        die "documentation build reported success but produced no files in '${AURUMCODE_SITE_DIR}'."
    fi

    # A built site is not a published site. Only a publishing step can supply
    # the URL, so nothing is emitted here unless it did.
    if [ -n "$AURUMCODE_PAGES_URL" ]; then
        # A newline or CR here would append forged `key=value` lines to
        # GITHUB_OUTPUT (review_result=passed, issues_found=0, ...).
        case $AURUMCODE_PAGES_URL in *[$'\n\r']*) die "AURUMCODE_PAGES_URL contains a newline or carriage return; it would forge extra GITHUB_OUTPUT entries." ;; esac
        DOCUMENTATION_URL="$AURUMCODE_PAGES_URL"
    else
        echo "NOTE: site built into '${AURUMCODE_SITE_DIR}' but not published by this step; documentation_url left empty. A publishing step must supply AURUMCODE_PAGES_URL."
    fi

    echo ""
}

# Run the QA testing pipeline.
run_qa() {
    echo "Running QA Testing Pipeline..."
    require_binary "$AURUMCODE_CLI" "qa"

    "$AURUMCODE_CLI" qa \
        --coverage-threshold="${COVERAGE_THRESHOLD:-}" \
        --github-token="${GITHUB_TOKEN:-}"

    # Parse coverage from `go tool cover -func` output when present. Uses awk
    # rather than `grep -oP`: the runtime image is Alpine, whose busybox grep
    # has no -P and would have failed here.
    if [ -f "$AURUMCODE_COVERAGE_FILE" ]; then
        local parsed
        parsed="$(awk '/^total:/ { v = $NF; sub(/%$/, "", v); print v }' "$AURUMCODE_COVERAGE_FILE" | tail -n 1)"
        if [ -n "$parsed" ]; then
            COVERAGE_PERCENTAGE="$parsed"
        fi
    fi

    if [ "$COVERAGE_PERCENTAGE" = "$UNKNOWN" ]; then
        # An unmeasured coverage value must not satisfy a configured gate.
        if [ -n "${COVERAGE_THRESHOLD:-}" ]; then
            die "COVERAGE_THRESHOLD='${COVERAGE_THRESHOLD}' was requested but coverage could not be measured from '${AURUMCODE_COVERAGE_FILE}'."
        fi
        echo "NOTE: no coverage data in '${AURUMCODE_COVERAGE_FILE}'; coverage_percentage reported as '${UNKNOWN}'."
    fi

    echo ""
}

# Run pipelines based on mode. An unrecognized mode is an error: it must not
# silently fall through to "all".
case "$MODE" in
    review)
        run_review
        ;;
    documentation)
        run_documentation
        ;;
    qa)
        run_qa
        ;;
    all)
        run_review
        run_documentation
        run_qa
        ;;
    *)
        die "unknown AURUMCODE_MODE '${MODE}'. Supported modes: ${SUPPORTED_MODES}."
        ;;
esac

# Set GitHub Action outputs.
if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
        echo "review_result=${REVIEW_RESULT}"
        echo "issues_found=${ISSUES_FOUND}"
        echo "coverage_percentage=${COVERAGE_PERCENTAGE}"
        echo "documentation_url=${DOCUMENTATION_URL}"
    } >> "$GITHUB_OUTPUT"
fi

echo "AurumCode Pipeline Complete"
echo ""
echo "Results:"
echo "  Review: ${REVIEW_RESULT}"
echo "  Issues: ${ISSUES_FOUND}"
echo "  Coverage: ${COVERAGE_PERCENTAGE}"
if [ -n "$DOCUMENTATION_URL" ]; then
    echo "  Docs: ${DOCUMENTATION_URL}"
fi
