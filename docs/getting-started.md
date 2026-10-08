# Primeiro review

1. No repositório que receberá os reviews, configure os secrets de Actions
   `LLM_API_KEY` e `LLM_BASE_URL` do seu serviço compatível com OpenAI.
2. Se necessário, configure a variável de Actions `LLM_MODEL` com o
   identificador exato do modelo. Sem ela, o endpoint precisa oferecer um default.
3. Copie o [workflow pronto](site/workflow.yml) para
   `.github/workflows/aurumcode.yml` no repositório de destino.
4. Integre o workflow e os arquivos de contexto na branch base.
5. Abra um PR com uma pequena alteração de código e confira o parecer e o job.

O exemplo acompanha `main`:

```yaml
    uses: Mpaape/AurumCode/.github/workflows/review.yml@main
```

Para fixar uma instalação, use um SHA revisado ou a tag de uma release
oficial: o roteiro de [Releases](releases.md) troca essa referência pela
versão publicada no exemplo, no download do site e nesta página. A tag
histórica `v1` não é atualizada por esse fluxo; instalações nela continuam na
versão anterior até mudar a referência.

O token de publicação é o `github.token` do workflow, com as permissões
declaradas no YAML. Ele é limitado ao repositório, não a um único PR.
Os secrets do repositório não são disponibilizados por padrão para PRs de forks.

O workflow reutilizável já faz o checkout no `ref: ${{ github.event.pull_request.head.sha }}`,
o head revisado do PR, então o contexto de codebase chega normalmente ao
modelo. Quem monta a Action Docker diretamente, com o próprio passo
`actions/checkout` (ver "Opções avançadas" em docs/configuration.md),
precisa declarar esse mesmo `ref`: o padrão do `actions/checkout` num evento
`pull_request` é o merge ref sintético, não o head revisado, e sem esse `ref`
explícito o HEAD do checkout diverge do commit revisado pela API, e o
contexto de codebase é omitido por esse descompasso de HEAD.

## Configuração mínima

Sem `.aurumcode/config.yml` o review já roda, mas só orienta. Para um gate,
estas linhas na branch base bastam, sem ajuste fino:

```yaml
gate:
  fail_on: [error]      # achado error reprova o check
  inconclusive: block   # revisão que não concluiu nunca aprova
ignore:
  - "tests/**"          # fora da revisão: listado como ignorado, nunca "parcial"
quality_gates:
  scanners:
    - engine: gitleaks  # segredos no intervalo de commits da PR
      required: true
    - engine: semgrep   # SAST: só achados em linhas que a PR adicionou
    - engine: govet     # go vet com o Go da imagem: só linhas adicionadas
      fail_on: warning
```

- Caminho em `ignore` e arquivo de formato binário conhecido (uma captura
  PNG) ficam fora da conta de cobertura: o parecer os lista como ignorados e a revisão não fica
  inconclusiva por eles.
- Semgrep e go vet varrem a árvore, mas só reprovam pelo que a PR mudou: um
  achado antigo num arquivo que ela não tocou não bloqueia.
- Convenções do time entram como `.aurumcode/skills/<nome>/SKILL.md`; cada
  seção `## ` vira regra citável `<nome>#<slug>` sem listar nada (ver
  "Skills em diretório" em docs/configuration.md).
- Falha de provedor e scanner ausente continuam bloqueando com
  `inconclusive: block`; o motivo inconclusivo de uma engine traz o detalhe
  da falha, resumido e redigido (`[detalhe: ...]`), no parecer e na auditoria.
- PR grande também é revisado: quando o GitHub recusa o diff por tamanho, o
  diff sai do checkout verificado do PR; quando ele não cabe num prompt, a
  revisão é feita em lotes por diretório, com um parecer e um gate. Só no teto
  de lotes (`batches`, padrão 4) a revisão fica parcial, listando os arquivos
  que ficaram fora (ver "PR grande" em docs/configuration.md).
- Saída gravada por script (logs de tutorial, golden files, capturas) repete
  de propósito exemplos de injeção e canários de segredo: ponha-a em `ignore`
  e revise a fonte que a gera.
- Com `deliberation`, o teto `max_cost_tokens` mede só o que as rodadas de
  ferramenta acrescentam ao prompt base; um prompt base grande, sozinho, não
  estoura o teto.
