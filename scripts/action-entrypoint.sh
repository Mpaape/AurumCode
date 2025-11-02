#!/bin/bash
# GitHub Action entrypoint script for AurumCode
# Runs the appropriate pipeline based on AURUMCODE_MODE

set -e

echo "🤖 AurumCode - Starting Pipeline"
echo "Mode: ${AURUMCODE_MODE:-all}"
echo ""

# Set up environment
export GITHUB_WORKSPACE=${GITHUB_WORKSPACE:-/github/workspace}
export GITHUB_EVENT_PATH=${GITHUB_EVENT_PATH:-/github/workflow/event.json}

cd "$GITHUB_WORKSPACE"

# Initialize outputs
REVIEW_RESULT="not_run"
ISSUES_FOUND=0
COVERAGE_PERCENTAGE=0
DOCUMENTATION_URL=""

# Function to run code review pipeline
run_review() {
    echo "📝 Running Code Review Pipeline..."

    if [ -f "/app/cli" ]; then
        /app/cli review \
            --provider="$LLM_PROVIDER" \
            --model="$LLM_MODEL" \
            --api-key="$LLM_API_KEY" \
            --github-token="$GITHUB_TOKEN" \
            --post-comments="$POST_PR_COMMENTS"

        REVIEW_RESULT="passed"
    else
        echo "⚠️  Review CLI not available, skipping..."
        REVIEW_RESULT="skipped"
    fi

    echo ""
}

# Function to run documentation pipeline
run_documentation() {
    echo "📚 Running Documentation Pipeline..."

    # Run documentation generation
    if [ -f "scripts/generate-enhanced-docs.sh" ]; then
        chmod +x scripts/generate-enhanced-docs.sh
        ./scripts/generate-enhanced-docs.sh "$DOCUMENTATION_MODE"
    else
        echo "⚠️  Documentation script not found, using simple generation..."

        # Fallback to simple CHANGELOG generation
        if [ -f "generate-docs-simple.sh" ]; then
            chmod +x generate-docs-simple.sh
            ./generate-docs-simple.sh
        fi
    fi

    # Build documentation site
    if [ -f "scripts/build-docs-site.sh" ]; then
        chmod +x scripts/build-docs-site.sh
        ./scripts/build-docs-site.sh

        # Set documentation URL
        REPO_OWNER=$(echo "$GITHUB_REPOSITORY" | cut -d'/' -f1)
        REPO_NAME=$(echo "$GITHUB_REPOSITORY" | cut -d'/' -f2)
        DOCUMENTATION_URL="https://${REPO_OWNER}.github.io/${REPO_NAME}/"
    fi

    echo ""
}

# Function to run QA testing pipeline
run_qa() {
    echo "🧪 Running QA Testing Pipeline..."

    if [ -f "/app/cli" ]; then
        /app/cli qa \
            --coverage-threshold="$COVERAGE_THRESHOLD" \
            --github-token="$GITHUB_TOKEN"

        # Parse coverage from output (if available)
        if [ -f "coverage.txt" ]; then
            COVERAGE_PERCENTAGE=$(grep -oP 'total.*?(\d+\.\d+)%' coverage.txt | grep -oP '\d+\.\d+' || echo "0")
        fi
    else
        echo "⚠️  QA CLI not available, skipping..."
    fi

    echo ""
}

# Run pipelines based on mode
case "${AURUMCODE_MODE}" in
    "review")
        run_review
        ;;
    "documentation")
        run_documentation
        ;;
    "qa")
        run_qa
        ;;
    "all"|*)
        run_review
        run_documentation
        run_qa
        ;;
esac

# Set GitHub Action outputs
if [ -n "$GITHUB_OUTPUT" ]; then
    echo "review_result=${REVIEW_RESULT}" >> "$GITHUB_OUTPUT"
    echo "issues_found=${ISSUES_FOUND}" >> "$GITHUB_OUTPUT"
    echo "coverage_percentage=${COVERAGE_PERCENTAGE}" >> "$GITHUB_OUTPUT"
    echo "documentation_url=${DOCUMENTATION_URL}" >> "$GITHUB_OUTPUT"
fi

echo "✅ AurumCode Pipeline Complete"
echo ""
echo "Results:"
echo "  Review: ${REVIEW_RESULT}"
echo "  Issues: ${ISSUES_FOUND}"
echo "  Coverage: ${COVERAGE_PERCENTAGE}%"
if [ -n "$DOCUMENTATION_URL" ]; then
    echo "  Docs: ${DOCUMENTATION_URL}"
fi
