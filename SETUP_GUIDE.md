# Guia de configuração

Como preparar um repositório para gerar e publicar a documentação com a Action
do AurumCode.

## 1. Configurar o GitHub Pages

A Action publica pelo fluxo de deploy do próprio GitHub Actions, não por um
branch `gh-pages`.

1. Abra Settings > Pages do repositório.
2. Em "Build and deployment", defina **Source** como **GitHub Actions**.

Sem esse ajuste, `publish: 'pages'` falha no passo de deploy. Detalhes em
[PAGES_SETUP.md](PAGES_SETUP.md).

## 2. Secrets do endpoint LLM (opcional)

A geração de documentação funciona sem nenhuma credencial: a chave só muda o
texto da página inicial (`index.md`). Se quiser a página escrita por LLM,
cadastre em Settings > Secrets and variables > Actions:

| Secret sugerido | Conteúdo |
|-----------------|----------|
| `LLM_API_KEY` | chave de um endpoint compatível com a API da OpenAI |
| `LLM_BASE_URL` | URL base desse endpoint |

Os nomes dos secrets são livres; o que importa é para quais inputs eles são
passados (`llm-api-key` e `llm-base-url`). Ambos são obrigatórios juntos: com
apenas um deles a Action registra um aviso e segue sem a página escrita por LLM.

## 3. Workflow

```yaml
name: Documentation

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  docs:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pages: write
      id-token: write
    environment:
      name: github-pages
      url: ${{ steps.docs.outputs.page-url }}
    steps:
      - uses: actions/checkout@v4

      - id: docs
        uses: Mpaape/AurumCode@main
        with:
          source-dir: '.'
          output-dir: '.aurumcode'
          publish: 'pages'
          llm-api-key: ${{ secrets.LLM_API_KEY }}
          llm-base-url: ${{ secrets.LLM_BASE_URL }}
```

As permissões precisam ficar no job: uma composite action não declara
`permissions`. A lista completa de inputs está em
[ACTION_USAGE.md](ACTION_USAGE.md).

## 4. O que é gerado

Para `output-dir: '.aurumcode'`:

| Caminho | Conteúdo |
|---------|----------|
| `.aurumcode/_config.yml` | configuração mínima do Jekyll; criada só se ainda não existir |
| `.aurumcode/index.md` | página inicial, com link para cada página gerada |
| `.aurumcode/<linguagem>/<nome>.md` | uma página por unidade documentada |

As linguagens efetivamente extraídas dependem das ferramentas presentes no
runner e do input `extra-toolchains`. Rust e C# não são extraídos pela Action;
o motivo está em [ACTION_USAGE.md](ACTION_USAGE.md).

## 5. Testar localmente (opcional)

```bash
go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0

# Opcional, só para a página inicial escrita por LLM
export LLM_API_KEY=sua_chave
export LLM_BASE_URL=https://seu-endpoint/v1

go run ./cmd/regenerate-docs
```

O argumento precisa ser o diretório (`./cmd/regenerate-docs`): o `package main`
do comando está dividido em mais de um arquivo, então apontar para um arquivo
isolado não compila.

## Solução de problemas

### O passo de geração falha ao resolver dependências

Confirme que `go.mod` e `go.sum` estão commitados e rode `go mod tidy`.

### A publicação para com "missing index.md/_config.yml"

A Action se recusa a publicar uma árvore que não é um site. Isso significa que a
geração não produziu nada: confira o output `docs-generated` e as linhas
`[Pipeline] Extracting ... documentation` no log.

### A página inicial não usa o LLM

`llm-api-key` e `llm-base-url` precisam estar os dois preenchidos. Com apenas um
deles, o provider é ignorado e o log registra um aviso; `llm-model` é ignorado
quando qualquer um dos dois falta.

### O deploy falha em Settings > Pages

O Source precisa ser "GitHub Actions". Um repositório ainda apontando para um
branch recusa o artefato publicado pela Action.

## Checklist

- [ ] Pages com Source = "GitHub Actions"
- [ ] Job com `permissions` de `pages: write` e `id-token: write`
- [ ] Secrets do endpoint LLM cadastrados (apenas se quiser a página por LLM)
- [ ] Workflow executado e `docs-generated` maior que zero

Dúvidas: https://github.com/Mpaape/AurumCode/issues
