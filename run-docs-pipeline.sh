#!/bin/bash

# Run AurumCode Documentation Pipeline in Docker

set -e

echo "🚀 Running AurumCode Documentation Pipeline in Docker"
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "❌ .env file not found!"
    echo "Please create .env with:"
    echo "  PROVEDOR_API_KEY=your_key"
    echo "  PROVEDOR_BASE_URL=your_url"
    exit 1
fi

# Load environment variables
export $(cat .env | grep -v '^#' | xargs)

echo "✓ Environment variables loaded"
echo "✓ PROVEDOR DTA URL: $PROVEDOR_BASE_URL"
echo ""

# Build and run
echo "📦 Building Docker image..."
docker-compose -f docker-compose.test.yml build

echo ""
echo "🏃 Running Documentation Pipeline..."
echo "─────────────────────────────────────────"
docker-compose -f docker-compose.test.yml run --rm test-docs-pipeline
echo "─────────────────────────────────────────"

echo ""
echo "✅ Pipeline completed!"
echo ""
echo "📊 Check generated files:"
echo "  - CHANGELOG.md"
echo "  - README.md (updated)"
echo ""
echo "Verify with:"
echo "  git status"
echo "  git diff README.md"
