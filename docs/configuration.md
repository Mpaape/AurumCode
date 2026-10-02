# Configuração

Sem arquivo: inglês, comentário na conversa e sem comentários nas linhas.
Para mudar, crie `.aurumcode/config.yml`:

```yaml
review:
  language: pt-BR
  publication: review
  inline_comments: true
```

`publication: review` usa a revisão formal do GitHub. `comments` publica na
conversa. `inline_comments` habilita comentários nas linhas; no review formal,
uma sugestão elegível pode aparecer como substituição aplicável pelo GitHub.
O autor decide se aplica. AurumCode não altera o código automaticamente.

O idioma é enviado ao modelo. Os títulos do parecer têm tradução específica
para português e inglês; os demais idiomas aceitos usam títulos em inglês.
Tags aceitas: `pt-BR`, `pt`, `en-US`, `en`, `es-ES`, `fr-FR`,
`de-DE`, `it-IT`, `ja-JP`.

## Prompts, skills e docs

Escreva orientações em `.aurumcode/prompt.md`; o arquivo é encontrado
automaticamente. Para expandir:

```yaml
review:
  language: pt-BR
  context:
    skills:
      - .aurumcode/skills/backend.md
    docs:
      - docs/architecture.md
```

Uma skill é um Markdown com orientações de revisão, sem execução de scripts.
Liste apenas arquivos existentes. `context.prompt` permite substituir o
caminho do prompt adicional, mantendo a política embutida do produto.

O PR usa configuração e contexto da branch base; o idioma pode vir da versão
do PR. Isso significa que um novo prompt só passa a orientar reviews depois
de integrado à base. O uso local lê os arquivos do checkout.

Exemplo de prompt:

```text
Revise correção e compatibilidade dos contratos públicos.
Valores monetários são armazenados em centavos inteiros.
Use os testes e contratos disponíveis para sustentar cada achado.
Sugira código apenas quando a substituição for local e segura.
Indique contexto ausente sem presumir um defeito.
```

As contribuições são contexto para o modelo. Não alteram permissões,
redação de segredos nem opções do programa. A implementação atual aceita até
64 KiB por contribuição e informa erro se esse tamanho for excedido.

## Modelo e credenciais

Credenciais ficam nos secrets `LLM_API_KEY` e `LLM_BASE_URL` do repositório
hospedeiro. O exemplo passa a variável `LLM_MODEL` para o workflow reutilizável.
Nenhuma credencial deve estar em Markdown, YAML versionado ou na página.

O modelo é escolhido pelo serviço quando não há identificador explícito.
Não há limite de saída imposto por padrão pelo AurumCode; o serviço continua
sujeito à janela de contexto, ao timeout e às restrições do modelo.

## Opções avançadas

- `ignore`: lista de globs de caminhos a excluir antes da análise.
- `rules`: overrides explícitos de regras reconhecidas, por identificador.
- `review.memory`: `off` (padrão, sem estado), `ephemeral` (em processo) ou
  `local` (persistido por repositório no diretório de cache). No review de PR,
  o arquivo fica em `$XDG_CACHE_HOME/aurumcode/memory/repo/OWNER/REPO/notes.json`
  (ou no cache padrão do sistema quando `XDG_CACHE_HOME` está ausente).
  Sem coordenadas do GitHub, a identidade usa um hash do remote origin ou do
  caminho absoluto do checkout. O antigo cache global não é importado nem apagado.
  Memória guarda observações de
  revisões anteriores para reduzir repetição; nunca altera regras, severidade
  ou veredito.
- Workflow reutilizável: `model`, `publication`, `inline_comments`, `security`.
  Ele publica um status de commit; só bloqueia merge se exigido pela branch.
