# Gate corporativo: SAST, SBOM, inventário e assinatura

Este guia mostra, com arquivos completos e uma demonstração executável, como
uma organização obriga todo pull request a passar por quatro controles:

| Controle | O que faz | Onde se configura |
|---|---|---|
| SAST | Semgrep sobre a árvore inteira do repositório | `quality_gates.sast` |
| SBOM | Trivy gera um CycloneDX do repositório | `quality_gates.ssor_dtrack.sbom_generator` |
| Inventário | o SBOM vai para um OWASP Dependency-Track v5 e o gate lê as métricas do projeto | `quality_gates.ssor_dtrack` |
| Assinatura | Cosign assina o SBOM (e imagens, se houver) | `quality_gates.supply_chain` |

A referência de cada chave está em [configuration.md](configuration.md); aqui
está o conjunto que funciona junto. Todo host, projeto e organização abaixo é
um placeholder (`ORG`, `example.org`, `127.0.0.1`).

Os blocos YAML deste guia **são os arquivos de `demo/gate-corporativo/`**, byte
a byte (`tests/acceptance/AUR-554.sh AC-001` compara). Copie-os como estão e
troque só os placeholders.

## 1. Política central

A organização mantém um repositório de política (aqui `ORG/aurumcode-policy`)
cujo diretório contém `.aurumcode/config.yml` e as regras. O repositório do
dev **não** consegue afrouxar nada disso: sob política central, `rules`,
`ignore`, `gate`, `exceptions` e cada seção de `quality_gates` do repositório
são ignorados, com um aviso nomeado.

<!-- arquivo: demo/gate-corporativo/politica/.aurumcode/config.yml -->
```yaml
gate:
  fail_on: [high]
  inconclusive: block
quality_gates:
  sast:
    engine: semgrep
    enabled: true
    fail_on_severity: ERROR
    rule_packs: ["/github/policy/regras/sast-demo.yml"]
  ssor_dtrack:
    enabled: true
    server_api_host: "http://127.0.0.1:8081"
    api_key_secret: DTRACK_API_KEY
    project_id_secret: DTRACK_PROJECT_ID
    thresholds: {max_critical: 0, max_high: 0, policy_violations: 0}
    timeout_seconds: 180
    poll_interval_seconds: 5
    sbom_generator:
      tool: trivy
      format: cyclonedx
      spec_version: "1.6"
      output_file: sbom_app_cyclonedx.json
  supply_chain:
    engine: cosign
    sign_sbom: true
    sign_artifacts: false
```

- `gate.fail_on: [high]` reprova achados de severidade alta ou acima;
  `gate.inconclusive: block` faz uma revisão inconclusiva (provedor fora,
  scanner quebrado, servidor de inventário inalcançável) **reprovar** em vez de
  passar em silêncio. É também o padrão: sem a chave, com `gate` declarado ou
  um scanner habilitado, o inconclusivo reprova. Use `warn`, escrito, só
  durante a adoção.
- Chave desconhecida (erro de digitação como `fial_on`) na política **ou** no
  `.aurumcode/config.yml` do repositório é erro de carga, antes do modelo, com a
  chave nomeada; `quality_gates.sast.engine` diferente de `semgrep` também.
- `quality_gates.sast`: `rule_packs` aceita `p/...` do registro do Semgrep
  (precisa de rede) ou arquivos de regra locais, determinísticos e offline,
  como abaixo. O caminho `/github/policy/...` é onde o workflow reutilizável
  monta o repositório da política dentro do container.
- `quality_gates.ssor_dtrack`: `server_api_host` precisa ser `https://` em
  produção (por exemplo `https://dtrack.example.org`); `http://127.0.0.1:...`
  existe só para o servidor local da demonstração. `api_key_secret` e
  `project_id_secret` são **nomes** de variáveis de ambiente, nunca valores.
  `thresholds` compara `critical`, `high` e `policyViolationsTotal` das
  métricas do projeto; `policy_violations: 0` reprova qualquer violação de
  política do Dependency-Track.
- `quality_gates.supply_chain`: `sign_sbom: true` assina o SBOM. Em CI a
  assinatura é keyless (identidade OIDC do job, sem chave para guardar).

### Regras de SAST locais

