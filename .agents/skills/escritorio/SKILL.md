---
name: escritorio
description: >
  Coordena o ciclo leve do board AurumCode: mede progresso, roda preflight,
  despacha um builder isolado, conduz exatamente um review no git do SHA
  imutavel e integra so o SHA aprovado. Git so no host; o container so executa
  codigo e testes.
---

# Escritorio AurumCode

Esta skill e controle operacional. Ela nao autoriza inventar cards, nao substitui
evidencia, nao move cards sozinha e nunca e dependencia do runtime da aplicacao.
O card e a autoridade do trabalho; este arquivo e apenas o procedimento para
operar o card.

Arquivo canonico: `.agents/skills/escritorio/SKILL.md`.
`.claude/skills/escritorio/SKILL.md` e um symlink para ele. Nunca crie uma
segunda copia nem edite uma das duas isoladamente: em 2026-08-12 uma copia
desatualizada fez o coordenador despachar a cerimonia proibida de Reviewer A/B
por tres dias de board.

## Fonte de verdade e legado congelado

Leia antes de agir: `AGENTS.md`, `.board/README.md`, `.board/AGENT_PLAYBOOK.md`
e o card inteiro.

- Gate estrutural: `bash .board/pipeline.sh` (exit 0 = verde). Leia sempre.
- Monitor: `.board/office-cycle.sh` (`--status`, `--start`, `--review`).
- Preflight: `PREFLIGHT_RUN=1 bash .board/card-preflight.sh AUR-NNN /worktree`.
- Um erro de pipeline, preflight, imagem, runtime, engine, loader ou dependencia
  e bloqueio. Nunca o converta em RED comportamental, review aprovado ou `done`.
- Um arquivo, JSON ou stdout dizendo `approved`, `valid` ou `authenticated` e
  observacao nao confiavel, nunca autoridade.
- **Legado congelado, proibido para cards ativos:** `.board/validate.sh`,
  `.board/REVIEW_PROTOCOL.md`, `.board/bin/second-reader`, a cerimonia de
  Reviewer A/B / segundo leitor / aprovador cetico / test designer separado, e
  evidence bundles manuais. Nao os execute, nao use o exit code deles e nao os
  cite como autoridade. Eles governam apenas o historico dos cards que ja estavam
  em `done` antes do ciclo leve; `.agents/skills/escritorio/frota.sh` e um
  diagnostico do modelo antigo de subagentes, tambem legado.

## Limite duro: git no host, container so executa codigo

- **Git sempre no host.** Branch, worktree, commit, merge/rebase, diff, `git log`
  e `git notes` rodam na maquina do coordenador. **Nunca execute `git` dentro do
  container:** o container existe para compilar e testar, nao para versionar.
- **Nada de Go, binario ou `go test` de codigo em revisao no host.** Um unico
  container de trabalho compartilhado (`aurum-go`) atende builder, revisor e
  coordenador, com o mesmo cache, para nao recompilar do zero e nao deixar lixo.
  - `./.board/bin/go-shared up` sobe ou reaproveita (idempotente).
  - `./.board/bin/go-shared exec -w <worktree> go test ...` executa.
  - `./.board/bin/go-shared status` inspeciona; `down` encerra.
- Nenhum agente cria imagem, container ou volume proprio. `./.board/bin/office-clean`
  fecha a sessao e remove somente o que e nosso.
- O aceite selado `oci-run` e o portao de integracao do coordenador, executado
  uma vez por card no worktree candidato; nao e ferramenta de rodada por agente.

## Review no git, no SHA imutavel

- O review acontece **no git do host**: o revisor le o diff
  `git -C <worktree> diff <base>..<sha-candidato>` e cobre cada hunk do SHA
  imutavel, a partir do worktree limpo do candidato.
- O card **permanece `ready` durante o review**; review nao move card. A fila
  ativa (`doing`/`review`/`validating`) nao recebe um candidato ainda nao
  aprovado.
- O parecer do revisor e gravado no proprio git:
  `git notes --ref=reviews add -f <sha-candidato> <<'EOF' ... EOF`.
- So depois de APPROVE o coordenador integra exatamente aquele SHA em `main`.
  Um SHA novo nunca herda aprovacao antiga; REQUEST_CHANGES exige correcao e
  novo review do novo SHA.

## Orcamento de papeis por card (limite duro)

