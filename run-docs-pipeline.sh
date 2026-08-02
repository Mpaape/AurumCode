#!/bin/bash
# Run the AurumCode documentation generator in a container.
#
# Credentials are not exported here: Compose reads .env from this directory by
# itself and substitutes it into docker-compose.test.yml. They are optional
# anyway - without them cmd/regenerate-docs still generates documentation and
# only skips the AI welcome page (main.go:84, main.go:99).

set -euo pipefail

cd -- "$(dirname -- "$0")"

if [ -f .env ]; then
    echo "✓ .env found - Compose will substitute it"
else
    echo "ℹ️  No .env found - running without LLM credentials"
    echo "   Optional: LLM_API_KEY, LLM_BASE_URL, LLM_MODEL (or OPENAI_API_KEY)"
fi

if docker compose version >/dev/null 2>&1; then
    compose=(docker compose)
else
    compose=(docker-compose)
fi

echo ""
echo "📦 Building image..."
"${compose[@]}" -f docker-compose.test.yml build

echo ""
echo "🏃 Running documentation generator..."
echo "─────────────────────────────────────────"
"${compose[@]}" -f docker-compose.test.yml run --rm test-docs-pipeline
echo "─────────────────────────────────────────"

echo ""
echo "✅ Generator finished. Check the output directory it reported above."
echo ""
echo "Verify with:"
echo "  git status"
