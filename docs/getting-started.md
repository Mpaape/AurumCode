# Primeiro review

1. No repositório que receberá os reviews, configure os secrets de Actions
   `LLM_API_KEY` e `LLM_BASE_URL` do seu serviço compatível com OpenAI.
2. Se necessário, configure a variável de Actions `LLM_MODEL` com o
   identificador exato do modelo. Sem ela, o endpoint precisa oferecer um default.
3. Copie o [workflow pronto](site/workflow.yml) para
   `.github/workflows/aurumcode.yml` no repositório de destino.
4. Integre o workflow e os arquivos de contexto na branch base.
5. Abra um PR com uma pequena alteração de código e confira o parecer e o job.

O exemplo acompanha `main`. Para fixar uma instalação, use um SHA revisado.
A tag histórica `v1` não é atualizada por esse fluxo; instalações nela continuam
na versão anterior até mudar a referência.

O token de publicação é o `github.token` do workflow, com as permissões
declaradas no YAML. Ele é limitado ao repositório, não a um único PR.
Os secrets do repositório não são disponibilizados por padrão para PRs de forks.

## Uso local

Construa a imagem a partir do checkout do AurumCode:

```bash
docker build -t aurumcode:local /caminho/para/AurumCode
```

Exporte `LLM_API_KEY`, `LLM_BASE_URL` e, se necessário, `LLM_MODEL` no terminal.
Dentro do repositório que deseja revisar:

```bash
docker run --rm \
  -v "$PWD:/workspace:ro" -w /workspace \
  -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL \
  aurumcode:local review --base HEAD~1
```

A comparação é entre a base e o commit HEAD: mudanças não commitadas ficam fora.
A saída local inclui resumo, achados e um diagrama Mermaid dos arquivos alterados.
O contexto do checkout e a memória opcional usam os mesmos passes do review de PR.
O diagrama representa referências inferidas, não uma prova do fluxo em runtime.

Sem configurar um provedor, execute apenas a análise determinística:

```bash
docker run --rm -v "$PWD:/workspace:ro" -w /workspace \
  aurumcode:local review --base HEAD~1 --fail-on error
```

A saída declara que o review por LLM não aconteceu, seguida de um relatório como:

```text
LLM quality review did not run. The following report covers deterministic analysis only.
## Code Review Summary
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

## Diagnóstico

- Erro de autenticação: confira a credencial e o serviço em `LLM_BASE_URL`.
- Modelo ausente: defina `LLM_MODEL` com um identificador servido pelo endpoint.
- Erro 403 ao publicar: confira `pull-requests: write` e políticas de Actions.
- Contexto configurado não encontrado: adicione os arquivos na branch base.
- CI falhando: o workflow conserva os estados dos checks, inclusive falhas.
  Os estados sozinhos não fornecem logs nem demonstram a causa.
- Achado descartado: o job informa o motivo. Leia os diagnósticos antes de
  interpretar uma lista vazia como garantia de qualidade.

O review por LLM que falha encerra com código 1. Sem provedor configurado, o
caminho local pode executar somente a análise determinística, declarando que
o review por modelo foi omitido.
Use `--exigir-qualidade` com `--base` para exigir também o modelo.
`--fail-on error` encerra com código 3 quando há achados graves.

Veja [configuração](configuration.md) para idioma e publicação.
