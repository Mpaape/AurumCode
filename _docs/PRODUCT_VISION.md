---
layout: default
title: PRODUCT VISION
parent: Documentation
nav_order: 10
---

# AurumCode - Visão de Produto Definitiva

**Data**: 2025-11-01
**Status**: ✅ **CLARIFICADO - Pronto para Implementação**

---

## Visão Executiva

AurumCode é uma plataforma de **automação inteligente para repositórios de código**, oferecendo 3 serviços principais integrados ao CI/CD do GitHub.

---

## 3 Casos de Uso Principais

### 🔍 **Caso de Uso #1: Code Review Automático**

**Trigger:** Push, Pull Request (GitHub Actions/Webhooks)

**Fluxo:**
```
PR criado/atualizado
    ↓
GitHub webhook/CI/CD
    ↓
AurumCode recebe evento
    ↓
Analisa diff (linguagens múltiplas)
    ↓
LLM analisa código:
  - Clean Code violations
  - Clean Architecture issues
  - Análise estática (bugs, smells)
  - Vulnerabilidades de segurança
  - ISO/IEC 25010 scoring
    ↓
Posta comentários no PR
    ↓
Atualiza status do commit
```

**Linguagens Suportadas:**
- Go, Python, JavaScript/TypeScript
- C, C++, C#
- Java, Kotlin, Swift
- Rust, Ruby, PHP
- Bash, PowerShell
- E mais...

**Customização:**
- `.aurumcode/prompts/code-review.md` - Instruções customizadas para review
- `.aurumcode/rules/code-standards.yml` - Padrões específicos do projeto
- `.aurumcode/rules/security-rules.yml` - Regras de segurança customizadas

**Output:**
- Comentários inline no PR
- Commit status (success/failure)
- Review summary com scores ISO/IEC 25010

---

### 📚 **Caso de Uso #2: Geração Automática de Documentação**

**Trigger:** Push para branch principal, mudanças em código-fonte

**Fluxo:**
```
Código alterado/commitado
    ↓
GitHub webhook/CI/CD
    ↓
AurumCode detecta mudanças
    ↓
Identifica tipo de documentação necessária:
  - Documentação inline (código)
  - CHANGELOG.md (commits convencionais)
  - README.md (overview do projeto)
  - API.md (OpenAPI/endpoints)
  - Site estático (Hugo + Pagefind)
    ↓
LLM gera/atualiza documentação
    ↓
Commit automático ou PR com docs
```

**Modos de Operação:**

#### Modo 1: Baseado em Mudanças (Padrão)
- Analisa commits/PRs
- Gera docs incrementalmente
- Mantém histórico de mudanças

#### Modo 2: Investigação (Geração Completa)
- Quando não há documentação existente
- Analisa todo o repositório
- Gera documentação completa do zero
- Utiliza RAG para contexto profundo

**Customização:**
- `.aurumcode/prompts/documentation/inline.md` - Estilo de docs inline
- `.aurumcode/prompts/documentation/changelog.md` - Formato de changelog
- `.aurumcode/prompts/documentation/readme.md` - Estrutura de README
- `.aurumcode/config.yml` - Quais docs gerar (changelog, readme, api, site)

**Output:**
- CHANGELOG.md atualizado
- README.md atualizado/gerado
- docs/API.md (se OpenAPI detectado)
- Site estático (Hugo) em gh-pages (opcional)
- Comentários inline no código

---

### 🧪 **Caso de Uso #3: QA Tester Automático**

**Trigger:** Pull Request (antes de merge)

**Fluxo:**
```
PR criado/atualizado
    ↓
GitHub webhook/CI/CD
    ↓
AurumCode analisa mudanças
    ↓
Identifica linguagem e stack
    ↓
QA Orchestrator:
  1. Detecta/Gera Dockerfile
     - Se existe: usa
     - Se não: LLM gera baseado no projeto
  2. Constrói imagem Docker
  3. Sobe ambiente(s) isolado(s)
  4. Executa bateria de testes:
     - Unit tests (se existem)
     - Integration tests
     - API tests (chamadas reais)
     - E2E tests (simulação de uso)
  5. Coleta resultados:
     - Coverage
     - Logs
     - Erros
  6. Derruba ambientes
    ↓
Gera relatório de QA
    ↓
Posta no PR + atualiza status
```

