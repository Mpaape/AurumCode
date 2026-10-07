# Arquitetura

Como o AurumCode está organizado, como uma revisão flui e onde cada tipo de
extensão se encaixa. Os princípios fixados pelo dono vêm primeiro porque todas
as outras seções decorrem deles.

## Princípios

- **Sem hardcode.** Linguagens, regras, scanners e tipos de BOM vêm de
  catálogos YAML e gramáticas (`internal/analyzer/language_catalog.yml`,
  `internal/grammar/catalog`, `internal/xbom/catalog`), não de instruções
  `switch`. Um novo item é dado.
- **LLM primeiro, com evidência determinística.** O modelo revisa; AST,
  linters, Semgrep, SBOM e fontes públicas são evidências que ele pode explicar,
  mas nunca remover ou rebaixar. A severidade do próprio modelo não é confiável.
- **A configuração é YAML e Markdown.** O comportamento é definido em
  `.aurumcode.yml` e em seções de skill escritas em Markdown. Uma política
  central e um repositório são resolvidos por seção, com uma precedência
  explícita para cada uma.
- **Contêiner reproduzível.** Builds e testes de Go rodam apenas na imagem de
  contêiner compartilhada, a partir de um Dockerfile versionado
  (`.board/bin/go-shared`, `oci-run` selado para aceite), nunca no host de uma
  pessoa desenvolvedora.
- **Falha fechada.** Uma fonte do gate que dá erro, uma identidade de
  repositório que não pode ser verificada, um artefato inutilizável: a revisão
  é inconclusiva e nunca é aprovada pelo silêncio.

## Mapa de módulos

`cmd/aurumcode` interpreta flags, monta as dependências e publica. As regras de
negócio vivem em `internal/`.

