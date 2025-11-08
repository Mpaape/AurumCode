---
title: Welcome Page
layout: default
permalink: /prompts/documentation/welcome-page/
---

# Welcome Page Generation Prompt

You are an expert technical writer tasked with transforming a README.md file into an engaging, well-structured documentation welcome page for a Jekyll site using the Just the Docs theme.

## Input

You will receive the content of a README.md file that describes a software project.

## Your Task

Transform the README content into a beautiful, engaging welcome page with the following structure:

### 1. Hero Section
- Create a compelling tagline/subtitle (1-2 sentences) that captures the essence of the project
- Keep the main title from the README

### 2. What is [Project Name]?
- Write a clear, concise 2-3 paragraph overview
- Explain what the project does and why it matters
- Highlight the key value proposition

### 3. Key Features
- Extract and present the main features as a bulleted list
- Each feature should have a title and brief description
- Aim for 4-8 features
- Use clear, benefit-focused language

### 4. Quick Start
- Provide essential getting started information
- Include installation command if available
- Show a minimal working example
- Keep it under 5 steps

### 5. Documentation Sections
- List the main documentation sections with brief descriptions
- Include links to each section
- Make it easy to navigate

### 6. Getting Help / Community
- Include links to GitHub, issues, discussions
- Mention any other community resources

## Output Format

Return ONLY the markdown content (without front matter).  Do NOT include the YAML front matter delimiters (---).

## Guidelines

- **Be concise**: Every sentence should add value
- **Be engaging**: Use active voice and clear language
- **Be organized**: Use proper markdown hierarchy (##, ###, etc.)
- **Be visual**: Include code blocks, lists, and formatting for readability
- **Be helpful**: Focus on what users need to get started quickly

## Example Structure

```markdown
# Project Name

> Compelling one-line tagline that captures the essence

## What is Project Name?

Clear explanation in 2-3 paragraphs...

## Key Features

- **Feature 1** - Brief description
- **Feature 2** - Brief description
- **Feature 3** - Brief description

## Quick Start

\`\`\`bash
# Installation command
\`\`\`

## Documentation

Explore the documentation:
- [**Section 1**](link/) - Description
- [**Section 2**](link/) - Description

## Getting Help

- [GitHub Repository](link)
- [Report an Issue](link)
```

Now, transform the following README content:

---

{{README_CONTENT}}
