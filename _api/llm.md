---
layout: default
title: LLM
parent: API Reference
nav_order: 2
---

# Package `internal/llm`

LLM provider abstraction with cost tracking and fallback chains.

## Overview

```go
import "aurumcode/internal/llm"
```

Provides unified interface for multiple LLM providers with automatic fallbacks, cost tracking, and token budgeting.

## Supported Providers

- **OpenAI** - GPT-3.5, GPT-4
- **Anthropic** - Claude 3 (Opus, Sonnet, Haiku)
- **LiteLLM** - Proxy supporting 100+ models
- **Ollama** - Local models (llama, mistral, etc.)

## Types

### Orchestrator

```go
type Orchestrator struct {
    providers   []Provider
    costTracker *cost.Tracker
    fallback    bool
}
```

### Options

```go
type Options struct {
    MaxTokens   int
    Temperature float64
    Model       string
}
```

### Response

```go
type Response struct {
    Text   string
    Tokens int
    Cost   float64
    Model  string
}
```

## Methods

### Complete

```go
func (o *Orchestrator) Complete(ctx context.Context, prompt string, opts Options) (*Response, error)
```

Sends completion request with automatic:
- Provider selection
- Fallback on failure
- Token counting
- Budget enforcement
- Cost calculation

### Usage Example

```go
orch := llm.NewOrchestrator(cfg, providers, tracker)

resp, err := orch.Complete(ctx, "Review this code...", llm.Options{
    MaxTokens:   2000,
    Temperature: 0.3,
})

fmt.Printf("Response: %s\nCost: $%.4f\n", resp.Text, resp.Cost)
```

## Cost Tracking

```go
tracker := cost.NewTracker(cost.Config{
    DailyBudget: 100.00, // $100/day
    PerRunLimit: 5.00,   // $5/run
})

// Check before expensive operation
if !tracker.Allow(estimatedCost) {
    return errors.New("budget exceeded")
}

// Record actual cost
tracker.Spend(actualCost)
```

---

[View Source](https://github.com/Mpaape/AurumCode/tree/main/internal/llm)