| Pacote | Responsabilidade |
| --- | --- |
| `internal/analysis` | Passagem determinística de análise estática sobre um diff (catálogo embutido, go vet). |
| `internal/analyzer` | Parsing de diff, detecção de linguagem a partir do catálogo de linguagens, diffs de texto. |
| `internal/apply` | Transforma sugestões validadas em patches seguros e aplicáveis. |
| `internal/artifacts` | O artefato de dados de análise: cópia resolvida, verificada por idade e digest e em cache dos dados publicados dos scanners. |
| `internal/changelog` | Monta seções de changelog e incrementos de versão semântica a partir de commits revisados, e decide o changelog obrigatório (`Requirement.Verify`, padrões em `require_defaults.yml`). |
| `internal/config` | Configuração efetiva: seções, precedência da política central, gate, exceções, quality gates. |
| `internal/context` | Contexto de código limitado e determinístico, leitor de skills e o cliente das fontes MCP de contexto (`internal/context/mcp`: servidor e ferramenta declarados na configuração confiável, payload declarado e redigido, resposta só como contexto com origem). |
| `internal/dependencies` | A verificação de dependências: o modelo lê o diff de qualquer manifesto ou lockfile e nomeia os pacotes alterados, o código aterra a resposta no diff e a confere nos dois sentidos com o osv-scanner (todo arquivo alterado que ele reconhece), consulta a base OSV pelos dois lados e classifica cada advisory como introduzido, pré-existente ou corrigido; os metadados vivos do deps.dev alimentam a suspeita de typosquat do modelo (mantida só com evidência dos metadados) e a licença registrada, avaliada pela expressão SPDX; `Scan` faz o mesmo sobre a árvore inteira para a varredura agendada. Fonte inalcançável, vencida ou scanner ausente é inconclusivo. |
| `internal/deliberation` | Conversa limitada com ferramentas junto a um modelo, sem semântica de revisão: `Tool` (`Spec`, `Run`), `Limits` (rodadas, tokens, timeout por ferramenta), validação de argumentos antes de qualquer execução, o `Transcript` e o `LimitError` tipado que quem chama trata como inconclusivo. |
| `internal/dtrack` | Cliente do OWASP Dependency-Track para o gate de SBOM. |
| `internal/evidence` | Manifesto do pacote de evidências endereçado por conteúdo. |
| `internal/feedback` | O ciclo de realimentação (AUR-532): sinais de uso vindos do GitHub (alerta dispensado como falso positivo, achado corrigido entre auditorias, `/aurum perdeu`), propostas do modelo validadas contra os sinais citados, o registro de sinais no repositório da política, a medição antes/depois do corpus e a PR única. Nada é aplicado à política sem merge humano. |
| `internal/gate` | O pipeline do gate: `Run`, `Result`, `Contributor`, `Pipeline`, a regra de falha, o ranking de inconclusivo (`RankReason`) e a decisão de saída (`ExitPolicy`). |
| `internal/git` | Cliente do GitHub e acesso ao git usados pelo caminho `--pr`. |
| `internal/gittest` | Ambiente hermético para o git real dos fixtures de teste: sem configuração global nem de sistema, `HOME` privado e sem prompt, para o teste dar o mesmo resultado no container de desenvolvimento e no CI. |
| `internal/governance` | Especificação de tarefas e modelo de grafo de dependências do board. |
| `internal/grammar` | A única fonte da estrutura por linguagem, a partir de catálogos de gramáticas. |
| `internal/i18n` | Catálogo de textos de interface por idioma (YAML embutido, pt-BR e en): toda chave existe nos dois idiomas. |
| `internal/llm` | Provedores, orquestração, orçamento e estimativa de custo. |
| `internal/mcpserver` | O adaptador MCP (Model Context Protocol) por stdio de `aurumcode mcp`: o subconjunto JSON-RPC 2.0 que o MCP exige (`initialize`, `tools/list`, `tools/call`), as quatro ferramentas só leitura (`aurum_review`, `aurum_gate`, `aurum_rules`, `aurum_explain`) com JSON Schema fechado, a validação dos argumentos antes de executar e um único ponto de redação de toda resposta. Não decide nada: pergunta à porta `Gateway`, que `cmd/aurumcode` implementa com a mesma sessão `--base`. |
| `internal/memory` | Memória de revisão opcional. |
| `internal/prompt` | Montagem de prompt, orçamento, parsing de resposta (dividido por responsabilidade, com os padrões compilados uma vez no pacote), filtro de comentários, notas de cobertura. |
| `internal/render` | Relatórios determinísticos, registros de auditoria, SARIF (inclusive com categoria própria, `automationDetails.id`) e identidade de achados. |
| `internal/review` | O revisor, o escopo, as regras (incluindo regras dinâmicas de skill), o cache de revisão, a sessão de revisão (`internal/review/session`: ordem das fases e dados por fonte) e as ferramentas que uma revisão oferece ao modelo (`internal/review/tools`: scanners opcionais, contexto de código, seções de skill e as ferramentas que leem a revisão revisada, com o custo declarado no manifesto). |
| `internal/reviewprofile` | Perfis de revisor: os embutidos são YAML versionado no binário (`builtin.yml`), lidos pelo mesmo decodificador do arquivo de perfis do time. |
| `internal/sandbox` | Perfis de execução selados. |
| `internal/scanner` | O contrato de scanner (`Scanner`, `Report`, `Finding.ToIssue`), o registro fechado de engines compiladas e o executor; as engines vivem em subpacotes (`internal/scanner/semgrep`, SAST sobre a árvore; `internal/scanner/govet`, lint com `go vet`; `internal/scanner/gitleaks`, segredos sobre o intervalo de commits revisado `Request.Range`) listados em `internal/scanner/engines`. O escopo de mudança (`scanner.AddedLines`, `LineSet.Keep`) é um lugar só: as engines de código (semgrep, govet) varrem a árvore, mas guardam só achados em linhas que o intervalo revisado adicionou, e o semgrep usa o mesmo conjunto para decidir se um erro de parse alcança a mudança. A revisão entrega a cada engine o intervalo revisado (`--pr`: a base e o head do pull request; `--base`: a ref e `HEAD` resolvidos para ids completos), e a identidade informada por cada engine (`Outcome.Version`) entra no digest de evidências da chave de cache. |
| `internal/scanner/engines/exemplo` | A engine de exemplo do guia de extensão (`docs/extensao.md`): informa linhas que contêm um marcador, registrada apenas em um binário compilado com a tag `aurum_exemplo`. |
| `internal/sbom` | Geração e validação de SBOM CycloneDX. |
| `internal/security` | Redação de segredos em todos os destinos de saída. |
| `internal/supplychain` | Assinatura Sigstore/Cosign de SBOMs e imagens. |
| `internal/testgen` | Propostas determinísticas de testes a partir de um diff. |
| `internal/xbom` | BOMs além do SBOM (build, dados e assim por diante) a partir de catálogos. |

## Layers

