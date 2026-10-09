# Plano de produto e entrega — AurumCode

Data: 2026-09-28 (America/Sao_Paulo). Base auditada:
`a73ea97344ca7c7f63ed05a6464538cfac178095`.

## Objetivo do usuario

Um time instala o AurumCode com poucos passos, escolhe seu modelo e recebe
reviews uteis, ancorados no diff e consistentes entre rodadas. Contexto,
prompts e skills ajudam a entender o projeto; comentarios precisam demonstrar
o impacto da mudanca. O produto tambem deve exigir changelog conciso nos
merges, suportar releases semanticas e ensinar cada capacidade com exemplos
executaveis no site. Gratuito e aberto, com custo do provedor explicitado.

## Base consolidada

- Apenas `main` local/remota; mesmo SHA, checkout limpo no inicio, zero PR aberta.
- Pipeline estrutural inicial: exit 0, 505 cards, 105 done, 389 cancelled,
  7 backlog e 4 ready. Esse resultado nao mede qualidade semantica do review.
- CI e Pages do SHA auditado passaram. A release `v1.1.0` aponta para uma
  versao anterior: nao e uma release do produto atual.
- Os cards antigos em done permanecem como evidencia historica. Um contrato
  antigo que diverge do codigo atual pede regressao/correcao nova, nao uma
  reescrita retroativa da entrega.

## Casos de uso e aceite de produto

| Caso | Situacao observada na base | Entrega e prova exigida |
| --- | --- | --- |
| Instalar em repo GitHub | Workflow reutilizavel existente; exemplo em main mutavel | AUR-506, AUR-510, AUR-512: copiar exemplo pinado em repo consumidor e obter parecer real |
| Configurar idioma e publicacao | CLI le `.aurumcode/config.yml`; configurador do site nomeia outro arquivo | AUR-506: caminho correto e escolhas copiadas no navegador |
| CLI local em container | Fluxo documentado e testes de pacote; compara commits | AUR-491/AUR-501: exemplo do zero, incluindo base inexistente e modo sem provedor |
| Review LLM falha ou inconclusivo | Reutilizavel exige qualidade; Action direta nao exige | AUR-507: Action propaga falha; AUR-512: demonstracao com imagem real |
| Review de codigo amplo | Prompt e catalogo incluem corretude, manutencao, performance e seguranca | AUR-511: medir defeitos reais e negativos por categoria e linguagem |
| Foco no diff | Ha filtro de localizacao/evidencia; nao prova veracidade do defeito | AUR-511: precisao de finding e ancora; contexto fora do diff informa, nao amplia escopo |
| Menos ruido | Dois passes podem comentar o mesmo local; volume sozinho nao mede qualidade | AUR-454: consolidar achados equivalentes e aplicar preferencias explicitas sem esconder erros |
| Cache e personalizacao | A chave atual nao inclui prompt/skills/docs efetivos ou perfis selecionados | AUR-513: trocar orientacao invalida cache e contexto cross-file continua disponivel |
| Evidencia apos combinar perfis | Conversao atual perde Evidence/Impact/Verification e outros campos | AUR-514: preservar os campos ate o parecer publicado |
| Parecer coerente | Resumo livre pode repetir acusacao de um finding filtrado | AUR-517: resumo e veredito coerentes com o resultado aceito |
| Identidade do contexto remoto | Resolucao atual usa cwd, mesmo ao revisar outro repo | AUR-515: verificar repo/SHA ou avisar que contexto foi omitido |
| Rodadas sucessivas | Historico entra no prompt; nao ha dedup garantida | AUR-494: mesmo diff idempotente, linha movida, bug corrigido e feedback anterior |
| Sugestoes aplicaveis | Parte da capacidade ja implementada; card ainda aberto | AUR-453: provar resumo e sugestao no CLI/PR, faixa correta e recusa de alteracao insegura |
| Comments/review/request changes | Modos existentes; dependem de permissoes e qualidade | AUR-512: testar publicacao real, gate, sugestao e falhas de permissoes |
| Prompts, skills e docs | Arquivos de contexto suportados; PR usa base confiavel | AUR-480/AUR-491: configuracao minima, timeout/erro visivel e config malformada rejeitada |
| Contexto de codebase | Resolucao heuristica implementada; promessas de RAG sao mais amplas | AUR-470: trechos de chamadores/testes, referencias rastreaveis e omissoes visiveis |
| MCP opcional | Card sem integracao CLI demonstrada | AUR-469 depois de AUR-480: contexto consultado com limites de execucao e sem autoridade sobre gates |
| Memoria entre reviews | Opt-in local/ephemeral; nao equivale a dedup garantida | AUR-494/AUR-501: isolamento por repo, cache persistente explicado, feedback nao vira instrucao |
| Cobertura parcial | Metadados de cobertura existem; exposicao publica ainda pendente | AUR-476: mostrar omissoes no terminal e PR sem poluir caso completo |
| CI falhando | Contexto contem estado/link dos checks, nao a causa comprovada | AUR-516/AUR-512: estado verificado separado de texto do modelo; orientar proximo diagnostico |
| Linters e dependencias | Catalogo proprio; integracoes externas nao entregues | AUR-492/AUR-495: adapter real, resultado normalizado, indisponibilidade visivel e diff preservado |
| Fix local | CLI gera patch de sugestoes | AUR-491/AUR-501: fluxo review -> sugestoes -> patch aplicavel, sem alterar repo automaticamente |
| Proposta de testes | Plano de testes existe; execucao autonoma nao comprovada | AUR-511 mede utilidade; execucao de codigo consumidor exige escopo e sandbox proprios antes de prometer |
| Changelog por merge | Sugestao opcional, sem gate obrigatorio | AUR-509: check deterministico recusa entrada ausente/vazia e passa entrada valida |
| Versao, tag e release | Calculo semver existe; falta fluxo de publicacao atual | AUR-510: versao monotona, tag imutavel no SHA integrado, notas concisas e identidade humana |
| Site e docs como prova | Site bonito e testado no navegador; exemplos ainda divergem | AUR-491/AUR-501: tutorial por capacidade, exemplos executados e limites honestos |
| Comparar com concorrentes | Pesquisa existe, resultado medido nao | AUR-511: corpus congelado, negativos, varias rodadas, custo/latencia e adjudicacao cega |