- Action Docker direta: usa `Mpaape/AurumCode@main`, exige
  `GITHUB_TOKEN`, `LLM_API_KEY`, `LLM_BASE_URL` no ambiente e evento de PR.
  Acrescenta inputs `check` e `fail-on`; não coleta CI automaticamente.
  Quem monta o próprio job (em vez do workflow reutilizável, que já faz isso)
  precisa chamar `actions/checkout` com
  `ref: ${{ github.event.pull_request.head.sha }}` antes da Action: o padrão
  do `actions/checkout` num evento `pull_request` é o merge ref sintético
  (`refs/pull/<n>/merge`), cujo commit não é o head revisado. Nesse caso o
  checkout local diverge do HEAD que a API reporta para o PR, e o contexto
  de codebase (AUR-515/AUR-536) é omitido por esse descompasso de HEAD; a
  revisão continua apenas com o diff remoto:

  ```yaml
  - uses: actions/checkout@v4
    with:
      ref: ${{ github.event.pull_request.head.sha }}
  - uses: Mpaape/AurumCode@main
    env:
      GITHUB_TOKEN: ${{ github.token }}
      LLM_API_KEY: ${{ secrets.LLM_API_KEY }}
      LLM_BASE_URL: ${{ secrets.LLM_BASE_URL }}
  ```
- Localmente, `.aurumcode/instructions/*.md` pode usar front matter
  `applyTo` para escopo por caminho. O fluxo remoto usa os arquivos
  explicitamente listados em `review.context`.

Sem configuração, o review já inclui análise estática determinística, contexto
de codebase limitado, resumo e diagrama Mermaid; essas capacidades funcionam
com os padrões, sem nenhum arquivo. `aurumcode fix` converte as sugestões de
uma revisão em um diff unificado aplicável.

`inline_comments: true` na configuração é cumulativo com o input do workflow;
para desligá-lo, remova-o do arquivo ou defina false e não habilite o input.

## Política central

Um workflow obrigatório pode carregar uma política central: outro diretório
(checkout próprio) que CONTÉM `.aurumcode/config.yml` e os mesmos arquivos
Markdown (`prompt.md`, `skills/*.md`, docs) que o repositório do dev já usa —
não é o diretório `.aurumcode/` em si, é o diretório pai dele. Nenhum formato
novo.

```yaml
# <diretório da política>/.aurumcode/config.yml (outro repositório/checkout)
rules:
  security/hardcoded-secret:
    enabled: true
ignore:
  - "vendor/**"
review:
  context:
    skills:
      - skills/security.md
```

O workflow passa esse diretório ao AurumCode com `--politica <dir>` (alias
`--policy`); sem a flag, a variável de ambiente `AURUMCODE_POLICY` é usada;
sem nenhum dos dois, o comportamento é o de hoje, sem política. O CLI
recusa, fechado, um `<dir>` que resolva (depois de symlinks) para dentro da
árvore sob revisão (o diretório de trabalho do processo) ou para ela mesma:
a política tem que vir de fora do que está sendo revisado, nunca o repositório
revisado pode fornecer a própria política.

No workflow reutilizável (`.github/workflows/review.yml`), `policy_repository`
(`owner/repo`) é a única forma de declarar uma política: o próprio workflow
dá checkout read-only (sem persistir credenciais) em `.aurumcode-policy` e
usa esse diretório como `--politica`. Não existe um input `policy_path` nesse
workflow — num job `workflow_call`, os únicos diretórios alcançáveis são o
checkout da ferramenta e o checkout do PR sob revisão, então aceitar "um
caminho já presente" deixaria o próprio PR apontar para a própria política.
`policy_ref` escolhe o que o checkout busca; vazio usa o branch padrão do
repositório da política.

Exemplo de workflow obrigatório da organização, chamando o reutilizável com a
política embutida:

```yaml
jobs:
  review:
    uses: SuaOrg/AurumCode/.github/workflows/review.yml@<sha-fixa-do-aurumcode>
    with:
      policy_repository: SuaOrg/aurumcode-policy
      policy_ref: a1b2c3d4e5f6...  # SHA fixa, não um branch
    secrets:
      LLM_API_KEY: ${{ secrets.LLM_API_KEY }}
      LLM_BASE_URL: ${{ secrets.LLM_BASE_URL }}
```