As dependências apontam em uma só direção: `pkg/types` (as formas de domínio compartilhadas) ← os
pacotes `internal/` ← `cmd`. Dentro de `internal/`, a configuração guarda valores
e não executa nada, o domínio decide sem saber como é apresentado, e a
apresentação (`internal/render`) consome os fatos do domínio: os achados
estruturados e as exceções do gate vivem em `internal/gate/facts`, que tanto
`internal/gate` quanto `internal/render` importam e que não importa nenhum dos dois. A tabela abaixo é lida por um teste estrutural
(`cmd/aurumcode/structure_test.go`): um arquivo de produção do pacote da
esquerda que importa o pacote da direita falha no teste, a menos que a coluna
de exceção permita exatamente esse arquivo ou apenas os símbolos listados. Uma
exceção que já não corresponde a nada também falha, para que não sobreviva à sua razão.

| Package | Must not import | Exception |
| --- | --- | --- |
| `pkg` | `internal` | |
| `pkg` | `cmd` | |
| `internal` | `cmd` | |
| `internal` | `internal/render` | |
| `internal/gate` | `internal/render` | |
| `internal/render` | `internal/config` | |
| `internal/config` | `internal/llm` | file `internal/config/wrap.go` |
| `internal/config` | `internal/dtrack` | only `ValidateHost` |

`internal/config/wrap.go` é o decorador de provedor que injeta contexto; ele
permanece em `internal/config` enquanto os scripts de aceite concluídos ainda
chamam `config.WrapProvider`. `internal/config` usa o Dependency-Track apenas
para recusar um `server_api_host` inseguro na hora do parse; submeter o SBOM e
julgá-lo cabe a `internal/gate`.

## Fluxo da revisão

`--base` revisa um diff local e imprime um relatório; `--pr` revisa um pull
request e publica comentários, uma revisão formal e status de commit. Ambos são
uma só sessão de revisão (`internal/review/session`) e diferem apenas na fonte
(de onde vem o diff) e no publicador (terminal ou GitHub).

`session.Order` é a única ordem de fases; `session.Run` a executa e cada passo
retorna `(exit, done)`, de modo que cada saída antecipada mantém seu código:

1. **resolve.** Valida a invocação; configuração, política central, diff,
   contexto, perfis, memória.
2. **model.** Seleção do provedor e a passagem de qualidade do modelo. Com
   `deliberation.enabled`, o modelo pode primeiro pedir ferramentas (abaixo).
   Seu resultado é um `gate.ModelOutcome` tipado: `reviewed`, `skipped` (nenhum
   provedor configurado), `provider failed` (sem resposta, ou uma revisão de
   qualidade exigida que não aconteceu) ou `parse failed` (uma resposta que não
   pôde ser validada).
3. **evidence.** A passagem de segurança, a análise estática, a configuração de
   regras do repositório, cada engine de scanner habilitada e a cobertura. Um
   único passo decide onde ficam os achados de segurança (nos issues da revisão
   em `--pr`, em sua própria seção em `--base`); o snapshot de reaproveitamento
   de veredito guarda os mesmos achados nos dois casos.
4. **gate.** O pipeline compartilhado abaixo, a partir do `gate.Run` da sessão.
   O motivo de inconclusivo é `gate.RankReason`: falha do provedor, revisão
   ignorada, resposta não interpretável, parse degradado, o motivo do primeiro
   scanner, cobertura parcial, nessa ordem.
5. **publish.** Os artefatos de conformidade (a partir dos achados da execução
   do gate), o relatório ou a publicação no GitHub e, por fim, `gate.ExitPolicy`.

No `--pr`, o diff vem da API; quando ela o recusa por tamanho
(`githubclient.ErrDiffTooLarge`, `406 too_large`), o mesmo intervalo
`base...head` é lido do checkout já verificado como o PR
(`analyzer.Repo.RangeDiff`, `cmd/aurumcode/pr_local_diff.go`) e passa pelo
mesmo parser do diff da API; checkout não verificado é falha.

Uma fonte é código de `cmd/aurumcode` (`baseReview`, `prReview`) que embute o
`reviewState` compartilhado; o que difere entre as duas e não é a fonte nem o
publicador é dado em `session.Source`:

| Resultado do modelo | `--base` (`session.LocalDiff`) | `--pr` (`session.PullRequest`) |
| --- | --- | --- |
| provider failed | não revisado (saída 1) | não revisado apenas com `--exigir-qualidade`; caso contrário o gate decide |
| parse failed | não revisado (saída 1) | não revisado apenas com `--exigir-qualidade`; caso contrário o gate decide |
| skipped | o gate decide (`--exigir-qualidade` o escala para provider failed) | inalcançável: nenhum provedor é um erro |

