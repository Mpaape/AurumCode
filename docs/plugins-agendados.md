# Plugins externos da validação agendada

A validação agendada pode rodar **plugins externos**: programas que o
AurumCode não embute nem clona, cada um no próprio container, sobre a branch
padrão. O primeiro exemplo pronto é um harness agêntico de vulnerabilidade
público (Apache-2.0, Python): `demo/plugins/vvah/`. Os achados de um plugin
vão para a aba Security (code scanning) do repositório; num repositório
**privado sem GitHub Code Security**, viram issues do próprio repositório
(só colaboradores veem). Em repositório público, nunca issue: só a aba
Security.

Tutorial executável: [plugin agendado](tutorials/plugin-agendado.md).
Referência da configuração: [configuração](configuration.md), seção
"Plugins externos da validação agendada".

## O que fica público e o que não fica

Num repositório público, quase tudo que um workflow produz é público: o log
do Actions, o resumo do job, as anotações (`::error`), os artefatos do run e
os comentários e anotações de PR. A lista de alertas de code scanning (aba
Security) exige permissão de escrita no repositório. Por isso o job
`plugins`:

| Canal | Usado? |
|---|---|
| Log do Actions | só contagens ("N alertas enviados para a aba Security") e motivos estáveis (`[degraded]`, `[coverage_lost]`) |
| Saída do plugin (stdout/stderr, texto cru do modelo) | arquivo em `$RUNNER_TEMP`, nunca impresso, nunca artefato |
| Resumo do job, anotações, artefatos | nunca |
| PR (comentário, status, gate) | nunca; em `pull_request*` os jobs de plugin são pulados pelo `if:` |
| Aba Security (code scanning) | sim: SARIF com categoria `aurumcode/plugin/<nome>` |
| Issues do próprio repositório | só em repositório privado (ou interno) sem Code Security; nunca em público |

Arquivos que o harness deixa em `security-scan/` além do `findings.json` (o
relatório em Markdown e o texto cru do modelo, `*_s8_raw.txt`) ficam na cópia
descartável da árvore e somem com o runner: nunca são impressos nem enviados.

## Contrato

Configuração (`scheduled.plugins`, no repositório ou na política central):

- `name`: `[a-z0-9][a-z0-9_-]*`, único. Vira o ruleId (`plugin/<nome>/...`) e
  a categoria (`aurumcode/plugin/<nome>`).
- `image` fixada por digest (`<ref>@sha256:<64 hex>`) **ou** `dockerfile`
  (caminho relativo, lido da branch padrão). Nunca os dois. Na **política
  central** só vale `image` por digest: o Dockerfile seria lido do
  repositório varrido, que poderia trocar o plugin por um que não acha nada
  e fecha os alertas; a política com `dockerfile` é recusada na leitura.
- `args`: argumentos do container.
- `env`: só **nomes** de variáveis. O valor de cada um vem do secret de mesmo
  nome do chamador; um secret ausente falha o job antes de rodar o plugin.
- `output.format`: `sarif` (genérico) ou `vvah-findings` (o `findings.json`
  do harness); `output.path`: relativo à árvore, num diretório próprio.
- `timeout_minutes`: padrão 60, sem teto. O job em volta tem esse tempo
  mais uma folga fixa de 30 minutos (build, ingest e envio), então o timeout
  do plugin estoura primeiro e o log diz só "O plugin estourou o próprio
  timeout".
- `required` (na política central): o repositório não remove nem troca a
  entrada de mesmo nome.

Execução, por plugin (job `plugins`, um item de matriz por plugin):

1. a imagem é baixada pelo digest ou construída do Dockerfile da branch
   padrão;
2. a árvore é copiada para um diretório descartável em `$RUNNER_TEMP`; o
   diretório de `output.path` é esvaziado (um arquivo versionado nunca passa
   por saída do plugin) e é o único gravável: a árvore é montada somente
   leitura em `/alvo`;
3. o container roda sem o `docker.sock`, sem capabilities, com
   `no-new-privileges`, e com stdout/stderr para
   `$RUNNER_TEMP/plugin-<nome>.log`;
4. `aurumcode plugin ingest` lê a saída como **dado não confiável**;
5. `github/codeql-action/upload-sarif` (fixado por SHA) envia o SARIF com
   categoria `aurumcode/plugin/<nome>`; o último passo imprime só a contagem.

O ingest:

- recusa achado com caminho absoluto, com `..`, inexistente, que sai da
  árvore por link simbólico ou que aponta para dentro do diretório gravável
  de `output.path` (o que o plugin escreveu não é o código varrido): o ingest reprova (exit 1), o achado nunca some em
  silêncio, e a mensagem cita só a posição do achado, nunca o caminho;
