# Run Documentation Pipeline

This guide shows how to actually RUN the AurumCode Documentation Pipeline (not manually create docs).

## What This Does

The Documentation Pipeline (`internal/pipeline/docs_pipeline.go`) will:

1. **Fetch git commits** using `git log`
2. **Parse conventional commits** (feat, fix, docs, etc.)
3. **Generate CHANGELOG.md** automatically
4. **Update README.md** sections automatically
5. **Detect OpenAPI specs** and generate API docs
6. **Build Hugo static site** (if configured)

**No manual work - AurumCode does everything!**

## Quick Start

### Step 1: Load Environment Variables

```bash
# Load from .env file
export $(cat .env | grep -v '^#' | xargs)

# Verify
echo $TOTVS_DTA_API_KEY
echo $TOTVS_DTA_BASE_URL
```

### Step 2: Build and Run

```bash
# Build the test program
go build -o test-docs-pipeline.exe ./cmd/test-docs-pipeline

# Run it!
./test-docs-pipeline.exe
```

### Step 3: Check Generated Files

The pipeline will create:
- `CHANGELOG.md` - Generated from git commits
- `README.md` - Updated with current status
- (API docs if OpenAPI specs found)
- (Hugo site in `public/` if configured)

## What the Pipeline Does (Automatically)

### 1. Fetch Recent Commits
```go
// Uses git log internally
git log --pretty=format:"%H|%an|%ae|%at|%s|%b" -n 50
```

Parses:
- Commit hash
- Author name & email
- Timestamp
- Subject line
- Body

### 2. Parse Conventional Commits

Recognizes types:
- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation
- `style:` - Code style
- `refactor:` - Refactoring
- `perf:` - Performance
- `test:` - Tests
- `build:` - Build system
- `ci:` - CI/CD
- `chore:` - Maintenance

### 3. Generate CHANGELOG.md

Groups commits by type and creates keep-a-changelog format:

```markdown
# Changelog

## [1.0.0] - 2025-11-02

### Features
- feat: Add user authentication
- feat: Add rate limiting

### Bug Fixes
- fix: Resolve memory leak

### Documentation
- docs: Update README
```

### 4. Update README Sections

Looks for markers like:
```markdown
<!-- BEGIN STATUS -->
Current info here
<!-- END STATUS -->
```

Updates content between markers without destroying other sections.

### 5. API Documentation

If OpenAPI/Swagger specs found (`.yaml`, `.yml`, `.json`):
- Parses spec
- Generates markdown documentation
- Creates `.md` file next to spec

### 6. Hugo Static Site

If `config.Outputs.DeploySite = true`:
- Runs `hugo --minify`
- Builds to `public/` directory
- Runs `pagefind` for search indexing

## Configuration

Edit `.aurumcode/config.yml`:

```yaml
outputs:
  update_docs: true      # Enable CHANGELOG + README updates
  deploy_site: false     # Enable Hugo site build (optional)
```

## Prompt Files

The pipeline uses these prompts (loaded from files):

- `.aurumcode/prompts/documentation-generation.md` - For LLM-based doc generation
- `.aurumcode/prompts/changelog-generation.md` - For changelog formatting

These are **actual files** loaded at runtime, not hardcoded strings!

## Testing Without Webhooks

The test program simulates a webhook event:

```go
event := &types.Event{
    EventType:  "push",
    Branch:     "main",
    Repo:       "AurumCode",
    RepoOwner:  "Mpaape",
    CommitSHA:  "HEAD",
}
```

Then runs:
```go
docsPipeline.Run(ctx, event)
```

This is exactly what the real webhook handler does!

## LLM Provider (TOTVS DTA)

The test program uses your TOTVS DTA credentials:

```go
// From .env file
TOTVS_DTA_BASE_URL=https://proxy.dta.totvs.ai
TOTVS_DTA_API_KEY=sk-your-totvs-dta-api-key-here
```

Configured as OpenAI-compatible provider:
```go
provider := openai.NewProvider(apiKey, "gpt-4")
provider.SetBaseURL(baseURL) // Point to TOTVS
```

## Expected Output

When you run the pipeline:

```
🚀 Testing AurumCode Documentation Pipeline
✓ Using TOTVS DTA: https://proxy.dta.totvs.ai
✓ Configured to use TOTVS DTA endpoint
✓ LLM Orchestrator created
✓ Configuration loaded
✓ Documentation Pipeline created
✓ Simulated event: push to main

📝 Running Documentation Pipeline...
─────────────────────────────────────────
[Docs] Starting documentation generation for AurumCode
[Docs] Fetching commits for changelog generation...
[Docs] Generating changelog from 6 commits...
[Docs] Changelog updated successfully
[Docs] Updating README sections...
[Docs] README updated successfully
[Docs] Checking for API specifications...
[Docs] Documentation pipeline completed successfully
─────────────────────────────────────────
✅ Documentation Pipeline completed successfully!

📊 Check the following files:
   - CHANGELOG.md
   - README.md (updated sections)

💰 Token Usage:
   - Total Tokens: 1234
   - Estimated Cost: $0.0234

🎉 Done! Check the generated files.
```

## Verification

After running, check:

```bash
# CHANGELOG was created
cat CHANGELOG.md

# README was updated
git diff README.md

# Verify it's real git changes
git status
```

You should see:
- `CHANGELOG.md` - new file
- `README.md` - modified file

These were generated by **AurumCode**, not manually!

## Commit the Results

Only commit what AurumCode generated:

```bash
git add CHANGELOG.md README.md
git commit -m "docs: Generate documentation using AurumCode pipeline

Generated by running: ./test-docs-pipeline.exe

The Documentation Pipeline automatically:
- Parsed 6 conventional commits from git log
- Generated CHANGELOG.md in keep-a-changelog format
- Updated README.md sections

This is the ACTUAL pipeline output, not manually created.

🤖 Generated by AurumCode Documentation Pipeline"

git push origin main
```

## Troubleshooting

### "TOTVS_DTA_API_KEY not set"
```bash
# Load environment
source .env
# or
export $(cat .env | grep -v '^#' | xargs)
```

### "go: command not found"
```bash
# Install Go 1.21+
# https://go.dev/dl/
```

### "Pipeline failed"
Check logs - pipeline has detailed error messages:
- Git errors: Check you're in a git repository
- Config errors: Create `.aurumcode/config.yml`
- LLM errors: Check TOTVS DTA credentials

### No CHANGELOG created
Check:
1. Are there git commits? `git log`
2. Are they conventional commits? (feat:, fix:, etc.)
3. Is `update_docs: true` in config?

## Next Steps

1. **Run the pipeline**: `./test-docs-pipeline.exe`
2. **Verify output**: Check CHANGELOG.md and README.md
3. **Commit results**: Only commit what AurumCode generated
4. **Enable webhook**: For automatic runs on every push

---

**The key difference**: This actually RUNS the AurumCode code, rather than manually doing what the code is supposed to do!
