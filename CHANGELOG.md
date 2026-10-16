# Changelog

## Unreleased

- Corrigido: rodadas convergem; o modelo lista todos os bloqueios de uma vez, e achado novo em código já revisado e inalterado desde a rodada anterior vira observação (segurança e pendência antiga seguem bloqueando).
- Corrigido: o status aurumcode/review conta problemas distintos e nunca diz "nenhum achado grave" enquanto falha (o mesmo achado de dois analistas é um problema); exemplo do plugin vvah com testes do gerador de perfil.
- Novo: em repositório privado sem Code Security, achados do plugin agendado viram issues do próprio repositório (`plugin issues`, AUR-623): abre, mantém, fecha e reabre por fingerprint; fechada por pessoa fica fechada; público é recusado.
- Corrigido: a triagem mostra o trecho do commit em que o Gitleaks achou o segredo (nunca a linha atual sem relação); needs_context volta uma vez com o arquivo inteiro e, indeciso, aguarda o modelo; alarme contestado sai do parecer.

## 2.1.0 - 2026-10-14

Nenhum limite de tamanho derruba a revisão, e um agente de scan de vulnerabilidade entra como plugin da validação agendada, com achados só na aba Security.

### Entregue

- O modelo decide o veredito final: todo achado estático (segredo, análise, SAST, lint) sem avaliação vai a uma triagem dedicada; contestado não conta. Sem o modelo, os achados aguardam a avaliação e nunca reprovam o gate sozinhos.
- Parecer enxuto: o mesmo problema de duas fontes vira um item; a correção de um bloqueante vem como sugestão aplicável; status curto; sem listas técnicas, sem texto cru de scanner em inglês e com acentos.
- SARIF e auditoria sempre JSON válido; o achado que o modelo contestou sai do SARIF (o alerta da aba Security fecha) e fica só na auditoria, com o motivo do modelo.
- Plugins externos na validação agendada (`scheduled.plugins`): o plugin roda em container próprio e os achados vão só para a aba Security, nunca para log, resumo ou artefato (#185).
- Exemplo pronto do harness agêntico de vulnerabilidade da Visa, fixado por commit e só de descoberta; candidato que o verificador do plugin não decidiu vira alerta "não verificado" (#185).
- Plugin degradado, sem análise ou com cobertura perdida é inconclusivo e não envia SARIF, para não fechar os alertas abertos; entrada da política exige imagem por digest (#185).
- Sem teto de token por padrão; quando o modelo recusa o prompt por tamanho, a revisão se divide em lotes até caber e o terminal narra cada recusa (#179).
- Política grande não derruba mais a revisão: catálogo de regras, texto das skills e arquivos curados do repositório entram inteiros (#179).
- Sem teto de bytes nem de contagem: MCP, scanner, manifestos, OSV, Dependency-Track, verificação, ferramentas, filtro de segredos e commits do changelog entram inteiros.
- Arquivo de texto grande não fica de fora: o diff cai para substituição inteira quando a busca exata não cabe; no `--pr`, arquivo sem linhas na API tem o diff refeito do checkout.
- Contexto de código por relevância (mesma linguagem, só arquivos que alcançam a mudança); depois de uma recusa por tamanho ele é reduzido para caber, e a redução é declarada.
- Recusas de tamanho reconhecidas também no 400 de texto longo demais e no 429 de pedido maior que o limite por minuto; 429 comum espera o `retry-after` e tenta de novo.
- Workflow reutilizável: revisão que não terminou diz o motivo na anotação e no resumo; grupos de concorrência viraram `aurumcode/...`; o SARIF dos exemplos tolera revisão sem artefato (#179).
- Tutoriais: gravação de outra árvore é aviso, e só o tutorial cuja saída muda é regravado; tutorial novo `plugin-agendado`.

### Limites

- Repositório privado sem GitHub Code Security: o job de plugin falha antes de rodar e não publica nada; a sincronização com issues do próprio repositório ainda não existe.
- O gate da PR ainda não avisa sobre alerta sigiloso aberto num arquivo mexido.
- Recusa por tamanho nas rodadas de ferramenta da deliberação conta como do prompt base, e a auditoria da tentativa recusada se perde (falha fechada).
- Renomeação pura para um caminho que executa (como `.github/workflows`) não é mostrada ao modelo, como na 2.0.0.

### Migração do v1

- Da 2.0.0 para a 2.1.0, troque `@v2.0.0` por `@v2.1.0`. Quem quer os tetos antigos os declara em `.aurumcode/config.yml`; o plugin agendado precisa de `security-events: write` no job chamador.

## 2.0.0 - 2026-10-13

Primeira release do AurumCode atual: a revisão de pull request virou um gate
de conformidade que escreve para gente ler, verifica o que o modelo afirma
antes de publicar e falha fechado quando não consegue decidir.

### Entregue

#### Camadas de política e padrões, analistas com ferramentas

- Camada de padrões de engenharia: segundo repositório central (`--padroes`; `standards_repository` no workflow) com skills e analistas abaixo da política; `config.yml` com seção é recusado; parecer e auditoria nomeiam a camada.
- Analistas (`--perfis`/`review.profiles`) deliberam com as mesmas ferramentas da passada única, num teto somado por revisão; o terminal diz o motivo real quando nada é oferecido e a auditoria registra as rodadas por analista.
- Publicação recusada por permissão (401/403): o diagnóstico nomeia a permissão que o token precisa (`pull-requests: write` ou `statuses: write`) e onde declará-la, mantendo a resposta do GitHub.
- Dados de análise publicados como pre-release e cópias com mais de 7 dias apagadas pelo workflow diário (mínimo de 3 mantidas; só `analysis-data/*`); terminal sem "(s)": singular e plural pelo catálogo pt-BR/en.

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

- Demonstração `demo/do-zero` em três repositórios (segurança, engenharia e o serviço): PR bloqueada pelas três camadas, o serviço não afrouxa a segurança, correção aprovada; tutorial executável `camadas`.
- Política central em repositório privado ou interno: secret opcional `AURUMCODE_POLICY_TOKEN` (só leitura) nos workflows de revisão e changelog.
- Tutoriais executáveis por capacidade, com saídas gravadas e conferidas no CI; site com busca e navegação por capacidade; docs de changelog e de auditoria/SARIF reescritas.
- Guias com proteção de branch, secrets do Dependabot, precedência da política central, versões fixadas e como saber que há versão nova.

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
