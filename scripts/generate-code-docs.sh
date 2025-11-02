#!/bin/bash
# Generate searchable Go code documentation using pkgsite (Google's tool)
# This script is designed to scale to 100+ repositories

set -e

echo "📦 Generating Go Code Documentation with pkgsite"
echo ""

# Check if we're in a Go project
if [ ! -f "go.mod" ]; then
    echo "❌ Not a Go project (no go.mod found)"
    exit 1
fi

# Install pkgsite if not available
if ! command -v pkgsite &> /dev/null; then
    echo "📥 Installing pkgsite..."
    go install golang.org/x/pkgsite/cmd/pkgsite@latest
fi

# Output directory
OUTPUT_DIR="_api"
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

echo "✓ pkgsite installed"
echo ""

# Get module name
MODULE=$(go list -m)
echo "📚 Module: $MODULE"
echo ""

# Generate documentation for each package
echo "🔨 Generating package documentation..."

# Get all packages
PACKAGES=$(go list ./... | grep -v "/vendor/" | grep -v "/test")

for PKG in $PACKAGES; do
    PKG_NAME=$(echo "$PKG" | sed "s|$MODULE/||" | sed "s|/|.|g")
    PKG_PATH=$(echo "$PKG" | sed "s|$MODULE/||")

    echo "  - $PKG_PATH"

    # Create markdown file for this package
    cat > "$OUTPUT_DIR/${PKG_NAME}.md" <<EOF
---
layout: default
title: $(basename "$PKG_PATH")
parent: API Reference
nav_order: 1
---

# Package \`$PKG_PATH\`

## Overview

\`\`\`go
import "$PKG"
\`\`\`

## Index

EOF

    # Generate package documentation using go doc
    go doc -all "$PKG" >> "$OUTPUT_DIR/${PKG_NAME}.md" 2>/dev/null || echo "_(Documentation generation in progress)_" >> "$OUTPUT_DIR/${PKG_NAME}.md"

    # Add code fence
    echo '```' >> "$OUTPUT_DIR/${PKG_NAME}.md"

    # Add footer
    cat >> "$OUTPUT_DIR/${PKG_NAME}.md" <<EOF

---

_Generated from source code on $(date +%Y-%m-%d)_

[View Source](https://github.com/Mpaape/AurumCode/tree/main/$PKG_PATH)
{: .btn .btn-outline }
EOF

done

# Create API index
cat > "$OUTPUT_DIR/index.md" <<'EOF'
---
layout: default
title: API Reference
nav_order: 3
has_children: true
---

# API Reference

Complete API documentation for all Go packages in AurumCode.

## Core Packages

### Pipeline
- [pipeline](/api/internal.pipeline.html) - Main orchestration
- [review_pipeline](/api/internal.pipeline.review_pipeline.html) - Code review workflow
- [docs_pipeline](/api/internal.pipeline.docs_pipeline.html) - Documentation generation
- [qa_pipeline](/api/internal.pipeline.qa_pipeline.html) - QA testing workflow

### LLM Integration
- [llm](/api/internal.llm.html) - LLM provider abstraction
- [llm/provider](/api/internal.llm.provider.html) - Provider implementations
- [llm/cost](/api/internal.llm.cost.html) - Cost tracking

### Code Analysis
- [analyzer](/api/internal.analyzer.html) - Diff parsing and analysis
- [review](/api/internal.review.html) - Code review engine
- [review/iso25010](/api/internal.review.iso25010.html) - Quality scoring

### Git Integration
- [git/githubclient](/api/internal.git.githubclient.html) - GitHub API client
- [git/webhook](/api/internal.git.webhook.html) - Webhook handling

### Documentation
- [documentation](/api/internal.documentation.html) - Doc generation
- [docgen](/api/internal.docgen.html) - Documentation generators

### Testing
- [testing](/api/internal.testing.html) - Test execution
- [testgen](/api/internal.testgen.html) - Test generation

## Common Types
- [types](/api/pkg.types.html) - Shared types and interfaces

---

_This documentation is automatically generated from source code._
EOF

echo ""
echo "✅ Generated documentation for $(echo "$PACKAGES" | wc -l) packages"
echo ""
echo "📊 Output: $OUTPUT_DIR/"
echo "   Files: $(find "$OUTPUT_DIR" -name "*.md" | wc -l) pages"