`gate.ExitPolicy` é uma única escada, da mais alta para a mais baixa: um
comentário de revisão que não pôde ser postado (1), não revisado (1), um status
de commit que não pôde ser publicado (1), o gate (uma violação 3, um bloqueio
1), uma auditoria ou SARIF solicitados e não gravados (1), `--fail-on` (3), o
código próprio do status de `--check`.

Os colaboradores da sessão são injetados (`reviewDeps`), nunca variáveis de
pacote: o relógio contra o qual as exceções são julgadas, o executor de
scanners, o observador do pipeline do gate, o resolvedor de codebase, o
construtor de prompt que versiona os caches e o ambiente, lido uma só vez na
borda do comando.

## Adaptador MCP para agentes de código

`aurumcode mcp` serve o gate a um agente de código (Claude Code, Codex,
Cursor) por stdio. É um adaptador fino: `internal/mcpserver` fala o protocolo,
valida os argumentos contra o schema declarado e redige toda resposta;
`cmd/aurumcode/mcp_gateway.go` monta cada pergunta como a linha de comando
`review --base <ref> --seguranca --exigir-qualidade`, constrói a sessão com o
mesmo `newBaseReview` da CLI e a executa com `session.Run`. A decisão
(`pass`, `fail`, `inconclusive`) é lida do código de saída e do `gate.Result`
da própria sessão, nunca recalculada; só uma saída 0 de uma revisão
conclusiva é `pass`. O cliente escolhe apenas a ref; política central
(`AURUMCODE_POLICY`), skills, severidades e limites são do servidor. As regras
listadas por `aurum_rules` vêm de `resolveSkillRules`, a mesma resolução que a
sessão usa para aceitar citações.

## Prompt de revisão

O prompt de revisão é um único template, `internal/prompt/templates/review.md`.
Seu corpo é a mensagem de sistema (instruções, catálogo de regras, formato de
resposta); seus blocos `{{define}}` nomeados são os slots a partir dos quais a
mensagem de usuário é montada. O código Go decide quais slots são renderizados
e com quais dados; todo título de seção vive no template, e um teste falha
diante de um título `## ` em uma string Go dos pacotes de montagem do prompt.

Slots da mensagem de usuário, em ordem: resumo da mudança, contexto de CI e as
mudanças de código orçadas (`user_header`); histórico do PR; contexto da
codebase; memória de revisão; evidências determinísticas (um item por achado,
com origem, regra, `file:line`, severidade e um trecho redigido); ferramentas
disponíveis (com custo declarado); cobertura da revisão; contexto do
repositório. Um slot cuja entrada está vazia não renderiza nada, de modo que
uma revisão sem essa entrada mantém seus bytes anteriores.

Os orçamentos vêm de `internal/prompt/templates/limits.yml`: o teto do prompt
usado quando quem chama não define nenhum, o teto do catálogo de regras e os
tetos de evidências e ferramentas. Um slot de lista acima do teto admite itens
inteiros e informa quantos omitiu; trechos de código que não cabem são
declarados no slot de cobertura. Todo slot, inclusive o contexto do
repositório, é contado dentro do orçamento.

O `Reviewer` depende de um `Completer` (`CompleteMessages`), implementado por
`llm.Orchestrator`. O prompt viaja como uma mensagem de sistema e uma de
usuário. Um provedor com a capacidade `llm.MessageCompleter` as recebe
separadas; qualquer outro provedor recebe `System + "\n\n" + User` por
`Complete`, e a estimativa de custo é feita sobre esse mesmo texto. Decoradores
de provedor que apenas repassam requisições implementam `llm.Unwrapper`, de
modo que `llm.As` encontre uma capacidade atrás deles; um decorador que altera
a requisição não deve implementá-lo.

Um diff que não cabe num prompt (algum arquivo com patch omitido ou parcial)
é revisado em lotes (`internal/review/batches.go`): os arquivos são empacotados
por diretório em prompts que cabem, cada lote passa pelas mesmas quatro etapas
do prompt único (com a evidência dos seus arquivos) e os resultados são
consolidados num só (`batch_merge.go`). Os tetos de lotes e da soma dos
prompts vêm de `limits.yml` e da seção `batches`; os arquivos além deles são
contados e nomeados como omitidos (`code_files_omitted_paths`), de modo que a
cobertura fica parcial. `Reviewer.Batches` alimenta o campo `batches` da
auditoria.

