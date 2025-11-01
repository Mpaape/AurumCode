# AurumCode - Plano de Limpeza e Integração

**Data**: 2025-11-01
**Status**: 🔴 **AÇÃO NECESSÁRIA**
**Baseado em**: ARCHITECTURE_AUDIT.md + Decisões do Product Owner

---

## Resumo das Decisões

### ✅ **Decisão 1: Sistema de Testes**
**Manter apenas `internal/testgen/`** (LLM-based)
- Razão: Escalabilidade multi-linguagem (C, C#, bash, PowerShell, Python, Rust, Go, JS, etc.)
- Código estático para todas as linguagens seria inviável
- LLM é flexível e adaptável

**Ação:** ❌ Remover `internal/testing/*` (órfão e desnecessário)

### ✅ **Decisão 2: Test Executor**
**NÃO precisa de código dedicado**
- Razão: Executar testes é apenas uma chamada de comando
- LLM pode gerar e executar comandos de teste dinamicamente
- Executor fixo (GoExecutor, PythonExecutor, JSExecutor) é over-engineering

**Ação:** ❌ Remover `internal/testing/executor/*`

### ✅ **Decisão 3: Documentation System**
**Status investigado:**
- `internal/docgen/` - Simples, LLM-based, **NÃO integrado**
- `internal/documentation/*` - Avançado, completo, **NÃO integrado**

**Ação:** 🔍 Determinar qual usar e integrar (pendente)

---

## Descoberta Crítica: Pipeline Incompleto

### 🚨 **Problema Principal: TODO na Linha 110**

O webhook handler (`cmd/server/handlers.go:110`) tem um **TODO** crítico:

```go
// TODO: Process event (emit to channel/queue)
```

**Isso significa:**
- ✅ Webhook recebe eventos
- ✅ Valida assinatura
- ✅ Parseia evento
- ❌ **NÃO FAZ NADA** com o evento!

**Consequência:**
- Todos os componentes core existem (reviewer, docgen, testgen)
- **MAS** não há orquestração que os conecta
- Sistema não é funcional end-to-end

---

## Componentes e Status Real

### ✅ **Núcleo Funcional (Testado, Isolado)**

| Componente | Path | Integrado? | Coverage | Status |
|-----------|------|-----------|----------|--------|
| Config | `internal/config/` | ✅ | 79.4% | ✅ OK |
| LLM Orchestrator | `internal/llm/` | ✅ | 78.2% | ✅ OK |
| GitHub Client | `internal/git/githubclient/` | ❓ | 80.9% | ⚠️ Parcial |
| Webhook | `internal/git/webhook/` | ✅ | 96.7% | ✅ OK |
| Diff Analyzer | `internal/analyzer/` | ✅ | 83.2% | ✅ OK |
| Prompt Builder | `internal/prompt/` | ✅ | 83.0% | ✅ OK |
| **Reviewer** | `internal/reviewer/` | ❌ | 83.3% | ⚠️ Órfão |
| **DocGen** | `internal/docgen/` | ❌ | 100% | ⚠️ Órfão |
| **TestGen** | `internal/testgen/` | ❌ | 100% | ⚠️ Órfão |

### ❌ **Código Órfão (Remover)**

| Componente | Path | Linhas | Motivo |
|-----------|------|--------|--------|
| Testing Framework | `internal/testing/*` | ~1500 | Decisão: usar LLM, não código estático |
| ├─ Executor | `executor/` | ~400 | Decisão: LLM executa comandos |
| ├─ Unit Generator | `unit/` | ~600 | Substituído por testgen/ |
| ├─ API Generator | `api/` | ~200 | Não integrado |
| └─ Mock Generator | `mock/` | ~150 | Não integrado |

### ⚠️ **Status Incerto (Investigar)**

| Componente | Path | Questão |
|-----------|------|---------|
| Documentation System | `internal/documentation/*` | Usar ou remover? |
| ├─ API Docs | `api/` | Vs. docgen? |
| ├─ Changelog | `changelog/` | Integrar? |
| ├─ README Updater | `readme/` | Integrar? |
| ├─ Site Builder | `site/` | Hugo+Pagefind - integrar? |
| └─ Link Checker | `linkcheck/` | Útil? |
| Review (novo) | `internal/review/` | Vs. reviewer? Qual usar? |
| Analysis | `internal/analysis/` | Vs. analyzer? |
| Deploy | `internal/deploy/` | Implementado? |

---

## Plano de Ação Detalhado

### 🔴 **Fase 1: Limpeza Crítica (Imediato)**

#### 1.1 Remover Código Órfão
```bash
# Backup primeiro (caso necessário reverter)
git checkout -b cleanup/remove-orphaned-testing

# Remover testing framework
rm -rf internal/testing/

# Commit
git add -A
git commit -m "Remove orphaned testing framework

- Removed internal/testing/* (executor, unit, api, mock)
- Decision: Use testgen/ with LLM for multi-language scalability
- Reasoning: Static code for C, C#, bash, PowerShell, Python, Rust, Go, JS, etc. is unmaintainable
- LLM-based approach scales better

Refs: ARCHITECTURE_AUDIT.md, CLEANUP_PLAN.md"
```

**Impacto:**
- ✅ Reduz codebase em ~1500 linhas
- ✅ Remove confusão (dois sistemas)
- ✅ Simplifica manutenção
- ❌ Perde: Template-based generation, executor framework

**Validação:**
```bash
# Verificar que nada quebra
go test ./...
go build ./cmd/...
```

---

### 🟡 **Fase 2: Investigação e Decisão (1-2 dias)**

#### 2.1 Investigar `internal/documentation/*`

**Perguntas:**
1. Qual é melhor: `docgen/` (simples) ou `documentation/*` (avançado)?
2. `documentation/*` funciona standalone ou precisa integração?
3. Hugo + Pagefind é necessário para o MVP?

**Testes:**
```bash
# Testar documentation/api
cd internal/documentation/api
go test -v

# Testar documentation/site
cd internal/documentation/site
go test -v

# Verificar dependências
go mod graph | grep documentation
```

**Opções:**

**Opção A:** Manter `docgen/` (simples)
- ✅ Prós: LLM-based, flexível, já existe
- ❌ Contras: Limitado, sem features avançadas

**Opção B:** Migrar para `documentation/*` (avançado)
- ✅ Prós: OpenAPI, changelog, README updater, Hugo, linkcheck
- ❌ Contras: Mais complexo, precisa integração

**Opção C:** Híbrido
- `docgen/` para doc inline de código
- `documentation/changelog` para CHANGELOG.md
- `documentation/readme` para README.md
- `documentation/site` para Hugo site

#### 2.2 Investigar `internal/review/` vs. `internal/reviewer/`

**Situação:**
- `internal/reviewer/` - 55 linhas, simples
- `internal/review/` - 4 arquivos, inclui ISO 25010, rules

**Pergunta:** Qual é a versão correta?

**Teste:**
```bash
# Comparar
diff internal/reviewer/reviewer.go internal/review/reviewer.go

# Ver git history
git log --oneline --all -- internal/reviewer/ internal/review/
```

**Decisão necessária:** Consolidar em um só ou manter ambos (se diferentes)?

---

### 🟢 **Fase 3: Integração do Pipeline (Crítico)**

#### 3.1 Criar Main Pipeline Orchestrator

**Arquivo:** `internal/pipeline/orchestrator.go`

```go
package pipeline

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/docgen"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/internal/reviewer"
	"aurumcode/internal/testgen"
	"aurumcode/pkg/types"
	"context"
	"fmt"
)

// Orchestrator coordinates the full pipeline
type Orchestrator struct {
	githubClient *githubclient.Client
	reviewer     *reviewer.Reviewer
	docGen       *docgen.Generator
	testGen      *testgen.Generator
	analyzer     *analyzer.DiffAnalyzer
}

// NewOrchestrator creates a new pipeline orchestrator
func NewOrchestrator(
	ghClient *githubclient.Client,
	llmOrch *llm.Orchestrator,
) *Orchestrator {
	return &Orchestrator{
		githubClient: ghClient,
		reviewer:     reviewer.NewReviewer(llmOrch),
		docGen:       docgen.NewGenerator(llmOrch),
		testGen:      testgen.NewGenerator(llmOrch),
		analyzer:     analyzer.NewDiffAnalyzer(),
	}
}

// ProcessPullRequest handles a PR event end-to-end
func (o *Orchestrator) ProcessPullRequest(ctx context.Context, event *types.Event) error {
	// 1. Fetch PR diff
	diff, err := o.githubClient.GetPullRequestDiff(
		ctx,
		event.Repo,
		event.RepoOwner,
		event.PRNumber,
	)
	if err != nil {
		return fmt.Errorf("fetch diff: %w", err)
	}

	// 2. Code Review
	review, err := o.reviewer.Review(ctx, diff)
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}

	// Post review comments
	for _, issue := range review.Issues {
		comment := types.ReviewComment{
			Path:     issue.File,
			Line:     issue.Line,
			Body:     fmt.Sprintf("**[%s]** %s\n\n**Suggestion:** %s", issue.Severity, issue.Message, issue.Suggestion),
			CommitID: event.CommitSHA,
		}
		if err := o.githubClient.PostReviewComment(ctx, event.Repo, event.RepoOwner, event.PRNumber, comment); err != nil {
			return fmt.Errorf("post comment: %w", err)
		}
	}

	// 3. Generate Documentation
	docs, err := o.docGen.Generate(ctx, diff)
	if err != nil {
		return fmt.Errorf("generate docs: %w", err)
	}
	// TODO: Post docs as comment or commit

	// 4. Generate Tests
	tests, err := o.testGen.Generate(ctx, diff)
	if err != nil {
		return fmt.Errorf("generate tests: %w", err)
	}
	// TODO: Post tests as comment or commit

	// 5. Set commit status
	status := "success"
	if len(review.Issues) > 0 {
		status = "failure"
	}
	if err := o.githubClient.SetStatus(ctx, event.Repo, event.RepoOwner, event.CommitSHA, status, fmt.Sprintf("Found %d issues", len(review.Issues))); err != nil {
		return fmt.Errorf("set status: %w", err)
	}

	return nil
}
```

#### 3.2 Integrar no Webhook Handler

**Arquivo:** `cmd/server/handlers.go:110`

```go
// ANTES (linha 110):
// TODO: Process event (emit to channel/queue)

// DEPOIS:
// Process event
go func() {
	ctx := context.Background()
	if err := processEvent(ctx, cfg, event); err != nil {
		log.Printf("[%s] Failed to process event: %v", requestID, err)
	}
}()
```

**Nova função:**
```go
func processEvent(ctx context.Context, cfg *ServerConfig, event *types.Event) error {
	// Create GitHub client
	ghClient := githubclient.NewClient(cfg.GitHubToken)

	// Create LLM orchestrator
	provider := openai.NewProvider(cfg.OpenAIKey, "gpt-4")
	tracker := cost.NewTracker(cfg.BudgetPerRun, cfg.BudgetDaily, priceMap)
	llmOrch := llm.NewOrchestrator(provider, nil, tracker)

	// Create pipeline orchestrator
	pipelineOrch := pipeline.NewOrchestrator(ghClient, llmOrch)

	// Process based on event type
	switch event.EventType {
	case "pull_request":
		return pipelineOrch.ProcessPullRequest(ctx, event)
	default:
		return fmt.Errorf("unsupported event type: %s", event.EventType)
	}
}
```

---

### 🟣 **Fase 4: Documentação Atualizada (1 dia)**

#### 4.1 Atualizar ARCHITECTURE.md

**Seções a atualizar:**

1. **High-Level Architecture Diagram**
   - Adicionar Pipeline Orchestrator
   - Remover componentes órfãos (testing/*)
   - Clarificar fluxo real

2. **Component Deep Dive**
   - Adicionar Pipeline Orchestrator
   - Documentar Reviewer
   - Documentar DocGen
   - Documentar TestGen
   - **Remover** Testing Executor

3. **Data Flow Example**
   - Atualizar com fluxo real end-to-end
   - Incluir: Webhook → Parser → Orchestrator → Review/Docs/Tests → GitHub Comments

4. **Package Structure**
   - Refletir estrutura real
   - Remover internal/testing/

#### 4.2 Criar PIPELINE.md (Novo)

Documentar o fluxo completo:

```markdown
# AurumCode Pipeline

## End-to-End Flow

```
GitHub PR Opened
    ↓
Webhook Event (handlers.go)
    ↓
Event Parser (webhook.Parse)
    ↓
Pipeline Orchestrator (pipeline.ProcessPullRequest)
    ↓
    ├─→ 1. Fetch Diff (githubclient.GetPullRequestDiff)
    ├─→ 2. Code Review
    │      ├─→ Analyze Diff (analyzer.AnalyzeDiff)
    │      ├─→ Build Prompt (prompt.BuildReviewPrompt)
    │      ├─→ Call LLM (llm.Complete)
    │      ├─→ Parse Response (parser.ParseReviewResponse)
    │      └─→ Post Comments (githubclient.PostReviewComment)
    ├─→ 3. Generate Docs (docgen.Generate → LLM)
    ├─→ 4. Generate Tests (testgen.Generate → LLM)
    └─→ 5. Set Status (githubclient.SetStatus)
```
```

#### 4.3 Atualizar README.md

Refletir status real:
- ✅ Webhook receiving
- ✅ Review generation
- ✅ Doc generation
- ✅ Test generation
- ⚠️ Pipeline integration (in progress)

#### 4.4 Criar TESTGEN_GUIDE.md (Novo)

Documentar estratégia de escalabilidade multi-linguagem:

```markdown
# Test Generation Guide

## Philosophy

AurumCode uses **LLM-based test generation** for multi-language scalability.

### Why LLM Instead of Static Templates?

**Supported Languages:** C, C#, bash, PowerShell, Python, Rust, Go, JS, TS, Java, Kotlin, Swift, Ruby, PHP, and more.

**Problem with static templates:**
- Maintaining code for 15+ languages is impractical
- Each language has unique idioms (Go table tests, Python pytest, JS Jest, etc.)
- Language ecosystems evolve (new test frameworks)

**LLM Advantages:**
- Learns language-specific idioms
- Adapts to project conventions
- Scales to new languages without code changes
- Generates idiomatic tests

## How It Works

1. **Language Detection** (analyzer.DetectLanguage)
2. **Context Building** (extract changed functions)
3. **Prompt Construction** (prompt.BuildTestPrompt)
4. **LLM Generation** (llm.Complete with testgen instructions)
5. **Response Parsing** (parser.ParseTestResponse)

## Example: Multi-Language Tests

### Go
```go
func TestAdd(t *testing.T) {
	tests := []struct{
		name string
		a, b int
		want int
	}{
		{"positive", 1, 2, 3},
		{"negative", -1, -2, -3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Add(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
```

### Python
```python
import pytest

@pytest.mark.parametrize("a,b,expected", [
    (1, 2, 3),
    (-1, -2, -3),
])
def test_add(a, b, expected):
    assert add(a, b) == expected
```

### JavaScript
```javascript
describe('add', () => {
  test.each([
    [1, 2, 3],
    [-1, -2, -3],
  ])('add(%i, %i) returns %i', (a, b, expected) => {
    expect(add(a, b)).toBe(expected);
  });
});
```

**Notice:** LLM generates idiomatic tests for each language automatically!
```

---

## Cronograma

### Sprint 1: Limpeza (2 dias)
- **Dia 1**: Remover `internal/testing/*`, validar build
- **Dia 2**: Investigar `documentation/*`, decidir estratégia

### Sprint 2: Integração (3-5 dias)
- **Dia 1-2**: Criar Pipeline Orchestrator
- **Dia 3**: Integrar no Webhook Handler
- **Dia 4**: Testes de integração end-to-end
- **Dia 5**: Bug fixes

### Sprint 3: Documentação (2 dias)
- **Dia 1**: Atualizar ARCHITECTURE.md, criar PIPELINE.md
- **Dia 2**: Criar TESTGEN_GUIDE.md, atualizar README.md

**Total: 7-9 dias**

---

## Riscos e Mitigações

### 🚨 **Risco 1: Remover código útil**
**Mitigação:** Criar branch de backup antes de deletar
```bash
git checkout -b backup/testing-framework
git checkout main
git checkout -b cleanup/remove-orphaned-testing
```

### ⚠️ **Risco 2: Pipeline Orchestrator complexo**
**Mitigação:** Implementar incrementalmente
1. Só review primeiro
2. Adicionar docs
3. Adicionar tests

### ⚠️ **Risco 3: LLM custo/latência**
**Mitigação:**
- Budget tracking já implementado (llm.Orchestrator)
- Async processing (goroutine no handler)
- Cache de prompts (futuro)

---

## Critérios de Sucesso

### ✅ **Fase 1 (Limpeza)**
- [ ] `internal/testing/*` removido
- [ ] Todos os testes passam: `go test ./...`
- [ ] Build funciona: `go build ./cmd/...`
- [ ] Decisão sobre `documentation/*`

### ✅ **Fase 2 (Integração)**
- [ ] Pipeline Orchestrator criado
- [ ] Webhook handler integrado (TODO removido)
- [ ] Review comments postados no GitHub
- [ ] Teste end-to-end funcional

### ✅ **Fase 3 (Documentação)**
- [ ] ARCHITECTURE.md atualizado
- [ ] PIPELINE.md criado
- [ ] TESTGEN_GUIDE.md criado
- [ ] README.md atualizado
- [ ] Diagramas refletem realidade

---

## Comandos Úteis

```bash
# Limpeza
git checkout -b cleanup/remove-orphaned-testing
rm -rf internal/testing/
go test ./...
go build ./cmd/...

# Validação
go mod tidy
go vet ./...
golangci-lint run

# Cobertura (após limpeza)
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Comparação (antes/depois)
git diff main..cleanup/remove-orphaned-testing --stat
```

---

**Preparado por:** Claude Code
**Status:** 🔴 **AGUARDANDO APROVAÇÃO PARA EXECUTAR**
**Próximo passo:** Aprovar Fase 1 (Limpeza) e executar remoção de código órfão
