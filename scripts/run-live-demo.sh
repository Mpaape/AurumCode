#!/bin/bash
# AurumCode Live Demo Script
# This demonstrates all 3 use cases: Code Review, Documentation, QA Testing
# With baseline testing to prevent blocking on pre-existing failures

set -euo pipefail

echo "🎬 AurumCode Live Demo - All 3 Use Cases"
echo "========================================"
echo ""
echo "This demo will show:"
echo "  1. Code Review blocking bad code (ISO/IEC 25010 violations)"
echo "  2. Code Review approving fixed code"
echo "  3. Documentation auto-generation (Changelog)"
echo "  4. QA Testing with baseline comparison (smart failure detection)"
echo ""

# Check for required tools
if ! command -v gh &> /dev/null; then
    echo "❌ Error: GitHub CLI (gh) is required"
    echo "Install from: https://cli.github.com/"
    exit 1
fi

# Check for demo repo
read -r -p "Enter your demo repository (owner/repo): " DEMO_REPO

if [ -z "$DEMO_REPO" ]; then
    echo "❌ Error: Demo repository required"
    exit 1
fi

echo ""
echo "Using repository: $DEMO_REPO"
echo ""

# Clone or use existing repo
REPO_NAME=$(echo "$DEMO_REPO" | cut -d'/' -f2)

if [ ! -d "$REPO_NAME" ]; then
    echo "Cloning repository..."
    gh repo clone "$DEMO_REPO"
fi

cd "$REPO_NAME"

echo ""
echo "============================================"
echo "PHASE 1: BAD CODE (Will be blocked by review)"
echo "============================================"
echo ""
echo "Creating PR with intentional anti-patterns:"
echo "  - SQL Injection vulnerabilities"
echo "  - Missing authentication checks"
echo "  - Plain text passwords"
echo "  - No pagination (performance)"
echo "  - High cyclomatic complexity"
echo "  - CSRF vulnerabilities"
echo ""

# Create bad code branch
git checkout -b demo/bad-code-anti-patterns

# Copy bad code
mkdir -p demo
cp ../demo/bad-code.go demo/

git add demo/bad-code.go
git commit -m "feat: Add user service with multiple features

Implements user CRUD operations including:
- Get user by ID
- Create new users
- Delete users
- List all users
- Update user information
- Calculate discounts based on user type"

git push origin demo/bad-code-anti-patterns

