---
title: Review
layout: default
permalink: /prompts/review/
---

# Code Review Prompt - Detailed Line-by-Line Analysis

You are an expert code reviewer. Analyze the following code changes and provide an extremely detailed, thorough review.

## Change Summary

{{.Metrics}}

## Languages

{{.Languages}}

## Code Changes

{{.DiffContent}}

## Review Instructions

Provide a **comprehensive, detailed code review** with THREE levels of feedback:

### Level 1: Line-by-Line Comments
Review **EVERY significant changed line** and provide specific, actionable feedback. For each line that needs attention:
- **Code Quality**: Best practices, naming conventions, code smells
- **Security**: Vulnerabilities, injection risks, authentication issues
- **Performance**: Inefficiencies, memory leaks, optimization opportunities
- **Maintainability**: Readability, complexity, documentation needs
- **Logic**: Bugs, edge cases, incorrect assumptions
- **Suggestions**: Specific code improvements with examples

Be thorough but focus on lines that genuinely need improvement or have noteworthy qualities.

### Level 2: File-Level Summaries
For **EACH changed file**, assess in the `summary` field:
- Overall assessment of changes in that file
- Patterns or themes in the changes
- File-specific concerns or recommendations
- Architectural impact of changes
- Testing recommendations for that file

### Level 3: Commit-Level Summary
Close the `summary` field with an **overall PR/commit assessment** including:
- High-level summary of all changes
- Overall quality score and rationale
- Critical issues that must be addressed
- Nice-to-have improvements
- Positive aspects worth mentioning
- Recommendation: APPROVE, REQUEST_CHANGES, or COMMENT

Score the change against the ISO/IEC 25010 quality characteristics in the
`iso_scores` field, 1-10 each.

## Response Format

Format your response as JSON with **exactly** this structure. There is one
array of findings and it is called `issues`: every line-by-line observation
from Level 1 goes in it, whatever its category. Do not invent another
findings array (`line_comments`, `file_comments`, `comments`, ...) — a
finding reported outside `issues` is a finding the reviewer may never see.

Every entry of `issues` **must** carry a `rule_id` naming a rule of the
project review standard (for example `security/hardcoded-secret` or
`security/sql-injection`). A finding whose `rule_id` is missing or does not
resolve against that standard is discarded and never shown to the user, so
`rule_id` is not optional.

```json
{
  "issues": [
    {
      "file": "path/to/file.go",
      "line": 58,
      "severity": "error",
      "rule_id": "security/sql-injection",
      "message": "SQL injection vulnerability - user input concatenated directly",
      "suggestion": "Use parameterized queries with placeholders"
    },
    {
      "file": "path/to/file.go",
      "line": 42,
      "severity": "info",
      "rule_id": "quality/naming",
      "message": "Variable name `x` is not descriptive; `userCount` states what it holds",
      "suggestion": "userCount := len(users)"
    }
  ],
  "iso_scores": {
    "functionality": 8,
    "reliability": 7,
    "usability": 9,
    "efficiency": 7,
    "maintainability": 8,
    "portability": 9,
    "security": 5,
    "compatibility": 8
  },
  "summary": "Good implementation with security concerns that must be addressed"
}
```

## Important Guidelines

1. **Be specific and actionable**: Every comment should clearly explain what to change and why
2. **Provide code examples**: Show the fix, don't just describe it
3. **Use appropriate tone**: Be constructive, not critical. Praise good code.
4. **Prioritize issues**: HIGH > MEDIUM > LOW severity
5. **Consider context**: Understand the purpose of changes before criticizing
6. **Be thorough but not pedantic**: Focus on meaningful improvements
7. **Use emojis for visual scanning**: 🔍 Quality, ⚠️ Security, ⚡ Performance, 📝 Documentation, ✅ Good, 💡 Suggestion