<!-- arquivo: demo/gate-corporativo/politica/regras/sast-demo.yml -->
```yaml
rules:
  - id: demo-sem-eval
    languages: [javascript]
    severity: ERROR
    message: "eval() executa texto como codigo; use um parser ou uma tabela de operacoes"
    pattern: eval(...)
```

O achado aparece como `semgrep:<check_id>`; o `check_id` do Semgrep é derivado
do caminho do arquivo de regra (`github.policy.regras.demo-sem-eval`, porque
a política é montada em `/github/policy`). Um achado de severidade `ERROR`
reprova (`fail_on_severity: ERROR`); abaixo do limiar ele é publicado sem
reprovar. Em produção, troque ou some o arquivo por `p/security-audit` etc.

## 2. Configuração do repositório

O repositório do dev só precisa do básico; o gate vem da política.

<!-- arquivo: demo/gate-corporativo/repo-exemplo/.aurumcode/config.yml -->
```yaml
review:
  language: en
  publication: comments
```

## 3. Workflow obrigatório da organização

<!-- arquivo: demo/gate-corporativo/workflow-obrigatorio.yml -->
```yaml
# Workflow obrigatorio da organizacao (repositorio do dev ou ruleset da organizacao).
# ORG, <SHA...> e os secrets sao placeholders: troque pelos seus.
name: Revisao obrigatoria AurumCode
on:
  pull_request:

jobs:
  review:
    # Um workflow reutilizavel nao concede permissao que o chamador nao tem:
    # id-token: write e necessario para a assinatura keyless (Cosign).
    permissions:
      contents: read
      pull-requests: write
      statuses: write
      checks: read
      id-token: write
    uses: ORG/AurumCode/.github/workflows/review.yml@<SHA-FIXA-DE-40-HEX-DO-AURUMCODE>
    with:
      policy_repository: ORG/aurumcode-policy
      policy_ref: <SHA-FIXA-DE-40-HEX-DA-POLITICA>
    secrets:
      LLM_API_KEY: ${{ secrets.LLM_API_KEY }}
      LLM_BASE_URL: ${{ secrets.LLM_BASE_URL }}
      DTRACK_API_KEY: ${{ secrets.DTRACK_API_KEY }}
      DTRACK_PROJECT_ID: ${{ secrets.DTRACK_PROJECT_ID }}
```

- Fixe `review.yml` e `policy_ref` por **SHA de 40 hex**, nunca por branch ou
  tag: é isso que garante que todo PR da organização é julgado pela mesma
  política até alguém, de propósito, apontar outra SHA.
- O workflow publica o status `aurumcode/policy-gate`; ele só bloqueia o merge
  se a branch (ou o ruleset da organização) o exigir.
- Publique o SARIF no code scanning em um segundo job do seu próprio workflow
  (`security-events: write`): veja a seção "Trilha de auditoria e SARIF" de
  [configuration.md](configuration.md).

**Como o workflow reutilizável encadeia os passos (`.github/workflows/review.yml`,
AUR-555).** A ordem é: SBOM (`aurumcode sbom`, Trivy fixado por digest), depois
a revisão (gate: SAST, envio do SBOM ao Dependency-Track e leitura das
métricas), depois a assinatura (Cosign), que só roda quando a revisão termina
com sucesso: um artefato reprovado não é assinado. O bundle `.sigstore.json`
sobe como artefato do job. `DTRACK_API_KEY` e `DTRACK_PROJECT_ID` são secrets
opcionais do `workflow_call` (`required: false`), expostos só ao passo da
revisão; o workflow chamador os repassa em `secrets:` (como no bloco acima), ou
usa `secrets: inherit`, como o `code-review.yml` deste repositório. Sem
`ssor_dtrack` nada disso é exigido; com `ssor_dtrack` e secret ausente o gate
fica inconclusivo (`dtrack_secret_missing`) e, com `inconclusive: block`,
reprova, nunca aprova. Limites: os nomes repassados são os padrão
(`DTRACK_API_KEY`/`DTRACK_PROJECT_ID`); se `api_key_secret`/`project_id_secret`
forem renomeados, o workflow não repassa os novos nomes. Se o passo de SBOM
falha, o job falha antes da revisão. O que o AUR-555 provou é a estrutura do
YAML por testes; este guia e a demonstração **não** provam a execução em um
runner real do GitHub. A demonstração (seção 7) reproduz a mesma ordem
(`sbom`, `review`, `sign`) em containers locais.

## 4. Secrets

