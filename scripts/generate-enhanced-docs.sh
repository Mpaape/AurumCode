#!/bin/bash

# Generate enhanced documentation using static tools + LLM
# Mode: full or incremental
#
# This script only ENHANCES documentation that a generator already produced
# from the source tree. It is not the generator: the binary that reads the
# source tree is regenerate-docs, driven by action-entrypoint.sh. Nothing here
# can stand in for it, so a run of this script is never on its own evidence
# that documentation was generated.
#
# EXIT CODES are disjoint on purpose, mirroring tests/acceptance/*.sh:
#   0   enhancement work was performed
#   1   behavioral failure: the run was expected to enhance something and did
#       not (for example a full run over a godoc tree that holds no HTML)
#   3   environment failure: a tool or input this script cannot supply itself
#       is absent (git missing, workspace is not a repository, no HEAD~1)
#   20  legitimate no-op: incremental mode diffed the workspace and found no
#       changed Go file, so there was nothing to enhance. This is DELIBERATELY
#       not 0. "Nothing changed" and "documentation was produced" are different
#       outcomes and a caller must be able to tell them apart; collapsing them
#       into 0 is what let a run that generated nothing report success.

set -euo pipefail

EXIT_BEHAVIOR=1
EXIT_ENVIRONMENT=3
EXIT_NOOP=20
readonly EXIT_BEHAVIOR EXIT_ENVIRONMENT EXIT_NOOP

MODE=${1:-incremental}

# Where the source-reading generator (regenerate-docs) wrote its markdown. This
# script used to know only about 'docs/static/godoc', a tree NOTHING in this
# repository produces: no script, no workflow and no Makefile target creates it.
# The consequence was that the two branches below were both dead ends against
# the real generator - full mode aborted on the missing directory, and
# incremental mode failed as soon as a Go file actually changed - so the
# published image could not complete a documentation run over a Go repository
# at all. The generator's own output is used as the enhancement input whenever
# the godoc tree is absent.
AURUMCODE_OUTPUT_DIR="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"

echo "🤖 Generating Enhanced Documentation (Mode: $MODE)"
echo ""

# Check for API keys
if [ -z "${TOTVS_DTA_API_KEY:-}" ]; then
    echo "⚠️  TOTVS_DTA_API_KEY not set - skipping LLM enhancement"
    LLM_ENABLED=false
else
    echo "✓ TOTVS DTA configured"
    LLM_ENABLED=true
fi

mkdir -p docs/enhanced

# Function to call LLM for documentation enhancement
enhance_with_llm() {
    local file=$1
    local output=$2

    if [ "$LLM_ENABLED" = false ]; then
        cp "$file" "$output"
        return
    fi

    echo "  Enhancing with LLM: $file"

    # Read file content
    content=$(cat "$file")

    # Create prompt
    prompt="You are a technical documentation expert. Enhance the following Go documentation with:
1. Clear explanations of what each component does
2. Usage examples where applicable
3. Common pitfalls to avoid
4. Links to related components

Keep the original structure but add helpful explanations.

Documentation to enhance:
$content

Provide enhanced documentation in HTML format."

    # Call TOTVS DTA API (OpenAI-compatible)
    response=$(curl -s -X POST "${TOTVS_DTA_BASE_URL:?TOTVS_DTA_BASE_URL must be set when LLM enhancement is enabled}/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOTVS_DTA_API_KEY" \
        -d "{
            \"model\": \"gpt-4\",
            \"messages\": [{\"role\": \"user\", \"content\": $(echo "$prompt" | jq -Rs .)}],
            \"temperature\": 0.3,
            \"max_tokens\": 4000
        }")

    # Extract content
    enhanced=$(echo "$response" | jq -r '.choices[0].message.content')

    echo "$enhanced" > "$output"
}

# generator_docs
# Emits, NUL-separated, the markdown the source generator produced. The site
# landing page is excluded: the generator writes it on every run whether or not
# it documented anything, so enhancing it would let an empty generation report
# that this step did work.
#
# NUL-separated because these are file names from the repository being
# documented, which may contain a newline or a space.
generator_docs() {
    local file

    [ -d "$AURUMCODE_OUTPUT_DIR" ] || return 0

    while IFS= read -r -d '' file; do
        [ "$file" = "$AURUMCODE_OUTPUT_DIR/index.md" ] && continue
        case "$file" in *[[:cntrl:]]*) continue ;; esac
        printf '%s\0' "$file"
    done < <(find "$AURUMCODE_OUTPUT_DIR" -type f -name '*.md' -print0 2>/dev/null)
}

# enhance_generator_output
# Enhances every markdown file the generator produced, mirroring its directory
# layout under docs/enhanced. Prints the number of files it handled.
enhance_generator_output() {
    local doc rel out count=0

    while IFS= read -r -d '' doc; do
        rel="${doc#"$AURUMCODE_OUTPUT_DIR"/}"
        out="docs/enhanced/${rel}"
        mkdir -p "${out%/*}"
        enhance_with_llm "$doc" "$out"
        count=$((count + 1))
    done < <(generator_docs)

    printf '%s' "$count"
}