**Ambientes Suportados:**
- Docker containers (padrão)
- Docker Compose (múltiplos serviços)
- Kubernetes (futuro)

**Customização:**
- `.aurumcode/qa/environments.yml` - Definição de ambientes
  ```yaml
  environments:
    - name: api-tests
      dockerfile: Dockerfile.test
      ports:
        - "8080:8080"
      env:
        DATABASE_URL: postgresql://test:test@db:5432/test
      services:
        - postgres
      tests:
        - type: api
          command: npm run test:api
        - type: integration
          command: pytest tests/integration/

    - name: e2e-tests
      docker_compose: docker-compose.test.yml
      tests:
        - type: e2e
          command: npm run test:e2e
  ```

- `.aurumcode/qa/test-strategy.yml` - Estratégia de testes
  ```yaml
  test_strategy:
    unit:
      enabled: true
      coverage_threshold: 80
    integration:
      enabled: true
      services:
        - database
        - redis
    api:
      enabled: true
      endpoints:
        - GET /api/health
        - POST /api/users
    e2e:
      enabled: true
      scenarios:
        - login-flow
        - checkout-flow
  ```

- `.aurumcode/prompts/qa/dockerfile-generation.md` - Como gerar Dockerfile

**Geração Inteligente de Dockerfile:**

Se Dockerfile não existe, LLM analisa:
- Linguagem do projeto
- Dependências (package.json, requirements.txt, go.mod, etc.)
- Estrutura de pastas
- Framework detectado (Express, Flask, Spring Boot, etc.)

E gera Dockerfile otimizado para testes.

**Output:**
- Relatório de testes no PR
- Coverage report
- Logs de execução
- Status de cada ambiente testado
- Sugestões de melhorias (se falhas detectadas)

---

## Arquitetura Real

### Componentes Principais