## Sequencia e agentes

| Onda | Cards | Responsavel de implementacao | Condicao de saida |
| --- | --- | --- | --- |
| 1 — instalacao correta | AUR-506 e AUR-507, paths disjuntos | Luna: configurador; Sol: Action | Aceites shell, navegador/CI pertinentes e um review independente por candidato |
| 2 — execucao verificavel | AUR-508 | Sol | Profile canonico carregavel; uma assercao Go real passa e sua mutacao falha em container |
| 3 — previsibilidade do review | AUR-513/514/515/516/517, AUR-453/476/480/494/454 | Sol, uma posse de cmd/aurumcode por vez | Cache/contexto corretos, evidencia preservada, parecer coerente, dedup e sugestoes comprovadas |
| 4 — contrato de merge | AUR-509 | Sol | Changelog conciso obrigatorio nos merges |
| 5 — contexto e adapters | AUR-470, AUR-469, AUR-492, AUR-495 | Sol | Contexto relevante e fontes configuraveis; nenhuma indisponibilidade vira falso verde |
| 6 — demonstracao e comparacao | AUR-491, AUR-501, AUR-511, AUR-512 | Luna: docs/corpus; Sol: harness/QA | Site reproduzivel, PR consumidor real e relatorio sem claims nao medidos |
| 7 — publicacao oficial | AUR-510 | Sol | QA consumidor aprovada antes de publicar versao, tag e release pinada |

Docs e pesquisa podem andar em paralelo quando nao tocam paths de um builder.
O numero de agentes segue a independencia dos arquivos e a capacidade medida
do container; nao ha frota automatica. Luna e Sol sao os modelos solicitados
pelo usuario; pesquisa nao substitui review independente de codigo.

