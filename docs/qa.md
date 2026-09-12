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

O histórico de aceitação da reconstrução permanece no board e em `docs/specs`.
Não confunda scripts históricos com a suíte atual de produto.