- passa todo texto pelo filtro de segredos antes de gravar;
- grava SARIF 2.1.0 com `tool.driver.name` `AurumCode plugin <nome>`,
  `ruleId` `plugin/<nome>/<cwe ou classe>`, `partialFingerprints` estável (no
  harness, o `case_id`), `automationDetails.id` `aurumcode/plugin/<nome>/` e
  `security-severity` (o CVSS do achado, ou o da severidade);
- no formato `vvah-findings`, só achados do canal `main` com veredito
  `TRUE_POSITIVE` viram alerta; falso positivo, canal `posture` e sem
  veredito só contam;
- **dúvida nunca some**: candidato que o harness descartou com motivo
  `INCONCLUSIVE`, `VERIFY_ERROR` ou `UNCONFIRMED` vira alerta de aviso
  (`warning`) `plugin/<nome>/nao-verificado/<classe>`, com mensagem dizendo
  que a verificação do plugin foi inconclusiva e fingerprint estável de
  (arquivo, bloco de dez linhas, classe); o caminho passa pela mesma
  recusa. Descartes por `DUPLICATE` ou `FALSE_POSITIVE` não viram alerta.

### Inconclusivo: nenhum SARIF

Um SARIF vazio enviado na mesma categoria **fecharia** todos os alertas
abertos. Por isso, nos casos abaixo o ingest sai com exit 1 e não grava nada,
e os alertas abertos ficam como estão:

- plugin com exit diferente de 0, ou arquivo de saída ausente ou inválido;
- `schema_version` de outro major (o harness publica `1.x`);
- `degraded: true` no `findings.json`;
- o SARIF do próprio harness (o `*_report.sarif` mais novo ao lado do
  `findings.json`, ou `--harness-sarif`) ausente, com
  `invocations[0].executionSuccessful: false`, `properties.hollow: true` ou
  `properties.coverageDegraded: true` com algum motivo
  (`coverageDegradedReasons`) fora da lista dos representados nos alertas.
  Hoje a lista tem só `s6_undetermined` (verificações indeterminadas, que
  viram alertas "não verificado"): com só esse motivo o envio segue;
  `zero_analysis`, lista vazia ou motivo desconhecido continuam
  inconclusivos;
- `*_errors.jsonl` (ou `--harness-errors`) com erro de estágio de análise
  (`s2` a `s6`) ou com registro sem estágio legível.

O harness sai com exit 0 também quando perdeu cobertura. Numa execução real
com um modelo pequeno atrás de um proxy compatível com OpenAI, o estágio s4
falhou com `StructuredOutputValidationError`: o `findings.json` veio com
`degraded: false` e zero achados, e só o SARIF do harness dizia
`coverageDegraded: true` (`zero_analysis`). É exatamente o "zero falso" que o
ingest trata como inconclusivo. Numa segunda execução real (Anthropic,
montagem do job) o harness achou 4 verdadeiros positivos com
`coverageDegraded` só por `s6_undetermined` e um candidato `INCONCLUSIVE`:
o ingest envia 5 alertas, 1 deles "não verificado".

## O chamador

O workflow reutilizável não declara `permissions:` nos jobs de plugin: um job
reutilizável que pedisse `security-events: write` faria falhar na partida
todo chamador que não a concede (inclusive a revisão de PR). Quem concede é o
job do chamador que roda no agendamento:

```yaml
on:
  schedule:
    - cron: "17 4 * * *"
  workflow_dispatch:

jobs:
  agendado:
    permissions:
      contents: read
      actions: read            # o upload-sarif precisa em repositório privado
      security-events: write
      issues: write            # só repositório privado sem Code Security (destino issues)
    uses: Mpaape/AurumCode/.github/workflows/review.yml@<sha>
    secrets: inherit   # repassa ANTHROPIC_API_KEY, VVAH_MODEL e os LLM_*
```

O mesmo agendamento dispara também o job `dependencies` (varredura agendada
de dependências), que precisa de `LLM_API_KEY` e `LLM_BASE_URL`; sem eles
aquele job falha, independentemente dos plugins.

`secrets: inherit` é necessário porque um `workflow_call` só recebe por nome
os secrets que declara, e os nomes de `env` de cada plugin são livres.

## Repositório privado

Repositório privado **com** GitHub Code Security: igual ao público. **Sem**
Code Security, os achados viram **issues do próprio repositório privado**
(só colaboradores veem), uma por achado. O passo "Choose destination" decide
o destino (`aurumcode plugin destino`, pela visibilidade lida da API; só um
403 da API de code scanning conta como "sem Code Security", um 404 de
repositório ainda sem análise não). No destino `issues` o job roda o plugin
e o ingest como no destino `security` e, só depois de um ingest conclusivo,
o passo "Sync issues" roda:

```bash
aurumcode plugin issues --name <nome> --sarif plugin-<nome>.sarif --repo OWNER/REPO
```

O comando, com `GITHUB_TOKEN` (o `github.token` do job, com `issues: write`
concedido pelo chamador):

1. lê o SARIF localmente; ausente (ingest inconclusivo), malformado, de uma
   execução sem sucesso ou de outro plugin: **nenhuma** chamada ao GitHub,
   exit 1, as issues ficam como estão;
2. confere a visibilidade pela API: repositório público, ou visibilidade
   que não pode ser lida, é recusado (exit 1) **antes de qualquer escrita**;
3. garante o rótulo `aurumcode-plugin:<nome>` e lista **todas** as issues
   com ele (abertas e fechadas, página por página, sem teto);
4. sincroniza por fingerprint: cada issue leva na primeira linha do corpo o
   marcador oculto `<!-- aurumcode-plugin:<nome>:<fingerprint> -->`. Achado
   novo abre issue; achado que continua não duplica (o corpo é atualizado se
   mudou); issue aberta cujo achado sumiu da varredura é fechada com um
   comentário; issue fechada cujo achado volta é reaberta.

Título só com campos estruturados (regra, arquivo, linha, severidade); o
corpo, em pt-BR, traz esses campos, a descrição **redigida** de novo (num
bloco de código, sem markdown nem menção) e uma sugestão. Uma issue cuja
primeira linha não é o marcador deste plugin nunca é tocada. A saída do
comando é só a linha de contagens (abertas, mantidas, fechadas, reabertas):
nenhum título, arquivo ou descrição vai para o log. Exit: 0 sincronizado; 1
recusa, varredura inconclusiva ou falha da API (só o código HTTP, nunca o
corpo da resposta); 2 uso inválido. Uma falha no meio para na primeira
escrita que falhou; a próxima varredura converge a partir do estado atual.

## O exemplo do harness

`demo/plugins/vvah/`:

- `Dockerfile`: `python:3.12-slim` fixado por digest, `git`, o harness
  instalado por `pip` no commit fixado
  `4128e870377850db16cb082115105cc8ee6f1a72`, usuário não root, estado e
  `run_manifest` fora da árvore (`WORKDIR /tmp`).
- `perfil.py`: gera no build o perfil "só descoberta" a partir do
  `default.yaml` do próprio pacote, com estas edições (se o `default.yaml`
  mudar, o build falha):
  todo papel de modelo em `"${VVAH_MODEL}"` com `provider: anthropic` (as
  aspas são necessárias); `step1.auto_exclude: false`;
  `step6_exploit_verification.enabled: false` (sem exploração viva: com
  `EV_API_COLLECTION` presente o harness recusa, exit 2); `scan_progress`
  desligado; `output.ingest_url`, `ingest_token` e `ingest_verify` removidos
  (sem envio a hub). S10/S11 (`step_remediate`, `step_validate`) já vêm
  desligados e são conferidos.
- `entrypoint.sh`: roda `vvaharness scan --repo /alvo --config
  /cfg/descoberta.yaml --stop-after s9` com `VVAHARNESS_NO_LOCAL_CONFIG=1` e
  apaga as variáveis de rastreamento (`LANGSMITH_*`, `LANGCHAIN_*`) e de envio
  a hub antes de executar.

Provedor padrão documentado: Anthropic com `claude-haiku-5-5`
(`VVAH_MODEL`), chave em `ANTHROPIC_API_KEY`. Para um endpoint compatível com
OpenAI, troque `provider: anthropic` por `provider: openai` no `perfil.py` e
use `OPENAI_API_KEY`/`OPENAI_BASE_URL`; o aviso acima sobre modelos pequenos
vale.

Configuração no repositório (copie `demo/plugins/vvah/` para ele):

```yaml
scheduled:
  plugins:
    - name: vvah
      dockerfile: plugins/vvah/Dockerfile
      env: [ANTHROPIC_API_KEY, VVAH_MODEL]
      output:
        format: vvah-findings
        path: security-scan/findings.json
      timeout_minutes: 90
```

## Licença e custo

O harness é Apache-2.0 e roda como imagem separada: o AurumCode não
redistribui código nem o perfil dele (o perfil é gerado no build a partir do
pacote instalado). Cada execução agendada chama o modelo em vários estágios
por unidade de análise; o custo cresce com o tamanho da árvore e com o
modelo escolhido, e é do dono da chave.
