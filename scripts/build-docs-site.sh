#!/bin/bash

# Build complete documentation website
# Combines: static godoc + LLM-enhanced docs + CHANGELOG + existing docs/
#
# WHAT COUNTS AS A BUILT SITE
#
#   Only an artifact DERIVED FROM THE SOURCE TREE by a documentation generator
#   does. The homepage below is a hardcoded template: writing it needs no source
#   tree, no generator and no work at all, so its presence proves nothing. The
#   `<pre>`-wrapped copies of README.md / CHANGELOG.md / docs/*.md are likewise
#   transcriptions of hand-written files, not generated documentation.
#
#   The previous revision wrote the template unconditionally and exited 0, which
#   satisfied the caller's "did the build produce a site" check with a directory
#   that contained no generated documentation whatsoever. This revision collects
#   the derived artifacts FIRST, refuses to build at all when there are none,
#   and records what it absorbed in a manifest the caller can re-check against
#   disk.
#
# EXIT CODES are disjoint on purpose. This is action-entrypoint.sh's own
# convention (see that file): this script is dispatched by it, and its exit
# status is read there via a case block, by literal value. tests/acceptance/*.sh
# keeps a separate, independently documented convention of its own, in
# tests/acceptance/EXIT_CODE_CONVENTION.md:
#   0  a site containing at least one source-derived artifact was written
#   1  behavioral failure: nothing derived from the source was available, so
#      the only thing this script could have produced is the static template
#   3  environment failure: a required tool is absent

set -euo pipefail

EXIT_BEHAVIOR=1
EXIT_ENVIRONMENT=3
readonly EXIT_BEHAVIOR EXIT_ENVIRONMENT

echo "🏗️  Building Documentation Website"
echo ""

OUTPUT_DIR="docs/public"

# Directory the source-reading generator (regenerate-docs) writes into. Kept in
# sync with the caller through the environment so both agree on one path.
AURUMCODE_OUTPUT_DIR="${AURUMCODE_OUTPUT_DIR:-.aurumcode}"

# Name of the manifest, relative to OUTPUT_DIR. The caller reads it to tell a
# generated site from a static template.
DERIVED_MANIFEST_NAME=".aurumcode-derived.manifest"

for tool in find cp mkdir; do
    command -v "$tool" >/dev/null 2>&1 || {
        echo "ERROR: required tool '$tool' is not available in this image." >&2
        exit "$EXIT_ENVIRONMENT"
    }
done

# Staging area, so a build that turns out to have nothing derived to publish
# never leaves a homepage behind that looks like a finished site.
STAGE="$(mktemp -d "${TMPDIR:-/tmp}/aurumcode-site.XXXXXX")" || {
    echo "ERROR: cannot create a staging directory under '${TMPDIR:-/tmp}'." >&2
    exit "$EXIT_ENVIRONMENT"
}
trap 'rm -rf "$STAGE"' EXIT

# absorb <source-root> <destination-subdir> <label>
# Copies a tree of generator output into the staging site and records every
# file it took in DERIVED_ROWS. Returns without recording anything when the
# root is absent or empty; that is not an error here, it is simply one source
# of derived documentation that this run does not have.
DERIVED_ROWS="$STAGE/.rows"
: >"$DERIVED_ROWS"

