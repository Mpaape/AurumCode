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
#   * The documentation mode runs the generator that actually reads the source
#     tree (regenerate-docs, one of the two binaries this image ships) and
#     succeeds only when that generator left documentation derived from the
#     source on disk. Its hardcoded homepage template is not evidence of
#     anything: writing it requires no source tree and no generator run.
#
# EXIT CODES are disjoint on purpose. This is this script's own convention,
# self-contained to this file and the two helper scripts it dispatches to
# (build-docs-site.sh, generate-enhanced-docs.sh), whose statuses the case
# blocks below read by literal value. tests/acceptance/*.sh keeps a
# separate, independently documented convention of its own, in
# tests/acceptance/EXIT_CODE_CONVENTION.md, using 79 where this family uses
# 3 for the "environment failure" concept. The two never share a process -
# no acceptance program runs a scripts/ file - so neither number is ever
# read against the other's table:
#   0   the requested mode ran and produced what it promises
#   1   behavioral failure: the pipeline ran and the result is wrong or absent
#   3   environment failure: a binary, script or workspace this image cannot
#       supply itself is missing. Nothing about the code under test was
#       measured, so this is never valid evidence of a behavioral defect
#   64  usage error: no mode, a mode this entrypoint does not implement, or a
#       mode invoked without the configuration it requires (e.g. review
#       without AURUMCODE_BASE_REF)

set -euo pipefail

EXIT_BEHAVIOR=1
EXIT_ENVIRONMENT=3
EXIT_USAGE=64
readonly EXIT_BEHAVIOR EXIT_ENVIRONMENT EXIT_USAGE

# Exit status generate-enhanced-docs.sh uses for a genuine incremental no-op.
ENHANCE_NOOP=20
readonly ENHANCE_NOOP

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# Directory holding the AurumCode binaries inside the image.
AURUMCODE_BIN_DIR="${AURUMCODE_BIN_DIR:-/app}"

# CLI implementing the review pipeline (cmd/aurumcode; AUR-430/AUR-438). The
# image built from this repository's Dockerfile DOES ship it, compiled from
# this same source tree (Dockerfile: `go build -o aurumcode ./cmd/aurumcode`).
# It publishes exactly two subcommands, review and docs
# (cmd/aurumcode/main.go); it has no qa subcommand, so the qa mode below
# still fails with an explicit message instead of reporting a successful
# no-op.
AURUMCODE_CLI="${AURUMCODE_CLI:-${AURUMCODE_BIN_DIR}/aurumcode}"

# The generator that reads the source tree. This is the ONE binary the image
# built from this repository's Dockerfile installs, and it is what makes the
# documentation mode a real mode rather than a template writer.
AURUMCODE_GENERATOR="${AURUMCODE_GENERATOR:-${AURUMCODE_BIN_DIR}/regenerate-docs}"

# Directory the generator writes its markdown into, relative to the workspace.
# Exported so the generator and build-docs-site.sh agree on one path.
AURUMCODE_OUTPUT_DIR="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"

# Directory that build-docs-site.sh populates, relative to the workspace.
AURUMCODE_SITE_DIR="${AURUMCODE_SITE_DIR:-docs/public}"

# Manifest build-docs-site.sh writes inside the site directory, listing the
# artifacts it absorbed from generator output. Its absence, or a zero count in
# it, means the site is the static template and nothing else.
AURUMCODE_SITE_MANIFEST="${AURUMCODE_SITE_MANIFEST:-.aurumcode-derived.manifest}"

# Optional machine-readable review report used to measure issues_found.
AURUMCODE_REVIEW_REPORT="${AURUMCODE_REVIEW_REPORT:-aurumcode-review.json}"

# Set by the publishing step (for example the page_url output of
# actions/deploy-pages). Empty means "nothing was published".
AURUMCODE_PAGES_URL="${AURUMCODE_PAGES_URL:-}"

SUPPORTED_MODES="review, documentation, qa, all"