# Process based on mode
if [ "$MODE" = "full" ]; then
    echo "📚 FULL documentation generation"
    echo ""

    enhanced_count=0

    # Legacy input: a godoc HTML tree. Kept because an image that does have one
    # should still use it, but it is no longer required - nothing in this
    # repository produces docs/static/godoc, so demanding it made full mode
    # unreachable.
    if [ -d "docs/static/godoc" ]; then
        for godoc_file in docs/static/godoc/**/*.html docs/static/godoc/*.html; do
            if [ -f "$godoc_file" ]; then
                base=$(basename "$godoc_file" .html)
                dir=$(dirname "$godoc_file")
                rel_dir=${dir#docs/static/godoc/}

                mkdir -p "docs/enhanced/$rel_dir"
                output="docs/enhanced/$rel_dir/$base.html"

                enhance_with_llm "$godoc_file" "$output"
                enhanced_count=$((enhanced_count + 1))
            fi
        done
    else
        echo "  (no 'docs/static/godoc' tree; enhancing what the source generator wrote under '${AURUMCODE_OUTPUT_DIR}')"
    fi

    if [ "$enhanced_count" -eq 0 ]; then
        enhanced_count=$(enhance_generator_output)
    fi

    if [ "$enhanced_count" -eq 0 ]; then
        echo "ERROR: full mode found nothing to enhance: no godoc HTML under 'docs/static/godoc' and no generated markdown under '${AURUMCODE_OUTPUT_DIR}'." >&2
        echo "       A full regeneration that has no documentation to decorate did not do the work it claims." >&2
        exit "$EXIT_BEHAVIOR"
    fi

    echo ""
    echo "✅ Enhanced package documentation ($enhanced_count files)"
    echo "aurumcode-enhanced: result=applied mode=full files=${enhanced_count}"

else
    echo "📝 INCREMENTAL documentation update"
    echo ""

    # Find changed Go files. git is checked explicitly: previously a git
    # failure was swallowed by the trailing `|| true` and reported as the
    # benign "No Go files changed", exiting 0 without doing any work.
    if ! command -v git >/dev/null 2>&1; then
        echo "ERROR: incremental mode needs the 'git' executable, which is not installed in this image." >&2
        exit "$EXIT_ENVIRONMENT"
    fi

    # A container action runs as root over a workspace checked out by the
    # runner's own user, so plain git aborts with "detected dubious ownership"
    # and exits 128. The exception is scoped to this process with `-c`, never
    # written into a global config: the workspace is the very tree this script
    # was invoked to document, so reading its history is inside the trust
    # boundary the run already has, and no other repository gains trust.
    #
    # This used to be invisible: the failing `git diff` was swallowed by a
    # trailing `|| true` and reported as the benign "No Go files changed",
    # exiting 0 without doing any work.
    ws_git() {
        git -c safe.directory="$PWD" "$@"
    }

    if ! ws_git rev-parse --git-dir >/dev/null 2>&1; then
        echo "ERROR: incremental mode needs a git repository in '$(pwd)'." >&2
        exit "$EXIT_ENVIRONMENT"
    fi
    if ! ws_git rev-parse --verify --quiet HEAD~1 >/dev/null; then
        echo "ERROR: incremental mode needs at least two commits to diff (HEAD~1 is not reachable)." >&2
        exit "$EXIT_ENVIRONMENT"
    fi

    diff_output=$(ws_git diff --name-only HEAD~1 HEAD)
    changed_files=$(printf '%s\n' "$diff_output" | grep '\.go$' || true)

    if [ -z "$changed_files" ]; then
        # A genuine no-op of the incremental mode: the diff was computed and it
        # really held no Go file. It is reported with its own exit status and a
        # machine-readable verdict so the caller cannot mistake it for a run
        # that enhanced something. The caller still has to decide whether the
        # overall documentation run produced anything; this script says only
        # that IT had nothing to do.
        echo "No Go files changed"
        echo "aurumcode-enhanced: result=noop mode=incremental files=0 reason=no-go-files-changed"
        exit "$EXIT_NOOP"
    fi

    echo "Changed files:"
    echo "$changed_files"
    echo ""

    # Extract package paths
    packages=$(echo "$changed_files" | xargs -I {} dirname {} | sort -u)

    updated_count=0
    for pkg_dir in $packages; do
        # Convert to package path
        pkg_path=${pkg_dir#./}

        godoc_file="docs/static/godoc/${pkg_path}.html"

        if [ -f "$godoc_file" ]; then
            echo "Updating package: $pkg_path"

            mkdir -p "docs/enhanced/$pkg_path"
            output="docs/enhanced/${pkg_path}.html"

            enhance_with_llm "$godoc_file" "$output"
            updated_count=$((updated_count + 1))
        fi
    done

    if [ "$updated_count" -eq 0 ]; then
        # No godoc HTML matched. Before this, that was a hard behavioral
        # failure - which meant the pipeline failed for EVERY commit that
        # touched a Go file, because docs/static/godoc is never produced by
        # anything in this repository. What the run actually has to show for
        # itself is the generator's own output, so that is enhanced instead.
        echo "  (no godoc HTML matched the changed packages; enhancing what the source generator wrote under '${AURUMCODE_OUTPUT_DIR}')"
        updated_count=$(enhance_generator_output)
    fi

    if [ "$updated_count" -eq 0 ]; then
        # Nothing to decorate anywhere. This step reports only about itself:
        # whether the RUN produced documentation is decided by the caller,
        # which gates on the generator output and fails closed when the
        # generator wrote nothing.
        echo "No enhanceable documentation for the changed packages"
        echo "aurumcode-enhanced: result=noop mode=incremental files=0 reason=no-enhanceable-input"
        exit "$EXIT_NOOP"
    fi
    echo ""
    echo "✅ Updated documentation for changed packages ($updated_count files)"
    echo "aurumcode-enhanced: result=applied mode=incremental files=${updated_count}"
fi

echo ""
count_html() {
    if [ -d "$1" ]; then
        find "$1" -name '*.html' -type f | wc -l
    else
        echo "0 (directory absent)"
    fi
}

echo "📊 Documentation Statistics:"
echo "  Static docs: $(count_html docs/static/godoc) files"
echo "  Enhanced docs: $(count_html docs/enhanced) files"