`policy_ref` deve ser uma SHA fixa, pela mesma razão que o workflow já exige
uma SHA fixa do próprio AurumCode (o step "Verify tool version"): um branch
ou tag é mutável, então fixá-la é o que garante que toda PR da organização é
julgada pela MESMA política até alguém, deliberadamente, apontar para outra
SHA — sem isso, uma mudança na branch da política (um push aceitável ou não)
muda o gate de todo repositório que a usa, sem revisão própria desse PR.

Na Action Docker direta (`action.yml`), quem escreve os steps do job é quem
controla o que foi checado antes do container rodar, então `policy_path`
continua existindo lá como o único mecanismo (aponta para um diretório já
presente no workspace do runner, fora da árvore do repositório sob revisão).

Precedência: com política ativa, `rules` e `ignore` do repositório do dev são
ignorados por completo — vale só o que a política declara — e cada override
ignorado gera um aviso no terminal e no PR publicado, nomeando a regra ou o
padrão. `review.language` e `review.publication` vêm da política quando ela os
declara; o resto de `review` (contexto, memória, changelog, versão, perfis)
continua do repositório do dev. As skills e docs da política chegam ao
modelo primeiro; as do repositório do dev somam-se depois, sem substituir
nada. Uma política ausente ou inválida (config.yml faltando, YAML inválido,
skill/doc listada que não existe, ou um diretório dentro da própria árvore
revisada) falha o comando antes de qualquer chamada ao modelo.

## Gate: skills viram regra citável, e a política decide o que reprova (AUR-519)

Cada seção `## ` de cada skill Markdown — as da política e as do próprio
repositório — é lida a cada execução e vira uma regra citável, com id
`<nome-do-arquivo-da-skill>#<slug-da-seção>` (minúsculas, qualquer sequência
de caracteres não alfanuméricos some num único `-`, sem `-` nas pontas; ex.:
`security.md` com `## No Hardcoded Secrets` vira
`security#no-hardcoded-secrets`). Título = o texto do cabeçalho. Descrição =
o corpo da seção (limitado a 400 caracteres). Severidade do próprio achado é
`warning` por padrão; uma seção pode declarar a sua própria na primeira
linha do corpo, exatamente `severity: error` (ou `warning`/`info`) — qualquer
outra grafia é ignorada e o padrão vale. Acrescentar uma seção nova a uma
skill já configurada passa a ser citável na execução seguinte, sem mudança
de código (AC-007). Um achado que cita uma skill ou seção que não existe é
descartado e contado como não vinculado, exatamente como hoje um `rule_id`
desconhecido já é (o aviso de descarte do terminal/PR cobre os dois casos).

A política (nunca o repositório sozinho, a menos que ele opte) declara o que
reprova o check:

```yaml
# <diretório da política>/.aurumcode/config.yml
gate:
  fail_on: [critical, high]   # ou qualquer combinação de: critical, high,
                               # error, medium, warning, low, info
  inconclusive: block         # ou: warn (aceita também bloquear/alertar)
```

`gate.fail_on` aceita a mesma lista de severidades que `--fail-on` já aceita
(`high`/`error`, `medium`/`warning`, `low`/`info`), mais o alias `critical`
(mapeado no mesmo nível de `high`/`error` — este projeto não tem uma quarta
severidade). O limiar efetivo é o mais baixo entre as severidades listadas:
um achado do check na severidade do limiar ou acima dele reprova o check,
nomeando a skill e a seção que o sustentam (AC-001). Só contam achados cuja
regra é dinâmica E de origem aceita: sob política central, só as seções da
própria política (AC-005); sem política, só as do repositório, e somente
quando o repositório declarou seu próprio `gate` — sem isso, nada muda. O
limiar compara o MAIOR entre a severidade que o modelo deu ao achado e a
severidade que a própria seção da skill declarou (`severity:` no corpo):
a declaração do autor da skill é um piso que o texto do diff revisado não
pode rebaixar.

