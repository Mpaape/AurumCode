---
layout: default
title: DEMO SETUP GUIDE
parent: Documentation
nav_order: 5
---

# AurumCode Demo Setup Guide

**Status:** Code Review Pipeline 100% Complete - Ready for Live Demo

This guide walks through setting up a complete end-to-end demonstration of AurumCode's Code Review pipeline (Use Case #1).

---

## Prerequisites

### Required Software

- **Go 1.21+** - [Download](https://go.dev/dl/)
- **Git** - [Download](https://git-scm.com/downloads)
- **ngrok** (or similar tunnel) - [Download](https://ngrok.com/download)
- **GitHub Account** with admin access to create webhooks

### Required API Keys

- **GitHub Personal Access Token** with `repo` scope
  - Create at: https://github.com/settings/tokens
  - Required permissions: `repo` (full control of private repositories)

- **OpenAI API Key** (recommended) OR **Anthropic API Key**
  - OpenAI: https://platform.openai.com/api-keys
  - Anthropic: https://console.anthropic.com/

---

## Step 1: Clone and Build

```bash
# Clone the repository
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode

# Verify Go installation
go version  # Should be 1.21 or higher

# Download dependencies
go mod download

# Build the server
go build -o aurumcode-server ./cmd/server

# Verify build succeeded
ls -la aurumcode-server*
```

---

## Step 2: Configure Environment Variables

Create a `.env` file in the project root:

```bash
# GitHub Configuration
GITHUB_TOKEN=ghp_your_github_token_here
GITHUB_WEBHOOK_SECRET=your_webhook_secret_here

# LLM Provider (choose one)
OPENAI_API_KEY=sk-your_openai_key_here
# OR
# ANTHROPIC_API_KEY=sk-ant-your_anthropic_key_here

# Server Configuration
PORT=8080
DEBUG_LOGS=true
```

**Security Note:** Never commit `.env` to git (already in `.gitignore`)

### Generate Webhook Secret

```bash
# Generate a random webhook secret
openssl rand -hex 32
# OR on Windows PowerShell:
# -join ((1..32 | ForEach-Object { '{0:X2}' -f (Get-Random -Maximum 256) }))
```

Copy the output and use it as `GITHUB_WEBHOOK_SECRET`.

---

## Step 3: Create AurumCode Configuration

Create `.aurumcode/config.yml` in your project root:

```yaml
version: "2.0"

# LLM Configuration
llm:
  provider: "openai"        # or "anthropic"
  model: "gpt-4"           # or "claude-3-5-sonnet-20241022"
  temperature: 0.3
  max_tokens: 4000
  budgets:
    daily_usd: 50.0
    per_review_tokens: 8000

# Enable Code Review
features:
  code_review: true
  code_review_on_push: false
  documentation: false       # Use Case #2 not yet implemented
  qa_testing: false          # Use Case #3 not yet implemented

# GitHub Integration
github:
  post_comments: true
  set_status: true

# Code Review Settings
code_review:
  enabled: true
  triggers:
    - pull_request
  iso_scoring:
    enabled: true
    weights:
      functionality: 1.5
      reliability: 2.0
      security: 2.5
      maintainability: 1.0
```

Copy example config if needed:

```bash
cp configs/.aurumcode/config.example.yml .aurumcode/config.yml
# Then edit with your preferences
```

---

## Step 4: Run the Server

### Option A: Run Directly (Development)

```bash
# Load environment variables
export $(cat .env | xargs)  # Linux/Mac
# OR
Get-Content .env | ForEach-Object { $var = $_.Split('='); [Environment]::SetEnvironmentVariable($var[0], $var[1]) }  # Windows PowerShell

# Run server
go run cmd/server/main.go

# You should see:
# Server starting on port 8080...
# [INFO] Health endpoint: http://localhost:8080/health
# [INFO] Webhook endpoint: http://localhost:8080/webhook
```

### Option B: Run Built Binary

```bash
# Set environment variables, then:
./aurumcode-server

# Or on Windows:
.\aurumcode-server.exe
```

---

## Step 5: Expose Server with ngrok

In a **new terminal**:

```bash
# Start ngrok
ngrok http 8080

# You'll see output like:
# Forwarding: https://abc123.ngrok.io -> http://localhost:8080
```

**Copy the HTTPS forwarding URL** (e.g., `https://abc123.ngrok.io`) - you'll need it for the webhook.

**Keep ngrok running** throughout the demo.

---

## Step 6: Create Test GitHub Repository

### Option A: Use Existing Repository

You can use any repository where you have admin access.

### Option B: Create New Test Repository

```bash
# Using GitHub CLI
gh repo create aurumcode-demo --public --description "AurumCode Demo Repository"
cd aurumcode-demo

# Initialize with some code
echo "# AurumCode Demo" > README.md
git init
git add README.md
git commit -m "Initial commit"
git branch -M main
git push -u origin main
```

Or create via GitHub web interface: https://github.com/new

---

## Step 7: Configure GitHub Webhook

1. Go to your test repository on GitHub
2. Navigate to: **Settings → Webhooks → Add webhook**

3. Configure webhook:
   - **Payload URL:** `https://your-ngrok-url.ngrok.io/webhook`
   - **Content type:** `application/json`
   - **Secret:** (paste your `GITHUB_WEBHOOK_SECRET` from `.env`)
   - **Which events would you like to trigger this webhook?**
     - Select: **Let me select individual events**
     - Check: ✅ Pull requests
     - Uncheck everything else
   - **Active:** ✅ Checked

4. Click **Add webhook**

5. GitHub will send a test ping. Verify:
   - Check your server logs for: `[INFO] Event parsed: type=ping`
   - In GitHub, webhook should show green checkmark ✅

---

## Step 8: Demo Use Case #1 - Code Review

### Create a Test PR

```bash
# In your test repository:
git checkout -b feature/test-review

# Create a simple file with intentional issues
cat <<EOF > test.go
package main

import "fmt"

func main() {
    // TODO: implement this
    password := "hardcoded123"  // Security issue!
    fmt.Println("Password:", password)
}
EOF

git add test.go
git commit -m "Add test code with security issue"
git push origin feature/test-review

# Create PR
gh pr create --title "Test: Code Review Demo" --body "Testing AurumCode automated code review"
```

Or create PR via GitHub web interface.

### Watch the Magic! ✨

1. **Check Server Logs:**
   ```
   [<uuid>] Webhook received: pull_request
   [<uuid>] Event parsed: type=pull_request repo=yourname/aurumcode-demo
   [<uuid>] Processing event: type=pull_request repo=yourname/aurumcode-demo pr=1
   [Review] Starting code review for PR #1
   [Review] Analyzing diff: 1 files, 10 lines changed
   [Review] LLM analysis complete: 3 issues found
   [Review] Posting inline comments...
   [Review] Posted 3 review comments
   [Review] Posting summary comment...
   [Review] Setting commit status: failure (3 errors found)
   [<uuid>] Event processed successfully
   ```

2. **Check GitHub PR:**
   - Navigate to your PR
   - You should see:
     - ✅ Inline comments on problematic lines
     - ✅ Summary comment with:
       - Issue breakdown (errors/warnings/info)
       - ISO/IEC 25010 quality scores
       - Metrics (lines changed, files modified)
       - Cost information (tokens used, USD)
     - ✅ Commit status (success/failure)

3. **Example Review Comment:**
   ```
   🔴 Security Issue

   Hardcoded credentials detected. Storing passwords directly in source
   code is a critical security vulnerability (CWE-798).

   Suggestion: Use environment variables or a secure secrets manager.

   Rule: security-rules/no-hardcoded-credentials
   Severity: error
   ```

4. **Example Summary Comment:**
   ```
   ## 🤖 AurumCode Review Summary

   **Issues Found:** 3 total
   - 🔴 Errors: 1
   - 🟡 Warnings: 2
   - 🔵 Info: 0

   ### Quality Metrics
   - Files changed: 1
   - Lines added: 10
   - Lines removed: 0

   ### ISO/IEC 25010 Scores
   - Security: 45/100 ⚠️
   - Maintainability: 78/100
   - Reliability: 82/100
   - Overall: 68/100

   ### Cost
   - Tokens: 1,234
   - Cost: $0.024 USD

   ---
   🤖 Generated by AurumCode
   ```

---

## Step 9: Test Different Scenarios

### Scenario A: Security Issues

```go
// test_security.go
package main

import "crypto/md5"  // Weak cryptography

func hashPassword(pwd string) string {
    h := md5.New()   // MD5 is insecure for passwords
    return string(h.Sum([]byte(pwd)))
}
```

**Expected:** AurumCode flags weak cryptography, suggests bcrypt/argon2

### Scenario B: Code Quality Issues

```go
// test_quality.go
package main

// Missing documentation
func ProcessData(x int, y int, z int, a int, b int) int {  // Too many parameters
    if x > 0 {
        if y > 0 {
            if z > 0 {  // Deep nesting
                return x + y + z + a + b
            }
        }
    }
    return 0
}
```

**Expected:** AurumCode suggests refactoring, parameter object, documentation

### Scenario C: Best Practices

```go
// test_practices.go
package main

func divide(a, b int) int {
    return a / b  // No zero check!
}
```

**Expected:** AurumCode warns about potential panic, suggests error handling

---

## Step 10: Document Results

Create `docs/DEMO_RESULTS.md` with:

1. **Screenshots:**
   - GitHub PR with inline comments
   - Summary comment with ISO scores
   - Commit status check
   - Server logs

2. **Metrics:**
   - Response time (webhook → first comment)
   - Number of issues detected
   - Accuracy of suggestions
   - Token usage and cost

3. **GitHub PR Link:**
   - Direct link to demo PR showing all comments

---

## Troubleshooting

### Issue: "Failed to load config, using defaults"

**Solution:** Create `.aurumcode/config.yml` or check file permissions

### Issue: "Invalid signature"

**Solution:**
- Verify `GITHUB_WEBHOOK_SECRET` matches in both `.env` and GitHub webhook config
- Check ngrok is forwarding correctly: `curl https://your-ngrok-url.ngrok.io/health`

### Issue: "Pipeline error: failed to fetch diff"

**Solution:**
- Verify `GITHUB_TOKEN` has `repo` permissions
- Check token isn't expired
- Verify repository name is correct in logs

### Issue: "LLM provider error"

**Solution:**
- Verify `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` is set
- Check API key is valid and has credits
- Review server logs for specific error message

### Issue: No comments appear on PR

**Solution:**
- Check server logs for "Posted N review comments"
- Verify GitHub token has write permissions
- Check `.aurumcode/config.yml` has `github.post_comments: true`
- Verify PR has actual code changes (not just README updates)

---

## Expected Performance

### Timing Benchmarks

- **Webhook Receipt → Processing Start:** < 100ms
- **Diff Analysis:** 200-500ms (depends on PR size)
- **LLM Code Review:** 5-15 seconds (depends on code complexity)
- **Posting Comments:** 1-3 seconds
- **Total Time:** ~10-20 seconds for typical PR

### Cost Estimates (OpenAI GPT-4)

- **Small PR** (< 100 lines): $0.01-0.05 USD
- **Medium PR** (100-500 lines): $0.05-0.20 USD
- **Large PR** (500+ lines): $0.20-0.50 USD

Cost tracking is logged in summary comment.

---

## Next Steps After Demo

### Sprint 2: Complete Documentation (2 days)
- Implement Documentation Pipeline (Use Case #2)
- Conventional commit changelog generation
- README section updates
- API documentation
- Static site with Hugo + Pagefind

### Sprint 3: QA Testing Pipeline (1 week)
- Implement QA Testing Pipeline (Use Case #3)
- Docker environment orchestration
- Automatic Dockerfile generation
- Multi-language test execution
- Coverage parsing and reporting

### Production Deployment
- Deploy to cloud (AWS/GCP/Azure)
- Setup monitoring and alerting
- Configure CI/CD pipeline
- Add authentication/authorization
- Implement rate limiting
- Database for event history

---

## Support

**Issues:** https://github.com/Mpaape/AurumCode/issues
**Documentation:** https://github.com/Mpaape/AurumCode/tree/main/docs
**Discussions:** https://github.com/Mpaape/AurumCode/discussions

---

**Status:** ✅ Use Case #1 (Code Review) **FULLY OPERATIONAL**

🎉 **Enjoy your AurumCode demo!** 🎉
