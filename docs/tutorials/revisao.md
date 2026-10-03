# Tutorial: revisão de código com o AurumCode

## Objetivo

Ao final você terá rodado, na sua máquina, cinco usos da revisão: a primeira
revisão local, a revisão sem provedor de modelo, a revisão com provedor, a
aplicação de uma sugestão com `fix` e a ligação do mesmo fluxo a pull requests
pelo workflow reutilizável. Também verá o que acontece quando um arquivo não
pode ser revisado.

Cada comando e cada saída abaixo vêm de uma execução real, registrada em
`demo/tutoriais/revisao/out/` e conferida por `run.sh --check`. Os blocos de
configuração **são os arquivos de `demo/tutoriais/revisao/`**, byte a byte
(`tests/acceptance/AUR-561.sh AC-004` compara).

## Pré-requisitos

- `git`, `docker` e `bash`.
- A imagem do produto, construída do `Dockerfile` da raiz do repositório:

```bash
docker build -t aurumcode:local /caminho/para/AurumCode
```

- Um atalho para chamar o programa dentro do repositório que você revisa. Os
  exemplos abaixo escrevem `aurumcode ...`; defina o atalho uma vez:

```bash
alias aurumcode='docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD:/work" -w /work -e LLM_API_KEY -e LLM_BASE_URL -e LLM_MODEL -e AURUMCODE_LLM_FIXTURE -v "/caminho/para/demo/tutoriais/revisao:/fixtures:ro" --entrypoint /app/aurumcode aurumcode:local'
```

Em uso real, `LLM_API_KEY` e `LLM_BASE_URL` apontam para o seu serviço
compatível com OpenAI. Para repetir um caso à mão, entre num repositório com a mudança e aponte o provedor falso para o arquivo do tutorial (o volume `/fixtures` do atalho acima), por exemplo `export AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json`; para o caso sem provedor, `unset AURUMCODE_LLM_FIXTURE`. O `--user` evita arquivos com dono root no seu diretório. **Neste tutorial nenhuma credencial é usada**: o
provedor é um arquivo JSON determinístico (`AURUMCODE_LLM_FIXTURE`), então o
resultado é sempre o mesmo e nada sai da sua máquina.

### Rodar a demonstração

```bash
bash demo/tutoriais/revisao/run.sh all      # executa os seis casos e grava out/
bash demo/tutoriais/revisao/run.sh --check  # compara out/ com expected/, sem docker
```

Cada caso cria um repositório descartável em `demo/tutoriais/revisao/.estado/`
(ignorado pelo git), com a branch `main` e uma branch `feature` com a mudança
a revisar. O programa roda na imagem do produto, sem rede.

## Caso 1: primeira revisão local

A mudança troca uma mensagem. O repositório de exemplo tem `app.go` em `main` e
a branch `feature` com a mensagem alterada. Com o provedor (aqui, o JSON
`fixture-vazia.json`, que não encontra problemas):

```bash
aurumcode review --base main
```

`--base` compara a referência com o `HEAD`; mudanças não comitadas ficam de
fora.

<!-- saida: primeira-revisao -->
```text
**Verdict:** Approve
No issues found.
exit_code=0
```

O que observar: o parecer traz resumo, contagem por severidade e os arquivos
tocados; sem achados o veredito é `Approve` e o código de saída é 0. O código
de saída é o contrato com o CI: 0 é sucesso, 3 é "achado na severidade de
`--fail-on` ou acima", 1 é "a revisão não aconteceu ou falhou".

## Caso 2: sem provedor, só a análise determinística

Agora a mudança escreve uma senha no código e **nenhum provedor está
configurado** (sem `LLM_API_KEY`/`LLM_BASE_URL` e sem fixture):

<!-- arquivo: demo/tutoriais/revisao/repo-exemplo/segredo/app.go -->
```go
package main

import "fmt"

func main() {
	dbPassword := "hunter2"
	fmt.Println("conectando com", dbPassword)
}
```

```bash
aurumcode review --base main --seguranca --fail-on error
```

<!-- saida: sem-provedor -->
```text
aurumcode review: no LLM provider configured: quality review skipped; running deterministic analysis only
LLM quality review did not run. The following report covers deterministic analysis only.
**Verdict:** Comment
app.go:6: [error] Hardcoded credentials or API keys detected [standards/security-review SCR-003] (rule security/hardcoded-secret: Hardcoded Secrets)
aurumcode review: 1 finding(s) at severity error or above (--fail-on error)
exit_code=3
```

O que observar: o programa **declara** que a revisão por modelo não rodou, e o
veredito é `Comment`, nunca `Approve`. `--seguranca` soma o passe
determinístico (regras do catálogo embutido), que ainda acha o segredo, e
`--fail-on error` o transforma em saída 3. Para um CI que não pode confundir
"só determinístico" com "revisado", acrescente `--exigir-qualidade`:

