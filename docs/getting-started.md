---
layout: default
title: Getting Started em 10 minutos
nav_order: 2
---

# AurumCode em 10 minutos por feature

Este guia leva alguém que nunca viu o AurumCode a rodar as três features do
produto. Cada comando e cada saída abaixo foram executados de verdade; nada
aqui é ilustrativo. Quando uma coisa não funciona sem pré-requisito externo, o
guia diz isso em vez de esconder.

O AurumCode tem exatamente três features:

1. **Code review de um diff** — `aurumcode review`
2. **Geração de documentação** — `aurumcode docs`
3. **Publicação das páginas** — via GitHub Actions e GitHub Pages

## Antes de tudo: construir o binário

Pré-requisitos medidos nesta máquina: Go 1.22.5 (o módulo declara `go 1.21`) e
git 2.43.0. Nenhuma chave de LLM, nenhum serviço, nenhuma rede.

```bash
git clone https://github.com/Mpaape/AurumCode.git
cd AurumCode
go build -o aurumcode ./cmd/aurumcode
export PATH="$PWD:$PATH"
```

O `export PATH` não é decoração: o resto deste guia roda os comandos de dentro
de um repositório de teste em `/tmp`, e sem ele o shell não acha o binário.
Se preferir, use o caminho absoluto do binário em cada invocação.

O build leva menos de um segundo depois do primeiro download de dependências.
O binário resultante embute o catálogo de regras de segurança, então ele
funciona a partir de qualquer diretório.

Este repositório também produz um segundo binário, `./cmd/regenerate-docs`, que
é a versão do gerador de documentação dirigida por variáveis de ambiente. O
subcomando `aurumcode docs` usa exatamente os mesmos pacotes internos, dirigido
por flags. Para começar, use `aurumcode docs`.

Se você só quer ver as três features rodarem sem ler mais nada, o repositório
tem um roteiro pronto:

```bash
./demo.sh
```

Ele constrói os dois binários, gera documentação, roda a passagem de segurança
e roda uma review com resposta de modelo determinística — tudo offline, sem
chave. Ele termina com `exit 0` e a linha
`All three features ran offline, with no LLM key.`

## Feature 1 — seu primeiro code review em 2 minutos

Este é o melhor primeiro comando do produto porque **não precisa de credencial
nenhuma**. A passagem de segurança do AurumCode é determinística: ela casa as
linhas *adicionadas* do diff contra um catálogo de regras embutido no binário.
Sem modelo, sem rede, sem custo.

Monte um repositório descartável para ver o comportamento real:

```bash
mkdir /tmp/aurum-demo && cd /tmp/aurum-demo
git init -q -b main .
git config user.email demo@example.com
git config user.name "Demo"

mkdir app
cat > app/store.go <<'EOF'
package app

import "database/sql"

func FindUser(db *sql.DB, id string) (*sql.Rows, error) {
	return db.Query("SELECT * FROM users WHERE id = 1")
}
EOF
git add -A && git commit -q -m "base"
```

Agora introduza, em um commit, os três problemas que o catálogo sabe detectar:

```bash
cat > app/store.go <<'EOF'
package app

import "database/sql"

const API_KEY = "sk-demo-9f2a41c7b8"

func FindUser(db *sql.DB, id string) (*sql.Rows, error) {
	return db.Query("SELECT * FROM users WHERE id = " + id)
}
EOF
cat > app/backup.py <<'EOF'
import subprocess

def backup(name):
    subprocess.run("tar czf backup.tgz " + name, shell=True)
EOF
git add -A && git commit -q -m "add user lookup and backup"
```

Rode a review:

```bash
aurumcode review --base HEAD~1 --seguranca
```

Saída real, em duas partes. Primeiro, no **stderr**, dois avisos:

```
aurumcode review: no LLM provider configured: quality review skipped, running --seguranca only (...)
aurumcode review: security pass applied 4 of 8 security rules (security/command-injection, security/hardcoded-secret, security/sql-injection, security/xss); see internal/review/rules/security.yml for the full catalog
```

O primeiro aviso é importante e honesto: **sem provider de LLM, a review de
qualidade não roda**. O que você recebe é só a passagem de segurança. O
comando não finge que revisou o que não revisou.

Depois, no **stdout**, os achados:

```
Security findings (standards/security-review):
app/backup.py:4: [error] Potential command injection vulnerability [standards/security-review SCR-001] (rule security/command-injection: Command Injection)
app/store.go:5: [error] Hardcoded credentials or API keys detected [standards/security-review SCR-003] (rule security/hardcoded-secret: Hardcoded Secrets)
app/store.go:8: [error] Potential SQL injection vulnerability detected [standards/security-review SCR-001] (rule security/sql-injection: SQL Injection Vulnerability)
```