| Nome | Para quê | Onde fica |
|---|---|---|
| `LLM_API_KEY`, `LLM_BASE_URL` | provedor do modelo | secrets da organização/repositório |
| `DTRACK_API_KEY` | chave de API do time no Dependency-Track | secret; o nome vem de `api_key_secret` |
| `DTRACK_PROJECT_ID` | UUID do projeto do serviço | secret; o nome vem de `project_id_secret` |

Nada disso entra em Markdown ou YAML versionado. A chave do Dependency-Track é
registrada no filtro de redação assim que é lida: não aparece no parecer, na
auditoria, no SARIF nem nos logs. Dê ao time dono da chave só
`BOM_UPLOAD`, `VIEW_PORTFOLIO` e `VIEW_POLICY_VIOLATION`, e crie **um projeto
por serviço**: enviar SBOMs de serviços diferentes para o mesmo projeto faz um
sobrescrever o outro.

## 5. O que reprova e o que é inconclusivo

Reprova (código de saída 3 no `--base`; status `aurumcode/policy-gate`
`failure` no PR):

- achado do Semgrep na severidade de `fail_on_severity` ou acima, nomeando a
  regra e a linha (`src/calc.js:5 ... semgrep:github.policy.regras.demo-sem-eval`);
- métrica do Dependency-Track acima do limite
  (`ssor_dtrack: policy_violations 1 > policy_violations 0`,
  `critical 3 > max_critical 0`);
- achado de skill da política acima de `gate.fail_on`;
- falha de assinatura: `aurumcode sign` nunca tem modo "warn".

Inconclusivo (nunca aparece como "aprovado"; com `inconclusive: block`, ou
sem a chave, reprova; só `warn` escrito avisa): Semgrep ausente ou com saída inválida (`sast_*`), Trivy falhou
(`sbom_generation_failure`), servidor de inventário fora do ar, lento ou com
erro HTTP (`dtrack_unreachable`, `dtrack_timeout`, `dtrack_http_error`),
métricas incompletas (`dtrack_metrics_incomplete`: ausente nunca é lido como
zero), secret ausente (`dtrack_secret_missing`), SBOM ausente
(`dtrack_sbom_unavailable`), falha do provedor do modelo (`provider_failure`).

Em produção, as métricas `critical` e `high` vêm dos espelhos de
vulnerabilidade do **próprio servidor** (NVD, GitHub Advisories, OSV...),
sincronizados pelo Dependency-Track. Um servidor recém-criado não os tem; por
isso a demonstração usa uma política de violação do Dependency-Track
(coordenadas do componente), que dá `policy_violations > 0` de forma
determinística, sem depender de rede.

## 6. Onde ver auditoria, SARIF e métricas

- **Parecer**: no PR (status `aurumcode/policy-gate`) e na saída do comando,
  com as linhas `policy gate: ...` nomeando regra, limiar e origem.
- **Auditoria**: `--auditoria <arquivo>` grava o JSON com o digest da política,
  o SHA do workflow, a decisão do gate e as exceções aplicadas. O workflow
  reutilizável envia o artefato `aurumcode-audit-<PR>`.
- **SARIF**: `--sarif <arquivo>`; artefato `aurumcode-sarif-<PR>`, publicável
  no code scanning.
- **Métricas e violações**: na interface do Dependency-Track (projeto do
  serviço) ou pela API (`/api/v1/metrics/project/<uuid>/current`,
  `/api/v1/violation/project/<uuid>`), como `run.sh` faz.
- **Assinatura**: o bundle `<sbom>.sigstore.json` sai como artefato
  `aurumcode-sbom-bundle-<PR>`. Para verificar um SBOM assinado em CI
  (keyless) veja "Verificação por terceiros" em
  [configuration.md](configuration.md); na demonstração a chave é efêmera:

```sh
cosign verify-blob --key cosign.pub --bundle sbom_app_cyclonedx.json.sigstore.json \
  --insecure-ignore-tlog --insecure-ignore-sct sbom_app_cyclonedx.json
```

## 7. Demonstração executável

`demo/gate-corporativo/run.sh` sobe tudo em containers fixados por digest
(`images.lock`): imagem do produto (com Semgrep), Dependency-Track v5 +
PostgreSQL, Trivy e Cosign. No host só há bash, git, docker, curl e python3.
Precisa de rede para baixar as imagens e de uns 2 GB de disco.

