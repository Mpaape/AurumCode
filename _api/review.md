---
layout: default
title: Review
parent: API Reference
nav_order: 3
---

# Package `internal/review`

Automated code review engine with ISO/IEC 25010 quality scoring.

## Overview

```go
import "aurumcode/internal/review"
import "aurumcode/internal/review/iso25010"
```

Generates comprehensive code reviews using LLM analysis and objective quality metrics.

## Types

### ReviewIssue

```go
type ReviewIssue struct {
    Severity    string  // "error", "warning", "info"
    File        string
    Line        int
    Message     string
    RuleID      string
    Suggestion  string  // Optional fix
}
```

### ReviewResult

```go
type ReviewResult struct {
    Issues      []ReviewIssue
    ISOScores   map[string]float64  // 8 characteristics
    Summary     string
    Tokens      int
    Cost        float64
}
```

## ISO/IEC 25010 Quality Characteristics

The review engine scores code on 8 dimensions (0-100):

1. **Functional Suitability** - Correctness, appropriateness
2. **Performance Efficiency** - Time/resource usage
3. **Compatibility** - Interoperability, coexistence
4. **Usability** - Understandability, learnability
5. **Reliability** - Maturity, fault tolerance
6. **Security** - Confidentiality, integrity
7. **Maintainability** - Modularity, reusability
8. **Portability** - Adaptability, replaceability

## Methods

### ReviewDiff

```go
func (r *Reviewer) ReviewDiff(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error)
```

Main review function:
1. Analyzes diff (language, metrics)
2. Builds review prompt with rules
3. Sends to LLM
4. Parses structured response
5. Computes ISO scores
6. Maps issues to files/lines

## Usage Example

```go
reviewer := review.NewReviewer(cfg, llmOrch)

result, err := reviewer.ReviewDiff(ctx, diff)

fmt.Printf("Found %d issues\n", len(result.Issues))
fmt.Printf("Security Score: %.1f/100\n", result.ISOScores["security"])
fmt.Printf("Maintainability: %.1f/100\n", result.ISOScores["maintainability"])

for _, issue := range result.Issues {
    fmt.Printf("%s:%d - %s: %s\n", 
        issue.File, issue.Line, issue.Severity, issue.Message)
}
```

## Configuration

Customize review rules via `.aurumcode/rules/`:

```yaml
# .aurumcode/rules/security.yml
rules:
  - id: SEC001
    severity: error
    pattern: "hardcoded.*password|secret"
    message: "Hardcoded credentials detected"
  
  - id: SEC002
    severity: warning
    pattern: "eval\(|exec\("
    message: "Dangerous code execution"
```

---

[View Source](https://github.com/Mpaape/AurumCode/tree/main/internal/review)
