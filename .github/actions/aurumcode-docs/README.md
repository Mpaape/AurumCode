# AurumCode Documentation Generator

**Reusable GitHub Action** to generate searchable documentation for ANY codebase.

## Features

- 🌐 **Multi-Language Support**: Go, Python, JavaScript, Java, Ruby, Rust, PHP
- 🔍 **Full-Text Search**: Powered by Pagefind
- 🎨 **Professional Theme**: Uses just-the-docs (same as GitHub's docs)
- 📱 **Mobile-Friendly**: Responsive design
- ⚡ **Fast**: Incremental builds
- 🚀 **Zero Config**: Works out of the box

## Pin this action before you use it

Every example below uses `RELEASE_SHA` as a placeholder. **Do not copy a
`@main` reference into your workflow.** `main` is a moving branch: whoever can
push to it changes the code that runs inside your repository, with your
`GITHUB_TOKEN`, on your next build. Pin to an immutable commit SHA (preferred)
or, at minimum, to a release tag:

```bash
# Resolve the SHA of the release you want to pin
gh api repos/Mpaape/AurumCode/commits/v1.0.0 --jq .sha
```

```yaml
# Preferred: immutable commit SHA, with the human-readable tag beside it
uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@<RELEASE_SHA> # v1.0.0

# Acceptable: release tag (a tag can still be force-moved by the publisher)
uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@v1.0.0

# Not supported: a moving branch reference
uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@main
```

This action pins its own third-party dependencies by commit SHA for the same
reason; see `peaceiris/actions-gh-pages` in `action.yml`.

## Usage

### Option 1: In Your Repository

Create `.github/workflows/docs.yml`:

```yaml
name: Documentation

on:
  push:
    branches: [main]

permissions:
  contents: write
  pages: write

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate Documentation
        uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@<RELEASE_SHA> # v1.0.0
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

Your documentation will be available at: `https://YOUR_USERNAME.github.io/YOUR_REPO/`

### Option 2: With Custom Configuration

```yaml
- name: Generate Documentation
  uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@<RELEASE_SHA> # v1.0.0
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    language: 'python'          # Force specific language
    search: 'true'              # Enable search (default)
    theme: 'just-the-docs'      # Jekyll theme
    deploy: 'true'              # Auto-deploy to GitHub Pages
    overwrite_existing: 'false' # Never replace your own files (default)
```

## Your files are not overwritten

This action scaffolds a Jekyll site. With the default
`overwrite_existing: 'false'`, it writes `_config.yml`, `Gemfile`, `index.md`
and `_docs/*.md` **only when those paths do not already exist**. If a path
exists, the action logs a notice and leaves your file untouched. Set
`overwrite_existing: 'true'` only if you want the action's scaffolding to
replace files you committed.

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `github_token` | yes | — | Token used to publish to the `gh-pages` branch |
| `language` | no | `auto` | One of `auto`, `go`, `python`, `javascript`, `typescript`, `java`, `ruby`, `rust`, `php`, `markdown`. Any other value is rejected with an error |
| `deploy` | no | `true` | Publish `_site` to GitHub Pages. With `false`, the `site_url` output is empty — nothing was published. |
| `search` | no | `true` | Build a Pagefind index (requires `npx` on the runner) |
| `theme` | no | `just-the-docs` | Jekyll theme. **Only `just-the-docs` is implemented**; any other value fails the run instead of being silently ignored. |
| `overwrite_existing` | no | `false` | Allow the scaffolding to replace existing caller files |

## Supported Languages

| Language | Documentation Source | Status |
|----------|---------------------|---------|
| Go | `go doc -all` per package | Full API extraction — requires `go` on the runner |
| Python | file listing (`python3` required) | Stub pages per module; no docstring extraction yet |
| JavaScript/TypeScript | file listing (`package.json` required) | Stub pages per file; no AST parsing yet |
| Java | file listing | Stub pages per class; no Javadoc extraction yet |
| Ruby / Rust / PHP | — | Detected, then handled as Markdown-only; no API extraction |
| Any (Markdown) | Jekyll | Full support — renders `docs/*.md` and `README.md` |

The runner must already provide the toolchain for the selected language. If it
does not, the action fails with an explicit error rather than publishing an
empty documentation set.

## Runner requirements

Add these before this action; it does not install them for you, and it fails
loudly when they are missing:

- Ruby + Bundler (`ruby/setup-ruby`) — always required
- `actions/setup-go` — when `language` resolves to `go`
- `actions/setup-python` — when `language` resolves to `python`
- `actions/setup-node` — when `search: 'true'`

## What Gets Generated

### From Your Code
- API reference for all public functions/classes
- Type definitions and interfaces
- Function signatures and parameters
- Code examples (if present in comments)

### From Your Docs
- All Markdown files in `docs/`
- README.md
- CHANGELOG.md
- Any other `.md` files

### Automatic Features
- Full-text search across all pages
- Syntax highlighting for code blocks
- Mobile-responsive navigation
- Dark/light theme toggle
- Copy-to-clipboard for code examples

## Examples

### For Go Projects

```go
// Package api provides HTTP handlers
package api

// CreateUser creates a new user in the database
func CreateUser(name string) error {
    // Implementation
}
```

**Generates:**
- API page for `api` package
- Documentation for `CreateUser` function
- Searchable by "CreateUser", "user", "database"

### For Python Projects

```python
def calculate_score(data: List[int]) -> float:
    """
    Calculate the average score from a list of integers.

    Args:
        data: List of integer scores

    Returns:
        Float representing the average score
    """
    return sum(data) / len(data)
```

**Generates:**
- API page for the module
- Function signature with types
- Docstring rendered as description
- Searchable by function name or description

## Configuration

### Jekyll Customization

Add `_config.yml` to your repo:

```yaml
title: "My Project Documentation"
description: "Comprehensive documentation"

# Customize colors
color_scheme: dark  # or 'light'

# Customize navigation
nav_sort: case_sensitive

# Add custom links
aux_links:
  "GitHub": "https://github.com/you/repo"
```

### Directory Structure

```
your-repo/
├── .github/
│   └── workflows/
│       └── docs.yml          # This workflow
├── docs/                     # Your markdown docs
│   ├── guide.md
│   └── api.md
├── README.md                 # Included automatically
└── src/                      # Your source code
```

**Generates:**

```
https://you.github.io/repo/
├── /                         # Homepage from README
├── /docs/                    # All markdown docs
│   ├── /guide
│   └── /api
└── /api/                     # Auto-generated from code
    ├── /package1
    └── /package2
```

## Enterprise Deployment (100+ Repos)

### Option 1: Organization-Wide Action

Create a workflow template in `.github/workflow-templates/`:

```yaml
name: Documentation
on: [push]

jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@<RELEASE_SHA> # v1.0.0
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

Repos can then click "Actions" → "New workflow" → "Documentation"

### Option 2: Bulk Setup Script

```bash
#!/bin/bash
# setup-docs-for-all-repos.sh

ORG="your-org"
REPOS=$(gh repo list "$ORG" --limit 1000 --json name -q '.[].name')

for repo in $REPOS; do
  echo "Setting up $repo..."

  # Clone
  gh repo clone "$ORG/$repo" "/tmp/$repo"
  cd "/tmp/$repo"

  # Add workflow
  mkdir -p .github/workflows
  cat > .github/workflows/docs.yml <<'EOF'
name: Documentation
on: [push]
jobs:
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@<RELEASE_SHA> # v1.0.0
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
EOF

  # Commit and push
  git add .github/workflows/docs.yml
  git commit -m "docs: Add auto-documentation"
  git push

  cd -
done
```

## Troubleshooting

### Documentation not showing up

1. Enable GitHub Pages:
   - Repo → Settings → Pages
   - Source: Deploy from branch
   - Branch: `gh-pages` / `root`

2. Wait 2-3 minutes for deployment

3. Check Actions tab for errors

### Search not working

Ensure `search: 'true'` is set in the action inputs.

### Language not detected

Specify manually:

```yaml
- uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@<RELEASE_SHA> # v1.0.0
  with:
    language: 'python'  # Force Python
```

The action rejects a `language` value outside the documented list instead of
silently falling back, so a typo fails the build with a clear message.

## License

MIT License - Use freely in any project!

---

**Made with ❤️ by AurumCode**

[Report Issues](https://github.com/Mpaape/AurumCode/issues) | [View Source](https://github.com/Mpaape/AurumCode)