```bash
aurumcode review --base main --seguranca --exigir-qualidade
```

<!-- saida: sem-provedor -->
```text
aurumcode review: --exigir-qualidade: the quality review did not run, so this run is not a clean review
exit_code=1
RESULTADO: com --exigir-qualidade, sem provedor o comando falha
```

Sem o modelo a saída é 1 e a mensagem diz que a revisão por modelo não rodou.
(A linha `RESULTADO:` é conclusão do script, não saída do produto: vale quando o
`exit_code` é o esperado, 1 aqui.)

## Caso 3: com provedor, achados que citam a regra

Com um provedor, o modelo devolve achados. O provedor deste tutorial é este
arquivo, que é a forma exata aceita pelo motor:

<!-- arquivo: demo/tutoriais/revisao/fixture-llm.json -->
```json
{
  "issues": [
    {
      "file": "app.go",
      "line": 6,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A senha do banco esta escrita no codigo.",
      "impact": "Quem le o repositorio ou o historico reutiliza a senha.",
      "evidence": "A linha adicionada atribui o literal \"hunter2\" a dbPassword em app.go.",
      "suggestion": "Leia a senha do ambiente em vez de escreve-la no codigo.",
      "verification": "Remova o literal e rode a revisao de novo; o achado deve sumir."
    }
  ],
  "summary": "A mudanca escreve uma senha no codigo de app.go."
}
```

```bash
aurumcode review --base main --fail-on error
```

<!-- saida: com-provedor -->
```text
**Verdict:** Changes requested
app.go:6: [error] A senha do banco esta escrita no codigo. (rule security/hardcoded-secret: Hardcoded Secrets)
aurumcode review: 1 finding(s) at severity error or above (--fail-on error)
exit_code=3
```

O que observar: cada achado nomeia a **regra** que o sustenta
(`rule security/hardcoded-secret`). Um achado sem `rule_id`, com regra
desconhecida, sem evidência concreta ou fora das linhas alteradas é
descartado e o programa avisa quantos descartou; o parecer nunca mostra um
achado que o motor não conseguiu ancorar.

## Caso 4: `fix` aplica uma sugestão

`aurumcode fix` lê as sugestões (um array, ou a resposta completa de uma
revisão) e imprime um patch unificado. **Nada é escrito no repositório**:
você confere e aplica.

<!-- arquivo: demo/tutoriais/revisao/fix/sugestoes.json -->
```json
[
  {
    "title": "Ler a senha do ambiente",
    "description": "Evita o segredo escrito no codigo",
    "kind": "code",
    "file": "app.go",
    "line": 6,
    "current_code": "\tdbPassword := \"hunter2\"",
    "proposed_code": "\tdbPassword := os.Getenv(\"DB_PASSWORD\")"
  }
]
```

```bash
aurumcode fix --file sugestoes.json > fix.patch
git apply --unidiff-zero --check fix.patch
git apply --unidiff-zero fix.patch
```

<!-- saida: fix -->
```text
-	dbPassword := "hunter2"
+	dbPassword := os.Getenv("DB_PASSWORD")
git apply --unidiff-zero --check: o patch aplica
```

O que observar: o patch não tem linhas de contexto (`@@ -6,1 +6,1 @@`), por
isso o `git apply` precisa de `--unidiff-zero`. O patch troca só a linha
sugerida: acrescentar `import "os"` continua sendo seu. O `current_code` é
conferido contra o arquivo antes de o patch ser considerado aplicável.

**O que acontece quando a sugestão está velha.** Rodar o mesmo `fix` depois de
a linha já ter sido trocada:

<!-- saida: fix -->
```text
aurumcode fix: patch does not apply to the working tree
exit_code=1
RESULTADO: sugestao velha recusada, sem patch
```

Saída 1, o arquivo e a linha são nomeados no erro e nenhum patch é impresso.

## Caso 5: revisão de pull request pelo workflow reutilizável

No GitHub a revisão roda em cada PR pelo workflow reutilizável
`.github/workflows/review.yml`. O repositório que recebe os reviews só
precisa deste arquivo em `.github/workflows/aurumcode.yml` (`OWNER` e o SHA
são placeholders: troque pelos seus e fixe um SHA revisado):

<!-- arquivo: demo/tutoriais/revisao/workflow-chamador.yml -->
```yaml
# .github/workflows/aurumcode.yml do repositorio que recebe os reviews.
# OWNER e o SHA sao placeholders: troque pelos seus.
name: AurumCode
on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: write
  statuses: write
  checks: read

jobs:
  review:
    uses: OWNER/AurumCode/.github/workflows/review.yml@0000000000000000000000000000000000000000
    with:
      model: ${{ vars.LLM_MODEL }}
      security: true
    secrets:
      LLM_API_KEY: ${{ secrets.LLM_API_KEY }}
      LLM_BASE_URL: ${{ secrets.LLM_BASE_URL }}
```