Um card consome no maximo tres papeis: builder, **um** reviewer e um validator.
O coordenador nao ocupa nenhum deles e nao os substitui. Um unico agente pode
acumular reviewer e validator sobre o mesmo SHA imutavel, e essa e a forma
preferida.

Sao papeis proibidos, e `.board/pipeline.sh` rejeita a linguagem deles em
qualquer card nao concluido: Reviewer A, Reviewer B, segundo reviewer, terceiro
reviewer, aprovador cetico, segundo leitor e test designer separado. Se o texto
de um card ainda pedir esses papeis, o texto e legado stale: normalize o card
antes de despachar, nunca obedeca a ele.

Nao despache uma frota de agentes por card. Um builder, um reviewer, um
validator. Paralelismo so entre cards cujos paths sao disjuntos e cujas
dependencias estao em `done`; a maior frota segura e limitada pelos recursos
reais, nunca pelo numero de slots imaginados.

## Escopo do briefing (limite duro)

O que o coordenador pede a um builder ou a um reviewer e limitado pelo card:
Outcome, Non-goals, Acceptance scenarios, Public contract e `paths`. Um briefing
nao amplia contrato. Antes de despachar, releia os Non-goals: eles dizem o que o
card explicitamente NAO faz, e sao a fronteira que o briefing nao atravessa.

Reaproveitar entre cards o ataque que funcionou no card anterior acelera a
revisao e e recomendado, mas cada ataque herdado so entra no briefing depois de
mapeado para uma clausula deste card. Escreva o mapeamento junto do ataque. Se
voce nao consegue nomear a clausula que o ataque testa, o ataque esta fora de
escopo e nao vai no briefing.

Reconheca a classe do card antes de escolher ataque. Um card que publica
documento, schema, lock ou entrada de registry e validado por loader com zero
engine call: ataque-o pela validacao, pela cadeia de digests, pela aridade e
pela ordem. Ataca-lo pela execucao do que o documento descreve testa
comportamento que o card nao realiza, e produz finding contra algo que o card
declarou como nao-objetivo.

Um finding fora de contrato custa caro duas vezes: gasta o revisor e pode fazer
o coordenador mandar um builder consertar o que o card nunca prometeu, que e
trabalho inventado. Em 2026-08-12 o briefing do AUR-411 pediu que o revisor
tentasse instalar runtime por gerenciador de pacote e por download, sendo que o
card diz em Non-goals que apenas pina particao ja materializada e que o profile
nem chega a ser executado. O briefing foi corrigido durante a revisao.

## Ciclo, Ralph loop e parada

1. Rode `bash .board/office-cycle.sh --status` e `bash .board/pipeline.sh`.
2. Rode `bash .board/office-cycle.sh --start` uma vez no inicio da serie de
   ciclos. O estado fica fora do repositorio por padrao.
3. A cada janela real de 20 minutos, rode
   `bash .board/office-cycle.sh --review` antes de escolher trabalho.
4. O script conta `done_delta`. Dois reviews sem aumento encerram a abordagem:
   exit 75 significa parar, classificar a causa e mudar de lane/processo.
5. Atividade de agente, worktree novo, pipeline estrutural verde ou texto de
   progresso **nao** contam como progresso.
6. O monitor nao faz dispatch automatico. A coordenacao e explicita, para que
   uma sessao curta nao continue uma abordagem ja condenada. Um Ralph loop que
   atravessa janelas de 20 min so continua enquanto `done_delta` cresce; ao
   bater exit 75, pare e reclassifique.

## Fila e isolamento

- So `cards/ready` autoriza novo builder. Backlog e apenas especificacao.
- Confirme dependencias em `done`, posse unica e paths disjuntos antes de
  despachar. Nunca duplique builder ou worktree.
- Cada builder usa worktree proprio e devolve patch mais saida bruta; nao move
  card, nao aprova o proprio trabalho e nao escreve evidencia final.
- Toda lane, inclusive vazia, permanece representada por arquivo rastreado;
  valide o pipeline em clone limpo, pois diretorio local nao e artefato Git.
- O checkout coordenador pode permanecer sujo. Toda acceptance, review e
  validacao final rodam em worktree limpo do SHA candidato.
- Execute `oci-run` com cwd no worktree candidato. Path absoluto para o script
  nao muda a raiz resolvida por `git rev-parse` e pode testar o checkout errado.