```
┌─────────────────────────────────────────────────────────────┐
│                    EXTERNAL LAYER                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │  GitHub  │  │ LLM APIs │  │  Docker  │  │   RAG    │   │
│  │ Webhooks │  │ (OpenAI, │  │  Daemon  │  │  Store   │   │
│  │          │  │Anthropic)│  │          │  │ (Qdrant) │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘   │
└───────┼─────────────┼─────────────┼─────────────┼──────────┘
        │             │             │             │
┌───────┼─────────────┼─────────────┼─────────────┼──────────┐
│       │        ADAPTER LAYER      │             │          │
│  ┌────▼─────┐  ┌────▼─────┐  ┌───▼──────┐  ┌──▼───────┐  │
│  │ GitHub   │  │   LLM    │  │  Docker  │  │   RAG    │  │
│  │ Client   │  │ Provider │  │  Client  │  │  Client  │  │
│  └────┬─────┘  └────┬─────┘  └───┬──────┘  └──┬───────┘  │
└───────┼─────────────┼─────────────┼─────────────┼──────────┘
        │             │             │             │
┌───────┼─────────────┼─────────────┼─────────────┼──────────┐
│       │      CORE DOMAIN (Application Layer)    │          │
│  ┌────▼──────────────────────────────────────────────┐     │
│  │         MAIN PIPELINE ORCHESTRATOR                │     │
│  │  - Coordena os 3 casos de uso                     │     │
│  │  - Gerencia configurações                         │     │
│  │  - Controla custos/budgets                        │     │
│  └────┬──────────────────────────────────────────────┘     │
│       │                                                     │
│  ┌────▼──────────────────────────────────────────────┐     │
│  │  PIPELINE #1: CODE REVIEW                         │     │
│  │  ┌─────────────┐  ┌──────────┐  ┌─────────────┐  │     │
│  │  │ Diff        │→ │ Analyzer │→ │   LLM       │  │     │
│  │  │ Fetcher     │  │          │  │   Review    │  │     │
│  │  └─────────────┘  └──────────┘  └─────────────┘  │     │
│  │  ┌─────────────────────────────────────────────┐  │     │
│  │  │ ISO/IEC 25010 Scorer                        │  │     │
│  │  └─────────────────────────────────────────────┘  │     │
│  │  ┌─────────────────────────────────────────────┐  │     │
│  │  │ Comment Poster (GitHub)                     │  │     │
│  │  └─────────────────────────────────────────────┘  │     │
│  └───────────────────────────────────────────────────┘     │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  PIPELINE #2: DOCUMENTATION GENERATION             │   │
│  │  ┌──────────────┐  ┌──────────────┐                │   │
│  │  │ Mode         │  │ Investigation│                │   │
│  │  │ Detector     │  │ Mode (RAG)   │                │   │
│  │  └──────┬───────┘  └──────┬───────┘                │   │
│  │         │                  │                        │   │
│  │         └──────────┬───────┘                        │   │
│  │                    │                                │   │
│  │         ┌──────────▼──────────┐                     │   │
│  │         │  Documentation      │                     │   │
│  │         │  Generators:        │                     │   │
│  │         │  - Inline Docs      │                     │   │
│  │         │  - CHANGELOG        │                     │   │
│  │         │  - README           │                     │   │
│  │         │  - API Docs         │                     │   │
│  │         │  - Static Site      │                     │   │
│  │         └─────────────────────┘                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  PIPELINE #3: QA TESTING                           │   │
│  │  ┌──────────────────────────────────────────────┐  │   │
│  │  │  QA Orchestrator                             │  │   │
│  │  │  ┌──────────────┐  ┌──────────────────────┐ │  │   │
│  │  │  │ Environment  │  │ Dockerfile Generator │ │  │   │
│  │  │  │ Detector     │  │ (LLM-powered)        │ │  │   │
│  │  │  └──────┬───────┘  └──────┬───────────────┘ │  │   │
│  │  │         └──────────┬───────┘                 │  │   │
│  │  │                    │                         │  │   │
│  │  │         ┌──────────▼──────────┐              │  │   │
│  │  │         │  Docker Orchestrator│              │  │   │
│  │  │         │  - Build images     │              │  │   │
│  │  │         │  - Start containers │              │  │   │
│  │  │         │  - Network setup    │              │  │   │
│  │  │         └──────────┬──────────┘              │  │   │
│  │  │                    │                         │  │   │
│  │  │         ┌──────────▼──────────┐              │  │   │
│  │  │         │  Test Executor      │              │  │   │
│  │  │         │  - Unit tests       │              │  │   │
│  │  │         │  - Integration tests│              │  │   │
│  │  │         │  - API tests        │              │  │   │
│  │  │         │  - E2E tests        │              │  │   │
│  │  │         └──────────┬──────────┘              │  │   │
│  │  │                    │                         │  │   │
│  │  │         ┌──────────▼──────────┐              │  │   │
│  │  │         │  Results Collector  │              │  │   │
│  │  │         │  - Coverage         │              │  │   │
│  │  │         │  - Logs             │              │  │   │
│  │  │         │  - Reports          │              │  │   │
│  │  │         └──────────┬──────────┘              │  │   │
│  │  │                    │                         │  │   │
│  │  │         ┌──────────▼──────────┐              │  │   │
│  │  │         │  Cleanup            │              │  │   │
│  │  │         │  - Stop containers  │              │  │   │
│  │  │         │  - Remove volumes   │              │  │   │
│  │  │         └─────────────────────┘              │  │   │
│  │  └──────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  SHARED SERVICES                                    │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌────────────┐  │   │
│  │  │ Config      │  │ Cost        │  │ RAG System │  │   │
│  │  │ Loader      │  │ Tracker     │  │            │  │   │
│  │  └─────────────┘  └─────────────┘  └────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Estrutura de Configuração Customizável

### Diretório `.aurumcode/`

```
.aurumcode/
├── config.yml                          # Configuração principal
├── prompts/                            # Prompts customizados (Markdown)
│   ├── code-review/
│   │   ├── general.md                 # Review geral
│   │   ├── security-focus.md          # Foco em segurança
│   │   └── performance-focus.md       # Foco em performance
│   ├── documentation/
│   │   ├── inline.md                  # Docs inline
│   │   ├── changelog.md               # Formato changelog
│   │   ├── readme.md                  # Estrutura README
│   │   └── api.md                     # Documentação de API
│   └── qa/
│       ├── dockerfile-generation.md   # Como gerar Dockerfile
│       └── test-strategy.md           # Estratégia de testes
├── rules/                              # Regras customizadas (YAML)
│   ├── code-standards.yml             # Padrões de código
│   ├── security-rules.yml             # Regras de segurança
│   └── iso-compliance.yml             # Compliance ISO/IEC 25010
├── qa/                                 # Configuração QA (YAML)
│   ├── environments.yml               # Definição de ambientes
│   └── test-strategy.yml              # Estratégia de testes
└── rag/                                # Artefatos RAG (opcional)
    ├── chunks.jsonl                   # Chunks de documentação
    ├── embeddings.parquet             # Embeddings
    └── manifest.json                  # Metadados