`gate.inconclusive` decide o que uma revisão inconclusiva faz ao check:
falha do provedor, cobertura parcial (AUR-476, com os arquivos nomeados) ou
resposta do modelo que não pôde ser interpretada como JSON (parse
degradado — hoje publicado como se a revisão tivesse funcionado). Com
`block`, a revisão reprova o check sem nunca checar achados. Com `warn`,
ou quando `gate.inconclusive` nem está declarado, a revisão continua
visível como inconclusiva mas não bloqueia por si só — **e, nos dois
casos, um achado real que cruze `fail_on` ainda reprova o check**
(`exitFindings`): ser inconclusiva nunca é uma forma de escapar de um
achado que já cruzou o limiar. Em nenhum caso o parecer aparece como
aprovado.

Sob política central, `gate` do repositório é ignorado por completo — um
aviso nomeado explica o descarte, no mesmo lugar e do mesmo jeito que os
avisos de `rules`/`ignore` já existentes.

**Falha do provedor em `--pr` (AUR-537).** Até este card, uma falha de
transporte durante a chamada ao modelo em `--pr` — todos os provedores
configurados falharam, ou `--limite` recusou a chamada antes de qualquer
provedor ser alcançado — encerrava com código 1 antes mesmo de o gate ser
avaliado: nenhum status `aurumcode/policy-gate` era publicado e
`inconclusive: warn` não era honrado, mesmo com um gate declarado. Essa
falha específica agora é roteada pelo gate como o mesmo motivo inconclusivo
que `--base` já publica (`provider_failure`): com `block`, o status falha
nomeando o motivo e a saída usa o código de "revisão não concluída"; com
`warn` (ou sem `inconclusive` declarado), o status publica sucesso com o
alerta inconclusivo visível — nunca a palavra "aprovado" — e a saída é 0; em
ambos os casos a auditoria e o SARIF (quando pedidos) são escritos como
inconclusivos, e o corpo publicado da revisão diz que ela não foi executada.
**Sem nenhum `gate:` declarado, o comportamento é idêntico ao de antes deste
card, byte a byte: código 1, nenhum status, nenhuma auditoria/SARIF.** Uma
recusa de `--limite` antes da chamada (pré-chamada) segue a mesma regra: só
entra pelo gate como esse motivo inconclusivo quando um gate está declarado.

O gate está ligado em `aurumcode review --base` e `--pr`: achados de
severidade no limiar ou acima (de origem aceita) reprovam o código de
saída (reaproveitando os mesmos códigos de `--fail-on`/`--check`), o
motivo de inconclusivo (falha do provedor, cobertura parcial, parse
degradado) entra no resumo/limitações publicados, e o veredito nunca
aparece como aprovado nesses casos. No `--pr`, o status `aurumcode/policy-gate`
é publicado junto do `aurumcode/review` que `--check` já publica, só
quando um gate foi declarado. O gate é idêntico com ou sem `--perfis`: cada
perfil selecionado aprende o mesmo catálogo dinâmico.

## Trilha de auditoria e SARIF (AUR-521)

Qualquer `aurumcode review` (`--base` ou `--pr`) pode escrever, além do que já
publica, dois arquivos adicionais para o time de segurança da organização:

```
aurumcode review --base HEAD~1 \
  --auditoria /caminho/auditoria.json \
  --sarif     /caminho/revisao.sarif
```

