#!/bin/bash

# Generate enhanced documentation using static tools + LLM
# Mode: full or incremental

set -euo pipefail

MODE=${1:-incremental}

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

# Process based on mode
if [ "$MODE" = "full" ]; then
    echo "📚 FULL documentation generation"
    echo ""

    if [ ! -d "docs/static/godoc" ]; then
        echo "ERROR: full mode needs static godoc output in 'docs/static/godoc', which does not exist." >&2
        exit 1
    fi

    enhanced_count=0

    # Process all packages
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

    if [ "$enhanced_count" -eq 0 ]; then
        echo "ERROR: full mode found no godoc HTML under 'docs/static/godoc'; nothing was enhanced." >&2
        exit 1
    fi

    echo ""
    echo "✅ Enhanced package documentation ($enhanced_count files)"

else
    echo "📝 INCREMENTAL documentation update"
    echo ""

    # Find changed Go files. git is checked explicitly: previously a git
    # failure was swallowed by the trailing `|| true` and reported as the
    # benign "No Go files changed", exiting 0 without doing any work.
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        echo "ERROR: incremental mode needs a git repository in '$(pwd)'." >&2
        exit 1
    fi
    if ! git rev-parse --verify --quiet HEAD~1 >/dev/null; then
        echo "ERROR: incremental mode needs at least two commits to diff (HEAD~1 is not reachable)." >&2
        exit 1
    fi

    diff_output=$(git diff --name-only HEAD~1 HEAD)
    changed_files=$(printf '%s\n' "$diff_output" | grep '\.go$' || true)

    if [ -z "$changed_files" ]; then
        echo "No Go files changed"
        exit 0
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

    [ "$updated_count" -gt 0 ] || { echo "ERROR: no godoc HTML under 'docs/static/godoc' matched the changed packages; nothing was updated." >&2; exit 1; }
    echo ""
    echo "✅ Updated documentation for changed packages ($updated_count files)"
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
