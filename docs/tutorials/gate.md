# Tutorial: o gate de política

## Objetivo

Mostrar, com execuções reais, como o gate decide se uma revisão reprova o
merge: o limiar de severidade (`gate.fail_on`), o que acontece quando a revisão
é **inconclusiva** (`gate.inconclusive: block` ou `warn`, com cada causa
executada), quais origens de achado contam (`gate.sources`: `skills`,
`analysis`, `sast`), os códigos de saída, o texto do commit status
`aurumcode/policy-gate`, e um repositório que tenta afrouxar a política e não
consegue. Dez fases rodam de verdade, cada uma afirma seu exit code.

Os blocos de configuração **são os arquivos de `demo/tutoriais/gate/`**, byte a
byte, e as saídas vêm de uma execução real registrada em
`demo/tutoriais/gate/out/` (`run.sh --check` e `tests/acceptance/AUR-562.sh`
conferem). Referência: [configuration.md, seção Gate](../configuration.md#gate-skills-viram-regra-citavel-e-a-politica-decide-o-que-reprova-aur-519)
e [gate.sources](../configuration.md#gatesources-which-findings-count-toward-the-gate).

## Pré-requisitos

- `git`, `docker`, `bash` e `python3`; a imagem do produto, como em
  [revisao.md](revisao.md). O modelo é um JSON determinístico
  (`AURUMCODE_LLM_FIXTURE`) e os containers rodam sem rede.
- Ler [politica-central.md](politica-central.md) (o gate é declarado na política) e
  [skills.md](skills.md) (as seções das skills são as regras citáveis).
- Dois recursos de apoio, ambos locais e declarados nos casos que os usam: um
  `semgrep` **falso** que falha de propósito (`fakebin/`) e um **GitHub falso** em
  `127.0.0.1` (`demo/tutoriais/_lib/github-falso.py`) que só registra o que o
  produto publica. Não são um runner nem o GitHub.

```bash
bash demo/tutoriais/gate/run.sh all      # executa os dez casos e grava out/
bash demo/tutoriais/gate/run.sh --check  # compara out/ com expected/, sem docker
```

## Como o gate decide

| Situação | Resultado | Exit |
|---|---|---|
| achado de origem aceita na severidade de `fail_on` ou acima | reprova | 3 |
| revisão inconclusiva, `inconclusive: block` | reprova, sem avaliar achados | 1 |
| revisão inconclusiva, `warn` (ou ausente) | alerta, nunca "aprovado" | 0 |
| configuração inválida | erro de carga, antes do modelo | 1 |

Achado real que cruza o limiar reprova **mesmo** numa revisão inconclusiva em
`warn`, inclusive o do passe de segurança (caso 9). `high`, `error` e `critical` são o mesmo limiar; `medium`/`warning`
incluem os avisos.

## Caso 1: `fail_on` por severidade

Três políticas, idênticas exceto pelo limiar. O modelo (fixture) acha o mesmo
problema, a primeira vez como `warning`, a segunda como `error`:

<!-- arquivo: demo/tutoriais/gate/politica-high/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
review:
  context:
    skills:
      - skills/seguranca.md
```
<!-- arquivo: demo/tutoriais/gate/politica-high/skills/seguranca.md -->
```markdown
# Seguranca da organizacao

## Sem segredos no codigo
Nenhum segredo literal e aceito em nenhum repositorio da organizacao.
```

```bash
aurumcode review --base main --politica /caminho/da/politica
```

<!-- saida: fail-on-severidade -->
```text
--- fail_on [high], achado warning
exit_code=0
RESULTADO: warning abaixo do limiar high nao reprova
--- fail_on [medium], o mesmo achado warning
aurumcode review: policy gate: seguranca#sem-segredos-no-codigo - Sem segredos no codigo (severidade warning, limiar warning, origem skills)
exit_code=3
RESULTADO: warning no limiar medium reprova (exit 3)
--- fail_on [error], achado error
aurumcode review: policy gate: seguranca#sem-segredos-no-codigo - Sem segredos no codigo (severidade error, limiar error, origem skills)
RESULTADO: error reprova com fail_on error
--- fail_on [high], achado error: high e error sao o mesmo limiar
RESULTADO: high e error sao o mesmo limiar: o achado error reprova
```

O que observar: com `fail_on: [high]` o `warning` passa (exit 0); com `medium` o
mesmo achado reprova (exit 3); o `error` reprova com `[error]` **e** com
`[high]`, porque são o mesmo limiar. A linha `policy gate:` nomeia a skill e a
seção que sustentam o bloqueio. As políticas `politica-medium` e `politica-error`
só trocam a lista de `fail_on` e estão na demo.

## Caso 2: inconclusivo, provedor ausente

Sem modelo, só a análise determinística roda. A política que bloqueia o
inconclusivo:

<!-- arquivo: demo/tutoriais/gate/politica-bloqueia/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: block
review:
  context:
    skills:
      - skills/seguranca.md
```

<!-- saida: inconclusivo-provedor -->
```text
--- inconclusive: block, sem provedor
aurumcode review: policy gate: review inconclusive (quality_skipped)
exit_code=1
RESULTADO: block: provedor ausente reprova, motivo quality_skipped
--- inconclusive: warn, sem provedor
exit_code=0
RESULTADO: warn: provedor ausente alerta e sai 0, nunca aprovado
--- inconclusive: warn com --exigir-qualidade
aurumcode review: --exigir-qualidade: the quality review did not run, so this run is not a clean review
aurumcode review: policy gate: review inconclusive (provider_failure)
RESULTADO: --exigir-qualidade reprova mesmo com warn (provider_failure)
```

O que observar: com `block` o resultado é exit 1 e motivo `quality_skipped`;
com `warn` (`politica-alerta`) o aviso fica visível e a saída é 0. A flag
`--exigir-qualidade` é independente do gate: transforma "a revisão de qualidade
não aconteceu" em exit 1 mesmo com `warn` (motivo `provider_failure`), útil para
um job que não pode confundir "só análise determinística" com "revisado".

## Caso 3: inconclusivo, cobertura parcial

Um binário no diff não é revisado; um arquivo não revisado nunca conta como
aprovado.

<!-- saida: inconclusivo-cobertura -->
```text
--- inconclusive: block
aurumcode review: policy gate: review inconclusive (partial_coverage)
  - logo.png (binary)
exit_code=1
RESULTADO: block: cobertura parcial reprova
--- inconclusive: warn
exit_code=0
RESULTADO: warn: cobertura parcial alerta e sai 0
```

O que observar: `partial_coverage` com os arquivos nomeados; `block` reprova
(exit 1), `warn` alerta (exit 0).

## Caso 4: inconclusivo, SAST falhando

O SAST (Semgrep) que falha **nunca** vale como "sem achados". A demo põe um
`semgrep` falso na frente do `PATH` (`fakebin/erro/semgrep` sai com 2;
`fakebin/invalido/semgrep` imprime `{}`, que não é um relatório). A política
liga o SAST com uma regra local, sem rede:

<!-- arquivo: demo/tutoriais/gate/politica-sast/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: block
quality_gates:
  sast:
    engine: semgrep
    enabled: true
    fail_on_severity: ERROR
    rule_packs:
      - /policy/regras/senha.yml
review:
  context:
    skills:
      - skills/seguranca.md
```
<!-- arquivo: demo/tutoriais/gate/politica-sast/regras/senha.yml -->
```yaml
rules:
  - id: senha-literal
    languages: [generic]
    severity: ERROR
    message: Senha literal atribuida a variavel.
    pattern-regex: 'dbPassword\s*:=\s*"[^"]+"'
```

<!-- saida: inconclusivo-sast -->
```text
--- semgrep falso que falha (exit 2), inconclusive: block
aurumcode review: policy gate: review inconclusive (sast_execution_error)
aurumcode review: policy gate: SAST (semgrep, origem sast, secao policy) inconclusivo (sast_execution_error)
exit_code=1
RESULTADO: block: SAST que falhou nunca vira 'sem achados' (sast_execution_error)
--- o mesmo SAST falho, inconclusive: warn
exit_code=0
RESULTADO: warn: SAST falho alerta e sai 0
--- semgrep falso com saida que nao e relatorio, inconclusive: block
aurumcode review: policy gate: review inconclusive (sast_invalid_output)
aurumcode review: policy gate: SAST (semgrep, origem sast, secao policy) inconclusivo (sast_invalid_output)
RESULTADO: block: saida invalida do SAST e inconclusiva (sast_invalid_output)
--- Semgrep REAL (regra local): a mesma politica conclui e o achado SAST reprova
aurumcode review: policy gate: semgrep:policy.regras.senha-literal - Senha literal atribuida a variavel. (rule semgrep:policy.regras.senha-literal) (severidade error, limiar error, origem sast, secao policy)
exit_code=3
RESULTADO: com o Semgrep real o SAST conclui e o achado reprova (nao e inconclusivo)
```

O que observar: `sast_execution_error` e `sast_invalid_output` seguem o
`gate.inconclusive` como qualquer outra causa (`block` exit 1, `warn` exit 0). A
última execução usa o Semgrep **real** da imagem com a regra local: conclui e o
achado SAST reprova (exit 3), o que prova que o inconclusivo veio da falha, não
do SAST em si. (Sem `rule_packs` locais, os pacotes `p/...` do registro exigem
rede.)

## Caso 5: inconclusivo, `analysis_data`

A política declara o artefato de dados de análise. Sem rede e sem cópia em cache
ele fica indisponível, e isso é inconclusivo:

<!-- arquivo: demo/tutoriais/gate/politica-analysis-data/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: block
analysis_data:
  max_age_days: 7
review:
  context:
    skills:
      - skills/seguranca.md
```

<!-- saida: inconclusivo-analysis-data -->
```text
aurumcode review: policy gate: analysis_data: revisão inconclusiva (analysis_data_unavailable): <detalhe de rede omitido do registro>
exit_code=1
RESULTADO: block: artefato de analise indisponivel reprova (analysis_data_unavailable)
```

O que observar: `analysis_data_unavailable` com `block` reprova (exit 1). O texto
de rede que o produto acrescenta varia por ambiente e é trocado por
`<detalhe de rede omitido do registro>` por um filtro declarado no `run.sh`
(`TUT_SED`). **Não demonstrado aqui:** artefato vencido (`analysis_data_stale`) e
digest divergente (`analysis_data_digest_mismatch`): exigem um servidor de
releases com um artefato adulterado ou antigo, que esta demonstração não tem; o
caminho é o mesmo (`block` reprova, `warn` alerta) e é coberto por testes Go
(`cmd/aurumcode/aur533_test.go`).

## Caso 6: `gate.sources` e a origem de cada achado

Um mesmo PR tem três achados com o limiar `error`: o do modelo citando a skill
(`skills`), o catálogo determinístico embutido (`analysis/hardcoded-secret`) e o
Semgrep (`sast`). A política `politica-fonte-skills` restringe `sources`:

<!-- arquivo: demo/tutoriais/gate/politica-fonte-skills/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  sources: [skills]
quality_gates:
  sast:
    engine: semgrep
    enabled: true
    fail_on_severity: ERROR
    rule_packs:
      - /policy/regras/senha.yml
review:
  context:
    skills:
      - skills/seguranca.md
```

<!-- saida: fontes -->
```text
--- gate.sources: todas
aurumcode review: policy gate: seguranca#sem-segredos-no-codigo - Sem segredos no codigo (severidade error, limiar error, origem skills)
aurumcode review: policy gate: semgrep:policy.regras.senha-literal - Senha literal atribuida a variavel. (rule semgrep:policy.regras.senha-literal) (severidade error, limiar error, origem sast, secao policy)
aurumcode review: policy gate: analysis/hardcoded-secret - Hardcoded secret or credential assigned inline (rule analysis/hardcoded-secret) (severidade error, limiar error, origem analysis)
exit_code=3
RESULTADO: sources todas: o gate reprova
origens na auditoria: analysis, sast, skills
--- gate.sources: skills
RESULTADO: sources skills: o gate reprova
origens na auditoria: skills
--- gate.sources: analysis
RESULTADO: sources analysis: o gate reprova
origens na auditoria: analysis
--- gate.sources: sast
RESULTADO: sources sast: o gate reprova
origens na auditoria: sast
```

O que observar: com todas as origens a auditoria registra `analysis, sast,
skills`; com `sources: [skills]`, `[analysis]` ou `[sast]` só a origem escolhida
entra no gate e na auditoria, ainda que os outros achados apareçam no parecer.
**Origem uniforme (AUR-567):** a linha de gate diz `origem <fonte>` com o mesmo
valor tipado da auditoria e do SARIF para toda fonte: `skills`, `analysis`, `sast`,
`security`, `dtrack`. No SAST a seção de configuração que decidiu (`policy` ou
`repo`) vem depois, como `secao policy`, e nunca no lugar da origem. A linha e o
relatório mostram a mesma mensagem: o título da regra deixou de ser engolido pelo
filtro de redação (`[REDACTED] secret...`) porque o id da regra e a mensagem se
juntam por ` - `, não por `:` (um `...secret: Hardcoded` parece um par chave/valor
a esse filtro).

## Caso 7: commit status `aurumcode/policy-gate` e exit codes

No GitHub, o gate vira o status `aurumcode/policy-gate` (independente do
`aurumcode/review` de `--check`). Para proteger a branch, exija o contexto
`aurumcode/policy-gate` nas regras de proteção; isso é configuração do
repositório e **não é executado aqui**. O que a demo prova é o **texto que o
produto publica**, contra o GitHub falso local (`--pr`, `--publicar`, `--check`):

<!-- saida: status-pr -->
```text
--- aprovado
exit_code=0
status publicado: context=aurumcode/review state=success
  description: nenhum achado grave no pull request #7
status publicado: context=aurumcode/policy-gate state=success
  description: aprovado: gate de política aprovado no pull request #7
--- reprovado
exit_code=3
status publicado: context=aurumcode/review state=failure
  description: 1 achado(s) grave(s) no pull request #7
status publicado: context=aurumcode/policy-gate state=failure
  description: falha: achado(s) reprovam o gate no pull request #7: seguranca#sem-segredos-no-codigo - Sem segredos no codigo (severidade error, limiar er…
--- inconclusivo
exit_code=1
  description: inconclusivo: revisão inconclusiva (bloqueio) no pull request #7: review inconclusive (partial_coverage)
```

O que observar: a **descrição** abre com a palavra do estado (`aprovado:`,
`falha:`, `inconclusivo:`) seguida do motivo; o estado vai em `state`
(`success`/`failure`). Um inconclusivo em `block` é `failure` com
`inconclusivo:`; um inconclusivo em `warn` seria `success` com a mesma palavra,
nunca `aprovado`. A descrição é cortada em 140 caracteres (termina em `…`); o
texto completo está no parecer. Os dois status são independentes: no caso
reprovado ambos falham, mas o `aurumcode/review` só considera achado grave
(`error`).

**Review formal e gate (AUR-567).** Com `gate` declarado, a review formal segue
o gate: `REQUEST_CHANGES` só quando o gate reprova (no caso reprovado acima), e
`COMMENT` quando há achados abaixo do limiar (o aprovado acima, com um aviso
abaixo de `high`); uma execução limpa continua `APPROVE`. Sem `gate` declarado o
comportamento é o de sempre: um aviso já pede mudanças. Em ambos os casos, se a
auditoria ou o SARIF pedidos não puderam ser gravados, a aprovação é retida e o
processo sai com 1.

## Caso 8: o repositório tenta afrouxar

O repositório declara um gate permissivo; a política declara `block`:

<!-- arquivo: demo/tutoriais/gate/repo-exemplo/base-afrouxa/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: warn
  sources: [skills]
```

<!-- saida: repo-afrouxa -->
```text
--- so o repositorio: inconclusive warn
aurumcode review: policy gate: review inconclusive (partial_coverage)
exit_code=0
RESULTADO: o proprio repositorio aceita cobertura parcial como alerta
--- com a politica (inconclusive block): o gate do repositorio e ignorado
aurumcode review: politica central: gate do config do repositório foi ignorado: a política central decide sozinha
exit_code=1
RESULTADO: o repositorio nao consegue afrouxar o inconclusive da politica
```

O que observar: sozinho, o repositório aceita a cobertura parcial como alerta
(exit 0); sob `--politica`, o `gate` do repositório (inclusive `sources`) é
ignorado e a política decide (exit 1).

## Caso 9: achado determinístico sem provedor

O passe de segurança (`--seguranca`) é determinístico: acha o que o catálogo
embutido descreve sem nenhum modelo. Sem provedor e com `inconclusive: warn`, o
achado `[error]` desta execução **reprova** o gate. O modo de inconclusivo
governa a ausência do parecer do modelo, não a presença de um achado:

<!-- arquivo: demo/tutoriais/gate/politica-alerta/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: warn
review:
  context:
    skills:
      - skills/seguranca.md
```

<!-- saida: achado-deterministico -->
```text
--- --seguranca, sem provedor, fail_on [high], inconclusive: warn
aurumcode review: policy gate: review inconclusive (quality_skipped)
exit_code=3
origens na auditoria: security
RESULTADO: warn: o achado [error] do passe de seguranca reprova (exit 3), origem security
--- o mesmo, inconclusive: block
RESULTADO: block: o achado reprova com exit 3, nao so o inconclusivo
--- sem achado deterministico (diff sem segredo), inconclusive: warn
No security findings.
exit_code=0
RESULTADO: warn sem achado deterministico continua so avisando (exit 0)
```

O que observar: com o segredo no diff, `warn` e `block` saem 3 (e a auditoria
nomeia a origem `security`); sem achado, `warn` volta a só avisar (exit 0) e
`block` continua reprovando só pela ausência do parecer (exit 1). Antes do
AUR-569 este mesmo diff saía 0 em `warn`: o achado aparecia no relatório e não
contava no gate, porque só os achados de `analysis/*`, do SAST e das seções das
skills contavam. Os achados do passe de segurança contam sob a origem
`analysis` de `gate.sources` (o catálogo embutido); com `sources: [skills]`
não contam. Não é demonstrado aqui o `--pr`; ele é coberto por teste do `cmd`.

## Quando falha

Um valor desconhecido em `gate.sources` é erro de carga, antes de qualquer
chamada ao modelo (a política `politica-invalida` lista `supply`):

<!-- saida: falha-fonte-invalida -->
```text
aurumcode review: central policy: parsing /policy/.aurumcode/config.yml: gate.sources: unknown source "supply" (accepted: skills, analysis, sast)
exit_code=1
RESULTADO: gate.sources com valor desconhecido falha antes de qualquer chamada ao modelo
```

O que observar: nenhum parecer, exit 1 e a mensagem de carga. Uma política com
erro **nunca** vira "sem gate": o comando falha.

## Problemas comuns

- **Exit 0 com veredito "Comment":** revisão inconclusiva sob `warn` ou sem
  `inconclusive`. Declare `inconclusive: block` na política para reprovar.
- **O repositório declarou `gate` e nada mudou:** sob `--politica` o `gate` do
  repositório é ignorado (caso 8); um aviso nomeado diz isso.
- **`gate verdict reuse unavailable`:** só informa que `AURUMCODE_CACHE_DIR` não
  está definido; veja [reaproveitamento.md](reaproveitamento.md).
- **Status ausente na PR:** o `aurumcode/policy-gate` só é publicado quando há
  `gate` declarado e `--pr --publicar --check`.
- **SAST com `p/...` falhando offline:** pacotes do registro exigem rede; use um
  arquivo de regras local, como no caso 4.
- **Exceção que não vale:** veja [excecoes.md](excecoes.md).