- O próprio AurumCode se revisa assim: o
  [`.aurumcode/config.yml`](https://github.com/Mpaape/AurumCode/blob/main/.aurumcode/config.yml)
  do repositório acrescenta deliberação e seis skills de convenção, em menos
  de 30 linhas úteis.

## Uso local

Construa a imagem a partir do checkout do AurumCode:

```bash
docker build -t aurumcode:local /caminho/para/AurumCode
```

Exporte `LLM_API_KEY`, `LLM_BASE_URL` e, se necessário, `LLM_MODEL` no terminal.
Para um provedor conhecido, `LLM_PROVIDER` escolhe o perfil e dispensa montar a
URL (passe também `-e LLM_PROVIDER` e as variáveis do provedor ao `docker run`);
veja [Provedores de LLM](provedores.md).
Dentro do repositório que deseja revisar:

```bash
docker run --rm \
  -v "$PWD:/workspace:ro" -w /workspace \
  -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL \
  aurumcode:local review --base HEAD~1
```

A comparação é entre a base e o commit HEAD: mudanças não commitadas ficam fora.
A saída local abre com a mesma decisão do parecer da PR (Aprovado, Aprovado
com observações, Bloqueado ou Inconclusivo), seguida dos achados, um por
linha. O contexto do checkout e a memória opcional usam os mesmos
passes do review de PR.

Sem configurar um provedor, execute apenas a análise determinística:

```bash
docker run --rm -v "$PWD:/workspace:ro" -w /workspace \
  aurumcode:local review --base HEAD~1 --fail-on error
```

A saída declara que o review por LLM não aconteceu, seguida de um relatório como:

```text
LLM quality review did not run. The following report covers deterministic analysis only.
## AurumCode code review

> [!CAUTION]
> **Blocked: 1 problem(s) must be fixed before merge.**
...
app.go:3: [error] Hardcoded secret or credential assigned inline (rule analysis/hardcoded-secret)
```

`--fail-on error` retorna 3 quando há achados graves. Para um CI que exige revisão
por modelo, acrescente `--exigir-qualidade`: modelo ausente ou falhando retorna 1.
Uma configuração de provedor inválida nunca é convertida em sucesso offline.

Para persistir observações entre reviews locais, configure `review.memory: local`
e monte um cache gravável, separado do código somente leitura:

```bash
docker run --rm -v "$PWD:/workspace:ro" -w /workspace \
  -v aurumcode-cache:/review-cache -e XDG_CACHE_HOME=/review-cache \
  -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL \
  aurumcode:local review --base HEAD~1
```

O cache de memória é separado por repositório. Falhas ao ler ou gravar observações
aparecem no diagnóstico; memória não autoriza mudanças de regras ou aprovação.

## Corrigir sugestões com o fix

`aurumcode fix` lê a resposta da revisão — o objeto completo com `suggestions`
ou apenas o array de sugestões — e imprime um diff unificado padrão, com três
linhas de contexto do próprio arquivo, que o `git apply` (e o `patch -p1`)
aceita sem nenhuma flag. Nada é escrito no repositório: você inspeciona e aplica.

```bash
docker run --rm -v "$PWD:/workspace:ro" -w /workspace \
  aurumcode:local fix --file review-response.json > fix.patch
git apply --check fix.patch
git apply fix.patch
```

O `current_code` de cada sugestão é conferido contra a árvore de trabalho antes
de o patch ser considerado aplicável. Exemplo de resposta aceita:

```json
[
  {
    "title": "Carregar a senha do loader",
    "description": "Evita o segredo inline",
    "kind": "code",
    "file": "app.go",
    "line": 4,
    "current_code": "\tdbPassword := \"hunter2\"",
    "proposed_code": "\tdbPassword := loadPassword()"
  }
]
```

Se o `current_code` não corresponder ao arquivo na linha indicada, o comando sai
com código 1, nomeia o arquivo e a linha, e não imprime patch. As sugestões vêm
de `aurumcode review` (veja acima) ou de um parecer publicado no PR.

## Ajuda por subcomando

`aurumcode --help` lista todos os subcomandos, uma linha cada: `review`, `fix`,
`sbom`, `sign`, `xbom`, `dependencies`, `mcp`, `changelog` e `realimentacao`.
`aurumcode <subcomando> --help` imprime todas as flags
do subcomando (as mesmas que ele aceita: ajuda e parser leem o mesmo conjunto
de flags) e um exemplo executável. A ajuda é gerada de um registro único de
subcomandos, então um subcomando novo não existe sem uma linha de ajuda.

```bash
aurumcode --help
aurumcode xbom --help
```

Quando o `review` não acha nada, a última linha do parecer depende de todas as
fontes terem concluído. Se todas concluíram, é `No issues found.`. Se alguma
ficou inconclusiva (SAST, Dependency-Track, dados de análise, cobertura
parcial, provedor), o parecer não diz isso: imprime `Sem achados nas fontes
concluídas; inconclusivo: <motivos>`, com o motivo de cada fonte, por exemplo
`sast_execution_error`.

## Diagnóstico

- Erro de autenticação: confira a credencial e o serviço em `LLM_BASE_URL`.
- Modelo ausente: defina `LLM_MODEL` com um identificador servido pelo endpoint.
- Erro 403 ao publicar: confira `pull-requests: write` e políticas de Actions.
- Contexto configurado não encontrado: adicione os arquivos na branch base.
- CI falhando: o workflow conserva os estados dos checks, inclusive falhas.
  Os estados sozinhos não fornecem logs nem demonstram a causa.
- Achado descartado: o job informa o motivo. Leia os diagnósticos antes de
  interpretar uma lista vazia como garantia de qualidade.

Sem provedor configurado, o caminho local pode executar somente a análise
determinística, declarando que o review por modelo foi omitido. Use
`--exigir-qualidade` com `--base` ou `--pr` para exigir também o modelo; o
workflow reutilizável já usa essa opção e falha se o parecer for inconclusivo.
`--fail-on error` encerra com código 3 quando há achados graves.

Veja [configuração](configuration.md) para idioma e publicação.
