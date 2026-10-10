# Desenvolvimento e QA

Todo Go roda em container:

```bash
make build
make test
make test-race
make lint
```

O alvo `lint` verifica formatação de todo o repositório; existem fixtures
históricas não formatadas. Confira os arquivos apontados antes de fazer uma
reescrita em massa.

## Site de documentação

O site de documentação é gerado por MkDocs Material em container, numa imagem
fixada por digest (`scripts/docs/build.sh`, com `--strict`). A publicação usa o
workflow `docs.yml`, a partir de `main`, sem branch extra e sem regeneração via
LLM. O guia interativo antigo continua em `docs/site/`, apenas como arquivo.

A verificação de navegador em container cobre desktop, mobile, links locais,
geração de configuração, cópia para clipboard e carregamento do workflow:

```bash
docker run --rm --ipc=host \
  -v "$PWD:/src:ro" -w /src \
  mcr.microsoft.com/playwright:v1.58.2-noble \
  sh -c 'npm install --prefix /tmp/qa playwright@1.58.2 && NODE_PATH=/tmp/qa/node_modules node tests/docs/site.test.cjs'
```

O exemplo baixável em `docs/site/workflow.yml` deve ser igual ao de
`.github/workflows/examples/code-review.yml`.

## Testes do agente

A suíte verifica montagem de prompts, contexto, parsing, filtros de achados e
publicação com respostas controladas. Fixtures validam integração; não medem a
precisão de um modelo real. A avaliação com LLM exige PRs de código e análise dos
resultados entre rodadas.

`TestPRJourneyCarriesConversationAndPublishesDeletionAtBase` percorre três
rodadas pela CLI de review: remoção de proteção, correção com resposta do autor
e mudança em outro arquivo. Exercita os modos `comments` e `review`, captura o
prompt real e os requests de publicação, verifica `LEFT`/numeração antiga e
remoção de um segredo do histórico. O modelo é controlado: o teste prova o
transporte da conversa e da resposta, **não** que um LLM sempre julgará corretamente.

Os testes do cliente verificam paginação, atribuição, cancelamento e falhas sem
histórico parcial apresentado como completo. Links de paginação e redirects
não podem encaminhar o token para outro destino. Os testes de escopo rejeitam
linhas intactas, arquivo alheio e confusão entre numeração antiga e nova.

A próxima avaliação semântica deve repetir PRs rotulados com um modelo real e
registrar falsos positivos, defeitos perdidos, repetição de cobranças e rodadas
até encerramento. Ainda não há resultado medido que justifique afirmar redução
de ruído do modelo em produção.

O histórico de aceitação da reconstrução permanece no board e em `docs/specs`.
Não confunda scripts históricos com a suíte atual de produto.

## QA no repositório consumidor (AUR-512)

O self-review deste repositório não prova o produto instalado. O QA do
consumidor roda o AurumCode, pinado por SHA, num repositório separado, com a
imagem real, PRs de código e falhas observáveis. Os cenários estão em
`tests/consumer/cenarios.json`:

| Cenário | O que prova |
| --- | --- |
| `comments-inline` | Workflow reutilizável, modo `comments`, parecer em português, sugestão inline, gate reprovando; o commit de correção limpa o bloqueio. |
| `review-formal` | O mesmo no modo `review`. |
| `acao-direta` | Action direta pinada, status `aurumcode/review` igual ao resultado. |
| `modelo-ausente`, `modelo-inconclusivo` | Sem credencial ou com o modelo inalcançável a revisão falha fechada, com diagnóstico. |
| `sem-permissao` | Token só de leitura: a publicação falha e o diagnóstico diz permissão. |
| `ci-falhando` | O CI do consumidor falha e o parecer cita a falha concluída. |
| `pr-de-fork` | Manual (precisa de uma segunda conta): sem secrets, falha fechada; a configuração vem da base. Sem evidência fornecida, fica `nao_medido`. |
| `changelog-ausente`, `changelog-valido` | O changelog obrigatório bloqueia a PR sem entrada e passa a entrada válida. |
| `rodada-repetida` | Rodar de novo no mesmo SHA não multiplica os comentários. |

Preparação, uma vez: a `main` do consumidor recebe, por PR, o conteúdo de
`tests/consumer/fixtures/base` (config com `review.language: pt-BR`, gate e
`changelog_check.mode: required`), os secrets `LLM_API_KEY` e `LLM_BASE_URL`
e a variável `LLM_MODEL` (a mesma do repositório do AurumCode). Cadastre os
secrets num terminal interativo: `gh secret set LLM_API_KEY --repo OWNER/CONSUMIDOR`
pede o valor sem mostrá-lo; rodado sem terminal (um atalho de agente, um script
com a entrada fechada), o `gh` grava um valor vazio e o job de revisão para em
"AurumCode sem provedor de modelo" em toda PR. A `main` não pode ter outro
workflow do AurumCode além do que o QA instala por PR (um `code-review.yml`
antigo apareceria na medição como job `review`).

Rodada, no host do dono (git, `gh` autenticado e `jq`; nenhum Go):

```bash
tests/consumer/run.sh --repo OWNER/CONSUMIDOR --sha <SHA completo do AurumCode> --evidencia qa-evidencia
```

O script cria uma branch e uma PR por cenário, instala o workflow do cenário
com o SHA sob teste, commita com a identidade git **já configurada** (nunca
define uma), espera o run, coleta status, checks, comentários, review,
sugestões inline e o log de falha e grava `qa-evidencia/<cenario>.json` com
repo, SHA, id e URL do run. Run que não conclui (billing, fila) vira
`"medido": false` com a limitação. Depois, no container:

```bash
.board/bin/go-shared up
.board/bin/go-shared exec -w "$PWD" env AURUMCODE_QA_EVIDENCIA="$PWD/qa-evidencia" go test ./tests/consumer -count=1 -run TestAUR512
```

O verificador reprova evidência sem repo, SHA ou run, cenário negativo que
passou (um produto sem o gate de changelog ou de qualidade) e rodada
repetida que dobrou comentários; `nao_medido` nunca conta como aprovado. O
site só cita resultado de uma evidência dessas, com repo, SHA e run.
