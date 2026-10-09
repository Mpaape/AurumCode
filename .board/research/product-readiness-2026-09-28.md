# Auditoria de casos de uso — 2026-09-28

Base: `a73ea97344ca7c7f63ed05a6464538cfac178095`.
Pesquisa solicitada com modelos Luna e Sol; quatro escopos independentes:
documentacao/ergonomia, motor de review, entrega/release e referencias externas.
O coordenador conferiu os pontos de codigo abaixo antes de planejar correcoes.

## Evidencia e limites da auditoria

Leitura de codigo demonstra caminhos e contratos; nao substitui reproduzir o
defeito. Esta auditoria nao rodou benchmark semantico nem instalou concorrentes.
O pipeline do board passou, a branch era unica e o HEAD estava sincronizado.

| Achado | Evidencia na base | Classificacao | Card |
| --- | --- | --- | --- |
| Site instrui caminho de config ignorado pelo produto | docs/site/index.html:349/356; app.js:12; internal/config/config.go:252 | Divergencia confirmada entre artefatos | AUR-506 |
| Action direta nao exige review LLM | scripts/action-entrypoint.sh:82; reutilizavel review.yml:142; pr.go:691 | Fluxo confirmado por leitura; falta regressao do wrapper | AUR-507 |
| Changelog nao bloqueia merge | action.yml:34; review.yml:26; pr.go:246; aur499_test.go:92 | Sugestao opcional e ausencia de gate confirmadas | AUR-509 |
| Release atual nao representa o HEAD | refs remotas v1/v1.1.0 em 8e33c69; main 156 commits adiante | Consulta GitHub/Git confirmada | AUR-510 |
| Cache nao inclui prompt/perfil efetivo | main.go:803; review_cache.go:103 | Composicao da chave confirmada; reproduzir duas rodadas | AUR-513 |
| Cache retira arquivos antes da analise conjunta | review_cache.go:113; main.go:803 | Operacao confirmada; perda concreta de bug cross-file ainda e hipotese | AUR-513 |
| Fusao de perfis perde evidencia e outros campos | profiles.go:113/141 | Conversoes confirmadas por leitura | AUR-514 |
| Contexto de PR usa cwd sem vinculo demonstrado ao alvo | pr.go:268/1507 | Origem confirmada; testar dois repositorios distintos | AUR-515 |
| Texto do modelo pode aparecer como CI sem check fornecido | pr.go:1137/1348 | Contrato requer teste de fonte ausente | AUR-516 |
| Resumo pode manter acusacao de finding rejeitado | reviewer.go:203; pr.go:1283 | Caminho confirmado; reproduzir contradicao com diff substantivo | AUR-517 |
| Docs reconhecem ausencia de dedup; exemplo repete SQL no mesmo local | docs/review-quality.md:76; docs/site/index.html:180 | Limitacao/exemplo confirmados; nao e metrica de precisao | AUR-454/AUR-494 |
| Go profile nao e carregavel pelo runner atual | go-unit-offline-v1.json usa schema proprio e valores compostos; oci-run:418/426 exige formato plano | Incompatibilidade confirmada; imagem pinada tem Bash, Go e cache offline | AUR-508 |

O auditador de entrega executou o aceite shell AUR-493 em modo all: falhou em
`missing-v2`, porque o teste historico exige v2 e os exemplos atuais usam main.
Nao foi um teste do modelo nem do produto instalado. Corrigir contrato atual
em card novo sem alterar retroativamente a evidencia do card done.

## Jornadas de consumidor

Onboarding requer endpoint e secrets do repo hospedeiro; isso deve ser explicado
antes do primeiro PR. Personalizacao e opcional. O preset portugues/review/inline
do configurador pode existir, desde que nao seja anunciado como default do motor.
O guia precisa distinguir commits de mudancas nao commitadas e disponibilizar
exemplos Docker completos. Skills sao arquivos Markdown de contexto; a existencia
de um seletor de SKILL.md separado nao demonstra que ele esta conectado ao CLI.

Review de fork, diagnostico de CI e memoria persistente precisam de contratos
explicitos. Um status sem logs nao prova causa; feedback/contexto nao e autoridade
para alterar permissoes ou desabilitar controles. O plano detalhado e o board
ficam em [PRODUCT_PLAN.md](../PRODUCT_PLAN.md).

## Pesquisa externa: o que comparar

Fontes primarias consultadas em 2026-09-28:

- [Alibaba OCR](https://github.com/alibaba/open-code-review) e
  [AACR-Bench](https://github.com/alibaba/aacr-bench), incluindo
  [metricas](https://github.com/alibaba/aacr-bench/blob/main/docs/metrics.md).
  Comparar pipelines sob o mesmo modelo e diferente de comparar produtos
  hospedados; claims do mantenedor nao substituem nossa medicao.
- [CodeRabbit](https://docs.coderabbit.ai/guides/code-review-overview): testar
  configuracao e reducao de ruido, sem inferir qualidade apenas da lista de features.
- [Greptile benchmark](https://www.greptile.com/benchmarks): o ensaio publicado
  conta deteccao de bugs conhecidos e nao avalia falso positivo na taxa de acerto.
- [Copilot code review](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/request-a-code-review/use-code-review)
  e [customizacao](https://docs.github.com/en/copilot/tutorials/customize-code-review):
  testar instrucoes, sugestoes e rodadas sucessivas, sem presumir obediencia perfeita.
- [Martian code-review benchmark](https://github.com/withmartian/code-review-benchmark):
  considerar tamanho do corpus, juiz LLM e comportamento posterior do dev como
  proxies com limitacoes, nao verdade absoluta.
- [GitHub secrets](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)
  e [pull_request_target](https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target):
  suporte a forks precisa preservar a separacao entre codigo proposto e credenciais.

Protocolo proposto: negativos e defeitos reais, ground truth congelado, adjudicacao
cega, rodadas original/repetida/corrigida, findings ancorados e acionaveis,
precisao/recall por severidade, repeticao, tempo humano, latencia e custo.
Qwen local e endpoint do piloto; nenhum resultado comparativo foi medido nesta pesquisa.