Os secrets `LLM_API_KEY` e `LLM_BASE_URL` ficam nas configurações do repositório.
O workflow reutilizável constrói a imagem do produto, faz o checkout do head
do PR e roda `aurumcode review --pr N --repo OWNER/REPO --publicar --check
--exigir-qualidade` (mais `--seguranca` e `--politica`, quando configurados); o
parecer é publicado no PR e o job falha se a revisão for inconclusiva.

O que observar, e o que esta demonstração prova: não há runner do GitHub
aqui e não há servidor de GitHub reutilizável fora dos testes de unidade do
repositório, então este tutorial **não** executa uma revisão de PR. O que a
fase `pr-workflow` verifica de verdade é que o arquivo acima é coerente com o
reutilizável do repositório: cada entrada de `with:` e cada secret existem em
`review.yml` e as permissões do chamador cobrem as que ele declara. Os casos
1 a 3 e 5 usam o mesmo motor que o job executa.

Estas linhas são conclusão do script (não é saída do produto): cada uma é
impressa quando o `grep` do `run.sh` encontra, em `review.yml` e no workflow do
chamador, o trecho correspondente (input em `workflow_call.inputs`, secret em
`workflow_call.secrets`, permissão declarada nos dois).

<!-- saida: pr-workflow -->
```text
gatilho: pull_request
uses: workflow reutilizavel fixado em SHA
input model: existe em review.yml
input security: existe em review.yml
secret LLM_API_KEY: existe em review.yml
secret LLM_BASE_URL: existe em review.yml
permissao pull-requests: write: o chamador concede o que o reutilizavel declara
```

O que observar: um workflow `workflow_call` não concede permissão que o
chamador não tem; por isso o bloco `permissions:` do chamador é obrigatório.

## Quando falha: arquivo não revisado nunca conta como aprovado

E se a mudança é só um binário (`logo.png`), que o motor filtra antes da
revisão? Sem gate declarado:

```bash
aurumcode review --base main
```

<!-- saida: falha-nao-revisado -->
```text
binary file, skipped: logo.png
**Verdict:** Comment
a file that was not reviewed never counts as approved.
  - logo.png (binary)
RESULTADO: sem gate, exit 0 mas o veredito nao e Approve
```

(`RESULTADO:` é conclusão do script, não saída do produto: vale quando o
`exit_code` é o esperado.)

O veredito é `Comment`, o arquivo é nomeado como não revisado, e o parecer
diz a regra: *um arquivo não revisado nunca conta como aprovado*. A saída
continua 0 porque nenhum gate foi pedido. Para que a cobertura parcial
**reprove** o check, declare o gate no repositório (ou, melhor, numa
[política central](politica-central.md)):

<!-- arquivo: demo/tutoriais/revisao/repo-exemplo/gate-estrito/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: block
```

<!-- saida: falha-nao-revisado -->
```text
aurumcode review: policy gate: review inconclusive (partial_coverage)
exit_code=1
RESULTADO: com gate.inconclusive: block, cobertura parcial reprova
```

Com `inconclusive: block` a revisão parcial reprova (saída 1) e o motivo
`partial_coverage` aparece. Pela referência de configuração, com `warn` ela
continua visível como inconclusiva sem bloquear (não demonstrado aqui). O
veredito `Comment` sem `Approve` foi demonstrado nos dois casos acima.

## Problemas comuns

- **`no LLM provider configured`**: faltam `LLM_API_KEY` e `LLM_BASE_URL` (ou o
  fixture). Sem provedor, só a análise determinística roda, e isso é dito na
  saída; use `--exigir-qualidade` para tornar isso uma falha.
- **`No issues found.` não é garantia**: leia a seção de cobertura e as linhas
  `binary file, skipped`; arquivos filtrados não foram revisados.
- **Achado "descartado pelo gate de escopo e evidência"**: o modelo citou uma
  linha que não está no diff, ou sem evidência concreta. Isso é proposital.
- **`git apply` recusa o patch do `fix`**: use `--unidiff-zero`; se mesmo assim
  recusar, o arquivo mudou depois da revisão (a sugestão está velha).
- **`--base` não vê minha mudança**: ele compara com o commit `HEAD`; faça
  commit antes.
- **Erro 403 ao publicar no PR**: confira `pull-requests: write` no workflow
  chamador (não demonstrado aqui: não há publicação em PR nesta demonstração).

Próximos passos: [skills de convenção](skills.md) e
[política central](politica-central.md).
