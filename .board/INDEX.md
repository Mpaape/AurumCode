# Atomic Card Index

This is the readable registry for the executable TaskSpec files. State,
dependency, schema, path ownership, evidence, and safety validity are enforced
by [validate.sh](validate.sh).

- Total: **421 atomic cards**
- Execution order: dependency DAG, never a human calendar
- Common proof: locked TDD red/green/refactor, OCI acceptance, two sealed full reviews, skeptical mutation
- Requirements: [canonical product registry](requirements/REQUIREMENTS.md)
- Memory: stateless review first; optional modes are specified in [ADR-0001](decisions/ADR-0001-optional-local-memory.md)
- Review policy: [Skeptical Approval Protocol](REVIEW_PROTOCOL.md)

| Card | State | Office | Risk | Depends on | Outcome |
|---|---|---|---|---|---|
| [AUR-001](cards/backlog/AUR-001.md) | backlog | O00-governance | high | `[AUR-233]` | Inventariar capacidades reais do legado |
| [AUR-002](cards/backlog/AUR-002.md) | backlog | O00-governance | high | `[AUR-001]` | Fixar baseline de caracterização |
| [AUR-003](cards/backlog/AUR-003.md) | backlog | O00-governance | medium | `[AUR-001]` | Definir schema de card atômico |
| [AUR-004](cards/backlog/AUR-004.md) | backlog | O00-governance | high | `[AUR-003]` | Validar DAG e liberar dependências |
| [AUR-005](cards/backlog/AUR-005.md) | backlog | O00-governance | high | `[AUR-003]` | Definir manifesto content-addressed de evidência |
| [AUR-006](cards/backlog/AUR-006.md) | backlog | O00-governance | critical | `[AUR-233]` | Definir perfil OCI bootstrap fail-closed |
| [AUR-007](cards/backlog/AUR-007.md) | backlog | O00-governance | critical | `[AUR-006, AUR-227, AUR-228, AUR-229, AUR-230, AUR-231, AUR-232]` | Construir aprovador cético hermético |
| [AUR-008](cards/backlog/AUR-008.md) | backlog | O00-governance | critical | `[AUR-005, AUR-007]` | Exigir dupla revisão independente |
| [AUR-009](cards/backlog/AUR-009.md) | backlog | O00-security | critical | `[AUR-006]` | Redigir segredos em todos os sinks |
| [AUR-010](cards/backlog/AUR-010.md) | backlog | O00-governance | high | `[AUR-039]` | Bloquear atribuição de IA em commits |
| [AUR-011](cards/backlog/AUR-011.md) | backlog | O09-demo | medium | `[AUR-006, AUR-009]` | Criar fixture Git demo determinística |
| [AUR-012](cards/backlog/AUR-012.md) | backlog | O09-demo | medium | `[AUR-011]` | Bootstrapar repositório demo consumidor |
| [AUR-013](cards/backlog/AUR-013.md) | backlog | O00-governance | high | `[AUR-003, AUR-009]` | Auditar plugin ou skill escritorio |
| [AUR-014](cards/backlog/AUR-014.md) | backlog | O00-research | medium | `[AUR-233]` | Versionar standard de code review |
| [AUR-015](cards/ready/AUR-015.md) | ready | O00-research | high | `[]` | Versionar standard de secure code review |
| [AUR-016](cards/ready/AUR-016.md) | ready | O00-research | medium | `[]` | Definir uso evidencial de ISO 25010 |
| [AUR-017](cards/ready/AUR-017.md) | ready | O00-research | medium | `[]` | Fixar standards de artefatos interoperáveis |
| [AUR-018](cards/ready/AUR-018.md) | ready | O00-research | high | `[]` | Fixar contratos SCM e CI seguros |
| [AUR-019](cards/ready/AUR-019.md) | ready | O00-research | high | `[]` | Fixar contratos oficiais de providers |
| [AUR-020](cards/ready/AUR-020.md) | ready | O00-research | high | `[]` | Escolher memória e mapa incremental mínimos |
| [AUR-021](cards/ready/AUR-021.md) | ready | O00-research | high | `[]` | Fixar MCP conformance e segurança |
| [AUR-022](cards/backlog/AUR-022.md) | backlog | O00-governance | critical | `[AUR-002, AUR-003, AUR-004, AUR-005, AUR-006, AUR-007, AUR-008, AUR-009, AUR-010, AUR-011, AUR-012, AUR-013, AUR-014, AUR-015, AUR-016, AUR-017, AUR-018, AUR-019, AUR-020, AUR-021, AUR-233]` | Aprovar foundation reproduzível |
| [AUR-023](cards/backlog/AUR-023.md) | backlog | O01-core | high | `[AUR-003, AUR-005, AUR-014, AUR-015, AUR-016, AUR-017, AUR-233, AUR-403]` | Bloquear imports que violam arquitetura |
| [AUR-024](cards/backlog/AUR-024.md) | backlog | O01-core | high | `[AUR-023, AUR-404]` | Definir identidade canônica de repositório e snapshot |
| [AUR-025](cards/backlog/AUR-025.md) | backlog | O01-core | critical | `[AUR-024, AUR-017, AUR-403]` | Normalizar unified diff em ChangeSet |
| [AUR-026](cards/backlog/AUR-026.md) | backlog | O01-core | critical | `[AUR-025, AUR-014, AUR-015, AUR-016, AUR-017, AUR-403]` | Definir Finding e Evidence verificáveis |
| [AUR-027](cards/backlog/AUR-027.md) | backlog | O01-core | high | `[AUR-026, AUR-403]` | Definir ReviewResult, QARun e Artifact |
| [AUR-028](cards/backlog/AUR-028.md) | backlog | O01-core | high | `[AUR-027, AUR-403]` | Normalizar erros e exit codes |
| [AUR-029](cards/backlog/AUR-029.md) | backlog | O01-core | high | `[AUR-024, AUR-025, AUR-026, AUR-027, AUR-028, AUR-403]` | Segregar portas do application core |
| [AUR-030](cards/backlog/AUR-030.md) | backlog | O01-core | critical | `[AUR-029, AUR-019, AUR-403]` | Definir config v3 estrita |
| [AUR-031](cards/backlog/AUR-031.md) | backlog | O01-core | critical | `[AUR-030, AUR-009, AUR-403]` | Resolver referências de segredo fora do domínio |
| [AUR-032](cards/backlog/AUR-032.md) | backlog | O01-core | critical | `[AUR-030, AUR-031, AUR-403]` | Aplicar precedência de config com base confiável |
| [AUR-033](cards/backlog/AUR-033.md) | backlog | O01-core | high | `[AUR-032, AUR-403]` | Migrar config v2 para v3 somente em dry-run |
| [AUR-034](cards/backlog/AUR-034.md) | backlog | O01-core | critical | `[AUR-027, AUR-029, AUR-403]` | Implementar máquina de estados do run |
| [AUR-035](cards/backlog/AUR-035.md) | backlog | O01-core | critical | `[AUR-034, AUR-404]` | Calcular CandidateIdentityV1 canônica |
| [AUR-036](cards/backlog/AUR-036.md) | backlog | O01-core | critical | `[AUR-027, AUR-035, AUR-403]` | Verificar manifest do artifact bundle |
| [AUR-037](cards/backlog/AUR-037.md) | backlog | O01-core | medium | `[AUR-038, AUR-403]` | Compor CLI sem lógica de domínio |
| [AUR-038](cards/backlog/AUR-038.md) | backlog | O01-core | critical | `[AUR-023, AUR-024, AUR-025, AUR-026, AUR-027, AUR-028, AUR-029, AUR-030, AUR-031, AUR-032, AUR-033, AUR-034, AUR-035, AUR-036, AUR-403]` | Aprovar contratos do core |
| [AUR-039](cards/backlog/AUR-039.md) | backlog | O02-index | critical | `[AUR-024, AUR-029, AUR-233, AUR-404]` | Ler árvore Git sem executar checkout |
| [AUR-040](cards/backlog/AUR-040.md) | backlog | O02-index | critical | `[AUR-025, AUR-039, AUR-404]` | Parsear unified diff completo |
| [AUR-041](cards/backlog/AUR-041.md) | backlog | O02-index | high | `[AUR-040, AUR-404]` | Normalizar rename, copy, binary e diff grande |
| [AUR-042](cards/backlog/AUR-042.md) | backlog | O02-index | high | `[AUR-041, AUR-403]` | Filtrar arquivos por política observável |
| [AUR-043](cards/backlog/AUR-043.md) | backlog | O02-index | critical | `[AUR-024, AUR-020, AUR-407]` | Criar schema SQLite do entity index |
| [AUR-044](cards/backlog/AUR-044.md) | backlog | O02-index | critical | `[AUR-043, AUR-407]` | Publicar snapshots transacionalmente |
| [AUR-045](cards/backlog/AUR-045.md) | backlog | O02-index | high | `[AUR-044, AUR-039, AUR-407]` | Reutilizar blobs por conteúdo |
| [AUR-046](cards/backlog/AUR-046.md) | backlog | O02-index | medium | `[AUR-045, AUR-407]` | Indexar entidade de arquivo como fallback |
| [AUR-047](cards/backlog/AUR-047.md) | backlog | O02-index | high | `[AUR-046, AUR-222, AUR-403]` | Persistir entidades Go extraídas |
| [AUR-048](cards/backlog/AUR-048.md) | backlog | O02-index | critical | `[AUR-046, AUR-223, AUR-406]` | Integrar worker Tree-sitter ao índice |
| [AUR-049](cards/backlog/AUR-049.md) | backlog | O02-index | high | `[AUR-046, AUR-323, AUR-324, AUR-325, AUR-326, AUR-327, AUR-328, AUR-329, AUR-330, AUR-331, AUR-407]` | Persistir entidades polyglot canônicas |
| [AUR-050](cards/backlog/AUR-050.md) | backlog | O02-index | high | `[AUR-047, AUR-049, AUR-403]` | Derivar relações entre entidades |
| [AUR-051](cards/backlog/AUR-051.md) | backlog | O02-index | high | `[AUR-043, AUR-050, AUR-407]` | Indexar e consultar FTS5 |
| [AUR-052](cards/backlog/AUR-052.md) | backlog | O02-index | critical | `[AUR-044, AUR-045, AUR-050, AUR-051, AUR-407]` | Atualizar índice incrementalmente |
| [AUR-053](cards/backlog/AUR-053.md) | backlog | O02-index | critical | `[AUR-042, AUR-052, AUR-403]` | Montar ContextPack determinístico |
| [AUR-054](cards/backlog/AUR-054.md) | backlog | O02-index | high | `[AUR-052, AUR-403]` | Aplicar retenção e GC do índice |
| [AUR-055](cards/backlog/AUR-055.md) | backlog | O03-providers | critical | `[AUR-029, AUR-019, AUR-405]` | Definir request e response canônicos de modelo |
| [AUR-056](cards/backlog/AUR-056.md) | backlog | O03-providers | critical | `[AUR-055, AUR-405]` | Negociar capabilities antes da chamada |
| [AUR-057](cards/backlog/AUR-057.md) | backlog | O03-providers | critical | `[AUR-055, AUR-009, AUR-405]` | Definir contrato de transporte HTTP |
| [AUR-058](cards/backlog/AUR-058.md) | backlog | O03-providers | critical | `[AUR-031, AUR-057, AUR-405]` | Resolver credenciais por provider |
| [AUR-059](cards/backlog/AUR-059.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-341, AUR-342, AUR-343, AUR-344, AUR-345, AUR-346, AUR-347, AUR-348, AUR-349, AUR-350, AUR-405]` | Criar fake servers de providers |
| [AUR-060](cards/backlog/AUR-060.md) | backlog | O03-providers | high | `[AUR-056, AUR-058, AUR-059, AUR-234, AUR-235, AUR-236, AUR-405]` | Adaptar OpenAI Chat Completions |
| [AUR-061](cards/backlog/AUR-061.md) | backlog | O03-providers | high | `[AUR-056, AUR-058, AUR-059, AUR-234, AUR-235, AUR-236, AUR-405]` | Adaptar OpenAI Responses |
| [AUR-062](cards/backlog/AUR-062.md) | backlog | O03-providers | high | `[AUR-060, AUR-061, AUR-405]` | Adaptar LiteLLM OpenAI-compatible |
| [AUR-063](cards/backlog/AUR-063.md) | backlog | O03-providers | high | `[AUR-056, AUR-058, AUR-059, AUR-234, AUR-235, AUR-236, AUR-405]` | Adaptar Anthropic Messages |
| [AUR-064](cards/backlog/AUR-064.md) | backlog | O03-providers | medium | `[AUR-056, AUR-058, AUR-059, AUR-234, AUR-235, AUR-236, AUR-405]` | Adaptar Ollama local |
| [AUR-065](cards/backlog/AUR-065.md) | backlog | O03-providers | critical | `[AUR-055, AUR-056, AUR-405]` | Controlar tokens, custo e orçamento |
| [AUR-066](cards/backlog/AUR-066.md) | backlog | O03-providers | critical | `[AUR-060, AUR-061, AUR-062, AUR-063, AUR-064, AUR-065, AUR-405]` | Rotear fallback por capability e erro |
| [AUR-067](cards/backlog/AUR-067.md) | backlog | O03-providers | critical | `[AUR-060, AUR-061, AUR-062, AUR-063, AUR-064, AUR-065, AUR-066, AUR-405]` | Executar conformance suite em todo provider |
| [AUR-068](cards/backlog/AUR-068.md) | backlog | O04-agents | critical | `[AUR-026, AUR-030, AUR-014, AUR-015, AUR-016, AUR-403]` | Definir manifest imutável de skill |
| [AUR-069](cards/backlog/AUR-069.md) | backlog | O04-agents | critical | `[AUR-068, AUR-009, AUR-403]` | Carregar skill sem executar conteúdo |
| [AUR-070](cards/backlog/AUR-070.md) | backlog | O04-agents | critical | `[AUR-069, AUR-032, AUR-403]` | Resolver confiança de skills |
| [AUR-071](cards/backlog/AUR-071.md) | backlog | O04-agents | critical | `[AUR-069, AUR-056, AUR-410]` | Aplicar sandbox de tool e capability por skill |
| [AUR-072](cards/backlog/AUR-072.md) | backlog | O04-agents | high | `[AUR-070, AUR-071, AUR-210, AUR-403]` | Selecionar skills deterministicamente |
| [AUR-073](cards/backlog/AUR-073.md) | backlog | O04-agents | high | `[AUR-072, AUR-014, AUR-403]` | Entregar skill core de review |
| [AUR-074](cards/backlog/AUR-074.md) | backlog | O04-agents | high | `[AUR-072, AUR-224, AUR-403]` | Definir contrato de skill de linguagem |
| [AUR-075](cards/backlog/AUR-075.md) | backlog | O04-agents | critical | `[AUR-073, AUR-403]` | Promover skill somente após aprovação |
| [AUR-076](cards/backlog/AUR-076.md) | backlog | O00-governance | critical | `[AUR-039, AUR-040, AUR-041, AUR-042, AUR-043, AUR-044, AUR-045, AUR-046, AUR-047, AUR-048, AUR-049, AUR-050, AUR-051, AUR-052, AUR-053, AUR-054, AUR-055, AUR-056, AUR-057, AUR-058, AUR-059, AUR-060, AUR-061, AUR-062, AUR-063, AUR-064, AUR-065, AUR-066, AUR-067, AUR-068, AUR-069, AUR-070, AUR-071, AUR-072, AUR-073, AUR-074, AUR-075, AUR-237, AUR-238, AUR-239, AUR-240, AUR-241, AUR-242, AUR-243, AUR-244, AUR-245, AUR-246, AUR-247, AUR-248, AUR-249, AUR-410]` | Aprovar adapters, índice e skills |
| [AUR-077](cards/backlog/AUR-077.md) | backlog | O05-review | high | `[AUR-042, AUR-406]` | Classificar linguagem e risco sem LLM |
| [AUR-078](cards/backlog/AUR-078.md) | backlog | O05-review | critical | `[AUR-014, AUR-015, AUR-016, AUR-077, AUR-403]` | Executar regras determinísticas versionadas |
| [AUR-079](cards/backlog/AUR-079.md) | backlog | O05-review | critical | `[AUR-078, AUR-009, AUR-410]` | Normalizar scanner de segredos |
| [AUR-080](cards/backlog/AUR-080.md) | backlog | O05-review | high | `[AUR-078, AUR-026, AUR-410]` | Normalizar SAST e dependency scanners |
| [AUR-081](cards/backlog/AUR-081.md) | backlog | O05-review | critical | `[AUR-079, AUR-080, AUR-026, AUR-403]` | Validar findings antes de síntese |
| [AUR-082](cards/backlog/AUR-082.md) | backlog | O05-review | high | `[AUR-081, AUR-403]` | Deduplicar findings por fingerprint |
| [AUR-083](cards/backlog/AUR-083.md) | backlog | O04-agents | critical | `[AUR-034, AUR-055, AUR-403]` | Definir protocolo JSON de agentes |
| [AUR-084](cards/backlog/AUR-084.md) | backlog | O04-agents | high | `[AUR-083, AUR-210, AUR-405]` | Mapear impacto com agente limitado |
| [AUR-085](cards/backlog/AUR-085.md) | backlog | O04-agents | critical | `[AUR-083, AUR-072, AUR-073, AUR-074, AUR-077, AUR-405]` | Executar reviewers especializados em paralelo |
| [AUR-086](cards/backlog/AUR-086.md) | backlog | O04-agents | critical | `[AUR-083, AUR-079, AUR-080, AUR-405]` | Executar security reviewer independente |
| [AUR-087](cards/backlog/AUR-087.md) | backlog | O05-review | critical | `[AUR-082, AUR-073, AUR-084, AUR-085, AUR-086, AUR-217, AUR-405]` | Sintetizar sem inventar evidência |
| [AUR-088](cards/backlog/AUR-088.md) | backlog | O05-review | critical | `[AUR-087, AUR-073, AUR-405]` | Verificar findings com agente independente |
| [AUR-089](cards/backlog/AUR-089.md) | backlog | O05-review | high | `[AUR-088, AUR-016, AUR-403]` | Avaliar ISO 25010 somente por evidência |
| [AUR-090](cards/backlog/AUR-090.md) | backlog | O05-review | critical | `[AUR-088, AUR-040, AUR-404]` | Validar patch sugerido em worktree |
| [AUR-091](cards/backlog/AUR-091.md) | backlog | O05-review | high | `[AUR-027, AUR-403]` | Renderizar review JSON canônico |
| [AUR-092](cards/backlog/AUR-092.md) | backlog | O05-review | high | `[AUR-088, AUR-017, AUR-403]` | Renderizar SARIF 2.1.0 válido |
| [AUR-093](cards/backlog/AUR-093.md) | backlog | O05-review | high | `[AUR-088, AUR-403]` | Renderizar Markdown seguro |
| [AUR-094](cards/backlog/AUR-094.md) | backlog | O05-review | critical | `[AUR-078, AUR-089, AUR-091, AUR-092, AUR-093, AUR-097, AUR-210, AUR-212, AUR-213, AUR-214, AUR-215, AUR-216, AUR-217, AUR-218, AUR-219, AUR-220, AUR-410]` | Executar review local clean sem memória |
| [AUR-095](cards/backlog/AUR-095.md) | backlog | O05-review | high | `[AUR-037, AUR-094, AUR-097, AUR-403]` | Expor aurumcode review na CLI |
| [AUR-096](cards/backlog/AUR-096.md) | backlog | O00-governance | critical | `[AUR-094, AUR-095, AUR-291, AUR-292, AUR-293, AUR-410]` | Aprovar review local agnóstico |
| [AUR-097](cards/backlog/AUR-097.md) | backlog | O06-scm | high | `[AUR-039, AUR-040, AUR-041, AUR-042, AUR-404]` | Implementar ChangeSource Git local |
| [AUR-098](cards/backlog/AUR-098.md) | backlog | O06-scm | critical | `[AUR-029, AUR-034, AUR-009, AUR-409]` | Verificar assinatura de webhook em bytes exatos |
| [AUR-099](cards/backlog/AUR-099.md) | backlog | O06-scm | critical | `[AUR-018, AUR-098, AUR-296, AUR-297, AUR-298, AUR-409]` | Normalizar webhook GitHub |
| [AUR-100](cards/backlog/AUR-100.md) | backlog | O06-scm | critical | `[AUR-097, AUR-057, AUR-018, AUR-409]` | Ler mudanças pela API GitHub |
| [AUR-101](cards/backlog/AUR-101.md) | backlog | O06-scm | critical | `[AUR-018, AUR-098, AUR-296, AUR-297, AUR-298, AUR-409]` | Normalizar webhook Gitea |
| [AUR-102](cards/backlog/AUR-102.md) | backlog | O06-scm | critical | `[AUR-097, AUR-057, AUR-101, AUR-409]` | Ler mudanças pela API Gitea |
| [AUR-103](cards/backlog/AUR-103.md) | backlog | O06-scm | critical | `[AUR-026, AUR-029, AUR-018, AUR-403]` | Definir contrato idempotente de publisher |
| [AUR-104](cards/backlog/AUR-104.md) | backlog | O06-scm | critical | `[AUR-103, AUR-034, AUR-035, AUR-407]` | Persistir publicação em outbox |
| [AUR-105](cards/backlog/AUR-105.md) | backlog | O06-scm | critical | `[AUR-100, AUR-103, AUR-104, AUR-107, AUR-081, AUR-220, AUR-409]` | Publicar review no GitHub |
| [AUR-106](cards/backlog/AUR-106.md) | backlog | O06-scm | critical | `[AUR-102, AUR-103, AUR-104, AUR-107, AUR-081, AUR-220, AUR-409]` | Publicar review no Gitea |
| [AUR-107](cards/backlog/AUR-107.md) | backlog | O00-security | critical | `[AUR-036, AUR-104, AUR-009, AUR-403]` | Assinar bundle entre analyzer e publisher |
| [AUR-108](cards/backlog/AUR-108.md) | backlog | O06-scm | critical | `[AUR-094, AUR-105, AUR-030, AUR-412]` | Definir inputs e outputs da Action v3 |
| [AUR-109](cards/backlog/AUR-109.md) | backlog | O06-scm | critical | `[AUR-107, AUR-108, AUR-410]` | Executar analyzer job sem escrita |
| [AUR-110](cards/backlog/AUR-110.md) | backlog | O06-scm | critical | `[AUR-107, AUR-108, AUR-109, AUR-220, AUR-409]` | Executar publisher job privilegiado separado |
| [AUR-111](cards/backlog/AUR-111.md) | backlog | O08-runtime | high | `[AUR-099, AUR-101, AUR-034, AUR-403]` | Expor health liveness sanitizado |
| [AUR-112](cards/backlog/AUR-112.md) | backlog | O00-governance | critical | `[AUR-097, AUR-098, AUR-099, AUR-100, AUR-101, AUR-102, AUR-103, AUR-104, AUR-105, AUR-106, AUR-107, AUR-108, AUR-109, AUR-110, AUR-111, AUR-178, AUR-179, AUR-395, AUR-396, AUR-409]` | Aprovar SCM e publicação segura |
| [AUR-113](cards/backlog/AUR-113.md) | backlog | O07-docs | critical | `[AUR-024, AUR-308, AUR-408]` | Definir ownership de documentação gerada |
| [AUR-114](cards/backlog/AUR-114.md) | backlog | O07-docs | high | `[AUR-113, AUR-222, AUR-408]` | Renderizar Markdown determinístico de entidade Go |
| [AUR-115](cards/backlog/AUR-115.md) | backlog | O07-docs | critical | `[AUR-113, AUR-114, AUR-408]` | Publicar docs por staging atômico |
| [AUR-116](cards/backlog/AUR-116.md) | backlog | O07-docs | critical | `[AUR-115, AUR-052, AUR-408]` | Remover somente páginas obsoletas owned |
| [AUR-117](cards/backlog/AUR-117.md) | backlog | O07-docs | critical | `[AUR-113, AUR-408]` | Atualizar somente regiões gerenciadas do README |
| [AUR-118](cards/backlog/AUR-118.md) | backlog | O07-docs | medium | `[AUR-113, AUR-097, AUR-408]` | Gerar changelog de Conventional Commits |
| [AUR-119](cards/backlog/AUR-119.md) | backlog | O07-docs | high | `[AUR-113, AUR-017, AUR-408]` | Importar e validar OpenAPI existente |
| [AUR-120](cards/backlog/AUR-120.md) | backlog | O07-docs | high | `[AUR-114, AUR-408]` | Validar links e anchors gerados |
| [AUR-121](cards/backlog/AUR-121.md) | backlog | O07-docs | high | `[AUR-115, AUR-120, AUR-408]` | Construir site com Hugo pinado |
| [AUR-122](cards/backlog/AUR-122.md) | backlog | O07-docs | medium | `[AUR-121, AUR-408]` | Indexar busca com Pagefind pinado |
| [AUR-123](cards/backlog/AUR-123.md) | backlog | O07-docs | high | `[AUR-114, AUR-066, AUR-408]` | Enriquecer prosa opcionalmente com LLM |
| [AUR-124](cards/backlog/AUR-124.md) | backlog | O07-docs | critical | `[AUR-114, AUR-115, AUR-120, AUR-036, AUR-408]` | Executar baseline confiável de docs Go |
| [AUR-125](cards/backlog/AUR-125.md) | backlog | O07-docs | critical | `[AUR-116, AUR-124, AUR-052, AUR-408]` | Executar docs build incremental equivalente |
| [AUR-126](cards/backlog/AUR-126.md) | backlog | O07-docs | medium | `[AUR-124, AUR-033, AUR-408]` | Isolar compatibilidade temporária Jekyll v2 |
| [AUR-127](cards/backlog/AUR-127.md) | backlog | O07-docs | high | `[AUR-037, AUR-124, AUR-125, AUR-126, AUR-408]` | Expor aurumcode docs na CLI |
| [AUR-128](cards/backlog/AUR-128.md) | backlog | O00-governance | critical | `[AUR-113, AUR-114, AUR-115, AUR-116, AUR-117, AUR-118, AUR-119, AUR-120, AUR-121, AUR-122, AUR-123, AUR-124, AUR-125, AUR-126, AUR-127, AUR-250, AUR-408]` | Aprovar documentação incremental |
| [AUR-129](cards/backlog/AUR-129.md) | backlog | O08-testqa | critical | `[AUR-027, AUR-090, AUR-403]` | Definir TestProposal seguro |
| [AUR-130](cards/backlog/AUR-130.md) | backlog | O08-testqa | critical | `[AUR-129, AUR-053, AUR-406]` | Descobrir toolchain sem executar manifests |
| [AUR-131](cards/backlog/AUR-131.md) | backlog | O08-testqa | high | `[AUR-129, AUR-210, AUR-074, AUR-403]` | Montar contexto mínimo para gerar testes |
| [AUR-132](cards/backlog/AUR-132.md) | backlog | O08-testqa | critical | `[AUR-131, AUR-066, AUR-397, AUR-406]` | Validar proposta estruturada de testes |
| [AUR-133](cards/backlog/AUR-133.md) | backlog | O08-testqa | critical | `[AUR-132, AUR-039, AUR-397, AUR-404]` | Aplicar proposta em worktree seguro |
| [AUR-134](cards/backlog/AUR-134.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-006, AUR-411]` | Executar testes Go isolados |
| [AUR-135](cards/backlog/AUR-135.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-411]` | Executar testes Python isolados |
| [AUR-136](cards/backlog/AUR-136.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-411]` | Executar testes JS e TS isolados |
| [AUR-137](cards/backlog/AUR-137.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-411]` | Executar testes dotnet isolados |
| [AUR-138](cards/backlog/AUR-138.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-411]` | Executar testes Rust isolados |
| [AUR-139](cards/backlog/AUR-139.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-411]` | Executar testes Shell isolados |
| [AUR-140](cards/backlog/AUR-140.md) | backlog | O08-testqa | critical | `[AUR-134, AUR-135, AUR-136, AUR-137, AUR-138, AUR-139, AUR-411]` | Normalizar cobertura e aplicar gate |
| [AUR-141](cards/backlog/AUR-141.md) | backlog | O08-testqa | critical | `[AUR-140, AUR-411]` | Provar não vacuidade por revert |
| [AUR-142](cards/backlog/AUR-142.md) | backlog | O08-testqa | high | `[AUR-140, AUR-411]` | Detectar testes flaky |
| [AUR-143](cards/backlog/AUR-143.md) | backlog | O08-testqa | high | `[AUR-141, AUR-411]` | Executar mutation proof elegível |
| [AUR-144](cards/backlog/AUR-144.md) | backlog | O08-testqa | critical | `[AUR-131, AUR-132, AUR-133, AUR-134, AUR-135, AUR-136, AUR-137, AUR-138, AUR-139, AUR-140, AUR-141, AUR-142, AUR-143, AUR-411]` | Gerar e verificar unit tests Go |
| [AUR-145](cards/backlog/AUR-145.md) | backlog | O08-testqa | critical | `[AUR-131, AUR-132, AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-119, AUR-411]` | Gerar e verificar API tests |
| [AUR-146](cards/backlog/AUR-146.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-050, AUR-411]` | Gerar e verificar fakes mínimos |
| [AUR-147](cards/backlog/AUR-147.md) | backlog | O08-testqa | high | `[AUR-026, AUR-140, AUR-080, AUR-411]` | Definir contrato neutro de QAResult |
| [AUR-148](cards/backlog/AUR-148.md) | backlog | O08-testqa | high | `[AUR-144, AUR-145, AUR-146, AUR-147, AUR-037, AUR-411]` | Expor testgen na CLI |
| [AUR-149](cards/backlog/AUR-149.md) | backlog | O00-governance | critical | `[AUR-129, AUR-130, AUR-131, AUR-132, AUR-133, AUR-134, AUR-135, AUR-136, AUR-137, AUR-138, AUR-139, AUR-140, AUR-141, AUR-142, AUR-143, AUR-144, AUR-145, AUR-146, AUR-147, AUR-148, AUR-251, AUR-252, AUR-253, AUR-254, AUR-255, AUR-256, AUR-257, AUR-258, AUR-397, AUR-398, AUR-411]` | Aprovar testgen e QA sandboxed |
| [AUR-150](cards/backlog/AUR-150.md) | backlog | O08-runtime | critical | `[AUR-034, AUR-035, AUR-403]` | Persistir runs efêmeros com CAS |
| [AUR-151](cards/backlog/AUR-151.md) | backlog | O08-runtime | critical | `[AUR-150, AUR-036, AUR-410]` | Executar um stage atomicamente |
| [AUR-152](cards/backlog/AUR-152.md) | backlog | O08-runtime | critical | `[AUR-151, AUR-094, AUR-104, AUR-410]` | Orquestrar workflow de review |
| [AUR-153](cards/backlog/AUR-153.md) | backlog | O08-runtime | critical | `[AUR-151, AUR-124, AUR-125, AUR-410]` | Orquestrar workflow de docs |
| [AUR-154](cards/backlog/AUR-154.md) | backlog | O08-runtime | critical | `[AUR-151, AUR-144, AUR-145, AUR-146, AUR-147, AUR-410]` | Orquestrar workflow de testgen e QA |
| [AUR-155](cards/backlog/AUR-155.md) | backlog | O08-runtime | critical | `[AUR-152, AUR-153, AUR-154, AUR-099, AUR-101, AUR-410]` | Enfileirar webhook sem bloquear ACK |
| [AUR-156](cards/backlog/AUR-156.md) | backlog | O00-governance | high | `[AUR-003, AUR-004, AUR-005, AUR-007, AUR-008, AUR-150, AUR-403]` | Derivar metas de evidência |
| [AUR-157](cards/backlog/AUR-157.md) | backlog | O08-runtime | critical | `[AUR-152, AUR-153, AUR-154, AUR-065, AUR-083, AUR-410]` | Limitar loops, agentes e recursos |
| [AUR-158](cards/backlog/AUR-158.md) | backlog | O08-runtime | critical | `[AUR-155, AUR-009, AUR-403]` | Emitir observabilidade por allowlist |
| [AUR-159](cards/backlog/AUR-159.md) | backlog | O08-runtime | high | `[AUR-150, AUR-151, AUR-152, AUR-153, AUR-154, AUR-155, AUR-156, AUR-157, AUR-158, AUR-111, AUR-395, AUR-396, AUR-399, AUR-403]` | Expor status de run na CLI |
| [AUR-160](cards/backlog/AUR-160.md) | backlog | O10-memory | critical | `[AUR-151, AUR-083, AUR-030, AUR-403]` | Manter working memory somente efêmera |
| [AUR-161](cards/backlog/AUR-161.md) | backlog | O10-memory | critical | `[AUR-160, AUR-030, AUR-043, AUR-407]` | Criar store local opcional sem sujar repo |
| [AUR-162](cards/backlog/AUR-162.md) | backlog | O10-memory | high | `[AUR-161, AUR-026, AUR-407]` | Registrar ledger episódico opcional |
| [AUR-163](cards/backlog/AUR-163.md) | backlog | O10-memory | high | `[AUR-162, AUR-052, AUR-407]` | Modelar knowledge claims com evidência |
| [AUR-164](cards/backlog/AUR-164.md) | backlog | O10-memory | critical | `[AUR-163, AUR-407]` | Aprovar memória durável explicitamente |
| [AUR-165](cards/backlog/AUR-165.md) | backlog | O10-memory | high | `[AUR-162, AUR-026, AUR-407]` | Registrar feedback opcional de memória |
| [AUR-166](cards/backlog/AUR-166.md) | backlog | O10-memory | critical | `[AUR-163, AUR-165, AUR-051, AUR-053, AUR-407]` | Limitar recall por budget e confiança |
| [AUR-167](cards/backlog/AUR-167.md) | backlog | O10-memory | critical | `[AUR-161, AUR-162, AUR-163, AUR-164, AUR-165, AUR-166, AUR-054, AUR-211, AUR-407]` | Rebuildar ou remover memória sem efeito funcional |
| [AUR-168](cards/backlog/AUR-168.md) | backlog | O11-mcp | high | `[AUR-021, AUR-029, AUR-150, AUR-410]` | Iniciar MCP stdio conforme SDK Go |
| [AUR-169](cards/backlog/AUR-169.md) | backlog | O11-mcp | critical | `[AUR-168, AUR-053, AUR-152, AUR-153, AUR-154, AUR-166, AUR-072, AUR-410]` | Registrar MCP tools read-only |
| [AUR-170](cards/backlog/AUR-170.md) | backlog | O11-mcp | critical | `[AUR-169, AUR-103, AUR-164, AUR-165, AUR-410]` | Definir autorização de MCP tool mutável |
| [AUR-171](cards/backlog/AUR-171.md) | backlog | O11-mcp | critical | `[AUR-170, AUR-021, AUR-410]` | Autorizar MCP HTTP com OAuth |
| [AUR-172](cards/backlog/AUR-172.md) | backlog | O11-mcp | critical | `[AUR-169, AUR-170, AUR-009, AUR-406]` | Bloquear prompt e tool injection via MCP |
| [AUR-173](cards/backlog/AUR-173.md) | backlog | O00-governance | critical | `[AUR-150, AUR-151, AUR-152, AUR-153, AUR-154, AUR-155, AUR-156, AUR-157, AUR-158, AUR-159, AUR-160, AUR-161, AUR-162, AUR-163, AUR-164, AUR-165, AUR-166, AUR-167, AUR-168, AUR-169, AUR-170, AUR-171, AUR-172, AUR-225, AUR-259, AUR-260, AUR-261, AUR-262, AUR-263, AUR-264, AUR-265, AUR-266, AUR-267, AUR-268, AUR-269, AUR-270, AUR-399, AUR-411]` | Aprovar plataforma integrada |
| [AUR-174](cards/backlog/AUR-174.md) | backlog | O03-providers | high | `[AUR-061, AUR-067, AUR-405]` | Configurar profile Azure OpenAI |
| [AUR-175](cards/backlog/AUR-175.md) | backlog | O03-providers | high | `[AUR-060, AUR-067, AUR-405]` | Configurar profile Gemini OpenAI-compatible |
| [AUR-176](cards/backlog/AUR-176.md) | backlog | O03-providers | high | `[AUR-062, AUR-067, AUR-405]` | Configurar Bedrock via LiteLLM |
| [AUR-177](cards/backlog/AUR-177.md) | backlog | O03-providers | critical | `[AUR-174, AUR-175, AUR-176, AUR-063, AUR-064, AUR-405]` | Comparar paridade estrutural de providers |
| [AUR-178](cards/backlog/AUR-178.md) | backlog | O06-scm | high | `[AUR-018, AUR-057, AUR-097, AUR-234, AUR-235, AUR-236, AUR-409]` | Ler merge requests GitLab |
| [AUR-179](cards/backlog/AUR-179.md) | backlog | O06-scm | high | `[AUR-081, AUR-103, AUR-104, AUR-107, AUR-178, AUR-220, AUR-409]` | Publicar review no GitLab |
| [AUR-180](cards/backlog/AUR-180.md) | backlog | O06-scm | critical | `[AUR-097, AUR-057, AUR-404]` | Buscar Git genérico por SSH |
| [AUR-181](cards/backlog/AUR-181.md) | backlog | O00-security | critical | `[AUR-173, AUR-406]` | Executar corpus de ataques de input |
| [AUR-182](cards/backlog/AUR-182.md) | backlog | O08-runtime | critical | `[AUR-173, AUR-410]` | Injetar falhas em todas as fronteiras |
| [AUR-183](cards/backlog/AUR-183.md) | backlog | O00-security | critical | `[AUR-173, AUR-181, AUR-410]` | Executar race global |
| [AUR-184](cards/backlog/AUR-184.md) | backlog | O08-runtime | high | `[AUR-173, AUR-412]` | Aplicar resource profile fixo aos benchmarks |
| [AUR-185](cards/backlog/AUR-185.md) | backlog | O09-demo | high | `[AUR-014, AUR-015, AUR-016, AUR-096, AUR-406]` | Definir schema do corpus rotulado de review |
| [AUR-186](cards/backlog/AUR-186.md) | backlog | O09-demo | critical | `[AUR-177, AUR-185, AUR-271, AUR-405]` | Calcular recall precisão e duplicação |
| [AUR-187](cards/backlog/AUR-187.md) | backlog | O09-demo | critical | `[AUR-128, AUR-149, AUR-173, AUR-177, AUR-178, AUR-179, AUR-180, AUR-400, AUR-411]` | Executar cenário demo Go |
| [AUR-188](cards/backlog/AUR-188.md) | backlog | O09-demo | critical | `[AUR-181, AUR-187, AUR-411]` | Executar cenários demo de segurança |
| [AUR-189](cards/backlog/AUR-189.md) | backlog | O09-demo | critical | `[AUR-182, AUR-187, AUR-411]` | Executar cenários demo de resiliência |
| [AUR-190](cards/backlog/AUR-190.md) | backlog | O09-demo | critical | `[AUR-012, AUR-108, AUR-109, AUR-110, AUR-187, AUR-188, AUR-189, AUR-411]` | Validar repo demo consumidor externo |
| [AUR-191](cards/backlog/AUR-191.md) | backlog | O00-security | critical | `[AUR-173, AUR-233, AUR-412]` | Verificar supply chain e SBOM |
| [AUR-192](cards/backlog/AUR-192.md) | backlog | O00-governance | critical | `[AUR-174, AUR-175, AUR-176, AUR-177, AUR-178, AUR-179, AUR-180, AUR-181, AUR-182, AUR-183, AUR-184, AUR-185, AUR-186, AUR-187, AUR-188, AUR-189, AUR-190, AUR-191, AUR-271, AUR-272, AUR-273, AUR-274, AUR-275, AUR-276, AUR-277, AUR-278, AUR-279, AUR-280, AUR-400, AUR-401, AUR-412]` | Aprovar paridade adversarial e demo |
| [AUR-193](cards/backlog/AUR-193.md) | backlog | O12-release | high | `[AUR-033, AUR-108, AUR-109, AUR-110, AUR-192, AUR-404]` | Migrar workflows Action v2 para v3 |
| [AUR-194](cards/backlog/AUR-194.md) | backlog | O12-release | critical | `[AUR-033, AUR-177, AUR-404]` | Migrar envs de provider para secret refs |
| [AUR-195](cards/backlog/AUR-195.md) | backlog | O12-release | high | `[AUR-001, AUR-192, AUR-412]` | Remover claims públicos sem evidência |
| [AUR-196](cards/backlog/AUR-196.md) | backlog | O12-release | high | `[AUR-024, AUR-025, AUR-026, AUR-027, AUR-028, AUR-029, AUR-030, AUR-031, AUR-032, AUR-033, AUR-034, AUR-035, AUR-036, AUR-037, AUR-067, AUR-068, AUR-069, AUR-070, AUR-071, AUR-072, AUR-075, AUR-169, AUR-170, AUR-171, AUR-412]` | Documentar contratos do core com exemplo |
| [AUR-197](cards/backlog/AUR-197.md) | backlog | O00-security | critical | `[AUR-181, AUR-188, AUR-191, AUR-406]` | Fechar threat model rastreável |
| [AUR-198](cards/backlog/AUR-198.md) | backlog | O12-release | high | `[AUR-158, AUR-159, AUR-182, AUR-184, AUR-412]` | Definir contrato executável de incident drill |
| [AUR-199](cards/backlog/AUR-199.md) | backlog | O12-release | critical | `[AUR-191, AUR-412]` | Produzir builds reproduzíveis multiarch |
| [AUR-200](cards/backlog/AUR-200.md) | backlog | O12-release | critical | `[AUR-199, AUR-036, AUR-412]` | Montar bundle de release verificável |
| [AUR-201](cards/backlog/AUR-201.md) | backlog | O12-release | critical | `[AUR-193, AUR-194, AUR-200, AUR-404]` | Provar rollback forward seguro |
| [AUR-202](cards/backlog/AUR-202.md) | backlog | O12-release | high | `[AUR-195, AUR-196, AUR-197, AUR-198, AUR-199, AUR-200, AUR-201, AUR-190, AUR-399, AUR-412]` | Monitorar piloto sintético por evidência |
| [AUR-203](cards/backlog/AUR-203.md) | backlog | O00-governance | critical | `[AUR-022, AUR-076, AUR-193, AUR-194, AUR-195, AUR-196, AUR-197, AUR-198, AUR-199, AUR-200, AUR-201, AUR-202, AUR-221, AUR-226, AUR-281, AUR-282, AUR-283, AUR-284, AUR-285, AUR-286, AUR-287, AUR-288, AUR-289, AUR-290, AUR-294, AUR-295, AUR-302, AUR-303, AUR-304, AUR-305, AUR-306, AUR-307, AUR-308, AUR-309, AUR-310, AUR-311, AUR-312, AUR-313, AUR-314, AUR-315, AUR-316, AUR-317, AUR-318, AUR-319, AUR-333, AUR-334, AUR-335, AUR-336, AUR-337, AUR-338, AUR-339, AUR-340, AUR-341, AUR-342, AUR-343, AUR-344, AUR-345, AUR-346, AUR-347, AUR-348, AUR-349, AUR-350, AUR-351, AUR-352, AUR-353, AUR-354, AUR-355, AUR-356, AUR-357, AUR-358, AUR-365, AUR-366, AUR-367, AUR-368, AUR-369, AUR-370, AUR-371, AUR-372, AUR-373, AUR-374, AUR-375, AUR-376, AUR-377, AUR-378, AUR-379, AUR-380, AUR-381, AUR-382, AUR-383, AUR-384, AUR-385, AUR-386, AUR-387, AUR-388, AUR-389, AUR-390, AUR-391, AUR-412]` | Aprovar release candidate consumível |
| [AUR-204](cards/backlog/AUR-204.md) | backlog | O00-governance | critical | `[AUR-203, AUR-403]` | Auditar rastreabilidade de use cases |
| [AUR-205](cards/backlog/AUR-205.md) | backlog | O00-governance | critical | `[AUR-204, AUR-023, AUR-403]` | Auditar extensibilidade hexagonal |
| [AUR-206](cards/backlog/AUR-206.md) | backlog | O00-security | critical | `[AUR-204, AUR-197, AUR-412]` | Auditar segurança e privacidade independentemente |
| [AUR-207](cards/backlog/AUR-207.md) | backlog | O00-governance | critical | `[AUR-205, AUR-206, AUR-412]` | Reproduzir todos os gates em checkout frio |
| [AUR-208](cards/backlog/AUR-208.md) | backlog | O00-governance | critical | `[AUR-186, AUR-190, AUR-207, AUR-299, AUR-300, AUR-301, AUR-412]` | Comprovar todos os objetivos funcionais |
| [AUR-209](cards/backlog/AUR-209.md) | backlog | O00-governance | critical | `[AUR-208, AUR-319, AUR-412]` | Emitir aceite final somente pelo skeptic |
| [AUR-210](cards/backlog/AUR-210.md) | backlog | O02-index | critical | `[AUR-039, AUR-042, AUR-222, AUR-404]` | Resolver contexto vivo sem store persistente |
| [AUR-211](cards/backlog/AUR-211.md) | backlog | O10-memory | critical | `[AUR-030, AUR-161, AUR-166, AUR-210, AUR-403]` | Retornar augmentation opcional com status tipado |
| [AUR-212](cards/backlog/AUR-212.md) | backlog | O05-review | critical | `[AUR-025, AUR-042, AUR-403]` | Exigir CoverageManifest completo por pass |
| [AUR-213](cards/backlog/AUR-213.md) | backlog | O04-agents | critical | `[AUR-035, AUR-036, AUR-083, AUR-210, AUR-009, AUR-405]` | Selar snapshot e contexto por papel |
| [AUR-214](cards/backlog/AUR-214.md) | backlog | O05-review | critical | `[AUR-083, AUR-212, AUR-213, AUR-332, AUR-405]` | Executar primeiro pass completo cego |
| [AUR-215](cards/backlog/AUR-215.md) | backlog | O05-review | critical | `[AUR-083, AUR-212, AUR-213, AUR-332, AUR-405]` | Executar segundo pass completo cego |
| [AUR-216](cards/backlog/AUR-216.md) | backlog | O05-review | critical | `[AUR-035, AUR-212, AUR-213, AUR-214, AUR-215, AUR-320, AUR-321, AUR-322, AUR-405]` | Validar selagem independência e staleness |
| [AUR-217](cards/backlog/AUR-217.md) | backlog | O05-review | critical | `[AUR-026, AUR-216, AUR-405]` | Unir resultados completos deterministicamente |
| [AUR-218](cards/backlog/AUR-218.md) | backlog | O05-review | critical | `[AUR-212, AUR-213, AUR-405]` | Selar challenge plan antes dos reviews |
| [AUR-219](cards/backlog/AUR-219.md) | backlog | O05-review | critical | `[AUR-006, AUR-007, AUR-218, AUR-405]` | Executar mutações e fault injections céticas |
| [AUR-220](cards/backlog/AUR-220.md) | backlog | O05-review | critical | `[AUR-005, AUR-008, AUR-212, AUR-216, AUR-217, AUR-219, AUR-394, AUR-403]` | Decidir aprovação por função pura |
| [AUR-221](cards/backlog/AUR-221.md) | backlog | O05-review | critical | `[AUR-083, AUR-213, AUR-220, AUR-405]` | Julgar uma apelação com agente novo |
| [AUR-222](cards/backlog/AUR-222.md) | backlog | O02-index | high | `[AUR-025, AUR-029, AUR-406]` | Parsear blob Go sem estado |
| [AUR-223](cards/backlog/AUR-223.md) | backlog | O02-index | critical | `[AUR-020, AUR-222, AUR-406]` | Executar parser worker stateless |
| [AUR-224](cards/backlog/AUR-224.md) | backlog | O02-index | high | `[AUR-223, AUR-406]` | Definir contrato de grammar parser stateless |
| [AUR-225](cards/backlog/AUR-225.md) | backlog | O10-memory | critical | `[AUR-152, AUR-211, AUR-407]` | Integrar memória opcional sem bloquear review stateless |
| [AUR-226](cards/backlog/AUR-226.md) | backlog | O13-delivery | high | `[AUR-111, AUR-191, AUR-412]` | Empacotar servidor Go em imagem OCI |
| [AUR-227](cards/backlog/AUR-227.md) | backlog | O08-testqa | high | `[AUR-006, AUR-410]` | Selecionar engine OCI por contrato |
| [AUR-228](cards/backlog/AUR-228.md) | backlog | O08-testqa | critical | `[AUR-227, AUR-410]` | Executar worker não-root e read-only |
| [AUR-229](cards/backlog/AUR-229.md) | backlog | O08-testqa | critical | `[AUR-227, AUR-410]` | Negar egress e superfícies do host |
| [AUR-230](cards/backlog/AUR-230.md) | backlog | O08-testqa | critical | `[AUR-227, AUR-410]` | Limitar privilégios e recursos do worker |
| [AUR-231](cards/backlog/AUR-231.md) | backlog | O08-testqa | critical | `[AUR-228, AUR-229, AUR-230, AUR-410]` | Provar perfil seguro no Docker |
| [AUR-232](cards/backlog/AUR-232.md) | backlog | O08-testqa | critical | `[AUR-228, AUR-229, AUR-230, AUR-410]` | Provar perfil seguro no Podman |
| [AUR-233](cards/backlog/AUR-233.md) | backlog | O00-security | critical | `[AUR-359, AUR-360, AUR-361, AUR-362, AUR-363, AUR-364]` | Verificar lockset completo antes do bootstrap |
| [AUR-234](cards/backlog/AUR-234.md) | backlog | O03-providers | critical | `[AUR-057, AUR-233, AUR-405]` | Validar TLS e destino HTTP |
| [AUR-235](cards/backlog/AUR-235.md) | backlog | O03-providers | high | `[AUR-057, AUR-233, AUR-405]` | Propagar timeout e cancelamento HTTP |
| [AUR-236](cards/backlog/AUR-236.md) | backlog | O03-providers | critical | `[AUR-057, AUR-233, AUR-405]` | Limitar resposta HTTP antes do decode |
| [AUR-237](cards/backlog/AUR-237.md) | backlog | O04-agents | critical | `[AUR-015, AUR-072, AUR-403]` | Entregar skill security de review |
| [AUR-238](cards/backlog/AUR-238.md) | backlog | O04-agents | high | `[AUR-014, AUR-072, AUR-403]` | Entregar skill verify de findings |
| [AUR-239](cards/backlog/AUR-239.md) | backlog | O04-agents | high | `[AUR-014, AUR-072, AUR-403]` | Entregar skill synthesize de review |
| [AUR-240](cards/backlog/AUR-240.md) | backlog | O04-agents | medium | `[AUR-074, AUR-222, AUR-411]` | Entregar skill Go |
| [AUR-241](cards/backlog/AUR-241.md) | backlog | O04-agents | medium | `[AUR-074, AUR-224, AUR-411]` | Entregar skill Python |
| [AUR-242](cards/backlog/AUR-242.md) | backlog | O04-agents | medium | `[AUR-074, AUR-224, AUR-411]` | Entregar skill JavaScript |
| [AUR-243](cards/backlog/AUR-243.md) | backlog | O04-agents | medium | `[AUR-074, AUR-224, AUR-411]` | Entregar skill TypeScript |
| [AUR-244](cards/backlog/AUR-244.md) | backlog | O04-agents | medium | `[AUR-074, AUR-224, AUR-411]` | Entregar skill CSharp |
| [AUR-245](cards/backlog/AUR-245.md) | backlog | O04-agents | high | `[AUR-074, AUR-224, AUR-411]` | Entregar skill C |
| [AUR-246](cards/backlog/AUR-246.md) | backlog | O04-agents | high | `[AUR-074, AUR-224, AUR-411]` | Entregar skill CPlusPlus |
| [AUR-247](cards/backlog/AUR-247.md) | backlog | O04-agents | medium | `[AUR-074, AUR-224, AUR-411]` | Entregar skill Rust |
| [AUR-248](cards/backlog/AUR-248.md) | backlog | O04-agents | high | `[AUR-074, AUR-224, AUR-411]` | Entregar skill Bash |
| [AUR-249](cards/backlog/AUR-249.md) | backlog | O04-agents | high | `[AUR-074, AUR-224, AUR-411]` | Entregar skill PowerShell |
| [AUR-250](cards/backlog/AUR-250.md) | backlog | O06-scm | high | `[AUR-108, AUR-124, AUR-408]` | Expor docs na GitHub Action |
| [AUR-251](cards/backlog/AUR-251.md) | backlog | O08-testqa | critical | `[AUR-130, AUR-133, AUR-233, AUR-411]` | Executar testes PowerShell isolados |
| [AUR-252](cards/backlog/AUR-252.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test Python |
| [AUR-253](cards/backlog/AUR-253.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test JavaScript |
| [AUR-254](cards/backlog/AUR-254.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test TypeScript |
| [AUR-255](cards/backlog/AUR-255.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test dotnet |
| [AUR-256](cards/backlog/AUR-256.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test Rust |
| [AUR-257](cards/backlog/AUR-257.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test Shell |
| [AUR-258](cards/backlog/AUR-258.md) | backlog | O08-testqa | high | `[AUR-131, AUR-132, AUR-405]` | Gerar proposta de unit test PowerShell |
| [AUR-259](cards/backlog/AUR-259.md) | backlog | O08-runtime | high | `[AUR-150, AUR-151, AUR-152, AUR-407]` | Expor resume de run na CLI |
| [AUR-260](cards/backlog/AUR-260.md) | backlog | O08-runtime | high | `[AUR-150, AUR-151, AUR-152, AUR-407]` | Expor cancel de run na CLI |
| [AUR-261](cards/backlog/AUR-261.md) | backlog | O08-runtime | high | `[AUR-111, AUR-150, AUR-151, AUR-407]` | Expor runs por API read-only |
| [AUR-262](cards/backlog/AUR-262.md) | backlog | O11-mcp | high | `[AUR-169, AUR-210, AUR-404]` | Expor repo context por MCP |
| [AUR-263](cards/backlog/AUR-263.md) | backlog | O11-mcp | critical | `[AUR-152, AUR-169, AUR-403]` | Expor review por MCP read-only |
| [AUR-264](cards/backlog/AUR-264.md) | backlog | O11-mcp | high | `[AUR-153, AUR-169, AUR-408]` | Expor docs por MCP read-only |
| [AUR-265](cards/backlog/AUR-265.md) | backlog | O11-mcp | critical | `[AUR-154, AUR-169, AUR-411]` | Expor testqa por MCP read-only |
| [AUR-266](cards/backlog/AUR-266.md) | backlog | O11-mcp | high | `[AUR-166, AUR-169, AUR-407]` | Expor memory status por MCP |
| [AUR-267](cards/backlog/AUR-267.md) | backlog | O11-mcp | high | `[AUR-072, AUR-169, AUR-403]` | Expor skill list por MCP |
| [AUR-268](cards/backlog/AUR-268.md) | backlog | O11-mcp | critical | `[AUR-103, AUR-107, AUR-170, AUR-409]` | Expor publicação por MCP mutável |
| [AUR-269](cards/backlog/AUR-269.md) | backlog | O11-mcp | critical | `[AUR-164, AUR-170, AUR-407]` | Expor aprovação de memória por MCP |
| [AUR-270](cards/backlog/AUR-270.md) | backlog | O11-mcp | critical | `[AUR-165, AUR-170, AUR-407]` | Expor feedback de finding por MCP |
| [AUR-271](cards/backlog/AUR-271.md) | backlog | O09-demo | high | `[AUR-185, AUR-006]` | Materializar corpus rotulado v1 |
| [AUR-272](cards/backlog/AUR-272.md) | backlog | O09-demo | high | `[AUR-187, AUR-241, AUR-252, AUR-411]` | Executar cenário demo Python |
| [AUR-273](cards/backlog/AUR-273.md) | backlog | O09-demo | high | `[AUR-187, AUR-242, AUR-253, AUR-411]` | Executar cenário demo JavaScript |
| [AUR-274](cards/backlog/AUR-274.md) | backlog | O09-demo | high | `[AUR-187, AUR-243, AUR-254, AUR-411]` | Executar cenário demo TypeScript |
| [AUR-275](cards/backlog/AUR-275.md) | backlog | O09-demo | high | `[AUR-187, AUR-244, AUR-255, AUR-411]` | Executar cenário demo CSharp |
| [AUR-276](cards/backlog/AUR-276.md) | backlog | O09-demo | high | `[AUR-145, AUR-187, AUR-245, AUR-411]` | Executar cenário demo C |
| [AUR-277](cards/backlog/AUR-277.md) | backlog | O09-demo | high | `[AUR-145, AUR-187, AUR-246, AUR-411]` | Executar cenário demo CPlusPlus |
| [AUR-278](cards/backlog/AUR-278.md) | backlog | O09-demo | high | `[AUR-187, AUR-247, AUR-256, AUR-411]` | Executar cenário demo Rust |
| [AUR-279](cards/backlog/AUR-279.md) | backlog | O09-demo | high | `[AUR-187, AUR-248, AUR-257, AUR-411]` | Executar cenário demo Bash |
| [AUR-280](cards/backlog/AUR-280.md) | backlog | O09-demo | high | `[AUR-187, AUR-249, AUR-258, AUR-411]` | Executar cenário demo PowerShell |
| [AUR-281](cards/backlog/AUR-281.md) | backlog | O12-release | medium | `[AUR-067, AUR-196, AUR-408]` | Documentar contratos de providers |
| [AUR-282](cards/backlog/AUR-282.md) | backlog | O12-release | medium | `[AUR-112, AUR-196, AUR-408]` | Documentar contratos SCM |
| [AUR-283](cards/backlog/AUR-283.md) | backlog | O12-release | medium | `[AUR-075, AUR-196, AUR-408]` | Documentar contratos de skills |
| [AUR-284](cards/backlog/AUR-284.md) | backlog | O12-release | medium | `[AUR-172, AUR-196, AUR-408]` | Documentar contratos MCP |
| [AUR-285](cards/backlog/AUR-285.md) | backlog | O12-release | medium | `[AUR-196, AUR-200, AUR-408]` | Documentar contratos operacionais e release |
| [AUR-286](cards/backlog/AUR-286.md) | backlog | O12-release | high | `[AUR-066, AUR-198, AUR-405]` | Executar drill de outage de provider |
| [AUR-287](cards/backlog/AUR-287.md) | backlog | O12-release | high | `[AUR-112, AUR-198, AUR-409]` | Executar drill de rate limit SCM |
| [AUR-288](cards/backlog/AUR-288.md) | backlog | O12-release | high | `[AUR-150, AUR-198, AUR-407]` | Executar drill de corrupção de state store |
| [AUR-289](cards/backlog/AUR-289.md) | backlog | O12-release | critical | `[AUR-009, AUR-198, AUR-403]` | Executar drill de canário de segredo |
| [AUR-290](cards/backlog/AUR-290.md) | backlog | O12-release | critical | `[AUR-198, AUR-201, AUR-412]` | Executar drill de rollback de release |
| [AUR-291](cards/backlog/AUR-291.md) | backlog | O05-review | critical | `[AUR-094, AUR-405]` | Rejeitar review local com finding bloqueante |
| [AUR-292](cards/backlog/AUR-292.md) | backlog | O05-review | critical | `[AUR-065, AUR-094, AUR-405]` | Falhar review local ao exceder budget |
| [AUR-293](cards/backlog/AUR-293.md) | backlog | O05-review | critical | `[AUR-094, AUR-405]` | Falhar review local em erro de provider |
| [AUR-294](cards/backlog/AUR-294.md) | backlog | O13-delivery | high | `[AUR-226, AUR-412]` | Empacotar adapter Cloud Run |
| [AUR-295](cards/backlog/AUR-295.md) | backlog | O13-delivery | high | `[AUR-226, AUR-412]` | Empacotar adapter Cloud Functions |
| [AUR-296](cards/backlog/AUR-296.md) | backlog | O06-scm | critical | `[AUR-098, AUR-409]` | Rejeitar replay de webhook |
| [AUR-297](cards/backlog/AUR-297.md) | backlog | O06-scm | critical | `[AUR-098, AUR-409]` | Limitar payload de webhook |
| [AUR-298](cards/backlog/AUR-298.md) | backlog | O06-scm | high | `[AUR-098, AUR-409]` | Allowlistar evento de webhook |
| [AUR-299](cards/backlog/AUR-299.md) | backlog | O09-demo | high | `[AUR-125, AUR-184, AUR-187, AUR-408]` | Provar docs sync abaixo de cinco minutos |
| [AUR-300](cards/backlog/AUR-300.md) | backlog | O09-demo | high | `[AUR-094, AUR-184, AUR-403]` | Provar zero-config em oitenta por cento |
| [AUR-301](cards/backlog/AUR-301.md) | backlog | O09-demo | high | `[AUR-144, AUR-184, AUR-411]` | Provar delta de cobertura de vinte pontos |
| [AUR-302](cards/backlog/AUR-302.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-404]` | Caracterizar locks Go legados |
| [AUR-303](cards/backlog/AUR-303.md) | backlog | O14-legacy | high | `[AUR-001, AUR-226, AUR-412]` | Substituir Dockerfile legado |
| [AUR-304](cards/backlog/AUR-304.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-233, AUR-410]` | Quarentenar compose legado |
| [AUR-305](cards/backlog/AUR-305.md) | backlog | O14-legacy | high | `[AUR-001, AUR-108, AUR-409]` | Migrar action.yml legado |
| [AUR-306](cards/backlog/AUR-306.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-018, AUR-409]` | Criar workflows least-privilege a partir da ausência |
| [AUR-307](cards/backlog/AUR-307.md) | backlog | O14-legacy | medium | `[AUR-001, AUR-127, AUR-408]` | Migrar cmd regenerate-docs legado |
| [AUR-308](cards/backlog/AUR-308.md) | backlog | O14-legacy | high | `[AUR-001, AUR-006]` | Caracterizar internal documentation legado |
| [AUR-309](cards/backlog/AUR-309.md) | backlog | O14-legacy | high | `[AUR-001, AUR-006]` | Caracterizar internal pipeline legado |
| [AUR-310](cards/backlog/AUR-310.md) | backlog | O14-legacy | high | `[AUR-001, AUR-019, AUR-405]` | Caracterizar internal llm legado |
| [AUR-311](cards/backlog/AUR-311.md) | backlog | O14-legacy | high | `[AUR-001, AUR-023, AUR-403]` | Retirar pkg types por compatibilidade |
| [AUR-312](cards/backlog/AUR-312.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-233, AUR-365, AUR-366, AUR-367, AUR-368, AUR-369, AUR-370, AUR-371, AUR-372, AUR-373, AUR-374, AUR-375, AUR-376]` | Verificar migração dos doze arquivos de script |
| [AUR-313](cards/backlog/AUR-313.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-377, AUR-378, AUR-379, AUR-380, AUR-381, AUR-382]` | Verificar claims dos seis documentos públicos |
| [AUR-314](cards/backlog/AUR-314.md) | backlog | O14-legacy | high | `[AUR-001, AUR-113, AUR-006]` | Fixar ownership de outputs legados |
| [AUR-315](cards/backlog/AUR-315.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-408]` | Isolar dependências Jekyll legadas |
| [AUR-316](cards/backlog/AUR-316.md) | backlog | O14-legacy | medium | `[AUR-001, AUR-011, AUR-411]` | Separar demo legado do consumer repo |
| [AUR-317](cards/backlog/AUR-317.md) | backlog | O14-legacy | medium | `[AUR-001, AUR-011, AUR-411]` | Auditar fixture repo1 legada |
| [AUR-318](cards/backlog/AUR-318.md) | backlog | O14-legacy | medium | `[AUR-001, AUR-006]` | Congelar taskmaster como histórico |
| [AUR-319](cards/backlog/AUR-319.md) | backlog | O14-legacy | high | `[AUR-010, AUR-404]` | Validar instrução canônica sem autoria IA |
| [AUR-320](cards/backlog/AUR-320.md) | backlog | O05-review | critical | `[AUR-213, AUR-214, AUR-215, AUR-405]` | Isolar provider conversation dos reviewers |
| [AUR-321](cards/backlog/AUR-321.md) | backlog | O05-review | critical | `[AUR-213, AUR-214, AUR-215, AUR-405]` | Isolar cache e memória entre reviewers |
| [AUR-322](cards/backlog/AUR-322.md) | backlog | O05-review | critical | `[AUR-213, AUR-214, AUR-215, AUR-405]` | Classificar independência por backend real |
| [AUR-323](cards/backlog/AUR-323.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob Python sem estado |
| [AUR-324](cards/backlog/AUR-324.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob JavaScript sem estado |
| [AUR-325](cards/backlog/AUR-325.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob TypeScript sem estado |
| [AUR-326](cards/backlog/AUR-326.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob CSharp sem estado |
| [AUR-327](cards/backlog/AUR-327.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob C sem estado |
| [AUR-328](cards/backlog/AUR-328.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob CPlusPlus sem estado |
| [AUR-329](cards/backlog/AUR-329.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob Rust sem estado |
| [AUR-330](cards/backlog/AUR-330.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob Bash sem estado |
| [AUR-331](cards/backlog/AUR-331.md) | backlog | O02-index | high | `[AUR-224, AUR-406]` | Parsear blob PowerShell sem estado |
| [AUR-332](cards/backlog/AUR-332.md) | backlog | O03-providers | critical | `[AUR-055, AUR-059, AUR-234, AUR-235, AUR-236, AUR-405]` | Adaptar fake OpenAI endpoint ao ModelPort |
| [AUR-333](cards/backlog/AUR-333.md) | backlog | O00-security | critical | `[AUR-001, AUR-009, AUR-408]` | Redigir credencial literal da documentação de pipeline |
| [AUR-334](cards/backlog/AUR-334.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-068, AUR-408]` | Inventariar prompts AurumCode como dados não confiáveis |
| [AUR-335](cards/backlog/AUR-335.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-030, AUR-408]` | Caracterizar exemplos YAML de configuração legados |
| [AUR-336](cards/backlog/AUR-336.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-412]` | Caracterizar Makefile legado sem executar recipes |
| [AUR-337](cards/backlog/AUR-337.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-383, AUR-384, AUR-385, AUR-408]` | Verificar ownership dos três guias legados |
| [AUR-338](cards/backlog/AUR-338.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-233, AUR-410]` | Quarentenar Dockerfile documental legado |
| [AUR-339](cards/backlog/AUR-339.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-021, AUR-068, AUR-386, AUR-387, AUR-388, AUR-389, AUR-408]` | Verificar os quatro settings locais sem dispatch |
| [AUR-340](cards/backlog/AUR-340.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-013, AUR-068, AUR-403]` | Fixar equivalência dos skills locais de roteamento |
| [AUR-341](cards/backlog/AUR-341.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Servir OpenAI Chat unary determinístico |
| [AUR-342](cards/backlog/AUR-342.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Servir OpenAI Responses unary determinístico |
| [AUR-343](cards/backlog/AUR-343.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Servir Anthropic Messages unary determinístico |
| [AUR-344](cards/backlog/AUR-344.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Transmitir OpenAI Chat SSE determinístico |
| [AUR-345](cards/backlog/AUR-345.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Transmitir OpenAI Responses events determinísticos |
| [AUR-346](cards/backlog/AUR-346.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Transmitir Anthropic Messages events determinísticos |
| [AUR-347](cards/backlog/AUR-347.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Servir OpenAI Chat tool call determinístico |
| [AUR-348](cards/backlog/AUR-348.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Servir OpenAI Responses function call determinístico |
| [AUR-349](cards/backlog/AUR-349.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-350, AUR-405]` | Servir Anthropic tool_use determinístico |
| [AUR-350](cards/backlog/AUR-350.md) | backlog | O03-providers | high | `[AUR-055, AUR-057, AUR-405]` | Executar fault script provider-neutral determinístico |
| [AUR-351](cards/backlog/AUR-351.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-068, AUR-403]` | Inventariar rules AurumCode sem autoridade implícita |
| [AUR-352](cards/backlog/AUR-352.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-312, AUR-408]` | Inventariar documentação gerada de scripts sem execução |
| [AUR-353](cards/backlog/AUR-353.md) | backlog | O14-legacy | high | `[AUR-001, AUR-016, AUR-030, AUR-403]` | Caracterizar pesos ISO 25010 legados |
| [AUR-354](cards/backlog/AUR-354.md) | backlog | O14-legacy | medium | `[AUR-001, AUR-404]` | Caracterizar ignore local de AurumCode |
| [AUR-355](cards/backlog/AUR-355.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-390, AUR-391, AUR-404]` | Verificar semântica dos dois ignores raiz |
| [AUR-356](cards/backlog/AUR-356.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Fixar ownership manual do changelog legado |
| [AUR-357](cards/backlog/AUR-357.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-068, AUR-408]` | Quarentenar instruções locais GEMINI como não confiáveis |
| [AUR-358](cards/backlog/AUR-358.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-030, AUR-408]` | Caracterizar exemplos de ambiente sem carregar valores |
| [AUR-359](cards/ready/AUR-359.md) | ready | O00-security | critical | `[]` | Fixar trust root do runner OCI |
| [AUR-360](cards/backlog/AUR-360.md) | backlog | O00-security | critical | `[AUR-359]` | Fixar Go toolchain e módulos |
| [AUR-361](cards/backlog/AUR-361.md) | backlog | O00-security | critical | `[AUR-359]` | Fixar referências de GitHub Actions |
| [AUR-362](cards/backlog/AUR-362.md) | backlog | O00-security | critical | `[AUR-359]` | Fixar scanners de segurança |
| [AUR-363](cards/backlog/AUR-363.md) | backlog | O00-security | critical | `[AUR-359]` | Fixar parsers e grammars |
| [AUR-364](cards/backlog/AUR-364.md) | backlog | O00-security | critical | `[AUR-359]` | Fixar ferramentas de documentação |
| [AUR-365](cards/backlog/AUR-365.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar action entrypoint legado |
| [AUR-366](cards/backlog/AUR-366.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar build docs site legado |
| [AUR-367](cards/backlog/AUR-367.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-233, AUR-412]` | Caracterizar bulk enable repos legado |
| [AUR-368](cards/backlog/AUR-368.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar generate code docs legado |
| [AUR-369](cards/backlog/AUR-369.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar generate demo page legado |
| [AUR-370](cards/backlog/AUR-370.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar enhanced docs legado |
| [AUR-371](cards/backlog/AUR-371.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar live demo legado |
| [AUR-372](cards/backlog/AUR-372.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-233, AUR-412]` | Caracterizar setup GitHub App legado |
| [AUR-373](cards/backlog/AUR-373.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar gerador docs simples raiz |
| [AUR-374](cards/backlog/AUR-374.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-233, AUR-412]` | Caracterizar pipeline docs shell raiz |
| [AUR-375](cards/backlog/AUR-375.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-233, AUR-412]` | Caracterizar pipeline docs batch raiz |
| [AUR-376](cards/backlog/AUR-376.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-412]` | Caracterizar test Jekyll legado |
| [AUR-377](cards/backlog/AUR-377.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Qualificar claims do README raiz |
| [AUR-378](cards/backlog/AUR-378.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Qualificar claims do guia da Action |
| [AUR-379](cards/backlog/AUR-379.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-195, AUR-408]` | Qualificar claims do quickstart LiteLLM |
| [AUR-380](cards/backlog/AUR-380.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-195, AUR-408]` | Qualificar claims do setup Pages |
| [AUR-381](cards/backlog/AUR-381.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-195, AUR-333, AUR-408]` | Qualificar claims do pipeline de docs |
| [AUR-382](cards/backlog/AUR-382.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Qualificar claims do setup guide |
| [AUR-383](cards/backlog/AUR-383.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Mapear claims de docs README |
| [AUR-384](cards/backlog/AUR-384.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Mapear claims do getting started |
| [AUR-385](cards/backlog/AUR-385.md) | backlog | O14-legacy | high | `[AUR-001, AUR-195, AUR-408]` | Mapear claims do guia pages fix |
| [AUR-386](cards/backlog/AUR-386.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-021, AUR-068, AUR-408]` | Caracterizar settings MCP do Cursor |
| [AUR-387](cards/backlog/AUR-387.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-021, AUR-068, AUR-408]` | Caracterizar settings MCP do Gemini |
| [AUR-388](cards/backlog/AUR-388.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-021, AUR-068, AUR-408]` | Caracterizar settings MCP da raiz |
| [AUR-389](cards/backlog/AUR-389.md) | backlog | O14-legacy | critical | `[AUR-001, AUR-009, AUR-021, AUR-068, AUR-408]` | Caracterizar settings locais do Claude |
| [AUR-390](cards/backlog/AUR-390.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-404]` | Caracterizar gitignore da raiz |
| [AUR-391](cards/backlog/AUR-391.md) | backlog | O14-legacy | high | `[AUR-001, AUR-233, AUR-404]` | Caracterizar dockerignore da raiz |
| [AUR-392](cards/backlog/AUR-392.md) | backlog | O05-review | critical | `[AUR-026, AUR-035]` | Definir suppression stateless escopada e expirável |
| [AUR-393](cards/backlog/AUR-393.md) | backlog | O05-review | critical | `[AUR-035, AUR-082, AUR-392]` | Aplicar matching de suppression stale-safe |
| [AUR-394](cards/backlog/AUR-394.md) | backlog | O05-review | critical | `[AUR-035, AUR-036, AUR-392, AUR-393]` | Exigir autorização humana para suppression |
| [AUR-395](cards/backlog/AUR-395.md) | backlog | O08-runtime | high | `[AUR-099, AUR-101, AUR-034, AUR-403]` | Expor readiness dependente sanitizada |
| [AUR-396](cards/backlog/AUR-396.md) | backlog | O08-runtime | high | `[AUR-099, AUR-101, AUR-034, AUR-403]` | Emitir métricas runtime por allowlist |
| [AUR-397](cards/backlog/AUR-397.md) | backlog | O08-testqa | critical | `[AUR-027, AUR-090, AUR-129, AUR-403]` | Definir FileEdit seguro |
| [AUR-398](cards/backlog/AUR-398.md) | backlog | O08-testqa | high | `[AUR-147, AUR-037, AUR-411]` | Expor QAResult na CLI |
| [AUR-399](cards/backlog/AUR-399.md) | backlog | O00-governance | high | `[AUR-156, AUR-150, AUR-403]` | Abrir monitor de evidência sem autoridade |
| [AUR-400](cards/backlog/AUR-400.md) | backlog | O06-scm | critical | `[AUR-097, AUR-057, AUR-404]` | Buscar Git genérico por HTTPS |
| [AUR-401](cards/backlog/AUR-401.md) | backlog | O00-security | critical | `[AUR-173, AUR-181, AUR-410]` | Executar fuzz global bounded |
| [AUR-402](cards/backlog/AUR-402.md) | backlog | O00-governance | critical | `[AUR-006]` | Versionar registry de perfis OCI |
| [AUR-403](cards/backlog/AUR-403.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil Go unit offline |
| [AUR-404](cards/backlog/AUR-404.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil Go Git offline |
| [AUR-405](cards/backlog/AUR-405.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil fake provider offline |
| [AUR-406](cards/backlog/AUR-406.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil parser worker offline |
| [AUR-407](cards/backlog/AUR-407.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil SQLite offline |
| [AUR-408](cards/backlog/AUR-408.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil docs tool offline |
| [AUR-409](cards/backlog/AUR-409.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil fake SCM offline |
| [AUR-410](cards/backlog/AUR-410.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil OCI conformance |
| [AUR-411](cards/backlog/AUR-411.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil polyglot toolchain offline |
| [AUR-412](cards/backlog/AUR-412.md) | backlog | O00-governance | critical | `[AUR-006, AUR-402]` | Definir perfil release build offline |
| [AUR-413](cards/backlog/AUR-413.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-135, AUR-252, AUR-411]` | Verificar proposta de unit test Python |
| [AUR-414](cards/backlog/AUR-414.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-136, AUR-253, AUR-411]` | Verificar proposta de unit test JavaScript |
| [AUR-415](cards/backlog/AUR-415.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-136, AUR-254, AUR-411]` | Verificar proposta de unit test TypeScript |
| [AUR-416](cards/backlog/AUR-416.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-137, AUR-255, AUR-411]` | Verificar proposta de unit test dotnet |
| [AUR-417](cards/backlog/AUR-417.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-138, AUR-256, AUR-411]` | Verificar proposta de unit test Rust |
| [AUR-418](cards/backlog/AUR-418.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-139, AUR-257, AUR-411]` | Verificar proposta de unit test Shell |
| [AUR-419](cards/backlog/AUR-419.md) | backlog | O08-testqa | high | `[AUR-133, AUR-140, AUR-141, AUR-142, AUR-143, AUR-251, AUR-258, AUR-411]` | Verificar proposta de unit test PowerShell |
| [AUR-420](cards/backlog/AUR-420.md) | backlog | O03-providers | critical | `[AUR-009, AUR-236, AUR-405]` | Sanitizar resposta HTTP antes de persistir |
| [AUR-421](cards/backlog/AUR-421.md) | backlog | O13-delivery | critical | `[AUR-200, AUR-226, AUR-412]` | Provar release OCI reproduzível |