- `--auditoria <arquivo>`: um registro JSON com o digest da política, o SHA
  do workflow (`AURUMCODE_WORKFLOW_SHA`, com `GITHUB_SHA` como alternativa), o
  repositório, o SHA revisado, o modelo, o veredito, a decisão do gate
  (`pass`/`fail`/`inconclusive` + motivo), os achados que efetivamente
  reprovaram o gate, as exceções aplicadas (campo `exceptions_applied`,
  sempre presente como lista; AUR-520 — gravada achado por achado DENTRO do
  loop de limiar de severidade do gate, então só existe quando
  `gate.fail_on` está declarado e o loop de fato roda) e a cobertura
  (completa ou não, com os arquivos que ficaram de fora).
- `--sarif <arquivo>`: um documento SARIF 2.1.0 (`tool.driver` com as regras
  citadas, incluindo as seções dinâmicas de skill com seus títulos;
  `results` com `ruleId`, `level` (`error`/`warning`/`note`), `message`,
  `location` (arquivo relativo ao repositório + linha) e uma impressão
  digital estável por achado em `partialFingerprints`). Uma revisão
  inconclusiva ainda produz um SARIF válido, com
  `invocations[0].executionSuccessful=false` e uma notificação nomeando o
  motivo.

Nenhum dos dois é escrito sem a flag correspondente: sem `--auditoria` e sem
`--sarif`, o comportamento de hoje é idêntico, byte a byte.

O workflow reutilizável (`.github/workflows/review.yml`) escreve os dois
sempre e envia AMBOS como artefatos do job via `actions/upload-artifact`
(`if: always()`, para que um gate reprovado -- o caso que mais importa --
ainda produza evidência; um arquivo vazio, de uma rodada que nunca chegou a
escrevê-lo, nunca é enviado): `aurumcode-sarif-<PR>` e
`aurumcode-audit-<PR>`.

O workflow reutilizável **nunca** chama `github/codeql-action/upload-sarif`
ele mesmo. Essa action exige `security-events: write`, e uma reusable
workflow não consegue conceder a si mesma uma permissão que o CALLER não já
tem: se este workflow declarasse esse `permissions:` sozinho, toda chamada
cujo caller não concedesse o mesmo pararia de rodar -- não só o upload, o
job inteiro, para todo caller existente (`code-review.yml` deste
repositório, os exemplos, qualquer workflow de outro repositório que já
use este). Em vez disso, quem quer o SARIF no code scanning roda um
SEGUNDO job, no seu próprio workflow (onde conceder permissão a si mesmo é
normal, sem cruzar fronteira de reusable workflow), que baixa o artefato e
faz o upload:

```yaml
jobs:
  review:
    uses: ./.github/workflows/review.yml
    with:
      security: true
    secrets: inherit

  upload-sarif:
    needs: review
    # !cancelled() (não always()): o job de review FALHA quando o gate
    # reprova (exit 1/3) -- exatamente o caso em que o upload mais
    # importa -- e !cancelled() ainda roda nesse caso, só pulando um
    # cancelamento explícito do workflow.
    # A segunda condição pula PRs de fork: neles o token não recebe
    # security-events: write e o upload falharia.
    if: ${{ !cancelled() && github.event.pull_request.head.repo.full_name == github.repository }}
    runs-on: ubuntu-latest
    permissions:
      contents: read
      actions: read            # necessário para download-artifact em repo privado
      security-events: write
    steps:
      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1
        with:
          name: aurumcode-sarif-${{ github.event.pull_request.number }}
          path: .
      - uses: github/codeql-action/upload-sarif@2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2 # v4.38.2
        with:
          sarif_file: aurumcode-review.sarif
          category: aurumcode-policy-gate   # categoria fixa: um upload --pr
                                             # (ou um futuro run agendado)
                                             # atualiza a MESMA análise no
                                             # code scanning em vez de
                                             # acumular um conjunto de
                                             # alertas que nunca é limpo
```

Um PR de fork nunca recebe `security-events: write` (o GITHUB_TOKEN de um
`pull_request` vindo de fork é somente leitura para esse escopo). Sem a
condição de fork no `if`, o job `upload-sarif` rodaria e falharia (403 no
upload, ou artefato ausente quando o review não recebe secrets). Com ela, o
job é pulado nesses PRs; quando o review gera o SARIF, ele continua
disponível como artefato, só não chega ao code scanning automaticamente.