`Reviewer.PromptDigest` é o digest das mensagens exatas enviadas.
`Reviewer.RequestCacheKey` o combina com digests das evidências e dos
resultados de ferramentas (`review/cache.RequestKey`), de modo que evidências
que o teto deixou fora do texto ainda alteram a chave. O modelo pode anexar a
um issue uma `assessment` (`confirmed`, `disputed`, `needs_context`, com
justificativa); `origin` é escrita apenas pelo motor, e o parser descarta a que
vier do modelo.

## Deliberação

Com `deliberation.enabled` e um provedor que implementa `llm.ToolCaller`
(encontrado por `llm.As`), a fase do modelo é uma conversa limitada com
ferramentas (`internal/deliberation.Session`) em vez de uma única chamada:

- A fase de evidências executa todo scanner `required` como antes; um scanner
  habilitado que não é `required` é adiado e oferecido como a ferramenta
  `scanner_<engine>`, ao lado de `codebase_context` (o contexto limitado de um
  arquivo alterado) e das ferramentas do repositório (`read_file`,
  `search_text`, `find_symbol` sobre as gramáticas de `internal/grammar`,
  `changed_file_diff`), que leem só a revisão revisada
  (`internal/review/tools.Revision`: caminho da árvore do commit, bytes com o
  mesmo id de blob, sem link simbólico, `ignore` nem arquivo de segredo) e
  dividem o teto `max_read_bytes`. O manifesto vai para o slot de ferramentas do prompt com o
  custo declarado e o tamanho de resultado de cada ferramenta. O modelo decide;
  o código nunca pede uma ferramenta por conta própria.
- Cada rodada é uma chamada a `Orchestrator.CompleteWithTools`: seu custo é
  reservado antes da chamada e confirmado depois, e o fallback só transita
  entre provedores que são `ToolCaller`. A resposta é solicitada com um JSON
  Schema derivado de `types.ReviewResult` (`llm.SchemaOf`) onde o provedor
  oferece suporte (`response_format: json_schema` no LiteLLM), e em modo JSON
  nos demais casos.
- Os argumentos de cada chamada são verificados contra o schema da ferramenta
  antes de ela rodar; uma chamada recusada é registrada e o modelo é informado
  do motivo. Um scanner solicitado roda pelo mesmo caminho da fase de
  evidências e entra nas varreduras da sessão: seus achados contam no gate com
  a sua origem, e um binário ausente ou uma varredura que falhou é o motivo
  inconclusivo da varredura.
- Exceder `max_rounds`, `max_cost_tokens`, `per_tool_timeout_seconds` ou
  `max_read_bytes` é o
  resultado de modelo `gate.ModelDeliberationLimit`, nunca revisado em nenhuma
  das fontes (saída 1); o gate ainda roda com o motivo inconclusivo
  `deliberation_limit:<limit>` (`gate.RankReason`, classificado em primeiro),
  de modo que a auditoria, o SARIF e os status de `--pr` são escritos, e o
  único texto publicado é a limitação que diz isso. A transcrição (ferramentas
  oferecidas, solicitadas e não solicitadas, cada chamada com argumentos
  redigidos, duração e resultado resumido, o limite) é impressa em stderr e é o
  campo `deliberation` da auditoria (`render.AuditRecord.Deliberation`).
- Sem um provedor capaz de usar ferramentas (ou com perfis de revisão), os
  scanners adiados rodam como antes. Os decoradores do provedor (contexto do
  repositório em `internal/config`, perfis em `cmd/aurumcode`) repassam
  `llm.ToolCaller` só quando o provedor embrulhado o tem, aplicando o mesmo
  bloco ou prefixo à conversa com ferramentas. Uma revisão que ofereceu ferramentas
  dispensa o cache de modelo por arquivo, e a chave de reaproveitamento de
  veredito incorpora os digests dos resultados das ferramentas.

## Pipeline do gate

`assembleGatePipeline` é o único lugar onde o pipeline é declarado. Os
contribuidores se aplicam nesta ordem:

1. `exceptions`: declara que nenhuma exceção pode corresponder quando a identidade do repositório não é verificada.
2. `verdict-reuse`: reaproveita ou armazena um veredito concluído.
3. `policy-skills`: as seções de skill da política.
4. `scanners`: toda entrada habilitada de `quality_gates.scanners` (`quality_gates.sast` é o alias do semgrep), um `gate.Scan` cada; o contribuidor não nomeia nenhuma engine.
5. `embedded-analysis`: o catálogo de análise embutido.
6. `security-pass`: os achados da passagem `--seguranca`; um achado igual ou acima de `fail_on` conta em todos os modos de `gate.inconclusive` (sob a fonte `analysis`).
7. `analysis-data`: o artefato de dados de análise.
8. `dependency-track`: a submissão do SBOM; pode substituir o filtro de redação.
9. `dependencies`: a verificação de dependências da seção `dependencies` (vulnerabilidade introduzida ou pré-existente, pacote malicioso, licença); sem a seção, não acrescenta nada.

