# Custom Documentation

This folder is for **custom user-written documentation** (guides, tutorials, examples).

## Structure

```
docs/
├── README.md           # This file
├── guides/             # How-to guides
├── tutorials/          # Step-by-step tutorials
└── examples/           # Example projects
```

## How It Works

**AurumCode uses a hybrid approach:**

1. **`.aurumcode/`** = Auto-generated API docs (extracted from code)
   - ⚠️ Don't edit manually - regenerated automatically
   - Contains: API reference, function docs, etc.

2. **`docs/`** = Custom pages (this folder)
   - ✅ Your guides, tutorials, examples
   - ✅ Safe from regeneration
   - ✅ Committed to git

3. **Jekyll Build** merges both:
   ```
   .aurumcode/ (auto) + docs/ (custom) = .aurumcode/_site/ (final)
   ```

## Adding Custom Pages

Create markdown files in this folder:

```markdown
---
layout: default
title: My Guide
nav_order: 2
---

# My Custom Guide

Your content here...
```

The site will automatically include your custom pages in the navigation.

## Viewing the Site

**Production:** https://mpaape.github.io/AurumCode/

**Local Development:**
```bash
# Generate docs from code
go run cmd/regenerate-docs/main.go

# Build and serve Jekyll site
cd .aurumcode
bundle install
bundle exec jekyll serve

# Open http://localhost:4000/AurumCode/
```