O `code-review.yml` deste próprio repositório ainda não tem esse segundo
job -- está fora dos `paths` da AUR-521 e não foi criado por este card; até
que alguém o adicione, o SARIF deste repositório fica disponível como
artefato do job de review, mas não chega ao code scanning.

A impressão digital de cada achado (`internal/render.FindingFingerprint`) é a
identidade canônica de um achado neste projeto — a mesma que a AUR-494 deve
reaproveitar quando existir, nunca redefinir: regra + caminho + linha +
contexto de código normalizado, nunca o texto livre do modelo isoladamente, e
nunca um valor por execução (hora, nonce). O mesmo achado produz sempre a
mesma impressão digital, nesta execução ou em qualquer execução futura.

Os dois arquivos passam pelo mesmo filtro de redação único (AUR-009) que
qualquer outro destino deste processo usa: nenhum segredo (nem um valor
registrado em `AURUM_SECRET_CANARY`) sobrevive ao texto serializado.

## Exceções aprovadas: dono e validade (AUR-520)

`exceptions` é uma lista simples, no mesmo `config.yml` (do repositório ou da
política central), de exceções já aprovadas para um achado exato — falso
positivo ou risco aceito:

```yaml
exceptions:
  - repo: org/repo
    rule: seguranca.md#sql-injection   # secao da skill (dinamica) ou id de advisory
    path: legacy/report.py             # caminho exato, relativo ao repositório
    owner: time-seguranca
    reason: consulta fixa, sem entrada do usuario
    expires: 2026-12-31                # YYYY-MM-DD, sempre em UTC
```

Todos os seis campos são obrigatórios; falta de `owner`, `reason` ou
`expires`, ou uma `expires` que não seja exatamente `YYYY-MM-DD` (uma data
com fuso, hora, ou qualquer outro formato é recusada), invalida a política
inteira antes de qualquer chamada ao modelo (AC-005, falha fechado) — uma
exceção que um humano não assinou com essa precisão nunca é tratada como
ausente. `path` é sempre um caminho exato, nunca um glob: a exceção cobre
exatamente o achado que alguém revisou, nunca uma família de arquivos.

Uma exceção só se aplica quando `repo`, `rule` e `path` casam exatamente com
o achado (o `rule_id` e o arquivo publicados) E a data de hoje (UTC) é menor
ou igual a `expires`: o achado some do gate e aparece no resumo/limitações
como "aceito por exceção", com dono, motivo e validade (AC-001). Uma exceção
vencida para de valer sozinha — o achado volta a reprovar o check
normalmente, e a saída diz que a exceção venceu (AC-002). Uma exceção para
outro repositório, outra regra ou outro caminho simplesmente não casa
(AC-003). A identidade do repositório nunca vem do modelo ou do diff
revisado: no `--pr` é o `owner/repo` já autenticado pela própria chamada à
API; no `--base` vem do remoto `origin` do checkout local (os mesmos
mecanismos de leitura do AUR-515) — quando ela não pode ser confirmada,
nenhuma exceção com `repo` declarado casa (falha fechado), e a saída diz por
quê.

Sob uma política central, só as exceções DA POLÍTICA valem — exatamente como
`rules`/`ignore`/`gate` já funcionam: uma exceção declarada no config do
repositório é ignorada por completo, com um aviso nomeando a regra e o
caminho descartados (AC-004). O repositório sozinho não consegue criar uma
exceção para uma regra da política.

## Opções públicas

Esta é a superfície pública: o arquivo `.aurumcode/config.yml`, as flags do CLI
e as entradas do workflow. Variáveis usadas apenas pelos testes internos do
projeto não fazem parte desta referência e não devem ser configuradas pelo
consumidor.

### `.aurumcode/config.yml`

