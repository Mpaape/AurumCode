# AurumCode Documentation Site

This directory contains the Jekyll-powered documentation site for AurumCode.

## Local Development

### Prerequisites

- Ruby 2.7+ with bundler
- Jekyll 4.3+

### Setup

```bash
cd docs
bundle install
```

### Build

```bash
bundle exec jekyll build
```

### Serve Locally

```bash
bundle exec jekyll serve
```

Then visit http://localhost:4000/AurumCode/

## Structure

```
docs/
├── _config.yml          # Jekyll configuration
├── Gemfile              # Ruby dependencies
├── index.md             # Home page
├── _stack/              # Technology stack documentation
├── _architecture/       # Architecture documentation
├── _tutorials/          # Tutorials and guides
├── _api/                # API reference
├── _roadmap/            # Project roadmap
└── _custom/             # Custom documentation
```

## Theme

This site uses the [Just the Docs](https://github.com/just-the-docs/just-the-docs) theme with dark mode enabled.

## Deployment

The site is automatically deployed to GitHub Pages via GitHub Actions when changes are pushed to the main branch.

## License

See the main repository LICENSE file.