```

### Exemplo: `.aurumcode/config.yml`

```yaml
version: "2.0"

# LLM Configuration
llm:
  provider: "openai"        # openai, anthropic, ollama, litellm
  model: "gpt-4"
  temperature: 0.3
  max_tokens: 4000
  budgets:
    daily_usd: 50.0
    per_review_tokens: 8000

# Case de Uso #1: Code Review
code_review:
  enabled: true
  triggers:
    - pull_request
    - push
  rules:
    - code-standards.yml
    - security-rules.yml
  prompts:
    default: prompts/code-review/general.md
  iso_scoring:
    enabled: true
    weights:
      functionality: 1.5
      reliability: 2.0
      security: 2.5
      maintainability: 1.0

# Caso de Uso #2: Documentation
documentation:
  enabled: true
  mode: auto                # auto, investigation, manual
  triggers:
    - push (main)
    - pull_request (merged)
  outputs:
    inline: true
    changelog: true
    readme: true
    api_docs: true
    static_site: false      # Hugo + Pagefind
  prompts:
    inline: prompts/documentation/inline.md
    changelog: prompts/documentation/changelog.md
    readme: prompts/documentation/readme.md
  investigation_mode:
    enabled: true
    use_rag: true
    depth: full             # full, incremental

# Caso de Uso #3: QA Testing
qa_testing:
  enabled: true
  triggers:
    - pull_request
  environments_config: qa/environments.yml
  test_strategy_config: qa/test-strategy.yml
  docker:
    auto_generate_dockerfile: true
    build_timeout: 600      # seconds
    container_timeout: 1800 # seconds
  cleanup:
    always: true
    on_failure: false       # keep containers on failure for debugging
  reporting:
    post_to_pr: true
    coverage_threshold: 80
    fail_on_threshold: false

# RAG System (opcional)
rag:
  enabled: false
  provider: qdrant          # qdrant, local
  collection: aurumcode-docs
  embedding_model: text-embedding-ada-002

# GitHub Integration
github:
  post_comments: true
  set_status: true
  create_issues: false      # criar issues para problemas críticos

# Cost Control
cost_control:
  daily_limit_usd: 50.0
  per_run_limit_usd: 5.0
  alert_threshold: 0.8      # 80% do budget
