# Documentation System Implementation Plan

**Created:** 2025-11-03
**Total Tasks:** 32 (16 new documentation tasks)
**Approach:** Macro Phases with QA Gates

---

## Overview

This plan implements a multi-language documentation system following the PRD structure with **mandatory QA gates** at each macro phase.

### QA Gate Requirements (MANDATORY)

Every macro phase **MUST** complete:
1. ✅ **Commit** - All code changes committed to Git
2. ✅ **Code Review** - PR created, reviewed, and approved
3. ✅ **QA Testing** - All tests pass (unit + integration)
4. ✅ **Coverage Check** - Maintain or increase test coverage
5. ✅ **Build Verification** - `go build ./cmd/...` succeeds
6. ✅ **CI/CD** - GitHub Actions passes

**No phase starts until previous phase passes all gates!**

---

## Macro Phase 1: Foundation & Cleanup

**Tasks:** #17-18
**Goal:** Remove orphaned code, migrate Hugo to Jekyll
**Estimated Subtasks:** 7 total

### Phase 1A: Remove Orphaned Testing Framework (#17)
**Subtasks:** 3

1. Remove `internal/testing/` directory structure
2. Search and update all imports referencing removed packages
3. Validate build and test suite after removal

**QA Gate 1A:**
- ✅ Commit: `refactor: remove orphaned testing framework`
- ✅ Code Review: Create PR `refactor/remove-orphaned-testing`
- ✅ QA: Run `go test ./...` and verify all pass
- ✅ Coverage: Check coverage did not drop
- ✅ Build: `go build ./cmd/...` succeeds
- ✅ Merge: PR approved and merged to main

### Phase 1B: Migrate Hugo to Jekyll (#18)
**Subtasks:** 4

1. Create Jekyll builder implementation
2. Remove Hugo files and update references
3. Update SiteBuilder to use Jekyll
4. Create integration tests for Jekyll

**QA Gate 1B:**
- ✅ Commit: `refactor: migrate from Hugo to Jekyll`
- ✅ Code Review: Create PR `refactor/migrate-hugo-to-jekyll`
- ✅ QA: Run `go test ./internal/documentation/site/...`
- ✅ Integration: Test Jekyll validation works
- ✅ Build: Verify no Hugo references remain
- ✅ Merge: PR approved and merged to main

---

## Macro Phase 2: Extractor Architecture

**Tasks:** #19-20
**Goal:** Create pluggable extractor system and language detection
**Estimated Subtasks:** 7 total

### Phase 2A: Extractor Interface & Registry (#19)
**Subtasks:** 4

1. Define core Extractor interface and types
2. Implement Registry pattern for extractor management
3. Create unit tests for registry operations
4. Document extractor contract and usage

**QA Gate 2A:**
- ✅ Commit: `feat(docs): add extractor interface and registry`
- ✅ Code Review: PR `feat/extractor-architecture`
- ✅ QA: Unit tests with 85%+ coverage
- ✅ Tests: Mock extractor tests pass
- ✅ Build: Interface compiles cleanly
- ✅ Merge: Approved and merged

### Phase 2B: Language Detection System (#20)
**Subtasks:** 3

1. Implement language detector with file extension mapping
2. Add directory exclusion logic (vendor/, node_modules/)
3. Create comprehensive unit tests with mock filesystem

**QA Gate 2B:**
- ✅ Commit: `feat(docs): add language detector`
- ✅ Code Review: PR `feat/language-detector`
- ✅ QA: Unit tests with mock filesystem, 90%+ coverage
- ✅ Tests: Test all 10 supported languages detected
- ✅ Build: Detector integrates with extractor interface
- ✅ Merge: Approved and merged

---

## Macro Phase 3: Language Extractors (High Priority)

**Tasks:** #21-25
**Goal:** Implement extractors for 8+ programming languages
**Estimated Subtasks:** 23 total

### Phase 3A: Go Extractor (#21)
**Subtasks:** 4
- gomarkdoc installation check
- Extract command execution
- Output parsing and validation
- Integration test with sample Go project

**QA Gate 3A:**
- ✅ Commit: `feat(docs): add Go extractor`
- ✅ Code Review: PR `feat/go-extractor`
- ✅ QA: Integration test with `tests/fixtures/go-sample/`
- ✅ Tests: gomarkdoc command execution validated
- ✅ Build: Extractor registered in registry
- ✅ Merge: Approved and merged

### Phase 3B: JavaScript/TypeScript Extractor (#22)
**Subtasks:** 4
- typedoc + plugin installation check
- Extract for both JS and TS
- Output format validation
- Integration tests for JS and TS projects

**QA Gate 3B:**
- ✅ Commit: `feat(docs): add JavaScript/TypeScript extractor`
- ✅ Code Review: PR `feat/javascript-extractor`
- ✅ QA: Integration tests with JS and TS fixtures
- ✅ Tests: Both JS and TS projects documented
- ✅ Build: Extractor handles both languages
- ✅ Merge: Approved and merged