echo ""
echo "Creating PR..."
PR_URL=$(gh pr create \
    --title "feat: Add user service" \
    --body "Implements complete user management service with CRUD operations and discount calculation.

**Features:**
- User authentication
- CRUD operations
- Discount calculation
- Full API coverage" \
    --base main \
    --head demo/bad-code-anti-patterns)

echo ""
echo "✅ PR Created: $PR_URL"
echo ""
echo "⏳ Waiting for AurumCode to review..."
echo "    (This usually takes 15-30 seconds)"
echo ""

# Wait for status checks
sleep 10

echo "Checking PR status..."
gh pr view --json statusCheckRollup

echo ""
echo "Expected AurumCode Review:"
echo "  ❌ BLOCKED - Multiple security issues found"
echo ""
echo "Issues that will be found:"
echo "  🔴 Line 14: SQL Injection in GetUser"
echo "  🔴 Line 26: Plain text password storage"
echo "  🔴 Line 28: Missing authentication check"
echo "  🔴 Line 52: Exposing passwords in response"
echo "  🔴 Line 76: SQL injection in DeleteUser"
echo "  🔴 Line 91: Missing CSRF protection"
echo "  🔴 Line 102: High cyclomatic complexity (>10)"
echo ""
echo "ISO/IEC 25010 Scores (expected):"
echo "  - Security: 3/10 ⚠️"
echo "  - Maintainability: 4/10"
echo "  - Reliability: 5/10"
echo "  - Performance: 4/10"
echo ""

read -r -p "Press Enter to continue to Phase 2 (fixing the code)..."

echo ""
echo "============================================"
echo "PHASE 2: FIXED CODE (Will be approved)"
echo "============================================"
echo ""
echo "Creating new PR with all issues fixed:"
echo "  ✅ Parameterized queries (no SQL injection)"
echo "  ✅ Password hashing"
echo "  ✅ Authentication checks"
echo "  ✅ Pagination"
echo "  ✅ Reduced complexity (strategy pattern)"
echo "  ✅ CSRF protection"
echo ""

# Create fixed code branch
git checkout main
git checkout -b demo/good-code-fixed

# Copy good code
cp ../demo/good-code.go demo/

git add demo/good-code.go
git commit -m "fix: Secure user service implementation

Addresses all security and quality issues:

**Security:**
- Use parameterized queries to prevent SQL injection
- Hash passwords before storage
- Add authentication checks on all endpoints
- Add authorization checks for admin operations
- Implement CSRF token validation
- Never expose passwords in API responses

**Performance:**
- Add pagination to list endpoint
- Optimize loop string building

**Maintainability:**
- Reduce cyclomatic complexity with strategy pattern
- Extract discount calculation logic
- Improve variable naming
- Add proper error handling

**Reliability:**
- Replace panic with proper error handling
- Add comprehensive logging
- Validate all inputs

Fixes: #1"

git push origin demo/good-code-fixed

echo ""
echo "Creating PR..."
PR_URL_FIXED=$(gh pr create \
    --title "fix: Secure user service implementation" \
    --body "Fixes all security and quality issues from previous PR.

## Changes

### 🔒 Security Improvements
- ✅ Parameterized queries (prevents SQL injection)
- ✅ Password hashing (SHA-256)
- ✅ Authentication checks on all endpoints
- ✅ Authorization checks for admin operations
- ✅ CSRF token validation
- ✅ No password exposure in responses

### ⚡ Performance Improvements
- ✅ Pagination on list endpoint
- ✅ Efficient string building

### 🧹 Maintainability Improvements
- ✅ Strategy pattern for discount calculation
- ✅ Clear variable naming
- ✅ Single responsibility principle

### 🛡️ Reliability Improvements
- ✅ Proper error handling (no panics)
- ✅ Input validation
- ✅ Comprehensive logging

Fixes: #1" \
    --base main \
    --head demo/good-code-fixed)

echo ""
echo "✅ PR Created: $PR_URL_FIXED"
echo ""
echo "⏳ Waiting for AurumCode to review..."
sleep 10

echo "Checking PR status..."
gh pr view --json statusCheckRollup

echo ""
echo "Expected AurumCode Review:"
echo "  ✅ APPROVED - All issues resolved!"
echo ""
echo "ISO/IEC 25010 Scores (expected):"
echo "  - Security: 9/10 ✅"
echo "  - Maintainability: 8/10 ✅"
echo "  - Reliability: 9/10 ✅"
echo "  - Performance: 8/10 ✅"
echo ""

read -r -p "Press Enter to continue to Phase 3 (merging and documentation)..."

echo ""
echo "============================================"
echo "PHASE 3: DOCUMENTATION GENERATION"
echo "============================================"
echo ""
echo "Merging approved PR..."

gh pr merge "$PR_URL_FIXED" --squash --delete-branch

echo ""
echo "✅ PR Merged!"
echo ""
echo "⏳ Waiting for documentation generation..."
echo "    (AurumCode will auto-generate CHANGELOG.md on merge)"
sleep 15

echo ""
echo "Expected Documentation:"
echo "  📝 CHANGELOG.md updated with:"
echo "    - New 'Fixed' section"
echo "    - Conventional commit parsing"
echo "    - Grouped by type (feat, fix, docs)"
echo ""
echo "  📄 README.md sections updated (if markers present)"
echo ""
echo "  🌐 Documentation site deployed to GitHub Pages"
echo "    URL: https://$(echo "$DEMO_REPO" | cut -d'/' -f1).github.io/$(echo "$DEMO_REPO" | cut -d'/' -f2)/"
echo ""

# Show CHANGELOG
if [ -f "CHANGELOG.md" ]; then
    echo "Generated CHANGELOG.md:"
    echo "----------------------"
    head -n 30 CHANGELOG.md
    echo ""
fi

read -r -p "Press Enter to continue to Phase 4 (QA Testing with baseline)..."

echo ""
echo "============================================"
echo "PHASE 4: QA TESTING WITH BASELINE COMPARISON"
echo "============================================"
echo ""
echo "Creating PR that doesn't break tests..."

git checkout main
git pull
git checkout -b demo/qa-test-demo

# Make a safe change
echo "// Safe documentation update" >> demo/good-code.go

git add demo/good-code.go
git commit -m "docs: Add code comments"

git push origin demo/qa-test-demo

echo ""
echo "Creating PR..."
PR_URL_QA=$(gh pr create \
    --title "docs: Add code comments" \
    --body "Adds documentation comments to user service." \
    --base main \
    --head demo/qa-test-demo)

echo ""
echo "✅ PR Created: $PR_URL_QA"
echo ""
echo "⏳ Waiting for QA pipeline..."
sleep 15

echo ""
echo "Expected QA Report:"
echo "  🔍 Baseline Testing Active"
echo ""
echo "  ✅ BASELINE tests (before your changes): X passed, Y failed"
echo "  ✅ CURRENT tests (after your changes): X passed, Y failed"
echo ""
echo "  Result: ✅ No new failures (no regressions)"
echo ""
echo "  If tests were already failing:"
echo "    ⚠️ Pre-existing failures noted (doesn't block PR)"
echo "    ✅ Your changes didn't break anything"
echo ""
echo "Coverage:"
echo "  - Line Coverage: XX%"
echo "  - Branch Coverage: XX%"
echo "  - Gate: 80% (pass/fail)"
echo ""

echo "============================================"
echo "DEMO COMPLETE! 🎉"
echo "============================================"
echo ""
echo "Summary of what happened:"
echo ""
echo "1. ❌ BAD CODE PR - Blocked by Code Review"
echo "   - Found 8+ security/quality issues"
echo "   - ISO scores: Security 3/10, Maintainability 4/10"
echo "   - Line-by-line comments with fixes"
echo "   - File-level summaries"
echo "   - Overall PR assessment"
echo ""
echo "2. ✅ FIXED CODE PR - Approved by Code Review"
echo "   - All issues resolved"
echo "   - ISO scores: Security 9/10, Maintainability 8/10"
echo "   - Detailed positive feedback"
echo ""
echo "3. 📚 DOCUMENTATION AUTO-GENERATED"
echo "   - CHANGELOG.md updated from commits"
echo "   - README sections updated"
echo "   - GitHub Pages deployed"
echo ""
echo "4. 🧪 QA TESTING WITH BASELINE"
echo "   - Tests before/after comparison"
echo "   - Only blocks on NEW failures (regressions)"
echo "   - Pre-existing failures informational only"
echo "   - Coverage reporting and gates"
echo ""
echo "🔗 View PRs:"
echo "   Bad code: $PR_URL"
echo "   Fixed code: $PR_URL_FIXED"
echo "   QA test: $PR_URL_QA"
echo ""
echo "📚 View documentation:"
echo "   https://$(echo "$DEMO_REPO" | cut -d'/' -f1).github.io/$(echo "$DEMO_REPO" | cut -d'/' -f2)/"
echo ""
echo "🎬 Demo repository ready for presentations!"
