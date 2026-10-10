# Changelog

## Unreleased

- Cards AUR-607 a AUR-612 entregues no lote #158, com o aceite selado repetido no SHA do main e a tabela de controle do AUR-589 atualizada.
- Exemplos de senha dos testes da redação montados em tempo de execução, para a passada de segurança do próprio repositório não os ler como credencial (AUR-609).
- A versão da ferramenta nos workflows é resolvida por `scripts/ci/resolve-tool-version.sh`, um só script para os três builds (AUR-611).

## 2.0.0 - 2026-10-09

Primeira release do AurumCode atual: a revisão de pull request virou um gate
de conformidade que escreve para gente ler, verifica o que o modelo afirma
antes de publicar e falha fechado quando não consegue decidir.

### Entregue

#### Parecer da pull request

- A decisão abre o comentário num aviso nativo do GitHub (Aprovado, Aprovado
  com observações, Bloqueado ou Inconclusivo), lida do gate declarado em
  `.aurumcode/config.yml`; depois "Corrigir antes do merge" com um item por
  problema, observações em uma linha, resumo e o resto recolhido em
  "Detalhes da revisão". Sem elogio, sem emoji, no máximo três limitações e
  três testes propostos; testes afetados aparecem contados.
- Um parecer por PR, editado a cada rodada; comentário na linha só para o que
  bloqueia, com sugestão aplicável também no modo `comments`; comentário já
  resolvido é marcado em vez de repetido; eco de scanner publicado uma vez.
- O status `aurumcode/review` e o código de saída do `--pr` seguem o gate
  declarado: achado que o gate não reprova não conta como grave. O `--base`
  local abre com a mesma decisão.
- Com `review.language: pt-BR`, parecer, status do gate, terminal e MCP saem
  em pt-BR (motivo de inconclusivo em frase, com o código entre colchetes);
  auditoria e SARIF não mudam de idioma. Achado sem prova descartado deixa o
  título em "nenhum problema comprovado".
- O parecer, a auditoria e o SARIF mostram a versão do AurumCode que revisou.

#### Análise por time

- Analistas por time (`review.profiles`, `.aurumcode/profiles.yml`) no local, no
  MCP e na revisão da PR: uma passada do modelo por analista (segurança embutido;
  padrões, QA ou outro do time), lida da base; o achado diz quem o encontrou.

#### Confiança no que o modelo diz

- Todo achado do modelo é verificado contra o código revisado, os bloqueantes
  primeiro; refutado com citação literal sai do parecer e fica na auditoria;
  confirmado, incerto, citação inexistente, erro ou teto mantêm o bloqueio
  (`review.verification`: `enabled`, `max_calls`).
- A última rodada da deliberação pede o parecer com a evidência já reunida em
  vez de terminar sem resposta; `ignore` limita também o escopo dos scanners;
  regra de segurança com forma de código (SQL, XSS, injeção de comando) não
  olha `.txt`/`.log`/`.md`; achado da passagem de segurança já traz a
  correção da regra.
- `gate.triage` vale `model`: o modelo pesa a evidência das regras, contestar
  exige justificativa e resposta degradada nunca rebaixa. A redação antes do
  prompt não mutila código; o catálogo não dispara em literal nem em prosa.
- Contexto: o modelo lê e busca arquivos da revisão em qualquer linguagem,
  fontes MCP configuradas, trechos de impacto e o alcance da parte vulnerável
  de uma dependência; status do CI separado da inferência do modelo.

#### Provedores de modelo

- Provedor reserva: `LLM_FALLBACK_<n>_PROVIDER|_BASE_URL|_API_KEY|_MODEL`
  (até 5), tentados em ordem quando o principal falha, com a troca anunciada;
  todos falhando, a revisão falha fechada. O workflow reutilizável aceita duas
  reservas (`fallback_1_*`, `fallback_2_*`).

#### Dependências

- Cada dependência alterada é checada no OSV em qualquer ecossistema, com gate
  `dependencies.fail_on`, exceção por CVE e licença; pacote malicioso ou
  typosquat reprova; `aurumcode dependencies` agendado publica SARIF. Nível
  de `fail_on` ilegível ou lista vazia falha fechado.