### Phase 3C: Python Extractor (#23)
**Subtasks:** 3
- pydoc-markdown installation and validation
- Module extraction implementation
- Integration test with sample Python project

**QA Gate 3C:**
- ✅ Commit: `feat(docs): add Python extractor`
- ✅ Code Review: PR `feat/python-extractor`
- ✅ QA: Integration test with `tests/fixtures/python-sample/`
- ✅ Tests: Python docstrings extracted correctly
- ✅ Build: Works with various Python module structures
- ✅ Merge: Approved and merged

### Phase 3D: C# Extractor (#24)
**Subtasks:** 4
- xmldocmd tool installation
- XML doc file generation from build
- Markdown conversion
- Integration test with .NET project

**QA Gate 3D:**
- ✅ Commit: `feat(docs): add C# extractor`
- ✅ Code Review: PR `feat/csharp-extractor`
- ✅ QA: Integration test with `tests/fixtures/csharp-sample/`
- ✅ Tests: XML docs converted to markdown
- ✅ Build: .NET project documentation works
- ✅ Merge: Approved and merged

### Phase 3E: Multi-Language Extractors (#25)
**Subtasks:** 6 (C/C++, Rust, Bash, PowerShell)

1. Implement C/C++ extractor (doxygen + doxybook2)
2. Implement Rust extractor (rustdoc/cargo doc)
3. Implement Bash extractor (shdoc)
4. Implement PowerShell extractor (platyPS)
5. Create integration tests for all 4 languages
6. Validate extractor registration and execution

**QA Gate 3E:**
- ✅ Commit: `feat(docs): add C/C++, Rust, Bash, PowerShell extractors`
- ✅ Code Review: PR `feat/multi-language-extractors`
- ✅ QA: Integration tests for all 4 languages
- ✅ Tests: Each extractor validated with sample project
- ✅ Build: All extractors registered in registry
- ✅ Merge: Approved and merged

---

## Macro Phase 4: Site Infrastructure

**Tasks:** #26-27
**Goal:** Setup Jekyll site and markdown normalization
**Estimated Subtasks:** 6 total

### Phase 4A: Jekyll Site Structure (#26)
**Subtasks:** 3

1. Create Jekyll _config.yml with just-the-docs theme
2. Setup directory structure (stack/, architecture/, tutorials/, api/)
3. Test local Jekyll build

**QA Gate 4A:**
- ✅ Commit: `feat(docs): setup Jekyll site structure`
- ✅ Code Review: PR `feat/jekyll-site-structure`
- ✅ QA: `bundle exec jekyll build` succeeds locally
- ✅ Tests: Site renders correctly in browser
- ✅ Build: Navigation and search functional
- ✅ Merge: Approved and merged

### Phase 4B: Markdown Normalizer (#27)
**Subtasks:** 3

1. Implement front matter addition logic
2. Create NormalizeDir batch processing
3. Unit tests for normalization

**QA Gate 4B:**
- ✅ Commit: `feat(docs): add markdown normalizer`
- ✅ Code Review: PR `feat/markdown-normalizer`
- ✅ QA: Unit tests with 85%+ coverage
- ✅ Tests: Front matter added correctly
- ✅ Build: Idempotent normalization verified
- ✅ Merge: Approved and merged

---

## Macro Phase 5: LLM Features

**Tasks:** #28-29
**Goal:** LLM-powered welcome page and incremental support
**Estimated Subtasks:** 8 total

### Phase 5A: Welcome Page Generator (#28)
**Subtasks:** 4

1. Create generator with LLM orchestrator integration
2. Implement prompt template for welcome page
3. Test with mock LLM
4. Generate actual welcome page from README

**QA Gate 5A:**
- ✅ Commit: `feat(docs): add LLM-powered welcome page generator`
- ✅ Code Review: PR `feat/llm-welcome-page`
- ✅ QA: Unit tests with mock LLM
- ✅ Tests: Generate welcome page from real README
- ✅ Build: Beautiful index.md created
- ✅ Merge: Approved and merged

### Phase 5B: Incremental Documentation (#29)
**Subtasks:** 4

1. Implement git diff-based change detection
2. Create cache system for source→doc mappings
3. Add incremental extraction logic
4. Unit tests with mock git repo

**QA Gate 5B:**
- ✅ Commit: `feat(docs): add incremental documentation support`
- ✅ Code Review: PR `feat/incremental-docs`
- ✅ QA: Unit tests with mock git repo, 85%+ coverage
- ✅ Tests: Changed files detected correctly
- ✅ Build: Cache serialization works
- ✅ Merge: Approved and merged

---

## Macro Phase 6: Pipeline Integration

**Tasks:** #30
**Goal:** Complete end-to-end documentation pipeline
**Estimated Subtasks:** 5

### Phase 6: Documentation Pipeline (#30)
**Subtasks:** 5

1. Implement docs_pipeline.go Run() method
2. Wire up all extractors and components
3. Add full vs incremental mode logic
4. Implement GitHub Pages deployment
5. End-to-end integration test

