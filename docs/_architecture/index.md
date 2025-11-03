---
layout: default
title: Architecture
nav_order: 3
has_children: true
---

# Architecture

This section describes the system architecture, design patterns, and component interactions in AurumCode.

## System Overview

AurumCode follows a modular, plugin-based architecture with clear separation of concerns:

```
┌─────────────────────────────────────────────┐
│           Documentation Pipeline            │
├─────────────────────────────────────────────┤
│  Extract → Normalize → Generate → Publish   │
└─────────────────────────────────────────────┘
         │           │           │         │
         ▼           ▼           ▼         ▼
    Extractors   Normalizer   Generator  Publisher
```

## Core Components

### 1. Extractor Registry
- Language-specific documentation extractors
- Plugin-based registration system
- Validation and tool checking

### 2. Site Builder
- Jekyll configuration management
- Theme customization
- Content organization

### 3. Pipeline Orchestrator
- Coordinates extraction, normalization, and generation
- Handles incremental updates
- Manages build artifacts

### 4. LLM Integration
- AI-powered welcome page generation
- Documentation enhancement
- TOTVS DTA API integration

## Design Patterns

- **Strategy Pattern** - Language-specific extractors
- **Registry Pattern** - Dynamic extractor registration
- **Command Pattern** - Shell command execution
- **Template Method** - Common extraction workflow

---

*Note: Detailed architecture diagrams and component documentation will be added here.*
