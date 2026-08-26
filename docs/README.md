---
layout: default
title: Como escrever a documentação
nav_order: 4
---

# Como escrever a documentação

`docs/` é a fonte dos guias escritos por pessoas. O gerador não altera esses
arquivos. No build do GitHub Pages, eles entram no site em `guides/`; a API
extraída fica separada e é reconstruída a cada commit.

No site oficial do AurumCode, entram os guias públicos em `docs/*.md`. As
especificações internas em `docs/specs/` continuam disponíveis no repositório
para manutenção do projeto, mas não são exibidas para quem está usando a
ferramenta.

## O que é automático

| Parte do site | Fonte | Pode ser editada diretamente? |
| --- | --- | --- |
| API | código-fonte + extratores | não; corrija o código ou o extrator |
| Guias | `docs/**/*.md` | sim |
| Landing page | `.aurumcode/index.md` + scaffold | apenas o texto fora do bloco marcado |
| Revisão editorial | README + páginas publicadas + skills | não; ela é regenerada pelo LLM |

## Criando um guia

Adicione um Markdown em `docs/`. Use front matter para controlar o título e a
posição na navegação:

```markdown
---
layout: default
title: Meu guia
nav_order: 5
---

# Meu guia

Explique o caminho que o leitor precisa concluir.
```

Links entre guias devem apontar para a rota publicada, por exemplo
`guides/getting-started.html`, e não para um caminho do checkout como
`docs/getting-started.md`.

## Revisão com LLM

O modo padrão é `auto`: se `llm-api-key` e `llm-base-url` estiverem presentes,
o build cria `reviews/docs-review.md`. O relatório avalia somente o conteúdo do
site e não modifica as páginas de API.

Para personalizar sem preencher um arquivo grande:

- edite `.aurumcode/prompts/documentation/docs-review.md`;
- adicione skills curtas em `.aurumcode/skills/documentation/*.md`;
- selecione `docs-review: required` no workflow quando a revisão for obrigatória.

O prompt e as skills são contexto adicional. Eles não podem autorizar links,
comandos ou fatos que não estejam no material enviado ao modelo.

## Teste local

Para gerar apenas a documentação determinística:

```bash
go run ./cmd/regenerate-docs
```

Para gerar e revisar com um provider OpenAI-compatible:

```bash
LLM_API_KEY=... \
LLM_BASE_URL=https://seu-endpoint/v1 \
AURUMCODE_DOCS_REVIEW=required \
go run ./cmd/regenerate-docs
```

Depois, construa o site:

```bash
cd .aurumcode
bundle install
bundle exec jekyll serve
```

O workflow oficial usa um diretório temporário para publicar API, guias e
revisão sem sujar o checkout com `_site` ou cópias intermediárias.