- O worktree de review precisa estar limpo e no SHA exato; nao reutilize o
  worktree do builder, que pode conter trabalho fora do commit.

## Preflight que nao pode ser pulado

Antes de qualquer builder, reviewer ou validator, leia o card completo e rode o
preflight no worktree limpo. Ele exige:

- paths e read_paths canonicos; paths de candidato ativo existem e sao tracked;
- `validation`, `container_profile` e `profile_owner` declarados antes de
  `ready`; o dono deve ser upstream no DAG e estar registrado em
  `.board/profile-owners.tsv`;
- `read_paths: []` e obrigatorio quando vazio; qualquer card Go declara
  `go.mod` e `go.sum` em `read_paths`;
- o write-set cobre semanticamente cada artefato que Outcome, Postconditions,
  Public contract e Green mandam criar ou alterar. Se o card promete registrar,
  publicar ou atualizar um arquivo listado apenas em `read_paths`, corrija o card,
  rode o pipeline e reancore antes do primeiro builder;
- o write-set nao concede escrita sobre diretorio ou artefato que Non-goals,
  Compatibility ou Postconditions mandam preservar. Inputs apenas observados
  pertencem a `read_paths`, mesmo em cards de caracterizacao;
- acceptance executavel, `bash -n` valido e mutacao observavel;
- `accept` exatamente igual a
  `./.board/bin/oci-run --profile <profile> --card <card>`;
- profile registrado, lock existente, imagem digest-pinada e engine disponivel;
- smoke test real da imagem pinada com `bash`; se o acceptance mencionar Go,
  smoke test real com `go` na mesma imagem;
- candidato que publica profile OCI prova tambem cada imagem dos profiles que
  possui, mesmo que seu proprio acceptance rode no bootstrap. O probe exige
  `bash` porque `oci-run` sempre entra por Bash e exige Go quando o plano declara
  command/cache Go; validar apenas JSON/lock nao prova profile executavel;
- card com camada ou comando Go materializa `go.mod` e `go.sum` em
  `paths`/`read_paths`, sem modulo substituto criado pelo acceptance;
- cada arquivo tracked em `paths`/`read_paths` passa pelo mesmo scanner de
  credential shapes do `oci-run`. Fixture sintetica monta tokens em runtime ou
  divide o literal no source; nunca desabilite ou allowliste o scanner para
  fazer a materializacao passar;
- worktree limpo e, com `PREFLIGHT_RUN=1`, exit real do acceptance.

Para `ready`, o preflight de builder valida contrato, dependencias, posse,
profile, lock, imagem e runtime base, mas nao exige ainda os artifacts de
`paths` nem executa acceptance ausente. Reviewer/validator continuam exigindo
todos os paths tracked, acceptance executavel e exit real. Essa distincao evita
que cards novos fiquem impossiveis de despachar por ainda nao terem sido
construidos.

Quando todos os `paths` de um card `ready` ja estao rastreados, o preflight o
classifica como candidato completo e aplica automaticamente os checks fortes e
o acceptance nominal. Isso impede reviewer em `ready` de herdar o modo frouxo
reservado ao builder inicial.

Quando `PREFLIGHT_RUN=1` executa a acceptance nominal, somente exit `0` e
verde. Exit `1` e falha do card, nao RED aceitavel; apenas codigos explicitos
de infraestrutura podem ser classificados como indisponibilidade.

Uma imagem Go sem `bash` nao passa: o runner executa a acceptance com `bash`.
Uma imagem Bash sem Go nao passa para acceptance que chama Go. AUR-006 mostrou
que detectar somente o nome da imagem ou somente o host nao e suficiente.
Exit 125 do engine antes do processo e infraestrutura; 126/127 por entrypoint ou
ferramenta ausente dentro da imagem sao erro de contrato do profile, nao
indisponibilidade para empurrar ao consumidor.

Profile ausente ou incapaz e trabalho do coordenador, nao motivo para esperar o
usuario. Antes do builder, localize o owner do registry/schema/profile/lock e o
execute; se o contrato nao tiver owner viavel, repare `paths`, dependencia e
DAG, rode o pipeline e construa os artefatos checked-in. Nunca fabrique apenas
um profile local/untracked, mas tambem nunca deixe um consumidor parado quando
o requisito pode ser materializado por um card owner. AUR-402 e dono do
registry e AUR-403 e dono do profile Go nesta reconstrucao.

