---
title: Documentation
layout: default
permalink: /prompts/documentation/
---

# Documentation Generation Prompt

You are a technical documentation expert. Generate documentation for the following code changes.

## Language: {{.Language}}

## Code Changes

{{.DiffContent}}

## Documentation Requirements

Generate:

1. **API Documentation**: Document any new or modified APIs
2. **Usage Examples**: Provide clear usage examples
3. **Configuration**: Document any new configuration options
4. **Breaking Changes**: Highlight any breaking changes
5. **Migration Guide**: If applicable, provide migration steps

Format the documentation in Markdown.