## Pre-requisitos encontrados no board

1. Os quatro cards ready antigos indicavam bootstrap Bash para futuras provas Go.
   O profile Go existente tem schema/objetos que `oci-run` rejeita. AUR-508
   normaliza esse contrato antes de liberar esses builders. Eles voltaram ao
   backlog com dependencia explicita, sem perder trabalho ou evidencia.
2. AUR-494, AUR-492 e AUR-495 agora declaram `accept` e mutacao concreta.
3. AUR-469/AUR-470 receberam write-set do CLI/contexto; AUR-469 depende da
   degradacao segura do AUR-480 e ambos dependem da identidade do AUR-515.
4. AUR-454 teve caminhos removidos/selectors alheios eliminados. AUR-453
   recebeu cenarios de sugestao, faixa e publicacao, aproveitando o codigo atual.
5. AUR-491 distingue opcoes publicas de hooks internos de teste.

Esses reparos alteram apenas contratos de trabalho futuro. Nenhuma capacidade
ganha status done por haver um arquivo, um comentario de memoria ou CI agregado.

## Benchmark util ao time

Executar primeiro um piloto estratificado, com tamanho definido pela cobertura
dos cenarios e pela incerteza estatistica observada, sem prometer vitoria.
Congelar base/head e ground truth antes de rodar ferramentas. Separar fixtures
usadas no desenvolvimento do conjunto final e manter PRs sem defeito.

Medir precisao/recall de defeitos por severidade, erros fora do diff, duplicacao,
repeticao depois de corrigir/rejeitar um achado, sugestoes aplicaveis, rodadas
ate encerramento, tempo humano de triagem, latencia e custo. Rodar o mesmo PR
original, corrigido parcialmente, corrigido completamente e sem alteracao.

Comparar tambem contexto ligado/desligado e variantes versionadas de prompts e
skills, mantendo modelo e corpus constantes. Uma sugestao so conta como
aplicavel quando seu patch aplica, preserva os testes pertinentes e corrige o
defeito esperado; emitir um bloco de codigo nao basta. Registrar separadamente
tempo ativo de triagem e decisao humana (aceita, rejeitada ou inconclusiva),
sem inferir aceitacao a partir da quantidade de comentarios.

Usar Qwen local como endpoint semantico de teste, registrando o modelo realmente
servido. Comparacao de pipelines com mesmo modelo e comparacao de produtos
hospedados sao experimentos distintos. Ausencia de acesso a concorrente e
`nao medido`, nunca nota zero ou resultado inventado. Contas novas e gastos
nao fazem parte desta pesquisa.

Referencias primarias consultadas pelos agentes:

- [Alibaba Open Code Review](https://github.com/alibaba/open-code-review) e
  [AACR-Bench: metricas](https://github.com/alibaba/aacr-bench/blob/main/docs/metrics.md).
- [CodeRabbit: funcionamento](https://docs.coderabbit.ai/guides/code-review-overview).
- [Greptile: metodologia](https://www.greptile.com/benchmarks): taxa de deteccao
  isolada nao mede falso positivo.
- [Copilot: uso e limitacoes](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/request-a-code-review/use-code-review).
- [Martian: benchmark e limitacoes](https://github.com/withmartian/code-review-benchmark).

## Criterio para publicar a versao oficial

Instalacao pinada demonstrada em consumidor, review/falha publicados corretamente,
configuracao copiavel funcionando, changelog do release coerente, testes pertinentes
em container e docs correspondentes ao SHA. Publicar com a identidade humana ja
configurada. A tag historica v1 fica preservada; o contrato atual de code review
tem mudanca de produto e deve receber sua propria versao principal.

Cada janela real de 20 minutos registra cards concluidos e bloqueio medido.
Duas janelas sem conclusao encerram a abordagem: rever escopo, prova ou ambiente
antes de reenviar trabalho. Pesquisa e planejamento nao contam como done.
