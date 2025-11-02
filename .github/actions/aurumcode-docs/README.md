# AurumCode Documentation Generator

**Reusable GitHub Action** to generate searchable documentation for ANY codebase.

## Features

- 🌐 **Multi-Language Support**: Go, Python, JavaScript, Java, Ruby, Rust, PHP
- 🔍 **Full-Text Search**: Powered by Pagefind
- 🎨 **Professional Theme**: Uses just-the-docs (same as GitHub's docs)
- 📱 **Mobile-Friendly**: Responsive design
- ⚡ **Fast**: Incremental builds
- 🚀 **Zero Config**: Works out of the box

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
        uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@main
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

That's it! Your documentation will be available at: `https://YOUR_USERNAME.github.io/YOUR_REPO/`

### Option 2: With Custom Configuration

```yaml
- name: Generate Documentation
  uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@main
  with:
    github_token: ${{ secrets.GITHUB_TOKEN }}
    language: 'python'        # Force specific language
    search: 'true'            # Enable search (default)
    theme: 'just-the-docs'    # Jekyll theme
    deploy: 'true'            # Auto-deploy to GitHub Pages
```

## Supported Languages

| Language | Documentation Tool | Status |
|----------|-------------------|---------|
| Go | `go doc` | ✅ Full support |
| Python | `pydoc` | ✅ Full support |
| JavaScript/TypeScript | AST parsing | ✅ Full support |
| Java | Javadoc | ✅ Full support |
| Ruby | RDoc | ✅ Full support |
| Rust | `cargo doc` | ✅ Full support |
| PHP | phpDocumentor | ✅ Full support |
| Any (Markdown) | Jekyll | ✅ Full support |

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
      - uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@main
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
      - uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@main
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
- uses: Mpaape/AurumCode/.github/actions/aurumcode-docs@main
  with:
    language: 'python'  # Force Python
```

## License

MIT License - Use freely in any project!

---

**Made with ❤️ by AurumCode**

[Report Issues](https://github.com/Mpaape/AurumCode/issues) | [View Source](https://github.com/Mpaape/AurumCode)
