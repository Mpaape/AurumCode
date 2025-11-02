# Building AurumCode Documentation

Quick guide to build and view the documentation site.

## Install Hugo

### Windows

**Option 1: Chocolatey**
```bash
choco install hugo-extended -y
```

**Option 2: Scoop**
```bash
scoop install hugo-extended
```

**Option 3: Winget**
```bash
winget install Hugo.Hugo.Extended
```

**Option 4: Manual**
1. Download from https://github.com/gohugoio/hugo/releases
2. Get the `hugo_extended_*_windows-amd64.zip`
3. Extract `hugo.exe` to a folder in your PATH

### macOS
```bash
brew install hugo
```

### Linux
```bash
# Ubuntu/Debian
sudo apt install hugo

# Arch
sudo pacman -S hugo

# Or download from releases
```

## Build and Serve Locally

### Quick Start (Development Server)

```bash
# Navigate to Hugo directory
cd hugo

# Start development server
hugo server

# Or with draft content
hugo server -D

# Open in browser:
# http://localhost:1313
```

The development server will auto-reload when you make changes!

### Build for Production

```bash
# Navigate to Hugo directory
cd hugo

# Build static site
hugo --minify

# Output will be in: hugo/public/
```

### Add Search (Optional)

Install Pagefind for full-text search:

```bash
# Install Pagefind
npm install -g pagefind

# Or download from: https://github.com/CloudCannon/pagefind

# After building with Hugo, index the site
pagefind --source public

# This creates: public/_pagefind/ directory
```

## Deploy to GitHub Pages

### Option 1: Manual Deploy

```bash
# Build site
cd hugo
hugo --minify

# Create gh-pages branch if it doesn't exist
git checkout --orphan gh-pages

# Copy files from public/
cp -r public/* .

# Commit and push
git add .
git commit -m "Deploy documentation"
git push origin gh-pages
```

### Option 2: GitHub Actions (Automated)

Create `.github/workflows/hugo.yml`:

```yaml
name: Deploy Hugo Site

on:
  push:
    branches:
      - main

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          submodules: true
          fetch-depth: 0

      - name: Setup Hugo
        uses: peaceiris/actions-hugo@v2
        with:
          hugo-version: 'latest'
          extended: true

      - name: Build
        run: |
          cd hugo
          hugo --minify

      - name: Deploy
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./hugo/public
```

Then enable GitHub Pages:
1. Go to repo Settings → Pages
2. Source: Deploy from branch `gh-pages`
3. Save

Site will be at: `https://mpaape.github.io/AurumCode/`

## View Built Site Locally

After building with `hugo --minify`:

```bash
# Navigate to output directory
cd hugo/public

# Serve with Python
python -m http.server 8000

# Open browser to:
# http://localhost:8000
```

## Customize the Site

### Edit Homepage
- File: `hugo/content/_index.md`

### Edit Documentation
- Add files in: `hugo/content/docs/`

### Edit Configuration
- File: `hugo/config/hugo.toml`

### Edit Styling
- Template: `hugo/layouts/index.html`
- Add CSS in: `hugo/static/css/`

## Troubleshooting

### "Hugo not found"
- Make sure Hugo is in your PATH
- Restart terminal after installation

### Build errors
```bash
# Check Hugo version
hugo version

# Should be v0.100.0 or higher
```

### Port already in use
```bash
# Use different port
hugo server -p 1314
```

### Can't see changes
- Clear browser cache
- Restart hugo server
- Check file is in correct directory

## Documentation Structure

```
hugo/
├── config/
│   └── hugo.toml          # Site configuration
├── content/
│   ├── _index.md          # Homepage
│   └── docs/              # Documentation pages
│       └── _index.md
├── layouts/
│   ├── index.html         # Homepage template
│   └── _default/          # Default templates
│       └── single.html
├── static/                # Static assets (CSS, JS, images)
├── themes/                # Hugo themes (currently using custom)
└── public/                # Built site (generated)
```

## Next Steps

1. **Install Hugo** using one of the methods above
2. **Run `hugo server`** in the `hugo/` directory
3. **Open browser** to http://localhost:1313
4. **Edit content** in `hugo/content/` - changes auto-reload!
5. **Deploy** to GitHub Pages when ready

---

**Need help?** Check the [Hugo documentation](https://gohugo.io/documentation/)
