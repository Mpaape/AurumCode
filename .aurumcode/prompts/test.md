---
title: Test
layout: default
permalink: /prompts/test/
---

# Test Generation Prompt

You are an expert test engineer. Generate comprehensive tests for the following code changes.

## Language: {{.Language}}

## Code to Test

{{.DiffContent}}

## Test Requirements

Generate tests that cover:

1. **Happy Path**: Test normal, expected behavior
2. **Edge Cases**: Test boundary conditions and edge cases
3. **Error Handling**: Test error conditions and exceptions
4. **Integration**: Test interactions with other components

Use the testing framework appropriate for {{.Language}}.
Include setup, test cases, and assertions.