```sh
demo/gate-corporativo/run.sh all      # build up fail fix pass verify down
demo/gate-corporativo/run.sh --check  # compara out/ com expected/ (sem docker)
```

Cada fase grava `demo/gate-corporativo/out/<fase>.log`; `expected/<fase>.txt`
tem os trechos que `--check` exige.

| Fase | O que acontece |
|---|---|
| `build` | constrói a imagem do produto, confere o Semgrep contra `scanners.yml`, fixa e extrai Trivy e Cosign |
| `up` | sobe Dependency-Track + PostgreSQL, troca a senha inicial, cria time, chave, projeto e a política de violação |
| `fail` | `aurumcode sbom` + `aurumcode review`: reprova no SAST e no inventário |
| `fix` | remove o `eval` e sobe o `lodash` 4.17.15 para 4.17.21 |
| `pass` | o gate aprova, o SBOM é enviado e assinado |
| `verify` | `cosign verify-blob` aceita a assinatura e rejeita um SBOM adulterado |
| `down` | remove containers, rede e volumes |

Defeitos plantados em `repo-exemplo/`: `src/calc.js` chama `eval(...)` (regra
local do Semgrep) e `package-lock.json` fixa `lodash` 4.17.15, que a política
`demo-componente-proibido` do Dependency-Track reprova.

Saída real de `fail` (trecho de `out/fail.log`):

```text
sbom: CycloneDX 1.7, componentes: lodash@4.17.15
aurumcode review: policy gate: semgrep:github.policy.regras.demo-sem-eval - eval() executa texto como codigo; use um parser ou uma tabela de operacoes (rule semgrep:github.policy.regras.demo-sem-eval) (severidade error, limiar error, origem sast, secao policy)
aurumcode review: policy gate: ssor_dtrack: policy_violations 1 > policy_violations 0
**Verdict:** Changes requested
src/calc.js:5: [error] eval() executa texto como codigo; use um parser ou uma tabela de operacoes (rule semgrep:github.policy.regras.demo-sem-eval)
aurumcode review: exit_code=3
dependency-track metricas: critical=0 high=0 policyViolationsTotal=1
dependency-track violacao: politica=demo-componente-proibido componente=lodash versao=4.17.15 estado=FAIL
RESULTADO: gate reprovou, como esperado
```

E de `pass`:

```text
sbom: CycloneDX 1.7, componentes: lodash@4.17.21
aurumcode review: policy gate: ssor_dtrack: aprovado (critical=0, high=0, policy_violations=0)
**Verdict:** Approve
aurumcode review: exit_code=0
dependency-track metricas: critical=0 high=0 policyViolationsTotal=0
dependency-track violacoes: nenhuma
RESULTADO: gate aprovou
sign: sbom /github/workspace/sbom_app_cyclonedx.json -> /github/workspace/sbom_app_cyclonedx.json.sigstore.json
```

O gate roda sem provedor de modelo real: `AURUMCODE_LLM_FIXTURE` aponta para
uma resposta vazia (`fixture-llm.json`) e `--seguranca` liga o passe
determinístico, de modo que o parecer vem só das ferramentas. No aceite, a linha
`ssor_dtrack: aprovado (critical=0, high=0, policy_violations=0)` é exigida:
sem ela um servidor inalcançável (inconclusivo) poderia passar por aprovação.

A prova completa (data, digests, log) está em
[specs/AUR-554.md](specs/AUR-554.md).

## 8. Como pedir exceção

Uma exceção é uma entrada em `exceptions` **na política central** (a do
repositório do dev é ignorada), aprovada por quem é dono da política, para um
achado exato:

<!-- exemplo: nao e arquivo da demo -->
```yaml
exceptions:
  - repo: ORG/servico-exemplo
    rule: semgrep:github.policy.regras.demo-sem-eval
    path: src/calc.js
    owner: time-seguranca
    reason: expressao fixa em tempo de build, sem entrada do usuario
    expires: 2026-12-31
```

Os seis campos são obrigatórios; `path` é exato (sem glob); `expires` é
`YYYY-MM-DD` em UTC. Vencida, a exceção para de valer sozinha e o achado volta a
reprovar. O fluxo: abra um PR no repositório da política com a entrada acima e
o motivo; o time de segurança aprova; depois do merge, o `policy_ref` do
workflow obrigatório é atualizado para a nova SHA.