# Sentinel for values that were never measured.
UNKNOWN="unknown"

# die: the pipeline ran and its result is wrong or absent.
die() {
    echo "ERROR: $*" >&2
    exit "$EXIT_BEHAVIOR"
}

# die_env: something the image or the workspace had to supply is missing, so
# nothing about the behaviour under test was measured. Kept distinct from die
# on purpose: an environment gap must never be counted as a behavioral defect,
# and a caller must be able to retry it on a fixed image without treating the
# run as a real failure.
die_env() {
    echo "ERROR (environment): $*" >&2
    exit "$EXIT_ENVIRONMENT"
}

# die_usage: the entrypoint was invoked with a mode it does not implement, or
# with a mode that lacks the configuration it requires to run at all.
die_usage() {
    echo "ERROR (usage): $*" >&2
    exit "$EXIT_USAGE"
}

# require_binary <path> <mode>
# Fails with a message naming both the missing binary and the mode that needs
# it, so a caller can tell which capability the image lacks.
require_binary() {
    local binary="$1"
    local mode="$2"

    if [ ! -x "$binary" ]; then
        die_env "mode '${mode}' requires the executable '${binary}', which is not present in this image. The image built from this repository's Dockerfile installs two binaries, '${AURUMCODE_BIN_DIR}/regenerate-docs' (built from cmd/regenerate-docs) and '${AURUMCODE_BIN_DIR}/aurumcode' (built from cmd/aurumcode, publishing review and docs). Build and install '${binary}', or select a mode this image supports (${SUPPORTED_MODES})."
    fi
}

# require_script <path> <mode>
require_script() {
    local script="$1"
    local mode="$2"

    if [ ! -f "$script" ]; then
        die_env "mode '${mode}' requires the helper script '${script}', which is missing from this image."
    fi
}

MODE="${AURUMCODE_MODE:-${1:-}}"

if [ -z "$MODE" ]; then
    die_usage "AURUMCODE_MODE is not set. Set it to one of: ${SUPPORTED_MODES}."
fi

echo "AurumCode - Starting Pipeline"
echo "Mode: ${MODE}"
echo ""

# Set up environment.
GITHUB_WORKSPACE="${GITHUB_WORKSPACE:-/github/workspace}"
GITHUB_EVENT_PATH="${GITHUB_EVENT_PATH:-/github/workflow/event.json}"
export GITHUB_WORKSPACE GITHUB_EVENT_PATH

if [ ! -d "$GITHUB_WORKSPACE" ]; then
    die_env "GITHUB_WORKSPACE '${GITHUB_WORKSPACE}' does not exist or is not a directory."
fi

cd "$GITHUB_WORKSPACE"

# Initialize outputs. Counters start as "unknown", not 0: a literal zero would
# be indistinguishable from a real measurement of zero.
REVIEW_RESULT="not_run"
ISSUES_FOUND="$UNKNOWN"
COVERAGE_PERCENTAGE="$UNKNOWN"
DOCUMENTATION_URL=""
# generated | noop | not_run. Reported separately from the exit status so a
# run that legitimately had nothing to regenerate cannot be read as one that
# produced fresh documentation.
DOCUMENTATION_RESULT="not_run"