```

---

## Implementação por Fases

### ✅ **Fase 1: Fundação** (Já existe ~80%)

- [x] HTTP Server + Webhooks
- [x] Config Loader
- [x] LLM Orchestrator
- [x] GitHub Client
- [x] Diff Analyzer
- [x] Prompt Builder

### 🚧 **Fase 2: Pipeline Orchestrator** (PRÓXIMO)

**Criar:** `internal/pipeline/orchestrator.go`

```go
type MainOrchestrator struct {
    config      *config.Config
    githubClient *githubclient.Client
    llmOrch     *llm.Orchestrator

    // 3 Pipelines
    reviewPipeline   *ReviewPipeline
    docsPipeline     *DocumentationPipeline
    qaPipeline       *QATestingPipeline
}

func (o *MainOrchestrator) ProcessEvent(ctx context.Context, event *types.Event) error {
    // Decide quais pipelines rodar baseado em:
    // - Tipo de evento
    // - Configuração
    // - Triggers definidos

    var wg sync.WaitGroup
    errs := make(chan error, 3)

    // Pipeline 1: Code Review (se enabled)
    if o.config.CodeReview.Enabled && o.shouldRunReview(event) {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := o.reviewPipeline.Run(ctx, event); err != nil {
                errs <- fmt.Errorf("review: %w", err)
            }
        }()
    }

    // Pipeline 2: Documentation (se enabled)
    if o.config.Documentation.Enabled && o.shouldRunDocs(event) {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := o.docsPipeline.Run(ctx, event); err != nil {
                errs <- fmt.Errorf("docs: %w", err)
            }
        }()
    }

    // Pipeline 3: QA Testing (se enabled)
    if o.config.QATesting.Enabled && o.shouldRunQA(event) {
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := o.qaPipeline.Run(ctx, event); err != nil {
                errs <- fmt.Errorf("qa: %w", err)
            }
        }()
    }

    wg.Wait()
    close(errs)

    // Collect errors
    var allErrs []error
    for err := range errs {
        allErrs = append(allErrs, err)
    }

    if len(allErrs) > 0 {
        return fmt.Errorf("pipeline errors: %v", allErrs)
    }

    return nil
}
```

### 🚧 **Fase 3: QA Testing Pipeline** (Novo)

**Criar:** `internal/qa/` (evolução de `internal/testing/`)

```
internal/qa/
├── orchestrator.go         # QA Orchestrator principal
├── docker/
│   ├── client.go          # Docker API client
│   ├── builder.go         # Build images
│   ├── runner.go          # Run containers
│   └── generator.go       # Gera Dockerfiles via LLM
├── executor/              # Executa testes (já existe, melhorar)
│   ├── types.go
│   ├── go_executor.go
│   ├── python_executor.go
│   └── js_executor.go
├── environments/
│   ├── loader.go          # Carrega environments.yml
│   └── validator.go       # Valida configuração
└── reporter.go            # Gera relatórios
```

**Exemplo:** `internal/qa/orchestrator.go`

```go
type QAOrchestrator struct {
    dockerClient    *docker.Client
    dockerBuilder   *docker.Builder
    dockerGenerator *docker.Generator
    executors       map[string]executor.Executor
    envLoader       *environments.Loader
    reporter        *Reporter
    config          *config.QATestingConfig
}

