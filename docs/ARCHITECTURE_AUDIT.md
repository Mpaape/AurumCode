# AurumCode - Auditoria de Arquitetura

**Data**: 2025-11-01
**Status**: 🚨 CRÍTICO - Documentação Desatualizada, Código Órfão Identificado

## Resumo Executivo

O projeto AurumCode possui **discrepâncias significativas** entre:
- PRD (visão completa)
- ARCHITECTURE.md (documentação)
- Implementação real (código)

**Problemas críticos encontrados:**
1. ❌ Componentes implementados mas não documentados
2. ❌ Código órfão (não usado/integrado)
3. ❌ Dois sistemas concorrentes para mesma funcionalidade
4. ❌ Falta de clareza no fluxo real do sistema

---

## 1. Visão Real do Projeto (Clarificada)

Baseado na discussão com o Product Owner:

### Fluxo Principal
```
1. Recebe PR/Diff
   ↓
2. Identifica Linguagem
   ↓
3. Recupera Contexto (RAG/Docs/Imagens Docker)
   ↓
4. Code Review (Clean Code, Clean Arch, Análise Estática, Segurança)
   ↓
5. Geração de Documentação
   ↓
6. QA Automation (APIs, Ambientes Simulados)
```

### Escopo Core
- ✅ **Code Review Automatizado** (ISO/IEC 25010, segurança, clean code)
- ✅ **Geração de Documentação** (README, API docs, changelog)
- ✅ **Geração de Testes** (unit, API, mocks)
- ⚠️ **Execução de Testes em QA** (automações, APIs) - PROPOSTO mas não claro
- ❌ **Multi-Git** (Gitea, generic Git) - NÃO implementado
- ❌ **Monitoring/Observability** - NÃO implementado

---

## 2. Componentes Implementados vs. Documentados

### ✅ Documentado E Implementado

| Componente | Localização | Status | Coverage |
|-----------|-------------|--------|----------|
| HTTP Server | `cmd/server/` | ✅ | 96.7% |
| Config Loader | `internal/config/` | ✅ | 79.4% |
| LLM Orchestrator | `internal/llm/` | ✅ | 78.2% |
| GitHub Client | `internal/git/githubclient/` | ✅ | 80.9% |
| Webhook Handler | `internal/git/webhook/` | ✅ | 96.7% |
| Diff Analyzer | `internal/analyzer/` | ✅ | 83.2% |
| Prompt Builder | `internal/prompt/` | ✅ | 83.0% |
| Reviewer | `internal/reviewer/` | ✅ | 83.3% |
| Doc Generator (simple) | `internal/docgen/` | ✅ | 100% |
| Test Generator (simple) | `internal/testgen/` | ✅ | 100% |

### ⚠️ Implementado MAS NÃO Documentado

| Componente | Localização | Status | Integrado? |
|-----------|-------------|--------|-----------|
| **Documentation System** | `internal/documentation/` | ⚠️ | ❓ |
| ├─ API Doc Generator | `internal/documentation/api/` | ⚠️ | ❓ |
| ├─ Changelog Generator | `internal/documentation/changelog/` | ⚠️ | ❓ |
| ├─ README Updater | `internal/documentation/readme/` | ⚠️ | ❓ |
| ├─ Site Builder | `internal/documentation/site/` | ⚠️ | ❓ |
| └─ Link Checker | `internal/documentation/linkcheck/` | ⚠️ | ❓ |
| **Testing Framework** | `internal/testing/` | ⚠️ | ❌ **NÃO** |
| ├─ Unit Test Gen | `internal/testing/unit/` | ⚠️ | ❌ **NÃO** |
| ├─ API Test Gen | `internal/testing/api/` | ⚠️ | ❌ **NÃO** |
| ├─ Mock Generator | `internal/testing/mock/` | ⚠️ | ❌ **NÃO** |
| └─ **Test Executor** | `internal/testing/executor/` | ❌ **ÓRFÃO** | ❌ **NÃO** |
| Analysis | `internal/analysis/` | ⚠️ | ❓ |
| Deploy | `internal/deploy/` | ⚠️ | ❓ |
| ISO 25010 | `internal/review/iso25010/` | ⚠️ | ❓ |

---

## 3. Código Órfão Identificado

### 🚨 CRÍTICO: `internal/testing/` (Sistema Completo Não Integrado)