Um contribuidor que retorna um erro comum não aborta e nunca é lido como "sem
achados": o resultado se torna inconclusivo (e falha sob
`gate.inconclusive: block`). Um erro embrulhado com `gate.Fatal` aborta com o
código de saída de configuração.

## Pontos de extensão

- **Um contribuidor do gate.** Implemente `gate.Contributor` (`Name`, `Origin`,
  `Apply`) e acrescente uma linha a `assembleGatePipeline`. `Apply` retorna a
  decisão parcial do contribuidor e o pipeline a mescla no único
  `gate.Result` (`Result.Merge`); não publique de dentro dele.
- **Um scanner.** Acrescente um pacote em `internal/scanner/<engine>` que
  implemente `scanner.Scanner` (`Name`, `Run(ctx, Request) (Report, error)`)
  e registre um `scanner.Engine` (categoria, origem tipada, validador de
  opções) a partir do seu `init`; depois acrescente seu import a
  `internal/scanner/engines`. Nada em `internal/gate`, `internal/config`
  ou `cmd` muda: `quality_gates.scanners: [{engine: <name>}]` o habilita,
  `gate.sources`/`gate.triage` aceitam seu nome e categoria, seus achados
  chegam à linha do gate, à auditoria e ao SARIF com a sua origem, e um
  erro, um binário ausente ou `Complete: false` é inconclusivo. O modelo
  nunca decide se um achado de scanner existe.
- **Uma seção de configuração.** Acrescente o tipo em `internal/config`, sua
  validação e sua precedência entre política central e repositório como
  uma linha de `governedSections` (`internal/config/governance.go`), que
  `config.ApplyCentralPolicy` aplica em ordem. Um campo de `Config`,
  `ReviewConfig` ou `QualityGatesConfig` que nenhuma linha classifica falha em
  um teste reflexivo, de modo que uma seção nova nunca fica sob controle do
  repositório por omissão. Documente-a em `docs/configuration.md`.
- **Um tipo de BOM.** Acrescente uma entrada de catálogo em `internal/xbom/catalog`; a extração
  e a geração leem o catálogo. O SBOM permanece em `internal/sbom`.
- **Uma gramática.** Acrescente uma entrada de catálogo em `internal/grammar/catalog`; nenhuma
  mudança de código Go para uma linguagem que o runtime já suporta.

Veja o guia de extensão, [Estendendo o Aurum](extensao.md), em português,
com o contrato exato e um exemplo mínimo de cada ponto (engine de scanner,
ferramenta de deliberação, skill e fonte de contexto `ContextProvider`) e o
que não é ponto de extensão; o tutorial
[Estendendo o Aurum na prática](tutorials/extensao.md) roda a engine de
exemplo.

## Guardas

Testes estruturais em `cmd/aurumcode` mantêm este documento verdadeiro: nenhuma
função de produção de `cmd`, `internal` ou `pkg` passa de 150 linhas, nenhum
arquivo de produção leva o nome de um card, a tabela de camadas acima é
imposta a cada import, e a lista de pacotes acima é comparada com os pacotes em
disco, de modo que um pacote sem citação aqui falha no teste. `cmd/aurumcode`
também não declara lista de fases nem escada de saída própria: uma slice de
passos de fase, uma fonte de revisão que retorna um código de saída ou um
segundo chamador de `gate.ExitPolicy` falha no teste.

Os mesmos testes impõem a forma de `cmd/aurumcode`: nenhum arquivo de produção
passa de 400 linhas, nenhum comentário de pacote cita número de card (o
histórico vive em `docs/specs/`), e nenhuma função exportada de `cmd`,
`internal/i18n`, `internal/prompt`, `internal/render`, `internal/reviewprofile`
ou `internal/context` recebe parâmetro booleano. Texto de interface por idioma
vive no catálogo de `internal/i18n`: toda chave existe em pt-BR e en com os
mesmos verbos de formatação, toda chave usada em `cmd/aurumcode` e
`internal/render` existe no catálogo, e uma etiqueta de idioma português
escrita como literal nesses arquivos falha no teste. As mensagens de exceção de
`internal/gate` ainda escolhem o idioma no próprio código.
