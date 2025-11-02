# GitHub Pages Setup Guide

Quick guide to enable and access AurumCode documentation on GitHub Pages.

## 🌐 Live Site URL

Once enabled, your documentation will be available at:

### **https://mpaape.github.io/AurumCode/**

---

## 🚀 Enable GitHub Pages (First Time Setup)

### Step 1: Go to Repository Settings

**Direct Link:**
https://github.com/Mpaape/AurumCode/settings/pages

**Or Manually:**
1. Go to: https://github.com/Mpaape/AurumCode
2. Click the **Settings** tab (top right)
3. Scroll down left sidebar
4. Click **Pages**

### Step 2: Configure Source

In the "Build and deployment" section:

**Source:** Select **GitHub Actions** from the dropdown

That's it! Click save if prompted.

### Step 3: Wait for Deployment

The GitHub Actions workflow will automatically run:

**Check workflow progress:**
https://github.com/Mpaape/AurumCode/actions

**First deployment takes:** ~1-2 minutes

**Look for:** Green checkmark ✅ on "Deploy to GitHub Pages" workflow

### Step 4: Access Your Site

Once the workflow completes (green checkmark):

Visit: **https://mpaape.github.io/AurumCode/**

---

## 📄 Available Pages

| Page | URL |
|------|-----|
| **Homepage** | https://mpaape.github.io/AurumCode/ |
| **README** | https://mpaape.github.io/AurumCode/../README.html |
| **Changelog** | https://mpaape.github.io/AurumCode/../CHANGELOG.html |
| **Demo Setup Guide** | https://mpaape.github.io/AurumCode/DEMO_SETUP_GUIDE.html |
| **Current Status** | https://mpaape.github.io/AurumCode/CURRENT_STATUS.html |
| **Product Vision** | https://mpaape.github.io/AurumCode/PRODUCT_VISION.html |
| **Architecture** | https://mpaape.github.io/AurumCode/ARCHITECTURE.html |
| **Test Report** | https://mpaape.github.io/AurumCode/DOCUMENTATION_TEST.html |

All documentation files in `docs/` are accessible!

---

## 🔄 Automatic Updates

GitHub Pages automatically updates when you push to `main`:

1. **Push to main branch**
   ```bash
   git push origin main
   ```

2. **GitHub Actions automatically runs**
   - Workflow: `.github/workflows/pages.yml`
   - Deploys `docs/` directory
   - Takes ~1-2 minutes

3. **Site updates automatically**
   - No manual steps needed
   - Refresh browser to see changes

**Watch deployments:**
https://github.com/Mpaape/AurumCode/deployments

---

## 🔍 Troubleshooting

### "404 - Page not found"

**Check workflow status:**
1. Go to: https://github.com/Mpaape/AurumCode/actions
2. Look for latest "Deploy to GitHub Pages" workflow
3. Must have green checkmark ✅

**Verify Pages is enabled:**
1. Go to: https://github.com/Mpaape/AurumCode/settings/pages
2. Source should be: **GitHub Actions**

**Check deployment:**
1. Go to: https://github.com/Mpaape/AurumCode/deployments
2. Latest deployment should show "Active"

### Workflow Failed (Red X)

**View workflow logs:**
1. Go to: https://github.com/Mpaape/AurumCode/actions
2. Click the failed workflow
3. Click on the red X job
4. Review error messages

**Common fixes:**
- Ensure `.github/workflows/pages.yml` exists
- Ensure `docs/index.html` exists
- Check repository permissions

### Changes Not Showing Up

**Clear browser cache:**
- Hard refresh: `Ctrl + F5` (Windows) or `Cmd + Shift + R` (Mac)

**Wait for deployment:**
- Check: https://github.com/Mpaape/AurumCode/actions
- Latest workflow must complete (green checkmark)
- Can take 1-2 minutes after push

**Verify workflow ran:**
- Should auto-trigger on push to main
- Can manually trigger: Actions → Deploy to GitHub Pages → Run workflow

---

## ⚙️ Configuration Files

### `.github/workflows/pages.yml`
GitHub Actions workflow that deploys `docs/` to GitHub Pages.

**Triggers:**
- Push to `main` branch
- Manual dispatch (Actions tab → Run workflow)

**What it does:**
1. Checks out code
2. Uploads `docs/` folder as artifact
3. Deploys to GitHub Pages
4. Site available at: https://mpaape.github.io/AurumCode/

### `.nojekyll`
Tells GitHub Pages not to use Jekyll processing.

Required for:
- Serving raw HTML files
- Preserving file names with underscores
- Faster deployment

---

## 🎨 Customize Homepage

Edit the homepage by modifying:

```
docs/index.html
```

**After editing:**
```bash
git add docs/index.html
git commit -m "docs: Update homepage"
git push origin main
```

Site auto-updates in 1-2 minutes!

---

## 📊 Check Deployment Status

### Quick Check
Visit: https://github.com/Mpaape/AurumCode/deployments

### Detailed Workflow Logs
Visit: https://github.com/Mpaape/AurumCode/actions

### Pages Settings
Visit: https://github.com/Mpaape/AurumCode/settings/pages

---

## 🚀 Advanced: Hugo Site Deployment

Want to deploy the Hugo site instead of simple HTML?

**Option 1: Modify workflow to build Hugo**

Edit `.github/workflows/pages.yml`:

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Hugo
        uses: peaceiris/actions-hugo@v2
        with:
          hugo-version: 'latest'
          extended: true

      - name: Build Hugo Site
        run: |
          cd hugo
          hugo --minify

      - name: Upload artifact
        uses: actions/upload-pages-artifact@v3
        with:
          path: ./hugo/public
```

**Option 2: Keep both**
- Simple HTML: https://mpaape.github.io/AurumCode/
- Hugo site: https://mpaape.github.io/AurumCode/hugo/ (build manually)

---

## 📞 Support

**Issues:**
- Repository issues: https://github.com/Mpaape/AurumCode/issues
- GitHub Pages docs: https://docs.github.com/en/pages

**Workflow logs:**
- View at: https://github.com/Mpaape/AurumCode/actions
- Click on any workflow run for detailed logs

---

## ✅ Success Checklist

- [ ] GitHub Pages enabled in Settings → Pages
- [ ] Source set to "GitHub Actions"
- [ ] Latest workflow has green checkmark ✅
- [ ] Site loads at: https://mpaape.github.io/AurumCode/
- [ ] All docs accessible from homepage
- [ ] Site updates automatically on push to main

---

**Your documentation is live at:** https://mpaape.github.io/AurumCode/

🎉 **Share this link with your team!**