**Descoberta:**
- Existe um sistema INTEIRO de geração e execução de testes
- **NENHUM** arquivo no projeto importa este código
- Foi criado mas nunca integrado
- Duplica funcionalidade do `internal/testgen/` (mais antigo e integrado)

#### Componentes Órfãos

**1. `internal/testing/executor/`** (4 arquivos)
```go
// Executores de teste por linguagem
- go_executor.go      // Roda `go test`, parseia resultados
- python_executor.go  // Roda `pytest`, parseia XML
- js_executor.go      // Roda `npm test`, parseia Jest
- types.go            // TestResult, Coverage, Executor interface
```

**Propósito aparente:** Executar testes gerados e coletar coverage.

**Status:** ❌ Não usado em lugar nenhum

**2. `internal/testing/unit/`** (7 arquivos)
```go
- orchestrator.go       // Coordena geração multi-linguagem
- generator_go.go       // Gera testes Go (templates)
- generator_python.go   // Gera testes Python
- generator_js.go       // Gera testes JS/TS
- extractor.go          // Extrai funções de diff
- types.go              // Interfaces
- orchestrator_test.go  // Testes do orchestrator
```

**Propósito aparente:** Gerar testes sem LLM (template-based), mais barato.

**Status:** ❌ Não usado, duplica `internal/testgen/`

**3. `internal/testing/api/`** (2 arquivos)
```go
- generator.go  // Gera testes de API
- types.go
```

**Status:** ❌ Não usado

**4. `internal/testing/mock/`** (2 arquivos)
```go
- generator.go  // Gera mocks de interfaces
- types.go
```

**Status:** ❌ Não usado

---

## 4. Sistemas Concorrentes

### Test Generation: Dois Sistemas para Mesma Funcionalidade

#### Sistema 1: `internal/testgen/` (ATIVO)
```go
✅ INTEGRADO com:
   - internal/llm (usa LLM)
   - internal/analyzer
   - internal/prompt
   - Usado no pipeline principal

🔸 Limitações:
   - Sempre usa LLM (caro)
   - Só gera testes unitários
   - Simples, mas funcional
```

#### Sistema 2: `internal/testing/unit/` (ÓRFÃO)
```go
❌ NÃO INTEGRADO

🔸 Vantagens (potenciais):
   - Template-based (sem LLM, barato)
   - Suporte a Go, Python, JS, TS
   - Orchestrator para multi-linguagem
   - + API tests, mocks, executors

🔸 Problema:
   - Nunca foi integrado
   - Sem documentação
   - Não há plano de uso
```

---

## 5. Componentes Não Documentados (Status Incerto)

### `internal/documentation/` - Sistema Avançado de Docs

**Existe 5 subpacotes** não mencionados no ARCHITECTURE.md:

1. **`api/`** - Gera documentação de API (OpenAPI?)
2. **`changelog/`** - Gera CHANGELOG.md automático
3. **`readme/`** - Atualiza README.md
4. **`site/`** - Build Hugo site + Pagefind
5. **`linkcheck/`** - Valida links quebrados

**Questões:**
- ❓ Está integrado?
- ❓ Substitui `internal/docgen/`?
- ❓ Ou coexiste com ele?

### `internal/analysis/` vs. `internal/analyzer/`

- `analyzer/` - Documentado, integrado
- `analysis/` - Existe, mas não documentado

**Questão:** ❓ São diferentes ou duplicados?

### `internal/deploy/`

- Mencionado no PRD (Fase 9)
- Não documentado no ARCHITECTURE.md
- ❓ Implementado ou stub?

---

## 6. Diagrama de Arquitetura Real (Atualizado)

