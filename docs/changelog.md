# Changelog obrigatório

Cada pull request para a `main` acrescenta uma entrada curta, escrita para
quem usa o produto, na seção `## Unreleased` do `CHANGELOG.md`. O check
`aurumcode changelog` reprova a PR que não faz isso (modo `required`) ou só
sugere a entrada sem reprovar (modo `suggest`). Os modos estão em
[configuração do changelog](configuration.md#changelog-obrigatorio-aur-509).

O veredito é determinístico: compara as duas versões do arquivo; o modelo só
escreve a sugestão. A sugestão de entrada do review (`review.changelog` / `--changelog`)
continua existindo, separada e só consultiva: ela propõe um texto, mas não
aprova nem reprova o merge.

## Ligar no repositório

```yaml
# .aurumcode/config.yml
changelog_check:
  mode: required
```

O modo vem do `config.yml` do commit **base** da PR. Uma PR que desliga o
modo no próprio `config.yml` continua sujeita ao check; a mudança só vale
depois do merge.

PR aberta por bot (Dependabot, Renovate) segue `changelog_check.bots`, padrão
`suggest`: com `mode: required`, ela passa com a linha `changelog: autor é bot
(...)` e com a entrada sugerida, em vez de reprovar com `entrada_ausente`.
`bots` só rebaixa o modo (`off` pula o check para bots, `required` reprova
como para uma pessoa). Bot é o que o evento da PR diz: `user.type: Bot` ou
login terminado em `[bot]`.

No GitHub, chame o workflow reutilizável, pinado por SHA:

```yaml
# .github/workflows/changelog.yml do repositório consumidor
name: Changelog
on:
  pull_request:
permissions:
  contents: read
jobs:
  changelog:
    uses: Mpaape/AurumCode/.github/workflows/changelog.yml@<sha>
```

O verificador é construído do checkout do AurumCode naquele SHA, nunca do
código da PR; a PR entra só como dado. Marque o contexto
**changelog / Changelog obrigatório** (job do chamador / job do workflow) como
required check em *Settings → Branches → Branch protection rules → Require
status checks to pass*. Não renomeie o job depois de exigido. O workflow não
tem filtro de `paths` nem `if:`, porque um job pulado conta como verde num
required check. Neste repositório o mesmo arquivo roda direto em
`pull_request`, com o contexto **Changelog obrigatório**.

## O que reprova

| Motivo | Quando |
| --- | --- |
| `entrada_ausente` | A PR não altera o `CHANGELOG.md`. |
| `apenas_espacos` | A alteração muda só espaços, indentação ou quebras de linha. |
| `sem_informacao_nova` | Nenhuma linha nova com pelo menos `min_words` palavras em `Unreleased` ou numa seção de versão: só títulos, remoções, linhas reordenadas ou texto em outra seção. |
| `entrada_longa` | Mais de `max_entry_lines` linhas novas em `Unreleased`, notas de versão com mais de `max_release_lines` linhas, ou uma linha com mais de `max_line_length` caracteres. |
| `log_de_agente` | Uma linha nova contém um marcador de log de agente ou de ferramenta (`--- PASS`, `/pass`, `Co-Authored-By`, bloco de código, ...). |
| `indeterminado` | O diff não pôde ser lido, o arquivo é binário ou grande demais, ou o `config.yml` da base é inválido. Nunca vira aprovação. |

Saída: exit 0 com `changelog: aprovado (entrada_valida)`, exit 1 com
`changelog: reprovado (<motivo>)` ou `indeterminado`, exit 2 para uso errado.
O texto da entrada é dado: o check o compara e conta palavras, mas nunca o
repete na saída nem obedece ao que ele diz.

## Manter o `Unreleased`

Escreva o que muda para quem usa, não como foi feito:

```markdown
## Unreleased

- O review aponta a linha exata do achado também em arquivos renomeados.
- `aurumcode sbom` aceita `--imagem` com digest.
```

Evite log de agente, saída de teste e lista de commits:

```markdown
## Unreleased

- AUR-123/AC-001/pass, go test verde, 14 arquivos alterados.   <- reprova: log_de_agente
- ajuste                                                      <- reprova: sem_informacao_nova
```

## Consolidar uma release

Na PR de release, as linhas de `Unreleased` passam para a seção da versão e
ganham um resumo de uma ou duas páginas no máximo. Linhas só movidas não contam
como informação nova; o resumo conta:

```markdown
## Unreleased

## 1.2.0 - 2026-10-07

Destaques: changelog obrigatório nas PRs e parecer em português por padrão.

- O review aponta a linha exata do achado também em arquivos renomeados.
- `aurumcode sbom` aceita `--imagem` com digest.
```

Uma PR de release que só move linhas, sem resumo, é reprovada com
`sem_informacao_nova`.