Ao adicionar uma chave a registry ou um novo tipo de profile, prove antes do
dispatch que o write-set inclui o registry canonico e um schema que aceita o
documento nominal sem afrouxar profiles existentes. Rode tambem o acceptance do
owner anterior como regressao. Um profile validado apenas por loader paralelo em
teste, mas ausente do registry canonico, nao satisfaz um contrato de registro.

### Feasibility e DAG antes do dispatch

- Antes de repetir um preflight, derive a matriz `card -> acceptance -> profile
  -> capabilities` e confirme que o perfil concede todos os recursos que o
  aceite realmente precisa. Em particular, `go test` precisa de imagem Go,
  Bash, cache offline, armazenamento temporario gravavel e um ponto de
  execucao que nao seja `noexec`.
- Se essa matriz for impossivel, nao classifique a falha como RED nem gaste
  novas rodadas ajustando cache. Pare a abordagem, identifique o card owner do
  recurso ausente e repare a ordem do DAG para que esse owner possa executar.
- AUR-006 valida o schema; AUR-402 valida o registry; AUR-403 e o owner do
  profile Go. A ordem operacional deve ser `AUR-233 -> AUR-402 -> AUR-403 ->`
  consumidores Go quando o aceite de um consumidor exigir o profile Go.
- Toda alteracao de dependencia precisa ser aplicada no card, passar pelo
  pipeline e reancorar o candidato; nunca mover arquivo de lane para esconder
  dependencia. Depois de corrigir o DAG, executar o owner real antes de
  retomar o consumidor.

## Fluxo de entrega leve (passo a passo canonico)

1. `backlog -> ready` so quando toda `depends_on` esta em `done`, os campos
   `validation`, `paths`, `read_paths`, `container_profile` e `profile_owner`
   estao explicitos, e o owner do profile e upstream. `ready` autoriza **um**
   builder isolado, depois do preflight de builder.
2. O builder implementa somente `paths`, prova baseline/mutacao/restore, e cria
   um commit com a identidade humana ja configurada, sem qualquer atribuicao de
   IA. O card **continua `ready`**.
3. **Exatamente um reviewer independente** revisa o commit imutavel no git do
   host (ver "Review no git"). Cada selector Unit/Contract/Integration/E2E
   declarado deve executar uma assercao real: exit 0 com `[no test files]`, zero
   testes, branch vazia ou selector que retorna sem chamar a camada e veto,
   mesmo que o nominal agregado esteja verde. Alem do `accept` declarado, o
   reviewer roda `go test` sobre **todo** pacote em `paths`, nao so os arquivos
   que o diff tocou, porque um `accept` estreito pode ficar cego a regressao em
   teste pre-existente. Um unico agente pode ser reviewer e validator do mesmo
   SHA.
4. Apos APPROVE, o coordenador integra **exatamente aquele SHA** em `main` (por
   exemplo fast-forward do candidato linear), adiciona ao card o
   `## Delivery record` com `- commit: <SHA que ficou em main>`, `- review:
   approved`, e move o card para `validating` (ou `review`).
5. Se o card declara `validation: tested` ou `skeptical`, o validator executa o
   acceptance do proprio card no mesmo SHA, em worktree limpo, e adiciona
   `- validation: passed`. Exit 0 e evidencia; 69/79 e inconclusivo; exit 1 so e
   RED depois que o programa realmente iniciou.
6. `done` so quando o Delivery record esta completo para o kind declarado e
   `bash .board/pipeline.sh` passa. Apenas o coordenador integra e move cards.

Nenhum agente pode substituir review, mutacao ou evidencia por prosa.

## Evidencia minima

Cards com `validation: tested|skeptical` em `done` precisam de
`.board/evidence/AUR-NNN/validated.json` com o card, o SHA completo, o review
aprovado, a validacao passada e pelo menos um `exit_code: 0`. O pipeline recusa
`done` sem essa correspondencia. Um JSON de evidencia e dado, nao decisao: nao
autoriza transicao por si so.

## O card registra o SHA integrado, nunca o do candidato (limite duro)

Registre em `- commit:` e em `validated.json` o SHA que existe em `main` depois
da integracao, nao o tip da branch de candidato. O candidato e efemero por
construcao: ele so existe enquanto a branch `card/AUR-NNN` o segura, e some no
primeiro `gc --prune=now`.

