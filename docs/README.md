# Hand-written documentation

This folder holds pages written by hand, as opposed to `.aurumcode/`, which the
generator rewrites.

## The two trees

| Tree | Written by | Regenerated |
|------|-----------|-------------|
| `.aurumcode/` | `cmd/regenerate-docs` | yes, on every run |
| `docs/` | people | never touched by the generator |

The generator writes markdown, `_config.yml` and `index.md` under its output
directory. It does not read `docs/`, does not copy it anywhere, and does not
build a Jekyll site.

## Getting a hand-written page into the generated site

The landing page lists every `.md` file found under the output directory, at any
depth, except `index.md` itself. So a page placed under the output directory is
picked up by the next run:

```bash
cp docs/my-guide.md .aurumcode/my-guide.md
go run ./cmd/regenerate-docs
```

Alternatively, point `AURUMCODE_DOCS_DIR` at a different directory: `index.md`
and `_config.yml` are written there instead, while the extracted pages stay
under `AURUMCODE_OUTPUT_DIR`.

Nothing deletes files under the output directory, but `index.md` is rewritten on
every run, so it is not a place to keep hand-written content.

## Front matter

The generated `_config.yml` declares `theme: jekyll-theme-primer` and applies
`layout: default` to every page, so a page needs no front matter at all. When
present, `title` is used as the page title in the index listing.

```markdown
---
layout: default
title: My Guide
---

# My Custom Guide
```

## Building the site locally

The generator does not produce a `Gemfile`, so a local Jekyll build needs one
supplied by you:

```bash
go run ./cmd/regenerate-docs
cd .aurumcode
bundle install          # requires a Gemfile in this directory
bundle exec jekyll serve
```

Not verified in this environment: no Ruby or Bundler run was executed here.
Through the GitHub Action the build is done by `actions/jekyll-build-pages`,
which needs no Gemfile, and only when the `publish` input is set.