# Run the code review pipeline.
run_review() {
    echo "Running Code Review Pipeline..."
    require_binary "$AURUMCODE_CLI" "review"

    # cmd/aurumcode's review command (cmd/aurumcode/main.go) has exactly one
    # required flag, --base: the ref this run diffs against HEAD.
    # AURUMCODE_BASE_REF is this entrypoint's own input for it -- a caller
    # sets it to, for example, the pull request's base commit SHA
    # (github.event.pull_request.base.sha), the same value
    # .github/workflows/examples/code-review.yml already wires into --base.
    # Unset is a usage error, not a silent skip: reviewing nothing and
    # reporting success would be worse than failing loudly.
    if [ -z "${AURUMCODE_BASE_REF:-}" ]; then
        die_usage "mode 'review' requires AURUMCODE_BASE_REF (the ref '--base' diffs against HEAD, e.g. the pull request's base commit SHA)."
    fi

    # LLM_API_KEY, LLM_BASE_URL, LLM_MODEL, AURUMCODE_LLM_FIXTURE and
    # GITHUB_TOKEN are read straight from the process environment by
    # cmd/aurumcode itself (main.go selectProvider, pr.go newGitHubClient) --
    # none of them is a flag. This entrypoint inherits them from the
    # container's own environment and passes nothing extra for them.
    #
    # cmd/aurumcode review's other real flags -- --fail-on, --modelo,
    # --seguranca, --limite, and the --pr/--repo/--publicar/--na-linha
    # family -- are not wired into this mode: this card fixes the flags this
    # entrypoint already sends and does not add capability nobody can
    # observe by running the declared command (see docs/specs/AUR-444.md,
    # Non-goals).
    if "$AURUMCODE_CLI" review --base="${AURUMCODE_BASE_REF}"; then
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

# doc_files <output-dir> <scaffold-index>
# Emits, NUL-separated, every markdown file under the generator output
# directory that is a candidate for being documentation derived from the source
# tree. The site's landing page is excluded: the pipeline scaffolds it on every
# run, so counting it would let an empty run claim it documented something.
#
# NUL-separated on purpose, and every consumer of this stream reads it with
# `read -r -d ''`. These paths come from the CONSUMER's repository, which is
# free to contain a file whose name holds a newline, a tab, a quote or a
# semicolon. A line-based reader would let such a name forge extra records, and
# any reader that interpolated one into a command line would let it run.
doc_files() {
    local dir="$1" scaffold="$2" file

    [ -d "$dir" ] || return 0

    while IFS= read -r -d '' file; do
        [ "$file" = "$scaffold" ] && continue
        printf '%s\0' "$file"
    done < <(find "$dir" -type f -name '*.md' -print0 2>/dev/null)
}

# doc_fingerprint <output-dir> <scaffold-index>
# The sorted multiset of content hashes of those files. Comparing two of these
# answers exactly one question - "did the bytes on disk change" - and it does so
# without ever putting a consumer-controlled path into its own output, so a
# crafted file name cannot forge a row here either. Content is hashed from
# stdin so the path is never passed to sha256sum as an argument.
#
# This is read from DISK. It is the evidence the documentation mode is gated
# on, precisely because a line the generator prints about itself is a claim and
# a claim is never a proof.
doc_fingerprint() {
    local dir="$1" scaffold="$2" file

    while IFS= read -r -d '' file; do
        sha256sum <"$file" | cut -d' ' -f1
    done < <(doc_files "$dir" "$scaffold") | LC_ALL=C sort
}

# doc_count <output-dir> <scaffold-index>
doc_count() {
    local file n=0

    while IFS= read -r -d '' file; do
        n=$((n + 1))
    done < <(doc_files "$1" "$2")
    printf '%s' "$n"
}

# Run the documentation pipeline.
#
# Order matters. The generator that reads the source tree runs FIRST, and what
# it left on disk gates everything after it. generate-enhanced-docs.sh only
# decorates what the generator produced, and build-docs-site.sh only assembles
# it; neither can stand in for the generator, and the previous revision, which
# ran only those two, reported a complete pipeline over a workspace where
# nothing had been generated at all.
run_documentation() {
    echo "Running Documentation Pipeline..."

    local generate="${SCRIPT_DIR}/generate-enhanced-docs.sh"
    local build="${SCRIPT_DIR}/build-docs-site.sh"

    # Resolved from this script's own directory, not from the caller's
    # workspace: a consumer repository is not expected to vendor our scripts.
    require_script "$generate" "documentation"
    require_script "$build" "documentation"
    require_binary "$AURUMCODE_GENERATOR" "documentation"

    # The default is a FULL regeneration, not an incremental one.
    #
    # The generator's incremental mode asks its own cache which commit it last
    # documented, and when there is no cache it falls back to reporting the
    # workspace's UNSTAGED changes (internal/documentation/incremental:
    # GetChangedFiles -> DetectUnstagedChanges). A container action runs over a
    # freshly checked out workspace: there is no cache, and a fresh checkout has
    # no unstaged change, so the generator is asked to document nothing and
    # documents nothing - on every run, for every repository. Defaulting to
    # incremental here meant the shipped image could not produce documentation
    # at all, which is precisely the outcome the previous revision was hiding
    # behind a hardcoded homepage and a zero exit status.
    #
    # DOCUMENTATION_MODE is still honoured when the caller sets it, including
    # incremental for a workspace that really does carry a warm cache.
    local doc_mode="${DOCUMENTATION_MODE:-full}"

    # The generator reads these; exported so this script, the generator and the
    # site build cannot disagree about where the output lives.
    export AURUMCODE_OUTPUT_DIR
    export AURUMCODE_SOURCE_DIR="${AURUMCODE_SOURCE_DIR:-.}"
    if [ -z "${AURUMCODE_INCREMENTAL:-}" ]; then
        case "$doc_mode" in
            incremental) AURUMCODE_INCREMENTAL=true ;;
            *)           AURUMCODE_INCREMENTAL=false ;;
        esac
    fi
    export AURUMCODE_INCREMENTAL

    command -v sha256sum >/dev/null 2>&1 \
        || die_env "documentation mode needs 'sha256sum' to tell a regenerated artifact from an untouched one; it is not available in this image."

    local scaffold="${AURUMCODE_OUTPUT_DIR}/index.md"
    local before after
    before="$(doc_fingerprint "$AURUMCODE_OUTPUT_DIR" "$scaffold")"

    # PROOF OF DERIVATION.
    #
    # Counting the files that are under the output directory after the run
    # answers "does a directory exist", not "did this run document anything".
    # A `.aurumcode` tree committed to the consumer's repository, or left by an
    # earlier run, satisfies a file count while the generator reads no source
    # file at all - which is the same false green this mode exists to close,
    # only narrower.
    #
    # So every documentation file that ALREADY exists is stamped here with a
    # fixed, long-past timestamp, before the generator runs. Afterwards, a file
    # whose mtime is newer than that stamp is one this run's generator wrote.
    # The margin is decades rather than the microseconds a plain "touch a
    # reference file now" would give, so no filesystem timestamp granularity
    # can blur "written by this run" into "was already there".
    local stamp stamped=0
    stamp="$(mktemp "${TMPDIR:-/tmp}/aurumcode-stamp.XXXXXX")" \
        || die_env "cannot create a temporary file under '${TMPDIR:-/tmp}' to timestamp the pre-existing documentation."
    touch -t 200001010000 "$stamp" 2>/dev/null \
        || { rm -f "$stamp"; die_env "'touch -t' is not usable in this image, so this run cannot tell documentation it generated from documentation that was already on disk."; }

    local pre
    while IFS= read -r -d '' pre; do
        touch -r "$stamp" "$pre" 2>/dev/null || {
            rm -f "$stamp"
            die_env "cannot set the modification time of the pre-existing documentation file under '${AURUMCODE_OUTPUT_DIR}' (${#pre} byte path). Without it this run cannot prove which files its generator wrote, and an unprovable run is not a successful one."
        }
        stamped=$((stamped + 1))
    done < <(doc_files "$AURUMCODE_OUTPUT_DIR" "$scaffold")

    if [ "$stamped" -gt 0 ]; then
        echo "NOTE: ${stamped} documentation file(s) already exist under '${AURUMCODE_OUTPUT_DIR}'. They are marked so this run is judged on what its own generator writes, not on what was already there."
    fi

    echo "Running source generator: ${AURUMCODE_GENERATOR}"

    local gen_log rc
    gen_log="$(mktemp "${TMPDIR:-/tmp}/aurumcode-generator.XXXXXX")" \
        || die_env "cannot create a temporary file under '${TMPDIR:-/tmp}' to capture generator output."

    rc=0
    "$AURUMCODE_GENERATOR" >"$gen_log" 2>&1 || rc=$?
    cat "$gen_log"

    if [ "$rc" -ne 0 ]; then
        rm -f "$gen_log" "$stamp"
        die "the documentation generator '${AURUMCODE_GENERATOR}' exited ${rc}; no documentation was produced."
    fi

    # Optional cross-check. When the generator prints its own machine-readable
    # verdict it is used only to CONTRADICT the disk, never to substitute for
    # it: a generator that claims success while leaving nothing behind still
    # fails below, and a generator that prints nothing is judged on its output.
    local verdict result claimed
    verdict="$(sed -n 's/.*\(aurumcode: result=[^ ].*\)$/\1/p' "$gen_log" | tail -n 1)"
    rm -f "$gen_log"

    if [ -n "$verdict" ]; then
        result="$(printf '%s' "$verdict" | sed -n 's/.*result=\([^ ]*\).*/\1/p')"
        claimed="$(printf '%s' "$verdict" | sed -n 's/.*docs=\([0-9][0-9]*\).*/\1/p')"
        case "$result" in
            ok|partial|empty) ;;
            *) rm -f "$stamp"; die "the documentation generator reported result='${result}' (verdict: ${verdict})." ;;
        esac
    else
        verdict="(the generator printed no machine-readable verdict line; judging by its output on disk)"
        claimed=""
    fi

    # THE GATE. What counts is the documentation that is on disk after the run,
    # derived by the generator from the source tree. The previous revision never
    # invoked the generator at all and still reported a complete pipeline.
    after="$(doc_fingerprint "$AURUMCODE_OUTPUT_DIR" "$scaffold")"

    local produced fresh=0 post
    produced="$(doc_count "$AURUMCODE_OUTPUT_DIR" "$scaffold")"

    # Files this run's generator actually wrote: newer than the stamp put on
    # everything that pre-existed. `-nt` compares modification times directly,
    # so no consumer-controlled path is ever handed to a command.
    while IFS= read -r -d '' post; do
        [ "$post" -nt "$stamp" ] && fresh=$((fresh + 1))
    done < <(doc_files "$AURUMCODE_OUTPUT_DIR" "$scaffold")
    rm -f "$stamp"

    if [ "$produced" -eq 0 ]; then
        die "the documentation generator '${AURUMCODE_GENERATOR}' ran and left no documentation derived from the source tree '${AURUMCODE_SOURCE_DIR}' under '${AURUMCODE_OUTPUT_DIR}'. An empty generation is not a successful documentation run. Generator verdict: ${verdict}"
    fi

    if [ -n "$claimed" ] && [ "$claimed" -gt "$produced" ]; then
        die "the documentation generator claimed docs=${claimed} but only ${produced} documentation file(s) exist under '${AURUMCODE_OUTPUT_DIR}'. Generator verdict: ${verdict}"
    fi

    # The gate that "a file exists under the output directory" cannot pass on
    # its own. This run has to be able to point at documentation ITS generator
    # wrote.
    #
    # Deliberately fail-closed, and the tradeoff is stated rather than hidden:
    # a generator that legitimately skips writing because its own cache says
    # nothing changed also fails here. That is the intended direction. The
    # output directory lives in the consumer's workspace, so "these files were
    # already here" is not evidence anyone generated them - a committed
    # directory, a restored cache or a previous failed run all look identical
    # from the outside. When a run really has nothing to regenerate, the
    # honest way through this gate is a full regeneration
    # (DOCUMENTATION_MODE=full), which writes the files again and proves it.
    if [ "$fresh" -eq 0 ]; then
        die "the documentation generator '${AURUMCODE_GENERATOR}' ran but wrote none of the ${produced} documentation file(s) under '${AURUMCODE_OUTPUT_DIR}': every one of them pre-dates this run. Documentation that was already on disk proves nothing about this run - a '${AURUMCODE_OUTPUT_DIR}' directory committed to the repository, or left behind by an earlier run, satisfies a file count without any generator having read a single source file. Re-run with DOCUMENTATION_MODE=full to regenerate from '${AURUMCODE_SOURCE_DIR}'. Generator verdict: ${verdict}"
    fi

    # Distinguish a run that regenerated something from one that rewrote the
    # identical bytes. Only the incremental mode is allowed to be a no-op, and
    # even then the run is labelled as such instead of claiming it generated.
    if [ "$before" = "$after" ] && [ -n "$before" ]; then
        DOCUMENTATION_RESULT="noop"
        echo "NOTE: the generator rewrote ${fresh} of the ${produced} file(s) under '${AURUMCODE_OUTPUT_DIR}' with byte-identical content. This run is a no-op (mode=${doc_mode}); it did not change any documentation."
    else
        DOCUMENTATION_RESULT="generated"
        echo "Generator wrote ${fresh} of the ${produced} source-derived documentation file(s) under '${AURUMCODE_OUTPUT_DIR}' during this run."
    fi

    # Enhancement is decoration on top of what the generator produced. A
    # genuine incremental no-op is accepted here, and only here: it says this
    # step had nothing to add, never that the run generated something.
    rc=0
    bash "$generate" "$doc_mode" || rc=$?
    case "$rc" in
        0) ;;
        "$ENHANCE_NOOP")
            echo "NOTE: enhancement step was a legitimate no-op (no changed Go file to enhance). The generator output above is what this run produced."
            ;;
        "$EXIT_ENVIRONMENT")
            die_env "the enhancement step '${generate}' reported a missing tool or unusable workspace (exit ${rc})."
            ;;
        *)
            die "the enhancement step '${generate}' failed (exit ${rc})."
            ;;
    esac

    rc=0
    bash "$build" || rc=$?
    case "$rc" in
        0) ;;
        "$EXIT_ENVIRONMENT")
            die_env "the site build '${build}' reported a missing tool (exit ${rc})."
            ;;
        *)
            die "the site build '${build}' failed (exit ${rc})."
            ;;
    esac

    # Verify the site really carries an artifact DERIVED FROM THE SOURCE by
    # this run's generator, rather than trusting exit 0 or the mere presence of
    # files. The old check counted any file, and the unconditional homepage
    # template satisfied it on its own.
    #
    # The manifest the build writes is a claim, so it is never believed as a
    # count: every row it lists is confirmed against disk first, and only rows
    # that name this run's generator output directory are counted. A site that
    # is only the homepage template has no such rows - it has no manifest at
    # all - and fails here.
    # Every row is verified with shell BUILTINS only. The path in a row is a
    # file name from the consumer's repository, so it is data and never code:
    # it is compared with `=`, tested with `[ -f ]`, and at no point spliced
    # into a command line, an `eval`, a printf format or an awk `system()`.
    # A repository containing `.aurumcode/x";touch OWNED;".md` gets its file
    # counted or skipped, and nothing else.
    local manifest from_generator row_root row_path row_extra
    manifest="${AURUMCODE_SITE_DIR}/${AURUMCODE_SITE_MANIFEST}"
    from_generator=0

    if [ -f "$manifest" ]; then
        while IFS=$'\t' read -r row_root row_path row_extra || [ -n "${row_root:-}" ]; do
            # Comment lines and the trailing derived_files=N summary hold no
            # tab, so they arrive with an empty path and are skipped here.
            [ -n "${row_path:-}" ] || continue
            # A well-formed row has exactly two fields. Anything else is
            # malformed and is not counted rather than being guessed at.
            [ -z "${row_extra:-}" ] || continue
            [ "$row_root" = "$AURUMCODE_OUTPUT_DIR" ] || continue
            # A control character cannot occur in a path this pipeline wrote,
            # and would mean the manifest itself was tampered with.
            case $row_path in
                *[[:cntrl:]]*) continue ;;
                /*|*'../'*|*'/..') continue ;;
            esac
            [ -f "${AURUMCODE_SITE_DIR}/${row_path}" ] || continue
            from_generator=$((from_generator + 1))
        done <"$manifest"
    fi

    if [ "$from_generator" -eq 0 ]; then
        die "the site built into '${AURUMCODE_SITE_DIR}' carries no artifact derived from this run's generator output '${AURUMCODE_OUTPUT_DIR}' (checked against '${manifest}'). A homepage template requires no source tree and no generator run, so it is not a documentation site."
    fi

    echo "Site in '${AURUMCODE_SITE_DIR}' carries ${from_generator} file(s) confirmed on disk and derived from '${AURUMCODE_OUTPUT_DIR}'."

    # A full regeneration has to account for EVERY documentation file it leaves
    # behind. Rewriting the same bytes is fine - that just means the source did
    # not change - but a file the generator did not write at all no longer has
    # a source this run confirmed, and shipping it inside the site would
    # publish documentation for code that may no longer exist. Only the
    # incremental mode is allowed to leave such a file in place.
    if [ "$doc_mode" != "incremental" ] && [ "$fresh" -ne "$produced" ]; then
        die "mode '${doc_mode}' is a full regeneration, but the generator rewrote only ${fresh} of the ${produced} documentation file(s) under '${AURUMCODE_OUTPUT_DIR}'. The other $((produced - fresh)) pre-date this run, so nothing in this run confirms they still describe the source tree '${AURUMCODE_SOURCE_DIR}'. Remove '${AURUMCODE_OUTPUT_DIR}' before a full regeneration, or run the incremental mode."
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

    # cmd/aurumcode (AURUMCODE_CLI, the binary this image ships) publishes
    # exactly two subcommands, review and docs (cmd/aurumcode/main.go); it
    # has no qa subcommand. Failing here, before the binary is ever invoked,
    # keeps this the documented EXIT_ENVIRONMENT this file's own contract
    # promises for "a binary this image cannot supply itself is missing" --
    # the closest existing category, since no binary this image could build
    # from this source tree serves this mode -- instead of letting the
    # binary's own usage-error exit code (2, a value this file's exit-code
    # table never documents) kill the script under `set -e` once the binary
    # exists and require_binary's file-existence check alone stops catching
    # this.
    die_env "mode 'qa' requires a 'qa' subcommand that '${AURUMCODE_CLI}' does not publish (it implements only review and docs; see cmd/aurumcode/main.go). Select a mode this image supports (${SUPPORTED_MODES})."
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
        die_usage "unknown AURUMCODE_MODE '${MODE}'. Supported modes: ${SUPPORTED_MODES}."
        ;;
esac

# Set GitHub Action outputs.
if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
        echo "review_result=${REVIEW_RESULT}"
        echo "issues_found=${ISSUES_FOUND}"
        echo "coverage_percentage=${COVERAGE_PERCENTAGE}"
        echo "documentation_result=${DOCUMENTATION_RESULT}"
        echo "documentation_url=${DOCUMENTATION_URL}"
    } >> "$GITHUB_OUTPUT"
fi

echo "AurumCode Pipeline Complete"
echo ""
echo "Results:"
echo "  Review: ${REVIEW_RESULT}"
echo "  Issues: ${ISSUES_FOUND}"
echo "  Coverage: ${COVERAGE_PERCENTAGE}"
echo "  Documentation: ${DOCUMENTATION_RESULT}"
if [ -n "$DOCUMENTATION_URL" ]; then
    echo "  Docs: ${DOCUMENTATION_URL}"
else
    echo "  Docs: (not published by this step)"
fi