# The fourth argument says WHICH of the copied files may be recorded as derived
# documentation:
#
#   documentation  only markdown, and never the `index.md` landing page at the
#                  root of the tree. The generator writes that page, plus
#                  `_config.yml` and its incremental cache, on every run
#                  regardless of whether it documented anything; recording them
#                  would let scaffolding and a cache file satisfy the caller's
#                  "is there anything derived here" check all by themselves.
#   any            every file, for trees that only exist because a tool
#                  produced them.
absorb() {
    local src="$1" destsub="$2" label="$3" record="$4"
    local staged rel

    [ -d "$src" ] || return 0
    if [ -z "$(find "$src" -type f -print -quit)" ]; then
        echo "  (no $label to absorb: '$src' holds no file)"
        return 0
    fi

    echo "$label..."
    mkdir -p "$STAGE/$destsub"
    # `src/.` rather than `src/*`: an existing but empty directory made the
    # glob expand to a literal and aborted the build under `set -e`.
    cp -R "$src/." "$STAGE/$destsub/"

    # Record what actually landed, re-read from the staged copy rather than
    # from the copy instruction, so the manifest describes the site on disk.
    #
    # NUL-separated: these names come from the consumer's repository and may
    # contain a newline or a tab. Either one would forge extra rows in a
    # tab-separated, line-oriented manifest, so such a name is reported and
    # dropped instead of being written out.
    while IFS= read -r -d '' staged; do
        rel="${staged#"$STAGE/$destsub"/}"

        case "$record" in
            documentation)
                case "$rel" in
                    index.md) continue ;;
                    *.md) ;;
                    *) continue ;;
                esac
                ;;
        esac

        case "$rel" in
            *[[:cntrl:]]*)
                echo "  WARNING: not recording a file whose name contains a control character; it stays in the site but is not counted as derived documentation." >&2
                continue
                ;;
        esac

        printf '%s\t%s\n' "$src" "${destsub}/${rel}" >>"$DERIVED_ROWS"
    done < <(find "$STAGE/$destsub" -type f -print0)
}

absorb "$AURUMCODE_OUTPUT_DIR" "generated" "📦 Absorbing generated documentation" documentation
absorb "docs/static/godoc" "godoc" "📦 Absorbing static godoc documentation" any
absorb "docs/enhanced" "enhanced" "✨ Absorbing enhanced documentation" any

derived_count=$(wc -l <"$DERIVED_ROWS" | tr -d ' ')

if [ "${derived_count:-0}" -eq 0 ]; then
    echo "" >&2
    echo "ERROR: refusing to build a documentation site: nothing derived from the source tree is available." >&2
    echo "       Looked in: '${AURUMCODE_OUTPUT_DIR}' (generator output), 'docs/static/godoc', 'docs/enhanced'." >&2
    echo "       The only thing this script could still write is its hardcoded homepage template," >&2
    echo "       which requires no source tree and no generator run, so it is not a built site." >&2
    echo "       Run the documentation generator first." >&2
    exit "$EXIT_BEHAVIOR"
fi

# Only now is there something worth publishing, so the site is materialised.
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"
cp -R "$STAGE/." "$OUTPUT_DIR/"
rm -f "$OUTPUT_DIR/.rows"

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
                <li><strong>Your configured LLM</strong> - Enhanced explanations</li>
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

# Convert markdown files to HTML. These are transcriptions of hand-written
# repository files, so they are deliberately NOT recorded as derived artifacts.
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

# Record the derived artifacts. Every row names the generator output root it
# came from and the site-relative path it now occupies, so a caller can check
# each claim against the file system instead of believing the count.
{
    echo "# AurumCode documentation site: source-derived artifact manifest"
    echo "# Rows: <generator-output-root><TAB><site-relative-path>"
    echo "# The homepage template and the markdown transcriptions are not listed:"
    echo "# neither requires a source tree or a generator run."
    cat "$DERIVED_ROWS"
    echo "derived_files=${derived_count}"
} > "$OUTPUT_DIR/$DERIVED_MANIFEST_NAME"

echo ""
echo "✅ Documentation site built successfully"
echo ""
echo "📊 Site Statistics:"
echo "  Total files: $(find "$OUTPUT_DIR" -type f | wc -l)"
echo "  Source-derived files: ${derived_count}"
echo "  Size: $(du -sh "$OUTPUT_DIR" | cut -f1)"
echo ""
echo "This step only built the site locally into '$OUTPUT_DIR'."
echo "It did not publish anything. The public URL is known only to the"
echo "publishing step, which reports it as its own output."
