# Changelog

## Unreleased

- Changelog obrigatório: ao reprovar, o check imprime a entrada sugerida
  (pelo modelo configurado ou pelos assuntos dos commits), redigida, no log,
  no resumo do job e no parecer do review, pronta para colar em Unreleased.
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