O código de saída é `0`: por padrão, achados não mudam o exit code.

Se o diff não tiver nada, a seção aparece assim, e o exit continua `0`:

```
Security findings (standards/security-review):
No security findings.
```

Se você esquecer `--base`, o comando recusa em vez de adivinhar:

```
$ aurumcode review
aurumcode review: --base is required
```

Exit code `2`.

As flags aceitam tanto `-base` quanto `--base`; use a forma que preferir.

### Duas coisas que confundem quem testa pela primeira vez

**A review é escopada ao diff.** Um defeito que já existia na base *não* é
reportado. No repositório acima, se você adicionar um terceiro commit e rodar
`aurumcode review --base HEAD~1 --seguranca`, só o que aquele último commit
adicionou aparece — o segredo e a SQL injection que continuam no arquivo ficam
de fora, porque não são linhas adicionadas neste diff. Isso é intencional: a
ferramenta revisa a mudança, não o repositório. Se você quer ver um achado
antigo, aponte `--base` para antes de ele ter entrado.

**Só 4 das 8 regras do catálogo têm matcher hoje.** O binário embute oito
regras de segurança, mas apenas três carregam um padrão que a passagem
determinística consegue aplicar:

| Regra | Tem matcher? |
|---|---|
| `security/sql-injection` | sim |
| `security/command-injection` | sim |
| `security/hardcoded-secret` | sim |
| `security/xss` | não |
| `security/path-traversal` | não |
| `security/weak-crypto` | não |
| `security/insecure-random` | não |
| `security/missing-auth` | não |

As cinco restantes existem como metadados: um modelo pode citá-las num achado
de review de qualidade, mas a passagem `--seguranca` nunca as dispara sozinha.
Essa é uma decisão registrada, não um bug — XSS, path traversal e autenticação
ausente dependem de fluxo de dados que uma linha isolada do diff não mostra.
O próprio comando imprime `applied 4 of 8` justamente para você não achar que
teve cobertura de oito regras.

Os três matchers também são estreitos. `sql-injection` e `command-injection`
exigem a forma clássica `"..." + variável`; `command-injection` casa
`system(`, `popen(`, `execl(`/`execv(` e `subprocess.run/call/Popen(`, e **não**
casa `exec.Command(` do Go. Um `exec.Command("sh", "-c", "tar czf x " + name)`
passa despercebido pela passagem determinística.

### Barrar o merge quando há achado grave

`--fail-on` transforma achado em exit code. Com `error`, o comando sai `3`:

```bash
$ aurumcode review --base HEAD~1 --seguranca --fail-on error
$ echo $?
3
```

É assim que se barra um merge no CI: o job falha e o pull request não passa.
Os valores aceitos são `high|error`, `medium|warning` e `low|info`.

## Feature 1b — review com modelo

A review de qualidade (a que lê o código e escreve prosa sobre ele) precisa de
um provider. O AurumCode é agnóstico de provider: qualquer endpoint compatível
com a API da OpenAI serve, inclusive um modelo local rodando na sua máquina.

### Offline, com fixture

Para testar o caminho de modelo sem chamar nada, aponte
`AURUMCODE_LLM_FIXTURE` para um JSON com a resposta:

```bash
cat > /tmp/aurum-demo/fixture.json <<'EOF'
{
  "issues": [
    {
      "file": "app/store.go",
      "line": 8,
      "severity": "error",
      "rule_id": "security/sql-injection",
      "message": "FindUser concatena o parametro id direto na query.",
      "suggestion": "Use db.Query(\"SELECT * FROM users WHERE id = ?\", id)."
    }
  ],
  "summary": "A mudanca introduz uma query montada por concatenacao."
}
EOF

AURUMCODE_LLM_FIXTURE=/tmp/aurum-demo/fixture.json \
  aurumcode review --base HEAD~1
```

Saída real:

```
app/store.go:8: [error] FindUser concatena o parametro id direto na query. (rule security/sql-injection: SQL Injection Vulnerability)
```

Note o formato: sem a seção `Security findings`, porque `--seguranca` não foi
passada. As duas passagens são independentes e podem ser combinadas na mesma
invocação — os achados do modelo saem primeiro, a seção determinística depois:

```
$ AURUMCODE_LLM_FIXTURE=$PWD/fixture.json aurumcode review --base HEAD~1 --seguranca
app/store.go:8: [error] FindUser concatena o parametro id direto na query. (rule security/sql-injection: SQL Injection Vulnerability)

Security findings (standards/security-review):
app/backup.py:4: [error] Potential command injection vulnerability [standards/security-review SCR-001] (rule security/command-injection: Command Injection)
app/store.go:5: [error] Hardcoded credentials or API keys detected [standards/security-review SCR-003] (rule security/hardcoded-secret: Hardcoded Secrets)
app/store.go:8: [error] Potential SQL injection vulnerability detected [standards/security-review SCR-001] (rule security/sql-injection: SQL Injection Vulnerability)
```

O mesmo defeito aparece duas vezes quando as duas passagens o encontram: o
AurumCode não deduplica entre elas.

O campo `rule_id` **não é opcional**: um achado cujo `rule_id` está ausente ou
não existe no catálogo embutido é descartado silenciosamente e nunca aparece.
Se sua fixture "não produz nada", esse é o primeiro lugar para olhar. O arquivo
`tests/fixtures/review/known-problem-response.json`, neste repositório, é um
exemplo funcionando.

### Ao vivo, com um provider

```bash
export LLM_API_KEY=...          # a chave do seu provider
export LLM_BASE_URL=...         # o endpoint compatível com OpenAI
export LLM_MODEL=...            # opcional; também dá para usar --modelo

aurumcode review --base HEAD~1 --seguranca
```

Use `--modelo` para escolher o modelo por invocação e `--limite` para pôr um
teto em dólares: segundo `aurumcode review --help`, o comando estima o custo
*antes* de chamar o modelo e recusa a chamada — sem gastar nada — se a
estimativa passar do valor. Essas duas flags só têm efeito no caminho com
provider, e por isso não foram exercitadas na máquina em que este guia foi
escrito; o que está documentado aqui é o texto da própria ajuda do comando.

Escolha o endpoint que você quiser: um provider comercial, um gateway próprio
ou um servidor local. O guia não recomenda nenhum em particular.

## Feature 2 — documentação

`aurumcode docs` varre uma árvore de código e escreve markdown pronto para
Jekyll. No repositório de demonstração acima:

```bash
cd /tmp/aurum-demo
aurumcode docs --source . --output ./site
```

Saída real:

```
Generated 1 documentation page(s) in ./site:
  - site/go/app.md
aurumcode docs: partial run: 0 extraction error(s), 1 language(s) skipped
```

Exit `0`. A árvore produzida:

```
site/_config.yml
site/go/app.md
site/index.md
```

`site/index.md` já vem com front matter, o título derivado do nome do
diretório e um índice das páginas geradas, delimitado por marcadores
`<!-- aurumcode:pages:start -->` e `<!-- aurumcode:pages:end -->`. `_config.yml`
é uma configuração mínima de Jekyll (tema `jekyll-theme-primer`, markdown
`kramdown`) e — pelo comentário que o próprio arquivo carrega — só é criada
quando ainda não existe: editá-la é seguro, ela não é sobrescrita.

As flags são três:

- `--source` — diretório varrido (padrão `.`)
- `--output` — diretório de saída (padrão `.aurumcode`)
- `--languages` — filtro por linguagem, separado por vírgula, em minúsculas

### O que "1 language(s) skipped" quer dizer

Go é extraído em processo, com `go/parser` e `go/doc`: não precisa de nada
instalado. As outras linguagens dependem de ferramenta externa. No exemplo
acima o arquivo Python foi pulado, e o comando diz por quê quando você o
isola:

```
$ aurumcode docs --source . --output ./site3 --languages python
aurumcode docs: documentation extraction produced no documentation: 1 source files, 0 processed, 0 docs generated, 0 error(s), 1 language(s) skipped {skipped: python: required tool not in PATH (pydoc-markdown not found: please install with 'pip install pydoc-markdown') [1 file(s)]}
```

Exit `1`. Ou seja: um `partial run` no meio de uma execução maior é um aviso,
mas uma execução que não gerou nada falha de verdade em vez de sair verde.

Restringir a Go deixa a saída limpa, sem o aviso de linguagem pulada:

```
$ aurumcode docs --source . --output ./site2 --languages go
Generated 1 documentation page(s) in ./site2:
  - site2/go/app.md
```

Um nome de linguagem que o AurumCode não conhece não casa com nada, e o
comando falha em vez de gerar um site vazio:

```
$ aurumcode docs --source . --output ./site4 --languages ruby
No documentation pages were generated in ./site4.
aurumcode docs: no documentation page was generated
```

Exit `1`.

