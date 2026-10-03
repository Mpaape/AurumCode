# Tutorial: auditoria e SARIF

## Objetivo

Mostrar onde ficam o registro de auditoria e o SARIF de uma revisão, como ler
cada campo, como o SARIF chega ao Code Scanning pelo job do chamador, e provar
que um segredo não vaza para nenhum dos dois. Seis fases (cinco executadas e uma
conferência).

Os blocos são os arquivos de `demo/tutoriais/auditoria-sarif/` e as saídas vêm de
`demo/tutoriais/auditoria-sarif/out/`. Referência:
[configuration.md, Trilha de auditoria e SARIF](../configuration.md#trilha-de-auditoria-e-sarif-aur-521).

## Pré-requisitos

- `git`, `docker`, `bash`, `python3` e a imagem do produto, como em
  [revisao.md](revisao.md); sem rede, modelo falso.
- Ler [gate.md](gate.md) e [excecoes.md](excecoes.md): a auditoria registra a
  decisão do gate e as exceções.

```bash
bash demo/tutoriais/auditoria-sarif/run.sh all
bash demo/tutoriais/auditoria-sarif/run.sh --check
```

## Onde ficam

Nenhum dos dois é escrito sem a flag: `--auditoria ARQUIVO` e `--sarif ARQUIVO`.
No workflow reutilizável o produto escreve os dois e o job os envia como
artefatos (`aurumcode-audit-<PR>` e `aurumcode-sarif-<PR>`), mesmo quando o gate
reprova. Em execução local, o caminho é o que você passar.

## Caso 1: a auditoria de uma revisão que reprova

```bash
aurumcode review --base main --politica /caminho/da/politica --modelo modelo-demo --auditoria auditoria.json
```

<!-- saida: auditoria-reprovado -->
```text
exit_code=3
RESULTADO: a revisao reprova e a auditoria e escrita mesmo assim
chaves: policy_digest, workflow_sha, repo, reviewed_sha, model, verdict, gate, blocking_findings, exceptions_applied, coverage
policy_digest: 64 caracteres hex
workflow_sha: 4444444444444444444444444444444444444444
repo: OWNER/REPO
reviewed_sha: 3333333333333333333333333333333333333333
model: modelo-demo
verdict: changes_requested
gate.decision: fail
blocking_findings: seguranca#sem-segredos-no-codigo app.go:6 error origin=skills
exceptions_applied: []
coverage.complete: True
```

O que observar, campo a campo: `policy_digest` identifica a política aplicada;
`workflow_sha` vem de `AURUMCODE_WORKFLOW_SHA` (ou `GITHUB_SHA`), `repo` de
`GITHUB_REPOSITORY` e `reviewed_sha` de `GITHUB_SHA` (a demo os fixa); `model` é
o `--modelo`; `verdict` e `gate.decision` dizem o resultado; `blocking_findings`
lista só o que reprovou, cada um com `origin` (`skills`, `analysis`, `sast`);
`exceptions_applied` lista as exceções aceitas (vazia aqui); `coverage` diz se a
revisão foi completa. Os campos de `analysis_data` e de SBOM **só aparecem quando
a política os declara** (`analysis_data`, SBOM com Trivy); **não demonstrado
aqui:** o SBOM exige o scanner e rede, e o `analysis_data` exige um servidor de
releases.

## Caso 2: o SARIF

```bash
aurumcode review --base main --politica /caminho/da/politica --sarif revisao.sarif
```

<!-- saida: sarif-campos -->
```text
exit_code=3
RESULTADO: o SARIF e escrito com o gate reprovado
version: 2.1.0
tool.driver: AurumCode
executionSuccessful: True
result: seguranca#sem-segredos-no-codigo level=error app.go:6 origin=skills
partialFingerprints: aurumcode/findingId/v1
```

O que observar: SARIF 2.1.0; cada achado vira um `result` com `ruleId`, `level`
(`error`/`warning`/`note`), arquivo relativo e linha; `partialFingerprints`
traz a impressão digital estável do achado (regra, caminho, linha e contexto
normalizado, nunca o texto livre do modelo); `properties.origin` existe para
os achados que o gate contou.

## Caso 3: uma revisão inconclusiva também gera os dois

<!-- saida: sarif-inconclusivo -->
```text
exit_code=1
RESULTADO: inconclusivo em block reprova
auditoria gate: decision=fail reason=review inconclusive (partial_coverage)
auditoria coverage.complete: False
sarif executionSuccessful: False
sarif notificacoes: ['partial_coverage']
```

O que observar: `gate.decision` é `fail` com o motivo `partial_coverage`,
`coverage.complete` é falso, e o SARIF continua válido com
`executionSuccessful: false` e uma notificação nomeando o motivo. Nunca há
documento de "aprovado" para uma revisão que não aconteceu.

## Caso 4: canário de redação

O modelo falso devolve um achado cuja mensagem contém o valor
`CANARIO-LOCAL-7f3a91c2`, e a variável `AURUM_SECRET_CANARY` o registra como
segredo:

<!-- saida: canario-de-redacao -->
```text
app.go:6: [error] Credencial literal [REDACTED] proibida pela politica. (rule seguranca#sem-segredos-no-codigo: Sem segredos no codigo)
exit_code=3
RESULTADO: a revisao reprova
stdout/stderr: canario ausente
auditoria.json: canario ausente
revisao.sarif: canario ausente
revisao.sarif: o valor foi trocado por [REDACTED]
RESULTADO: o canario nao vazou para saida, auditoria nem SARIF
```

O que observar: o valor aparece como `[REDACTED]` no parecer e no SARIF, e não
existe em lugar nenhum da saída, da auditoria ou do SARIF. A demo falharia
(`ERRO:`) se o canário vazasse.

## Caso 5: upload para o Code Scanning pelo job do chamador

O workflow reutilizável **nunca** chama `upload-sarif`: essa action pede
`security-events: write`, que um workflow reutilizável não consegue conceder ao
chamador. Quem quer o SARIF no Code Scanning roda um segundo job, no próprio
workflow:

<!-- arquivo: demo/tutoriais/auditoria-sarif/workflow-chamador.yml -->
```yaml
# Workflow do chamador: revisao pelo reutilizavel e upload do SARIF para o Code Scanning.
# OWNER e os SHAs sao placeholders: troque pelos seus.
name: Revisao com SARIF
on:
  pull_request:

permissions:
  contents: read
  pull-requests: write
  statuses: write
  checks: read

jobs:
  review:
    uses: OWNER/AurumCode/.github/workflows/review.yml@0000000000000000000000000000000000000000
    with:
      security: true
    secrets:
      LLM_API_KEY: ${{ secrets.LLM_API_KEY }}
      LLM_BASE_URL: ${{ secrets.LLM_BASE_URL }}

  upload-sarif:
    needs: review
    if: ${{ !cancelled() && github.event.pull_request.head.repo.full_name == github.repository }}
    runs-on: ubuntu-latest
    permissions:
      contents: read
      actions: read
      security-events: write
    steps:
      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c
        with:
          name: aurumcode-sarif-${{ github.event.pull_request.number }}
          path: .
      - uses: github/codeql-action/upload-sarif@2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2
        with:
          sarif_file: aurumcode-review.sarif
          category: aurumcode-policy-gate
```

<!-- saida: upload-workflow -->
```text
workflow do chamador: workflow-chamador.yml
job upload-sarif: concede security-events: write a si mesmo
job review: nao pede security-events (um reutilizavel nao concede permissao ao chamador)
review.yml: nao chama upload-sarif; so publica o artefato
nome do artefato: igual no review.yml e no download do chamador
arquivo: aurumcode-review.sarif nos dois
PR de fork: o job de upload e pulado
NAO EXECUTADO AQUI: o upload ao Code Scanning (nao ha runner nem GitHub neste ambiente)
```

O que observar: esta fase **não executa** o upload (não há runner nem GitHub aqui:
`NAO EXECUTADO AQUI`); ela confere o workflow contra o `review.yml` real: o nome
do artefato e o arquivo coincidem, o job `review` não pede `security-events`, o
job de upload pede e é pulado em PR de fork. O envio de fato ao Code Scanning não
está demonstrado.

## Quando falha

O caminho da auditoria não é gravável (o diretório não existe):

<!-- saida: falha-caminho-invalido -->
```text
aurumcode review: writing audit record: open /work/nao-existe/auditoria.json: no such file or directory
exit_code=0
RESULTADO: ACHADO: a auditoria nao foi gravada (veja o aviso) e o exit continua 0; confira o arquivo no job
o arquivo de auditoria nao existe
```

O que observar, e é um **achado do tutorial**: o produto avisa em stderr
(`writing audit record: ...`) mas a saída continua 0 numa revisão limpa; a
auditoria **não existe** e nada reprova. Num pipeline de compliance, confira que
o arquivo foi criado (por exemplo, com `test -s auditoria.json` no job) em vez de
confiar só no exit code.

## Problemas comuns

- **Auditoria vazia de exceções:** `exceptions_applied` só é preenchido com
  `gate.fail_on` declarado.
- **`origin` ausente num `result`:** só os achados contados pelo gate têm origem.
- **Upload falha com 403 em PR de fork:** o token do fork não tem
  `security-events: write`; o job de upload é pulado nesses PRs e o SARIF fica só
  como artefato.
- **Nome de artefato diferente:** o download do chamador precisa usar
  `aurumcode-sarif-<número do PR>`.
