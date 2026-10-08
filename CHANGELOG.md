# Changelog

## Unreleased

- Parecer da PR redesenhado: a decisão abre o comentário (Aprovado, Aprovado
  com observações, Bloqueado ou Inconclusivo, num bloco de aviso do GitHub),
  seguida de "Corrigir antes do merge", das observações em uma linha cada e do
  resumo; pontos fortes, sugestões, CI, testes e limitações ficam recolhidos em
  "Detalhes da revisão". Um parecer por PR: a rodada seguinte edita o anterior
  e marca como resolvido o comentário de achado que não reencontrou. Só o que
  bloqueia ganha comentário na linha. Todo achado do modelo passa pela
  verificação contra o código (os bloqueantes primeiro); o refutado sai do
  parecer e fica nos detalhes; sem revisão revisada, a verificação avisa uma
  vez só. Achado do modelo que repete um scanner no mesmo trecho, categoria e
  problema é publicado uma vez, sob a regra que o gate reconhece. A lista de
  testes afetados vira uma contagem; prosa repetida entre partes da revisão é
  removida; o aviso de cache do veredito fica só no stderr. O relatório do
  terminal (`--base`) abre com a mesma decisão e a linha de fatos, sem o
  resumo antigo nem o diagrama Mermaid.
- Provedor reserva: `LLM_FALLBACK_<n>_PROVIDER`, `_BASE_URL`, `_API_KEY` e
  `_MODEL` (até 5) declaram provedores tentados em ordem quando o principal
  falha; cada troca aparece no stderr e, se todos falharem, a revisão falha
  fechada nomeando cada erro. O workflow reutilizável aceita duas reservas.
- Status `aurumcode/review` e código de saída do `--pr` seguem o gate declarado:
  achado que o gate não reprova não é contado como grave.
- Site: diagramas Mermaid renderizados localmente; a biblioteca é baixada no
  build com versão e sha256 fixos, sem ser versionada.
- Changelog: `changelog_check.mode: suggest` só sugere a entrada no parecer da
  PR, sem reprovar; `off` e `required` seguem como antes. Docs de changelog e
  de auditoria/SARIF reescritas, mais curtas e com diagrama.
- Revisão: cada achado do modelo que bloquearia o gate passa por uma
  verificação adversarial com o código da revisão revisada; só refutação com
  citação literal o rebaixa a comentário não bloqueante marcado (parecer,
  stderr e `verification` na auditoria). Confirmado, incerto, citação
  inexistente, erro ou teto mantêm o bloqueio. `review.verification`
  (`enabled`, `max_calls`).
- Deliberação: a última rodada permitida não oferece ferramenta e pede o
  parecer com a evidência já reunida, em vez de terminar sem resposta.
- `dependencies.Fails`: limiar de `fail_on` calculado à parte e explícito
  (nível ilegível ou lista vazia falha fechado); `fail_on: [severe]` recusado
  ao ler a configuração.
- Check de changelog: o resumo do job recebe a entrada sugerida (o container
  grava no workspace e o passo anexa ao resumo).
- Autorrevisão do lote: `dependencies.fail_on` com qualquer nível ilegível falha fechado;
  caminhos da API de refs da realimentação montados por função; fixture do
  tutorial de memória sem literal de senha inteiro.
- Documentação de configuração sem variável interna de teste na seção de
  changelog.
- Changelog obrigatório: ao reprovar, o check imprime a entrada sugerida
  (pelo modelo configurado ou pelos assuntos dos commits), redigida, no log,
  no resumo do job e no parecer do review, pronta para colar em Unreleased.
- Pré-verificação do lote: shell do aceite AUR-526 parseável pelo semgrep e
  exemplos do QA de consumidor pinados por SHA (placeholder de 40 hex).
- Dependências do PR: cada dependência alterada é checada no OSV em qualquer
  ecossistema, com gate `dependencies.fail_on`, exceção por CVE e licença,
  pacote malicioso ou typosquat reprovando e varredura agendada
  (`aurumcode dependencies`) publicando SARIF.
- Qualidade do parecer: evidência completa na fusão de perfis, status do CI
  separado da inferência do modelo, rodadas que não repetem comentário e
  consolidação de ruído (`review.presentation.collapse`).
- Contexto: o modelo lê e busca arquivos da revisão (qualquer linguagem),
  fontes MCP configuradas, trechos de impacto e o alcance da parte vulnerável
  de uma dependência.
- Changelog obrigatório: `aurumcode changelog --base <sha>` reprova a PR sem
  entrada útil nesta seção; o repositório liga com `changelog_check.mode: required`.
- Realimentação da política: `aurumcode realimentacao` abre uma única PR no
  repositório da política com propostas citando falsos positivos, achados
  corrigidos e `/aurum perdeu`; nada é aplicado sem merge humano.
- Tutoriais executáveis de changelog obrigatório e de realimentação da
  política, e um mapa que liga cada caso de tutorial ao comando e à evidência.
- QA no repositório consumidor (`tests/consumer`) e roteiro de release
  (`scripts/release.sh`, `docs/releases.md`); nenhuma release foi publicada.

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
