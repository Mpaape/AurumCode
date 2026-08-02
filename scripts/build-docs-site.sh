#!/bin/bash

# Build complete documentation website
# Combines: static godoc + LLM-enhanced docs + CHANGELOG + existing docs/

set -euo pipefail

echo "🏗️  Building Documentation Website"
echo ""

OUTPUT_DIR="docs/public"
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Create homepage
echo "📄 Creating homepage..."

cat > "$OUTPUT_DIR/index.html" <<'EOF'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AurumCode Documentation</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f5f5;
        }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 60px 40px;
            border-radius: 12px;
            margin-bottom: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
        }
        header h1 { font-size: 3em; margin-bottom: 10px; }
        header p { font-size: 1.3em; opacity: 0.95; }
        nav {
            background: white;
            padding: 20px;
            border-radius: 10px;
            margin-bottom: 30px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            display: flex;
            gap: 20px;
            flex-wrap: wrap;
        }
        nav a {
            color: #667eea;
            text-decoration: none;
            font-weight: 600;
            font-size: 1.1em;
            padding: 10px 20px;
            border-radius: 6px;
            transition: background 0.3s;
        }
        nav a:hover { background: #f0f0f0; }
        main {
            background: white;
            padding: 50px;
            border-radius: 12px;
            box-shadow: 0 4px 10px rgba(0,0,0,0.1);
        }
        .docs-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 25px;
            margin: 30px 0;
        }
        .doc-card {
            background: #f9f9f9;
            padding: 30px;
            border-radius: 10px;
            border-left: 5px solid #667eea;
        }
        .doc-card h3 {
            color: #667eea;
            margin-bottom: 15px;
            font-size: 1.5em;
        }
        .doc-card a {
            color: #667eea;
            text-decoration: none;
            font-weight: 500;
        }
        .doc-card a:hover { text-decoration: underline; }
        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.85em;
            font-weight: 600;
            background: #10b981;
            color: white;
            margin-left: 10px;
        }
        footer {
            text-align: center;
            margin-top: 40px;
            padding: 30px;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🤖 AurumCode Documentation</h1>
            <p>Automated AI-Powered Code Quality Platform</p>
        </header>

        <nav>
            <a href="#api">API Reference</a>
            <a href="CHANGELOG.html">Changelog</a>
            <a href="godoc/">Go Packages</a>
            <a href="enhanced/">Enhanced Docs</a>
            <a href="https://github.com/Mpaape/AurumCode">GitHub</a>
        </nav>

        <main>
            <h2>📚 Documentation</h2>
            <p>Welcome to AurumCode's auto-generated documentation. This site is automatically updated on every commit.</p>

            <div class="docs-grid">
                <div class="doc-card">
                    <h3>📖 API Reference</h3>
                    <p>Complete API documentation for all packages.</p>
                    <a href="godoc/">Browse API Docs →</a>
                </div>

                <div class="doc-card">
                    <h3>✨ Enhanced Docs</h3>
                    <p>LLM-enhanced documentation with examples and explanations.</p>
                    <a href="enhanced/">Browse Enhanced Docs →</a>
                </div>

                <div class="doc-card">
                    <h3>📝 Changelog</h3>
                    <p>All changes and releases.</p>
                    <a href="CHANGELOG.html">View Changelog →</a>
                </div>

                <div class="doc-card">
                    <h3>🚀 Getting Started</h3>
                    <p>Quick start guide and tutorials.</p>
                    <a href="QUICKSTART.html">Get Started →</a>
                </div>
            </div>

            <h2 id="api">🔧 Core Packages</h2>
            <ul>
                <li><a href="godoc/internal/pipeline.html">internal/pipeline</a> - Pipeline orchestration</li>
                <li><a href="godoc/internal/llm.html">internal/llm</a> - LLM provider abstraction</li>
                <li><a href="godoc/internal/analyzer.html">internal/analyzer</a> - Code analysis</li>
                <li><a href="godoc/internal/review.html">internal/review</a> - Code review engine</li>
            </ul>

            <h2>🤖 Auto-Generated</h2>
            <p>This documentation is automatically generated and deployed on every commit using:</p>
            <ul>
                <li><strong>godoc</strong> - Static Go documentation</li>
                <li><strong>TOTVS DTA LLM</strong> - Enhanced explanations</li>
                <li><strong>GitHub Actions</strong> - Automated deployment</li>
            </ul>

            <p style="margin-top: 20px;">
                <strong>Last Updated:</strong> <span id="timestamp"></span>
            </p>
        </main>

        <footer>
            <p>&copy; 2025 AurumCode | <a href="https://github.com/Mpaape/AurumCode">GitHub</a></p>
            <p>🤖 Documentation auto-generated by AurumCode</p>
        </footer>
    </div>

    <script>
        document.getElementById('timestamp').textContent = new Date().toLocaleString();
    </script>
</body>
</html>
EOF

# copy_tree <source> <destination> <label>
# Uses `src/.` rather than `src/*`: an existing but empty directory made the
# glob expand to a literal and aborted the build under `set -e`.
copy_tree() {
    local src="$1" dest="$2" label="$3"

    if [ ! -d "$src" ]; then
        return 0
    fi
    if [ -z "$(find "$src" -mindepth 1 -print -quit)" ]; then
        echo "  (no $label to copy: '$src' is empty)"
        return 0
    fi

    echo "$label..."
    mkdir -p "$dest"
    cp -R "$src/." "$dest/"
}

copy_tree "docs/static/godoc" "$OUTPUT_DIR/godoc" "📦 Copying static documentation"
copy_tree "docs/enhanced" "$OUTPUT_DIR/enhanced" "✨ Copying enhanced documentation"

# Convert markdown files to HTML
echo "📄 Converting markdown to HTML..."

for md in docs/*.md CHANGELOG.md README.md; do
    if [ -f "$md" ]; then
        base=$(basename "$md" .md)
        # Simple markdown to HTML (would use proper converter in production)
        echo "<html><head><title>$base</title></head><body><pre>" > "$OUTPUT_DIR/$base.html"
        cat "$md" >> "$OUTPUT_DIR/$base.html"
        echo "</pre></body></html>" >> "$OUTPUT_DIR/$base.html"
    fi
done

echo ""
echo "✅ Documentation site built successfully"
echo ""
echo "📊 Site Statistics:"
echo "  Total files: $(find "$OUTPUT_DIR" -type f | wc -l)"
echo "  Size: $(du -sh "$OUTPUT_DIR" | cut -f1)"
echo ""
echo "This step only built the site locally into '$OUTPUT_DIR'."
echo "It did not publish anything. The public URL is known only to the"
echo "publishing step, which reports it as its own output."
