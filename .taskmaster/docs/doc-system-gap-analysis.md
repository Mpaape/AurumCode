# Documentation System Gap Analysis

**Date:** 2025-11-02
**Purpose:** Identify what exists vs what's needed for multi-language documentation system

---

## What EXISTS in AurumCode

### ✅ internal/documentation/ (Advanced but NOT integrated)
```
internal/documentation/
├── api/           - OpenAPI detector, generator, parser (100% coverage)
├── changelog/     - Conventional commit parser, writer (100% coverage)
├── readme/        - README updater with markers
├── site/          - Hugo builder + Pagefind search (WORKS!)
│   ├── hugo.go
│   ├── pagefind.go
│   └── runner.go
└── linkcheck/     - Link validation
```

**Status:** Code exists, has tests, but NOT integrated in any pipeline

### ✅ docs/ Directory (Basic markdowns only)
```
docs/
├── ARCHITECTURE.md
├── CURRENT_STATUS.md
├── DEMO_SETUP_GUIDE.md
├── PRODUCT_VISION.md
├── QUICKSTART.md
└── [15+ other .md files]
```

**Status:** No site generator structure (no _config.yml, no layouts/, no content/)

### ✅ Code Review Pipeline (100% Functional)
- Pipeline Orchestrator
- Review Pipeline implemented
- GitHub integration working
- LLM provider abstraction

### ✅ internal/analyzer/language.go
**Assumption:** Likely exists for diff analysis
**Need:** Verify if it detects languages in project files

---

## What's MISSING (User Requirements)

### ❌ Source Code Documentation Extractors
**Required languages:** Go, JavaScript/TypeScript, Python, C#, C/C++, Rust, Bash, PowerShell

**What's needed:**
- Extractors that call opensource tools:
  - **Go:** gomarkdoc
  - **JS/TS:** typedoc-plugin-markdown
  - **Python:** mkdocstrings (or pydoc-markdown)
  - **C#:** docfx or xmldocmd
  - **C/C++:** doxygen
  - **Rust:** rustdoc
  - **Bash/PowerShell:** Custom (or skip if no docstrings)

**Output:** Markdown files with function/class documentation

### ❌ Adapter Architecture
**Pattern needed:**
```go
type Extractor interface {
    Language() Language
    Extract(ctx, files, outputDir) error
    SupportsIncremental() bool
    ExtractIncremental(ctx, changedFiles, outputDir) error
}
```

**Registry:**
- Auto-register extractors
- Select appropriate extractor based on language detection

### ❌ Language Detection for Documentation
- Scan project files (not just diffs)
- Detect all languages used
- Return list of languages found

### ❌ Incremental Documentation Generation
- Detect changed files in commit
- Extract only modified files
- Update only affected pages
- Cache previous documentation

### ❌ Jekyll/Hugo Structure in docs/
**Expected structure:**
```
docs/
├── _config.yml          # Jekyll config
├── index.md             # Homepage
├── stack/               # Category 2
├── architecture/        # Category 3 (optional)
├── tutorials/           # Category 4
├── api/                 # Category 5 (GENERATED)
│   ├── go/
│   ├── javascript/
│   ├── python/
│   └── csharp/
├── roadmap/             # Category 6 (optional)
└── custom/              # User markdown files
```

### ❌ Normalizer (Jekyll/Hugo front matter)
- Add YAML front matter to all generated markdowns
- Ensure consistent template
- Support just-the-docs theme

### ❌ Documentation Pipeline Implementation
**File:** `internal/pipeline/docs_pipeline.go` (currently just a stub)

**Flow needed:**
1. Detect languages in project
2. Run appropriate extractors
3. Normalize markdowns (add front matter)
4. Build site (Jekyll or Hugo)
5. Deploy to GitHub Pages

### ❌ Code Review Integration for Documentation
- AurumCode can suggest documentation comments during review
- Comments follow language standards (GoDoc, JSDoc, XML docs, etc)
- Comments are extracted in next doc generation

### ❌ GitHub Actions Workflow
- Containerized (Docker)
- Installs all doc tools (gomarkdoc, typedoc, etc)
- Runs on push to main or PR
- Deploys to gh-pages

---