Em 2026-08-26, ao consolidar o repositorio numa branch so -- apagando branches
mergeadas e podando -- doze cards `done` passaram a apontar para objetos
coletados, e `pipeline.sh` recusou o board. Os objetos nao estavam no `origin`.
A recuperacao foi impossivel; o conserto foi reapontar cada card para o commit
que de fato entrega seus `paths` em `main`.

Antes de podar objetos ou apagar branch de candidato, rode a auditoria barata:
para todo card `done`, `git cat-file -t <sha>` e
`git merge-base --is-ancestor <sha> main`. Um card que falha em qualquer um dos
dois afirma entrega sem lastro inspecionavel.

## Progresso e continuidade

Ao final de cada ciclo relate somente: `done_delta`, cards fechados, blocker
medido, cards despachados e proximo gate. Nao emita promessa de conclusao
enquanto houver backlog, ready, doing, review, validating ou blocker de
especificacao/infraestrutura.

Se a sessao terminar, deixe handoff do ai-memory com o resultado de
`pipeline.sh`, a fila `ready`, os cards tocados e o proximo comando
verificavel. O agente seguinte deve rerodar `pipeline.sh` antes de confiar no
handoff.

---

# Licoes historicas que continuam valendo

## Todo despacho carrega prazo (limite duro)

Nenhum builder ou revisor sai sem prazo explicito no briefing. O briefing termina
com: "PRAZO DURO: N minutos. Ao vencer, commite o que existir e reporte, mesmo
incompleto, declarando o que ficou de fora. Nao investigue alem disso." Um agente
sem prazo trata o problema como aberto e reescreve o diagnostico do zero em vez de
entregar; um agente com prazo devolve trabalho parcial util, que e integravel.

Um agente pensando e um agente travado escrevem a mesma coisa: nada. Do lado de
fora nao ha como distinguir, entao a unica evidencia honesta e tempo sem escrita
em disco. `bash .board/bin/office-watch [minutos]` lista os worktrees parados
(exit 3 se houver algum) e serve de fonte para um monitor de fundo. Cobrar o
agente e trabalho do coordenador: se o usuario precisa avisar que um agente esta
parado, o processo falhou, nao o usuario.

Ao vencer o prazo, leia o worktree antes de decidir -- `git status --porcelain` e
`git log -1` dizem se ha trabalho a salvar. So entao escolha: cobrar entrega
parcial com sequencia numerada minima, redespachar do zero, ou assumir o card.
Nunca deixe a decisao para a proxima vez que o usuario perguntar.

## read_paths nomeia pacote, nao arquivo (limite duro)

`read_paths` que enumera arquivo por arquivo apodrece em silencio. O pacote e a
unidade de compilacao: nomear `pkg/extractor.go` materializa um pacote
incompleto assim que alguem adicionar `pkg/native.go`, e o aceite selado quebra
com `undefined: <simbolo>` enquanto o mesmo fonte compila no host. Foi assim que
o AUR-002 carregou um exit 1 por dias lido como deriva de caracterizacao: o
AUR-427 adicionou `native.go` a `extractors/rust` e `extractors/csharp`, e o
`read_paths` do AUR-002 citava so `extractor.go` de cada um.

Use forma de diretorio para dependencia de codigo. `undefined:` num aceite
selado que passa no host e sintoma desta classe, nao de comportamento: procure o
`read_paths` antes de suspeitar do produto.

E quando um builder falsificar o diagnostico do coordenador com execucao,
acredite nele e verifique voce mesmo. O relatorio honesto de "o que voce me
mandou nao reproduz" vale mais que um conserto que fecha o card.

## `strings.Contains` num portao e quase sempre defeito

Dois revisores independentes acharam a mesma classe de bug em cards diferentes
no mesmo dia. Em `tests/integration/AUR-002.go:430` o oraculo de conteudo usava
`Contains`, entao ACRESCENTAR sufixo a um permalink passava verde --
`permalink: /go/root/` e prefixo de `permalink: /go/root/qualquer/`, e so
substituicao falhava. Em `internal/documentation/welcome/sanitize.go:80` a
sanitizacao de link usava `Contains`, entao
`notes/docs/getting-started.md-old.md` e
`evil/docs/getting-started.md/../../etc/passwd` escapavam intactos.