func (q *QAOrchestrator) Run(ctx context.Context, event *types.Event) error {
    // 1. Load environment configuration
    envs, err := q.envLoader.Load(".aurumcode/qa/environments.yml")
    if err != nil {
        return fmt.Errorf("load environments: %w", err)
    }

    results := make([]TestResult, 0)

    for _, env := range envs {
        // 2. Check/Generate Dockerfile
        dockerfile := env.Dockerfile
        if dockerfile == "" && q.config.Docker.AutoGenerateDockerfile {
            dockerfile, err = q.dockerGenerator.Generate(ctx, event.Repo, event.Language)
            if err != nil {
                return fmt.Errorf("generate dockerfile: %w", err)
            }
        }

        // 3. Build Docker image
        imageID, err := q.dockerBuilder.Build(ctx, dockerfile, env.BuildArgs)
        if err != nil {
            return fmt.Errorf("build image: %w", err)
        }

        // 4. Start container(s)
        containerID, err := q.dockerClient.Run(ctx, imageID, env.Ports, env.Env, env.Volumes)
        if err != nil {
            return fmt.Errorf("run container: %w", err)
        }

        defer q.cleanup(ctx, containerID, imageID)

        // 5. Execute tests
        for _, test := range env.Tests {
            executor := q.executors[test.Type]
            result, err := executor.Execute(ctx, containerID, test.Command)
            if err != nil {
                result.Error = err
            }
            results = append(results, result)
        }
    }

    // 6. Generate report
    report := q.reporter.Generate(results)

    // 7. Post to GitHub
    return q.postReport(ctx, event, report)
}
```

### 🚧 **Fase 4: Documentation Pipeline** (Integração)

**Criar:** `internal/documentation/pipeline.go`

Integra os componentes já existentes em `internal/documentation/*`:
- api/ (OpenAPI docs)
- changelog/ (CHANGELOG.md)
- readme/ (README.md)
- site/ (Hugo + Pagefind)
- linkcheck/ (validação)

### 🚧 **Fase 5: Configuração Customizável**

**Criar:**
- `.aurumcode/` template directory
- Loaders para `.md` e `.yml`
- Validação de configuração

### ✅ **Fase 6: Documentação Completa**

**Criar:**
- ARCHITECTURE.md (atualizado)
- PIPELINE.md (3 casos de uso)
- QA_GUIDE.md (como usar QA testing)
- CUSTOMIZATION.md (como customizar via .md/.yml)
- EXAMPLES.md (exemplos reais)

---

## Cronograma de Implementação

### Sprint 1: Pipeline Orchestrator (3-5 dias)
- Dia 1-2: Criar `internal/pipeline/orchestrator.go`
- Dia 3: Integrar pipelines existentes (review, docs)
- Dia 4: Testes de integração
- Dia 5: Bug fixes

### Sprint 2: QA Testing Pipeline (5-7 dias)
- Dia 1: Criar `internal/qa/orchestrator.go`
- Dia 2: Docker client/builder/generator
- Dia 3: Melhorar executors existentes
- Dia 4: Environment loader
- Dia 5: Reporter
- Dia 6-7: Testes e2e

### Sprint 3: Configuração Customizável (3-4 dias)
- Dia 1: Template `.aurumcode/` directory
- Dia 2: Loaders de .md/.yml
- Dia 3: Validação
- Dia 4: Testes

### Sprint 4: Documentação (2-3 dias)
- Dia 1: ARCHITECTURE.md, PIPELINE.md
- Dia 2: QA_GUIDE.md, CUSTOMIZATION.md
- Dia 3: EXAMPLES.md, tutoriais

**Total: 13-19 dias (2-4 semanas)**

---

## Próximos Passos Imediatos

### ✅ **AGORA - O que vou fazer:**

1. **Criar estrutura de diretórios**
   ```bash
   mkdir -p internal/pipeline
   mkdir -p internal/qa/{docker,environments}
   mkdir -p configs/.aurumcode/{prompts/{code-review,documentation,qa},rules,qa}
   ```

2. **Implementar Pipeline Orchestrator**
   - `internal/pipeline/orchestrator.go`
   - `internal/pipeline/review_pipeline.go`
   - `internal/pipeline/docs_pipeline.go`
   - `internal/pipeline/qa_pipeline.go`

3. **Implementar QA Orchestrator** (reimaginar internal/testing)
   - `internal/qa/orchestrator.go`
   - `internal/qa/docker/` (client, builder, generator)

4. **Criar templates de configuração**
   - `configs/.aurumcode/config.yml` (exemplo completo)
   - Prompts customizados (.md)
   - Regras customizadas (.yml)

5. **Integrar no webhook handler**
   - Remover TODO
   - Chamar MainOrchestrator

6. **Documentar tudo**
   - ARCHITECTURE.md
   - PIPELINE.md
   - QA_GUIDE.md
   - CUSTOMIZATION.md

---

**Status:** 🚀 **PRONTO PARA COMEÇAR**

**Aguardando aprovação para executar!**
