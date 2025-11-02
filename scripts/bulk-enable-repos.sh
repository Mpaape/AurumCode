#!/bin/bash
# Bulk enable AurumCode across multiple repositories
# This script adds the AurumCode workflow to all repos in your organization

set -e

echo "🚀 Bulk Enable AurumCode for Multiple Repositories"
echo ""

# Check for required tools
if ! command -v gh &> /dev/null; then
    echo "❌ Error: GitHub CLI (gh) is required"
    echo "Install from: https://cli.github.com/"
    exit 1
fi

# Check GitHub authentication
if ! gh auth status &> /dev/null; then
    echo "❌ Error: Not authenticated with GitHub"
    echo "Run: gh auth login"
    exit 1
fi

# Get organization
read -p "Enter your GitHub organization name: " ORG_NAME

# Confirm
echo ""
echo "This will enable AurumCode for ALL repositories in: $ORG_NAME"
read -p "Continue? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

echo ""
echo "Fetching repositories..."

# Get all repos
REPOS=$(gh repo list "$ORG_NAME" --limit 1000 --json name -q '.[].name')

if [ -z "$REPOS" ]; then
    echo "❌ No repositories found in organization: $ORG_NAME"
    exit 1
fi

REPO_COUNT=$(echo "$REPOS" | wc -l)
echo "Found $REPO_COUNT repositories"
echo ""

# Workflow template
WORKFLOW_CONTENT='name: AurumCode - Automated Code Quality

on:
  pull_request:
    types: [opened, synchronize, reopened]
  push:
    branches: [main, master]

permissions:
  contents: write
  pull-requests: write
  issues: write

jobs:
  aurumcode:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run AurumCode
        uses: Mpaape/AurumCode@main
        with:
          mode: "all"
          llm_provider: "openai"
          llm_api_key: ${{ secrets.ORG_OPENAI_API_KEY }}
          github_token: ${{ secrets.GITHUB_TOKEN }}
          coverage_threshold: "80"
'

# Process each repo
SUCCESS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

echo "Processing repositories..."
echo ""

for REPO_NAME in $REPOS; do
    echo "  [$((SUCCESS_COUNT + FAIL_COUNT + SKIP_COUNT + 1))/$REPO_COUNT] $ORG_NAME/$REPO_NAME"

    # Clone repo
    TMP_DIR=$(mktemp -d)
    cd "$TMP_DIR"

    if ! gh repo clone "$ORG_NAME/$REPO_NAME" . &> /dev/null; then
        echo "    ❌ Failed to clone"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        cd - > /dev/null
        rm -rf "$TMP_DIR"
        continue
    fi

    # Check if workflow already exists
    if [ -f ".github/workflows/aurumcode.yml" ]; then
        echo "    ⏭️  Workflow already exists, skipping"
        SKIP_COUNT=$((SKIP_COUNT + 1))
        cd - > /dev/null
        rm -rf "$TMP_DIR"
        continue
    fi

    # Create workflow
    mkdir -p .github/workflows
    echo "$WORKFLOW_CONTENT" > .github/workflows/aurumcode.yml

    # Commit and push
    git config user.name "AurumCode Bot"
    git config user.email "bot@aurumcode.dev"

    git add .github/workflows/aurumcode.yml
    git commit -m "ci: Add AurumCode automated code quality checks

Enables automated code review, documentation generation, and QA testing.

See: https://github.com/Mpaape/AurumCode" &> /dev/null

    if git push origin HEAD &> /dev/null; then
        echo "    ✅ Workflow added successfully"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo "    ❌ Failed to push changes"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi

    # Cleanup
    cd - > /dev/null
    rm -rf "$TMP_DIR"
done

echo ""
echo "============================================"
echo "Summary:"
echo "  ✅ Success: $SUCCESS_COUNT"
echo "  ⏭️  Skipped: $SKIP_COUNT (already enabled)"
echo "  ❌ Failed:  $FAIL_COUNT"
echo "============================================"
echo ""

if [ $SUCCESS_COUNT -gt 0 ]; then
    echo "🎉 AurumCode enabled for $SUCCESS_COUNT repositories!"
    echo ""
    echo "Next steps:"
    echo "  1. Configure organization secret ORG_OPENAI_API_KEY:"
    echo "     https://github.com/organizations/$ORG_NAME/settings/secrets/actions"
    echo ""
    echo "  2. Create a test PR in any repository to see AurumCode in action!"
else
    echo "⚠️  No repositories were successfully enabled."
fi