**QA Gate 6:**
- ✅ Commit: `feat(docs): implement complete documentation pipeline`
- ✅ Code Review: PR `feat/docs-pipeline-implementation`
- ✅ QA: End-to-end test with multi-language project
- ✅ Tests: Full pipeline generates docs correctly
- ✅ Build: Jekyll site validates
- ✅ Integration: Deploy to gh-pages branch works
- ✅ Merge: Approved and merged

---

## Macro Phase 7: Automation & Configuration

**Tasks:** #31-32
**Goal:** GitHub Actions workflow and user configuration
**Estimated Subtasks:** 7 total

### Phase 7A: GitHub Actions Workflow (#31)
**Subtasks:** 4

1. Create Dockerfile with all documentation tools
2. Implement GitHub Actions workflow YAML
3. Test workflow on sample repository
4. Verify deployment to GitHub Pages

**QA Gate 7A:**
- ✅ Commit: `ci: add GitHub Actions workflow for documentation`
- ✅ Code Review: PR `ci/documentation-workflow`
- ✅ QA: Dockerfile builds successfully
- ✅ Tests: Workflow runs on test push
- ✅ Build: Documentation generated in Docker
- ✅ Deploy: Site accessible at GitHub Pages URL
- ✅ Merge: Approved and merged

### Phase 7B: Configuration Support (#32)
**Subtasks:** 3

1. Add DocumentationConfig to types
2. Update config.example.yml
3. Add configuration parsing tests

**QA Gate 7B:**
- ✅ Commit: `feat(docs): add documentation configuration`
- ✅ Code Review: PR `feat/documentation-config`
- ✅ QA: Config parsing tests pass
- ✅ Tests: Example config validates
- ✅ Build: Configuration integrates with pipeline
- ✅ Merge: Approved and merged

---

## Macro Phase 8: Production Validation (Final QA)

**Goal:** End-to-end system validation with real project
**Duration:** 1-2 days

### Final QA Checklist

#### Code Quality
- ✅ All unit tests pass: `go test ./...`
- ✅ Integration tests pass
- ✅ Coverage >= 80% overall
- ✅ No linting errors: `golangci-lint run`
- ✅ Build succeeds: `go build ./cmd/...`

#### Functional Testing
- ✅ Test with AurumCode repository itself
- ✅ All 8+ languages detected and documented
- ✅ Jekyll site builds without errors
- ✅ Search functionality works (Pagefind)
- ✅ Navigation hierarchy correct
- ✅ Welcome page generated from README
- ✅ GitHub Pages deployment successful

#### Performance Testing
- ✅ Full generation completes in <5 minutes
- ✅ Incremental generation <30 seconds
- ✅ Memory usage acceptable
- ✅ No file descriptor leaks

#### User Experience
- ✅ Documentation site is accessible
- ✅ Links are not broken (link checker)
- ✅ Mobile responsive
- ✅ Dark mode works (if supported by theme)
- ✅ Code examples render correctly

#### CI/CD Validation
- ✅ GitHub Actions workflow triggers on push
- ✅ Docker container builds successfully
- ✅ Workflow completes in <10 minutes
- ✅ Deployment to gh-pages succeeds
- ✅ Site updates visible within 5 minutes

#### Documentation
- ✅ README updated with documentation features
- ✅ Configuration examples provided
- ✅ Extractor installation instructions clear
- ✅ Troubleshooting guide complete

### Final Gate: Production Release

**After all validations pass:**
1. ✅ Create release tag (e.g., `v2.0.0-docs`)
2. ✅ Update CHANGELOG.md
3. ✅ Publish GitHub release with notes
4. ✅ Update main documentation site
5. ✅ Announce feature completion

---

## Progress Tracking

Use TaskMaster to track progress:

```bash
# See current task
task-master next

# Mark task in progress
task-master set-status --id=<id> --status=in-progress

# Mark subtask complete
task-master set-status --id=<id>.<subtask> --status=done

# View overall progress
task-master list

# View complexity report
task-master complexity-report
```

---

## Risk Mitigation

### Risk: Docker Build Time
**Mitigation:** Use Docker layer caching in GitHub Actions, pre-built base images

### Risk: Extractor Tool Failures
**Mitigation:** Graceful degradation - skip failed extractors, log warnings

### Risk: LLM API Costs
**Mitigation:** Cache welcome page, only regenerate when README changes

### Risk: GitHub Pages Build Failures
**Mitigation:** Validate Jekyll locally before push, use CI checks

---

## Success Criteria

✅ All 16 new documentation tasks completed
✅ All 61 subtasks completed
✅ All QA gates passed
✅ 8+ languages supported and tested
✅ GitHub Pages site deployed and accessible
✅ Full and incremental modes working
✅ CI/CD automation functional
✅ Documentation complete and accurate

---

**Next Step:** Begin Macro Phase 1A - Remove Orphaned Testing Framework (#17)

Run: `task-master set-status --id=17 --status=in-progress`
