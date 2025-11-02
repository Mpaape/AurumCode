#!/bin/bash

# Generate enhanced documentation using static tools + LLM
# Mode: full or incremental

set -e

MODE=${1:-incremental}

echo "🤖 Generating Enhanced Documentation (Mode: $MODE)"
echo ""

# Check for API keys
if [ -z "$TOTVS_DTA_API_KEY" ]; then
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
    response=$(curl -s -X POST "$TOTVS_DTA_BASE_URL/v1/chat/completions" \
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

    # Process all packages
    for godoc_file in docs/static/godoc/**/*.html; do
        if [ -f "$godoc_file" ]; then
            base=$(basename "$godoc_file" .html)
            dir=$(dirname "$godoc_file")
            rel_dir=${dir#docs/static/godoc/}

            mkdir -p "docs/enhanced/$rel_dir"
            output="docs/enhanced/$rel_dir/$base.html"

            enhance_with_llm "$godoc_file" "$output"
        fi
    done

    echo ""
    echo "✅ Enhanced all package documentation"

else
    echo "📝 INCREMENTAL documentation update"
    echo ""

    # Find changed Go files
    changed_files=$(git diff --name-only HEAD~1 HEAD | grep '\.go$' || true)

    if [ -z "$changed_files" ]; then
        echo "No Go files changed"
        exit 0
    fi

    echo "Changed files:"
    echo "$changed_files"
    echo ""

    # Extract package paths
    packages=$(echo "$changed_files" | xargs -I {} dirname {} | sort -u)

    for pkg_dir in $packages; do
        # Convert to package path
        pkg_path=${pkg_dir#./}

        godoc_file="docs/static/godoc/${pkg_path}.html"

        if [ -f "$godoc_file" ]; then
            echo "Updating package: $pkg_path"

            mkdir -p "docs/enhanced/$pkg_path"
            output="docs/enhanced/${pkg_path}.html"

            enhance_with_llm "$godoc_file" "$output"
        fi
    done

    echo ""
    echo "✅ Updated documentation for changed packages"
fi

echo ""
echo "📊 Documentation Statistics:"
echo "  Static docs: $(find docs/static/godoc -name '*.html' | wc -l) files"
echo "  Enhanced docs: $(find docs/enhanced -name '*.html' | wc -l) files"
