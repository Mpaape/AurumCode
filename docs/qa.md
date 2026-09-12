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

O site fica em `docs/site/`, com HTML, CSS e JavaScript sem dependências de
runtime. A publicação usa o workflow `pages.yml`, a partir de `main`, sem
branch extra e sem regeneração via LLM.

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