## Feature 3 — publicar as páginas

O site gerado por `aurumcode docs` é uma árvore Jekyll. Publicá-lo é feito pela
GitHub Action deste repositório, que gera a documentação e faz o deploy para o
GitHub Pages no mesmo job.

**Aviso de honestidade:** os passos desta seção descrevem workflows que vivem
neste repositório e a configuração que o GitHub exige. Diferente das duas
seções anteriores, eles não foram executados na máquina em que este guia foi
escrito — um deploy de Pages só acontece dentro do GitHub Actions. O exemplo
ao vivo abaixo é a evidência de que funcionam.

### Configuração obrigatória, uma vez

Em **Settings > Pages** do seu repositório, mude **Source** para
**GitHub Actions**. A action publica via `actions/upload-pages-artifact` e
`actions/deploy-pages`; um repositório cuja fonte do Pages ainda é um branch
rejeita esse artefato. Detalhes em [PAGES_SETUP.md](https://github.com/Mpaape/AurumCode/blob/main/PAGES_SETUP.md).

### O workflow

O menor workflow que gera e publica:

```yaml
name: Documentation

on:
  push:
    branches: [main]

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
        uses: Mpaape/AurumCode@v1
        with:
          source-dir: '.'
          output-dir: '.aurumcode'
          publish: 'pages'
          # Optional: add the two secrets to generate an editorial review page.
          docs-review: 'auto'
```

`publish` é o que faz a action construir e subir o site; o padrão, `none`, só
gera arquivos. As permissões `pages: write` e `id-token: write` têm de estar no
job, porque uma composite action não pode declarar permissões por conta
própria.

A action expõe quatro saídas: `docs-generated` (quantidade de markdown gerado),
`languages-detected`, `docs-path` e `page-url` — esta última fica **vazia** a
menos que `publish` tenha sido `pages`, o que é uma forma direta de checar se a
publicação realmente aconteceu.

Uma versão completa e comentada, com actions fixadas por commit imutável e
`concurrency` configurada para não cancelar um deploy pela metade, está em
[workflow de documentação de exemplo](https://github.com/Mpaape/AurumCode/blob/main/.github/workflows/examples/documentation.yml).
Copie para `.github/workflows/documentation.yml` no seu repositório.

Quando `LLM_API_KEY` e `LLM_BASE_URL` estão configurados como secrets, o mesmo
build também cria `reviews/docs-review.md`. Para tornar a revisão obrigatória,
troque `docs-review: 'auto'` por `docs-review: 'required'`; sem credenciais ou
sem resposta válida, o Pages não é publicado.

## Juntando tudo: review automática em cada pull request

Copie
[workflow de code review de exemplo](https://github.com/Mpaape/AurumCode/blob/main/.github/workflows/examples/code-review.yml)
para `.github/workflows/aurumcode.yml` no seu repositório. O arquivo contém
somente o gatilho, as permissões e a chamada do workflow oficial. Ele não pede
PR number, SHA, checkout, Go, token, comandos internos ou montagem manual de
comentário.

Configure apenas os secrets do provedor no repositório que hospeda o código, em
**Settings > Secrets and variables > Actions**: `LLM_API_KEY` e `LLM_BASE_URL`.
Eles são encaminhados explicitamente para o workflow reutilizável; não ficam
armazenados no AurumCode. O modelo padrão do code review é `gpt-5.6-terra`; só configure outro
se precisar. Sem esses secrets o job falha claramente e não publica um falso
review verde. O workflow
usa `pull_request`, nunca `pull_request_target`, e não compila nem executa o
código do pull request; ele lê a mudança pelo diff do GitHub.

Para definir o idioma do comentário sem tocar no workflow, crie
`.aurumcode/config.yml` no repositório:

```yaml
review:
  language: pt-BR
  publication: review
  inline_comments: true
  context:
    # O prompt embutido continua ativo; adicione apenas contexto relevante.
    # prompt: .aurumcode/prompt.md
    # skills:
    #   - .aurumcode/skills/code-review.md
    # docs:
    #   - docs/architecture.md
```

Sem esse arquivo, o padrão é `en-US`, `comments` e sem comentários inline. Os
campos de publicação e idioma são preferências de apresentação; não podem
desligar regras, alterar gates, remover redação ou ampliar permissões. O bloco
`review.context` é opcional: `prompt` define um arquivo adicional,
`skills` lista orientações focadas e `docs` lista documentação de apoio. Esses
arquivos apenas enriquecem o contexto do modelo; o prompt de code review do
AurumCode continua sendo aplicado. No workflow oficial, `publication` e
`inline_comments` são lidos deste arquivo, e o contexto vem da branch base do
pull request.

O review formal publicado contém veredito, pontos fortes, achados acionáveis,
sugestões não bloqueantes, plano de testes, limitações e o diagnóstico de CI
disponível. Sugestões de código localizadas podem aparecer como blocos nativos
aplicáveis diretamente no GitHub. Logs e transcript ficam apenas no log da
Action. Quando o GitHub ainda não expôs evidência suficiente para uma falha de
CI, o comentário declara essa limitação em vez de inventar uma causa.

Há também [o workflow completo](https://github.com/Mpaape/AurumCode/blob/main/.github/workflows/examples/all-pipelines.yml)
e [o workflow de QA](https://github.com/Mpaape/AurumCode/blob/main/.github/workflows/examples/qa-testing.yml)
no mesmo diretório.

Para bloquear o merge em vez de só comentar, o workflow oficial publica também
o status `aurumcode/review`; configure esse status como required check nas
regras de branch. O job precisa somente de `pull-requests: write` para os
comentários e `statuses: write` para esse status — não precisa de
`contents: write`.

### Exemplo ao vivo

O repositório <https://github.com/Mpaape/aurumcode-demo> roda essa
configuração de verdade; o pull request #1 mostra a action comentando a review
e barrando o merge. Use-o como referência copiável de um repositório que já
está funcionando.

## Limitações conhecidas

Reunidas aqui para você não descobri-las na hora errada:

- **Sem provider de LLM, a review completa não roda.** O workflow falha de
  forma explícita; a passagem determinística `--seguranca` continua disponível
  quando executada localmente como uma verificação separada.
- **A passagem determinística aplica 3 de 8 regras.** XSS, path traversal,
  criptografia fraca, aleatoriedade insegura e autenticação ausente são
  metadados: nunca disparam sozinhos. Um diff limpo aos olhos de `--seguranca`
  não é um diff seguro.
- **Os três matchers são estreitos.** Eles dependem de formas sintáticas
  específicas na linha adicionada. `exec.Command(...)` do Go, por exemplo, não
  é detectado como command injection.
- **A review vê só o diff.** Dívida de segurança pré-existente é invisível para
  a ferramenta até que alguém toque naquelas linhas.
- **Documentação de linguagens além de Go exige ferramenta externa.** Python
  precisa de `pydoc-markdown` no PATH; sem ela, os arquivos são pulados (o
  comando diz qual ferramenta falta).
- **A publicação depende de configuração do repositório no GitHub.** Sem
  **Settings > Pages > Source = GitHub Actions** e sem as permissões
  `pages: write` / `id-token: write` no job, o deploy é rejeitado.

## Variaveis de ambiente da action de documentacao

Estas variaveis configuram o gerador quando ele roda pela action; a
documentacao completa de entradas e saidas fica em
[ACTION_USAGE.md](https://github.com/Mpaape/AurumCode/blob/main/ACTION_USAGE.md).

| Variable | Effect |
|----------|--------|
| `AURUMCODE_SOURCE_DIR` | tree to scan, default `.` |
| `AURUMCODE_OUTPUT_DIR` | output directory, default `.aurumcode` |
| `AURUMCODE_DOCS_DIR` | where `index.md` and `_config.yml` go, defaults to the output directory |
| `AURUMCODE_LANGUAGES` | comma-separated allow-list, empty means all registered |
| `AURUMCODE_INCREMENTAL` | `true` only documents files changed since the last run |
| `AURUMCODE_VALIDATE_JEKYLL` | `true` runs `bundle exec jekyll build` after generation |
| `AURUMCODE_ALLOW_REPO_CODE_EXECUTION` | opt-in list for `rust`, `csharp` |
| `LLM_API_KEY` + `LLM_BASE_URL` | OpenAI-compatible endpoint for the landing page |
| `LLM_MODEL` | model id, defaults to Claude Haiku |

## Referências no repositório

- [README.md](https://github.com/Mpaape/AurumCode/blob/main/README.md) — visão geral
- [ACTION_USAGE.md](https://github.com/Mpaape/AurumCode/blob/main/ACTION_USAGE.md) — todas as entradas e saídas da action
- [PAGES_SETUP.md](https://github.com/Mpaape/AurumCode/blob/main/PAGES_SETUP.md) — configuração do GitHub Pages
- `internal/review/rules/security.yml` — o catálogo embutido, com o porquê de
  cada regra sem matcher
- `demo.sh` — as três features rodando offline em quatro passos