```
┌────────────────────────────────────────────────────────────────┐
│                        EXTERNAL LAYER                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │   GitHub     │  │  LLM APIs    │  │   Docker     │         │
│  │   Webhooks   │  │  (OpenAI,    │  │   Images     │         │
│  │              │  │  Anthropic)  │  │  (QA Envs)   │         │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘         │
└─────────┼──────────────────┼──────────────────┼────────────────┘
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼────────────────┐
│         │      ADAPTER LAYER (Infrastructure) │                │
│  ┌──────▼───────┐  ┌──────▼───────┐  ┌───────▼──────┐         │
│  │  GitClient   │  │   Provider   │  │  (Reserved)  │         │
│  │  (GitHub)    │  │   Adapters   │  │              │         │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘         │
└─────────┼──────────────────┼────────────────────────────────┬──┘
          │                  │                                 │
┌─────────┼──────────────────┼─────────────────────────────────▼──┐
│         │         CORE DOMAIN (Application Layer)               │
│  ┌──────▼─────────────────┐                                     │
│  │   Orchestrator         │   ┌──────────────────┐             │
│  │   (Coordinates)        │◀──│  Cost Tracker    │             │
│  └──────┬─────────────────┘   └──────────────────┘             │
│         │                                                       │
│  ┌──────▼─────────────────┐                                    │
│  │   Review Pipeline      │                                    │
│  │  ┌─────────────────┐   │   ┌──────────────────┐            │
│  │  │ Diff Analyzer   │   │   │  Prompt Builder  │            │
│  │  └─────────────────┘   │   └──────────────────┘            │
│  │  ┌─────────────────┐   │   ┌──────────────────┐            │
│  │  │ Language Detect │   │   │ Response Parser  │            │
│  │  └─────────────────┘   │   └──────────────────┘            │
│  │  ┌─────────────────┐   │   ┌──────────────────┐            │
│  │  │ ISO/IEC 25010   │   │   │  Analysis        │            │
│  │  └─────────────────┘   │   └──────────────────┘            │
│  └────────────────────────┘                                    │
│                                                                 │
│  ┌──────────────────────────────────────────────────────┐      │
│  │          Documentation System (⚠️ NOT INTEGRATED?)   │      │
│  │  ┌─────────────┐  ┌──────────┐  ┌────────────┐      │      │
│  │  │ DocGen      │  │ API Docs │  │ Changelog  │      │      │
│  │  │ (simple)    │  │          │  │            │      │      │
│  │  └─────────────┘  └──────────┘  └────────────┘      │      │
│  │  ┌─────────────┐  ┌──────────┐  ┌────────────┐      │      │
│  │  │ README      │  │ Site     │  │ LinkCheck  │      │      │
│  │  │ Updater     │  │ Builder  │  │            │      │      │
│  │  └─────────────┘  └──────────┘  └────────────┘      │      │
│  └──────────────────────────────────────────────────────┘      │
│                                                                 │
│  ┌──────────────────────────────────────────────────────┐      │
│  │          Testing System (❌ ORPHANED - NOT USED!)    │      │
│  │  ┌──────────────┐  ┌──────────────┐                 │      │
│  │  │ TestGen      │  │ Unit Test    │                 │      │
│  │  │ (LLM-based)  │  │ Generator    │                 │      │
│  │  │ ✅ ACTIVE    │  │ (templates)  │                 │      │
│  │  │              │  │ ❌ ORPHAN    │                 │      │
│  │  └──────────────┘  └──────────────┘                 │      │
│  │  ┌──────────────┐  ┌──────────────┐                 │      │
│  │  │ API Test Gen │  │ Mock Gen     │                 │      │
│  │  │ ❌ ORPHAN    │  │ ❌ ORPHAN    │                 │      │
│  │  └──────────────┘  └──────────────┘                 │      │
│  │  ┌──────────────────────────────────┐               │      │
│  │  │ Test Executor (Go/Py/JS)         │               │      │
│  │  │ ❌ ORPHAN - Never Integrated     │               │      │
│  │  └──────────────────────────────────┘               │      │
│  └──────────────────────────────────────────────────────┘      │
│                                                                 │
│  ┌──────────────────┐   ┌──────────────────┐                  │
│  │  Deploy (???)    │   │  RAG System (TBD)│                  │
│  └──────────────────┘   └──────────────────┘                  │
└─────────────────────────────────────────────────────────────────┘
          │
┌─────────▼────────────────────────────────────────────────────────┐
│                      TYPES LAYER                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │  Config  │  │   Diff   │  │  Review  │  │  Event   │        │
│  │          │  │          │  │  Result  │  │          │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└───────────────────────────────────────────────────────────────────┘
```

---

## 7. Decisões Necessárias

### 🚨 CRÍTICO: Sistema de Testes

**Problema:** Dois sistemas concorrentes, um órfão.

**Opções:**

#### Opção A: Manter `testgen/` (LLM-based)
```
✅ Prós:
   - Já integrado e funcional
   - Cobertura 100%
   - Simples e direto

❌ Contras:
   - Usa LLM sempre (caro)
   - Só unit tests
   - Sem API tests, mocks, execution
```

#### Opção B: Migrar para `testing/*` (Template-based + Executor)
```
✅ Prós:
   - Mais barato (templates)
   - Mais completo (unit, API, mock, executor)
   - Melhor arquitetura

❌ Contras:
   - Precisa integração completa
   - Precisa documentação
   - Trabalho significativo
```