#### Changelog obrigatório

- `aurumcode changelog --base <sha>` reprova a PR sem entrada útil em
  `Unreleased` (`changelog_check.mode: required`; `suggest` só sugere); ao
  reprovar, a entrada sugerida vai ao log, ao resumo do job e ao parecer.
  PR aberta por bot não reprova (`changelog_check.bots`, padrão `suggest`).

#### Realimentação da política

- `aurumcode realimentacao` abre uma única PR no repositório da política com
  propostas citando falsos positivos, achados corrigidos e `/aurum perdeu`;
  nada é aplicado sem merge humano.

#### Documentação e operação

- Demonstração `demo/do-zero`: projeto novo com o AurumCode configurado por um
  agente de IA (Aurum como servidor MCP) ou por roteiro; PR bloqueada pela
  ferramenta e pela regra da segurança, aprovada após a correção, com três
  analistas; local sem rede ou no GitHub real. Guia "Instalar com ajuda da IA".
- Política central em repositório privado ou interno: secret opcional `AURUMCODE_POLICY_TOKEN`
  (só leitura no repositório da política) nos workflows de revisão e changelog.
- Tutoriais executáveis por capacidade, com saídas gravadas e conferidas no
  CI; site com busca, navegação por capacidade e diagramas renderizados
  localmente; docs de changelog e de auditoria/SARIF reescritas.
- Guias com proteção de branch, secrets do Dependabot, precedência da política
  central, versões fixadas das ferramentas e como saber que há versão nova.

### Limites

- Sem benchmark com modelos reais: a qualidade do parecer foi conferida em
  casos gravados, não medida em escala; a taxa de pareceres inconclusivos
  também não é medida.
- Custo por revisão não é medido automaticamente e não há guia de qual
  modelo basta para cada tipo de repositório.
- Sem chave de modelo, o gate local roda só a parte determinística (segredos,
  SAST, vet, changelog) e termina em `quality_skipped`.
- xBOM: CBOM e Build BOM existem, sem AIBOM; xBOM não é critério do gate.
- O parecer não avisa quando existe versão mais nova: quem fixa `@v2.0.0`
  acompanha as releases do repositório.

### Migração do v1

- As tags `v1.x` são do produto anterior e não recebem este conteúdo. No
  workflow do repositório, troque a referência de
  `Mpaape/AurumCode/.github/workflows/review.yml` (`@main` ou `@v1...`) por
  `@v2.0.0`; `@main` recebe toda mudança sem aviso.
- Secrets de Actions `LLM_API_KEY` e `LLM_BASE_URL` (serviço compatível com
  OpenAI); opcionais a variável `LLM_MODEL`, o input `fallback_1_provider` e
  os secrets `LLM_FALLBACK_1_API_KEY` e `LLM_FALLBACK_1_BASE_URL`.
- `.aurumcode/config.yml` na branch base declara o gate (`gate.fail_on`,
  `gate.inconclusive`, `quality_gates.scanners`, `dependencies.fail_on`,
  `changelog_check.mode`) e as skills de convenção em `.aurumcode/skills/`;
  sem o arquivo, o review só orienta.
- Não existem mais o gerador de docs de terceiros, os extratores, o runtime
  Jekyll e o resumo Mermaid do `--base`.

## Reconstrução focada em code review

- Um CLI Go para revisão local e de pull requests.
- Prompts, skills, docs de domínio, idioma e publicação configuráveis.
- Modelo definido pelo endpoint ou pelo operador.
- Filtros de localização e estrutura dos achados.
- Comentários, reviews formais e sugestões de código no GitHub.
- Nova documentação interativa, publicada de `main`.
- Removidos o gerador de docs de terceiros, extratores, runtime Jekyll e QA de
  páginas geradas que já não fazem parte do produto.
- Corrigidos o uso de Git em volumes Docker, a propagação do SHA da base na
  Action e o descarte indevido dos estados de CI com falha.

Instalações novas usam `main`. Tags históricas permanecem no histórico e não
representam automaticamente o conteúdo atual da branch.
Os detalhes das versões anteriores estão no histórico Git e nas especificações
de reconstrução, sem validade como guia de configuração atual.