| Chave | Efeito | Padrão |
|---|---|---|
| `review.language` | Idioma enviado ao modelo e títulos do parecer | inglês |
| `review.publication` | `review` (revisão formal) ou `comments` (conversa) | `comments` |
| `review.inline_comments` | Comentários nas linhas alteradas | `false` |
| `review.context.prompt` | Caminho do prompt adicional | `.aurumcode/prompt.md` |
| `review.context.skills` | Lista de Markdown de orientação | vazio |
| `review.context.docs` | Lista de documentos de contexto | vazio |
| `review.memory` | `off`, `ephemeral` ou `local` | `off` |
| `review.changelog` | Publica versão sugerida e entrada de changelog | `off` |
| `review.version` | Versão-base `major.minor.patch` do changelog | `0.0.0` |
| `review.profiles` | Perfis de revisor executados na mesma revisão | vazio |
| `rules.<id>.enabled` | Liga/desliga uma regra reconhecida | embutido |
| `rules.<id>.severity` | Sobrescreve a severidade de uma regra | embutido |
| `ignore` | Globs de caminhos removidos antes da análise | vazio |
| `gate.fail_on` | Severidades (do vocabulário de `--fail-on`, mais `critical`) que reprovam o check | vazio (sem gate) |
| `gate.inconclusive` | `block` ou `warn` para uma revisão inconclusiva | vazio (sem gate) |
| `exceptions` | Exceções aprovadas (repo+rule+path, dono, motivo, validade) que tiram um achado exato do gate | vazio |

### CLI `aurumcode review`

| Flag | Efeito |
|---|---|
| `--base` | Diffa a referência contra `HEAD` (uso local) |
| `--fail-on` | Teto de severidade que faz o comando sair com código 3 |
| `--modelo` | Modelo que revisa (endpoint compatível com OpenAI ou fixture offline) |
| `--seguranca` | Soma o passe determinístico de segurança |
| `--pr`, `--repo`, `--publicar` | Revisa e publica em um pull request do GitHub |
| `--modo-publicacao` | `review` ou `comments` na publicação do PR |
| `--na-linha` | Inclui achados elegíveis comentados na linha exata |
| `--check` | Publica status de commit que bloqueia merge em achado grave |
| `--limite` | Teto em USD estimado antes de chamar o modelo |
| `--exigir-qualidade` | Falha se a revisão por modelo não aconteceu |
| `--changelog` | Força a seção de changelog |
| `--perfis`, `--profile` | Perfis de revisor selecionados para a revisão |
| `--politica`, `--policy` | Diretório que contém o `.aurumcode/config.yml` de uma política central, com precedência sobre `rules`/`ignore`/idioma/publicação do repositório (padrão: `AURUMCODE_POLICY`) |
| `--auditoria` | Caminho para escrever o registro de auditoria JSON desta execução (padrão: não escreve) |
| `--sarif` | Caminho para escrever o documento SARIF 2.1.0 desta execução (padrão: não escreve) |

### CLI `aurumcode fix`

| Flag | Efeito |
|---|---|
| `--file` | Arquivo JSON com sugestões ou resposta de revisão (padrão: stdin) |

### Workflow reutilizável e Action

- Workflow reutilizável: `model`, `publication`, `inline_comments`, `security`,
  `policy_repository` (`owner/repo` de uma política central; o próprio
  workflow faz o checkout, sem persistir credenciais — não há `policy_path`
  nesse workflow, só o repositório da política pode fornecer uma),
  `policy_ref` (branch/tag/SHA da política, fixe em SHA; vazio usa o branch
  padrão). Nenhum definido mantém o comportamento sem política.
- Action Docker direta: `publication`, `inline-comments`, `security`, `check`,
  `fail-on`, `model`, `changelog`, `policy_path` (diretório, já no workspace
  do runner e controlado por quem escreveu o job, que contém o
  `.aurumcode/config.yml` de uma política central).