#### Opção C: Híbrido
```
✅ Prós:
   - testing/unit para código simples (templates)
   - testgen/ (LLM) para código complexo
   - Melhor custo-benefício

❌ Contras:
   - Mais complexo
   - Precisa orquestração
```

**Recomendação:** ❓ **DECISÃO DO PRODUCT OWNER NECESSÁRIA**

---

### ⚠️ IMPORTANTE: Sistema de Documentação

**Problema:** `docgen/` documentado vs. `documentation/*` não documentado.

**Questões:**
1. `documentation/*` substitui `docgen/`?
2. Ou coexistem (docgen = simples, documentation = avançado)?
3. Está integrado ou órfão também?

**Ação:** ✅ **INVESTIGAR E DOCUMENTAR**

---

### ⚠️ Test Executor: Faz Sentido no Escopo?

**Questão fundamental:** O executor serve para:
- **A)** Validar testes gerados pelo AurumCode (QA interno)?
- **B)** Rodar testes do projeto sendo revisado (QA externo)?
- **C)** Ambos?

**Contexto do PRD:**
- Fase 7.4 menciona "Test executor (run, parse coverage, report)"
- User story menciona "QA Automation (APIs, Ambientes Simulados)"

**Se a resposta é A ou C:** Executor faz sentido, mas precisa integração.
**Se a resposta é B:** Está fora do escopo core (code review).

**Ação:** ❓ **CLARIFICAR COM PRODUCT OWNER**

---

## 8. Plano de Ação Proposto

### Fase 1: Auditoria e Decisões (URGENTE)
1. ✅ **Documentar código órfão** (este documento)
2. ❓ **Decisão: Sistema de testes** (A, B, ou C?)
3. ❓ **Decisão: Executor** (integrar, remover, ou reimaginar?)
4. ❓ **Investigar: `documentation/*`** (status e integração)
5. ❓ **Clarificar: Escopo QA** (interno vs. externo)

### Fase 2: Limpeza (CRÍTICO)
1. ❌ **Remover código órfão** OU
2. ✅ **Integrar e documentar** (dependendo das decisões)
3. ✅ **Sincronizar ARCHITECTURE.md com realidade**

### Fase 3: Documentação (BLOQUEANTE)
1. ✅ **Atualizar ARCHITECTURE.md** com todos os componentes
2. ✅ **Documentar componentes não documentados**
3. ✅ **Criar fluxo real do sistema** (end-to-end)
4. ✅ **Documentar integrações** (o que chama o quê)

### Fase 4: Validação
1. ✅ **Code walkthrough** completo
2. ✅ **Validar fluxo real** vs. documentado
3. ✅ **Testes de integração** dos componentes órfãos (se mantidos)

---

## 9. Riscos Identificados

### 🚨 ALTO: Progresso Bloqueado
**Problema:** Sem docs claras, impossível avançar com confiança.
**Impacto:** Time perdido, decisões erradas, retrabalho.
**Mitigação:** Priorizar Fase 1-3 do plano acima.

### ⚠️ MÉDIO: Código Morto
**Problema:** ~15% do código pode estar órfão.
**Impacto:** Confusão, manutenção desnecessária, testes inúteis.
**Mitigação:** Decisões claras + limpeza.

### ⚠️ MÉDIO: Duplicação
**Problema:** Dois sistemas para mesma funcionalidade.
**Impacto:** Manutenção dupla, bugs, escolha errada.
**Mitigação:** Escolher um caminho e seguir.

---

## 10. Conclusão

**Estado atual do projeto:**
- ✅ Core funcional (review pipeline)
- ⚠️ Documentação desatualizada
- ❌ Código órfão significativo
- ❌ Falta de clareza arquitetural

**Bloqueadores críticos:**
1. Documentação não reflete realidade
2. Decisões arquiteturais pendentes
3. Código não integrado sem plano

**Próximo passo crítico:**
📋 **REUNIÃO DE DECISÃO** com Product Owner para:
- Definir escopo real do sistema de testes
- Decidir sobre código órfão (integrar ou remover)
- Priorizar documentação vs. novas features

---

**Documento preparado por:** Claude Code (AI Assistant)
**Para revisão por:** Product Owner / Tech Lead
**Status:** 🔴 **DECISÕES URGENTES NECESSÁRIAS**