## Orphaned Code to REMOVE

### ❌ internal/testing/* (~1500 lines)
**Reason:** Decision made to use LLM-based testgen for multi-language scalability

**Files to remove:**
```
internal/testing/
├── executor/    # Static executors (Go, Python, JS)
├── unit/        # Template-based generation
├── api/         # API test generation
└── mock/        # Mock generation
```

**Validation:** Already identified in `docs/CLEANUP_PLAN.md`

---

## Decision: Jekyll vs Hugo

### What EXISTS: Hugo builder (internal/documentation/site/hugo.go)
### What GitHub Pages Supports Natively: Jekyll
### What PRD Original Said: Hugo + Pagefind

**Recommendation:** Keep Hugo
**Reasons:**
1. Hugo builder already implemented and tested
2. Faster than Jekyll (single binary)
3. Works in Docker easily
4. Pagefind integration already exists
5. Just-the-Docs theme can be ported to Hugo

**Alternative:** Add Jekyll support and let user choose via config

---

## Architecture to Implement

```
New Structure:

internal/documentation/
├── extractors/              # NEW
│   ├── interface.go         # Extractor interface
│   ├── detector.go          # Language detection
│   ├── registry.go          # Registry pattern
│   ├── go/
│   │   └── gomarkdoc.go     # Adapter for gomarkdoc CLI
│   ├── javascript/
│   │   └── typedoc.go       # Adapter for typedoc CLI
│   ├── python/
│   │   └── mkdocstrings.go  # Adapter for mkdocstrings
│   ├── csharp/
│   │   └── docfx.go         # Adapter for docfx
│   ├── cpp/
│   │   └── doxygen.go       # Adapter for doxygen
│   └── rust/
│       └── rustdoc.go       # Adapter for rustdoc
├── normalizer/              # NEW
│   ├── frontmatter.go       # Add YAML front matter
│   └── templates.go         # Front matter templates
├── incremental/             # NEW
│   ├── detector.go          # Detect changed files
│   └── cache.go             # Cache documentation state
├── api/                     # EXISTING (keep)
├── changelog/               # EXISTING (keep)
├── readme/                  # EXISTING (keep)
├── site/                    # EXISTING (keep & enhance)
└── linkcheck/               # EXISTING (keep)
```

---

## Gaps Summary

| Component | Status | Priority | Estimate |
|-----------|--------|----------|----------|
| Source Code Extractors | ❌ Missing | P0 - Critical | 2 weeks |
| Adapter Architecture | ❌ Missing | P0 - Critical | 3 days |
| Language Detector | ⚠️ Partial | P1 - High | 2 days |
| Incremental Generation | ❌ Missing | P1 - High | 3 days |
| Jekyll/Hugo Structure | ❌ Missing | P0 - Critical | 2 days |
| Normalizer | ❌ Missing | P0 - Critical | 2 days |
| Docs Pipeline | 🚧 Stub | P0 - Critical | 1 week |
| GitHub Actions Workflow | ❌ Missing | P1 - High | 3 days |
| Code Review Integration | ❌ Missing | P2 - Medium | 2 days |
| Cleanup (testing/*) | ❌ Pending | P0 - Critical | 1 day |

**Total Estimate:** 4-5 weeks for complete implementation

---

## Next Steps

1. **Remove orphaned code** (internal/testing/*)
2. **Verify language detector** exists in internal/analyzer/
3. **Create incremental PRD** with:
   - Cleanup tasks
   - Extractor implementation
   - Pipeline integration
   - QA gates for each phase
4. **Parse PRD with TaskMaster**
5. **Begin implementation**

---

## QA Gate Requirements (User Requirement)

Every macro phase MUST have:
1. **Commit** - Code committed to Git
2. **Code Review** - Reviewed and approved
3. **QA Testing** - Tests pass, coverage maintained

**Macro Phases:**
- Phase 1: Cleanup + Setup
- Phase 2: Extractors (Go, JS, Python)
- Phase 3: Extractors (C#, C/C++, Rust)
- Phase 4: Pipeline Integration
- Phase 5: Incremental Support
- Phase 6: GitHub Actions
- Phase 7: Production Deployment

Each phase = commit + review + QA before next phase starts.
