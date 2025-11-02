---
layout: default
title: Pipeline
parent: API Reference
nav_order: 1
---

# Package `internal/pipeline`

Main pipeline orchestration for AurumCode's 3 use cases.

## Overview

```go
import "aurumcode/internal/pipeline"
```

The pipeline package coordinates the three main workflows:
1. **Code Review** - Automated PR analysis with ISO scoring
2. **Documentation** - Auto-generated docs from code and commits
3. **QA Testing** - Automated test execution and coverage

## Types

### MainOrchestrator

```go
type MainOrchestrator struct {
    config          *config.Config
    reviewPipeline  *ReviewPipeline
    docsPipeline    *DocumentationPipeline
    qaPipeline      *QATestingPipeline
}
```

Coordinates all three pipelines, running them in parallel when applicable.

### Methods

#### NewMainOrchestrator

```go
func NewMainOrchestrator(
    cfg *config.Config,
    githubClient *githubclient.Client,
    llmOrch *llm.Orchestrator,
) *MainOrchestrator
```

Creates a new main orchestrator with all pipeline dependencies.

#### ProcessEvent

```go
func (o *MainOrchestrator) ProcessEvent(ctx context.Context, event *types.Event) error
```

Main entry point. Routes GitHub webhook events to appropriate pipelines based on:
- Event type (PR, push, etc.)
- Feature flags in config
- Trigger conditions

Runs pipelines concurrently using goroutines for optimal performance.

## Usage Example

```go
// Initialize
orch := pipeline.NewMainOrchestrator(cfg, githubClient, llmOrch)

// Process webhook event
event := &types.Event{
    EventType: "pull_request",
    Action:    "opened",
    PRNumber:  123,
}

err := orch.ProcessEvent(ctx, event)
```

---

[View Source](https://github.com/Mpaape/AurumCode/tree/main/internal/pipeline)