Quando um portao decide se algo e permitido, `Contains`, `HasPrefix` e
`HasSuffix` cada um deixa passar uma forma diferente. Normalize primeiro
(remova ancora, prefixo `./`, espaco) e compare por IGUALDADE, ou case a linha
inteira. Ao revisar, ataque todo comparador de string de portao com quatro
formas: prefixo, sufixo, no meio, e com travessia de caminho. Um caso vivo:
uma aceitacao que so rejeitava `npm ci|go test|make` deixava passar
`bash .aurumcode-target/build.sh`, provando "nao executa codigo do PR" sem
provar nada.

## Mutacao nao ancora nos bytes que outro card vai mudar

Uma mutacao de aceite precisa achar seu alvo por uma ancora ESTAVEL -- o
identificador da regra, o nome da funcao, a chave de metadado -- e nunca pelo
conteudo literal que ela reescreve. Ancorar nos bytes do proprio conteudo faz o
aceite quebrar no dia em que outro card evoluir aquele conteudo, e o sintoma
(`anchor-not-found`, `anchor-not-unique`) nao aponta para a causa.

Aconteceu duas vezes em 2026-08-26. O `MUT-001` do AUR-448 ancorava numa linha
`fmt.Fprintf` que deixou de ser unica quando o AUR-467 acrescentou outro print
de forma igual. O `MUT-001` do AUR-462 ancora na LINHA INTEIRA do pattern de
`security/command-injection`, entao qualquer card que estenda aquela regra --
como o AUR-481, que precisa acrescentar a grafia de Rust -- invalida o aceite
alheio.

Ao escrever mutacao: ancore no identificador estavel e reescreva a linha
seguinte. E faca a reescrita sair diferente de zero quando nao disparar, para
que uma mudanca futura quebre alto em vez de transformar o mutante em no-op
silencioso -- um mutante que nao muda nada deixa o aceite verde sem provar coisa
alguma.

## Nenhum codigo em revisao sem teto de recurso

O sandbox `oci-run` impoe 256 MB por cgroup. Um binario candidato executado no
host as 20:02 de 2026-08-12 (`aurumcode-bin`) estourou para ~31 GB de RSS e
derrubou a maquina com todos os agentes. Por isso: Go nunca no host, sempre no
`go-shared`; e qualquer outro comando pesado que precise rodar fora do container
leva `ulimit -v` (ex.: 2 GiB), `GOMEMLIMIT` (ex.: 1GiB) e `-timeout` explicito.
Se o comando nao sobrevive ao limite, isso e um achado contra o candidato, nao
motivo para soltar o limite.

## OCI e aceite

- `oci-run` materializa somente paths allowlisted e executa em rede none, rootfs
  read-only, sem socket, mount de host, device ou capability.
- O runtime smoke do preflight e repetido pelo `oci-run` antes de criar o
  materializer. Se os probes discordarem, pare.
- Docker/Podman ausente, imagem nao local, timeout ou cache Go indisponivel sao
  inconclusivos. Nao sao sucesso e nao sao falha de comportamento.
- Antes de chamar um wrapper de travado ou reinicia-lo, inspecione uma vez o
  processo do container dentro do timeout. Compilacao ativa nao e loop: preserve
  o limite e elimine recompilacoes identicas reutilizando cache privado entre
  mutacoes sequenciais. Nao repita o mesmo comando sem mudanca de hipotese,
  candidato ou instrumentacao.
- Acceptance que copia inputs read-only para staging deve tornar somente seu
  proprio `mktemp` gravavel no `trap` e nunca deixar erro de cleanup sobrescrever
  um resultado nominal verde. O cleanup deve ser idempotente.

## Modelo por papel (recomendacao)

Builder e revisor rodam em modelo mais barato; o modelo mais forte fica para
especificacao, planejamento, coordenacao e julgamento de evidencia. Nao promova
um builder para o modelo mais forte porque o card parece dificil: se o briefing
precisa disso para ser executado, o briefing esta incompleto, e a correcao e
escrever o diagnostico medido dentro dele.

Se um modelo atinge limite de uso e os agentes morrem, isso NAO significa conta
esgotada: teste um modelo menor antes de concluir que nao ha agente disponivel.
Em 2026-08-14 o escritorio foi declarado parado por essa conclusao apressada,
quando o modelo menor estava disponivel o tempo todo.
