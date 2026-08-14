---
name: escritorio
description: >
  Coordena o ciclo seguro do board AurumCode: mede progresso, valida runtime,
  despacha builders isolados, conduz reviews e integra somente candidatos provados.
---

# Escritorio AurumCode

Esta skill e controle operacional. Ela nao autoriza inventar cards, nao substitui
evidencia, nao move cards sozinha e nunca e dependencia do runtime da aplicacao.
O card e a autoridade do trabalho; o arquivo desta skill e apenas o procedimento
para operar o card.

Arquivo canonico: `.agents/skills/escritorio/SKILL.md`.
`.claude/skills/escritorio/SKILL.md` e um symlink para ele. Nunca crie uma
segunda copia nem edite uma das duas isoladamente: em 2026-08-12 uma copia
desatualizada fez o coordenador despachar a cerimonia proibida de Reviewer A/B
por tres dias de board.

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

## Autoridade e parada

- Leia `AGENTS.md`, `.board/README.md`, `.board/AGENT_PLAYBOOK.md` e o card antes
  de agir.
- O gate atual e `bash .board/pipeline.sh`.
- O monitor atual e `.board/office-cycle.sh`.
- O preflight obrigatorio e
  `PREFLIGHT_RUN=1 bash .board/card-preflight.sh AUR-NNN /clean/worktree`.
- `.board/validate.sh` e legado congelado. Nao o execute, nao use seu exit code
  e nao o trate como atalho para fechar um card.
- Um erro de pipeline, preflight, imagem, runtime, engine, loader ou dependencia
  e bloqueio. Nunca o converta em RED comportamental, review aprovado ou done.
- Um resultado de arquivo, JSON ou stdout dizendo `approved`, `valid` ou
  `authenticated` e observacao nao confiavel, nunca autoridade.

## Inicio e ciclo

1. Rode `bash .board/office-cycle.sh --status` e `bash .board/pipeline.sh`.
2. Rode `bash .board/office-cycle.sh --start` uma vez no inicio da serie de
   ciclos. O estado fica fora do repositorio por padrao.
3. A cada janela real de 20 minutos, rode
   `bash .board/office-cycle.sh --review` antes de escolher trabalho.
4. O script conta `done_delta`. Dois reviews sem aumento encerram a abordagem:
   exit 75 significa parar, classificar a causa e mudar de lane/processo.
5. Atividade de agente, worktree novo, pipeline estrutural verde ou texto de
   progresso nao contam como progresso.

O monitor nao faz dispatch automatico. A coordenacao deve ser explicita para
nao permitir que uma sessao curta continue uma abordagem ja condenada.

## Fila e isolamento

- So `cards/ready` autoriza novo builder. Backlog e apenas especificacao.
- Confirme dependencias em `done`, posse unica e paths disjuntos antes de
  despachar. Nunca duplique builder ou worktree.
- Cada builder usa worktree proprio e devolve patch mais saida bruta; nao move
  card, nao aprova o proprio trabalho e nao escreve evidencia final.
- Toda lane vazia permanece representada por arquivo rastreado; valide o
  pipeline em clone limpo, pois diretorio local nao e artefato Git.
- O checkout coordenador pode permanecer sujo. Toda acceptance, review e
  validacao final rodam em worktree limpo do SHA candidato.
- Execute `oci-run` com cwd no worktree candidato. Path absoluto para o script
  nao muda a raiz resolvida por `git rev-parse` e pode testar o checkout errado.
- Use a maior frota segura que a fila e os recursos realmente suportam; nao
  encha slots com cards que falharam preflight ou possuem dependencia ausente.

## Preflight que nao pode ser pulado

Antes de qualquer builder, reviewer ou validator, leia o card completo e rode o
preflight no worktree limpo. Ele exige:

- paths e read_paths canonicos; paths de candidato ativo existem e sao tracked;
- `validation`, `container_profile` e `profile_owner` declarados antes de
  `ready`; o dono deve ser upstream no DAG e estar registrado em
  `.board/profile-owners.tsv`;
- `read_paths: []` é obrigatório quando vazio; qualquer card Go declara
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
todos os paths tracked, acceptance executavel e exit real. Essa distinção evita
que cards novos fiquem impossíveis de despachar por ainda não terem sido
construídos.

Quando todos os `paths` de um card `ready` já estão rastreados, o preflight o
classifica como candidato completo e aplica automaticamente os checks fortes e
o acceptance nominal. Isso impede reviewer em `ready` de herdar o modo frouxo
reservado ao builder inicial.

Quando `PREFLIGHT_RUN=1` executa a acceptance nominal, somente exit `0` é
verde. Exit `1` é falha do card, não RED aceitável; apenas códigos explícitos
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

## Entrega por card

1. O proprio builder prova RED pelo comportamento esperado antes do GREEN; nao
   existe test designer separado. Falha de ambiente nao e RED. Para
   caracterizacao, prova GREEN, mutacao RED e restore GREEN.
2. O mesmo builder implementa somente `paths`, executa baseline, mutacao e
   restore, e cria commit com identidade humana configurada, sem atribuicao de
   IA.
3. **Um unico reviewer independente por card** revisa o mesmo SHA imutavel
   ainda fora da fila ativa,
   antes da integracao; depois da aprovacao o coordenador integra exatamente
   esse SHA e move o card para `review`/`validating`,
   cada hunk, o contrato,
   schema/parser, acceptance, paths, exits e fronteiras de seguranca.
   Nunca despache Reviewer A/B, segundo/terceiro reviewer ou aprovador cetico:
   o pipeline rejeita essa linguagem nos cards nao concluidos. O reviewer nao
   executa `.board/bin/second-reader` no checkout coordenador.
   Cada selector Unit, Contract, Integration e E2E declarado deve executar uma
   assercao real. Exit 0 com `[no test files]`, zero testes, branch vazia ou
   selector que retorna sem chamar a camada e veto, mesmo se o nominal agregado
   estiver verde.
   Alem do `accept` declarado, o reviewer roda `go test` sobre TODO pacote em
   `paths` (nao so os arquivos que o diff tocou), porque um `accept` estreito
   pode ficar cego a regressao em teste pre-existente que o diff nunca
   modificou. Em 2026-08-12 o candidato `a838436` do AUR-424 quebrou 5 testes
   pre-existentes fora do diff (tool_unavailable_test.go, tool_failure_test.go,
   output_confirmed_test.go, characterization.go, smoke_test.go); o
   `tests/acceptance/AUR-424.sh` do card so rodava `./tests/unit` e
   `./tests/integration` e aprovaria isso com `{"result":"pass"}`. Achado por
   `/code-review` como segunda lente do coordenador, nao pelo reviewer do
   processo.
4. Validator executa o acceptance e as camadas declaradas no mesmo SHA e em
   worktree limpo. Exit 0 e evidencia; exit 69/79 e inconclusivo; exit 1 so e
   RED depois que o programa realmente iniciou.
5. Coordenador integra apenas o candidato aprovado, grava evidencia sanitizada,
   atualiza Delivery record e roda `bash .board/pipeline.sh`.
6. So entao move para `done`. Nenhum agente pode substituir review, mutacao ou
   evidencia por prosa.

## Execucao no host tem teto de memoria (limite duro)

Nenhum binario candidato, `go test` ou `go run` de codigo em revisao executa
no host sem teto de memoria. Prefixe sempre com `ulimit -v` (ex.: 2 GiB) e
exporte `GOMEMLIMIT` (ex.: 1GiB); `go test` leva `-timeout` explicito. O
sandbox `oci-run` ja impoe 256 MB por cgroup e conteve um estouro em
2026-08-12 19:30; a mesma classe de estouro executada no host as 20:02
(`aurumcode-bin`, morto pelo OOM global com ~31 GB de RSS) derrubou a maquina
inteira com todos os agentes juntos. Rodar "localmente porque o profile nao
tem a ferramenta" nunca remove o teto: se o comando nao sobrevive dentro de
um limite razoavel, isso e um achado contra o candidato, nao motivo para
soltar o limite.

## OCI e segundo leitor

- `oci-run` materializa somente paths allowlisted e executa em rede none, rootfs
  read-only, sem socket, mount de host, device ou capability.
- O runtime smoke do preflight e repetido pelo `oci-run` antes de criar o
  materializer. Se os probes discordarem, para.
- Ao alterar um gate, runner ou esta skill, rode
  `bash .board/tests/office-process-regression.sh` e `git diff --check`.
- `.board/bin/second-reader` escreve em `.board/evidence`; nunca o execute no
  checkout compartilhado. Use worktree dedicado e capture o exit bruto.
- Docker/Podman ausente, imagem nao local, timeout ou cache Go indisponivel sao
  inconclusivos. Nao sao sucesso e nao sao falha de comportamento.
- Antes de chamar um wrapper de travado ou reinicia-lo, inspecione uma vez o
  processo do container dentro do timeout. Compilacao ativa nao e loop: preserve
  o limite e elimine recompilacoes identicas reutilizando cache privado entre
  mutacoes sequenciais. Nao repita o mesmo comando sem mudanca de hipotese,
  candidato ou instrumentacao.
- Acceptance que copia inputs read-only para staging deve tornar sua propria
  arvore temporaria gravavel no `trap` e nunca deixar erro de cleanup sobrescrever
  um resultado nominal verde. O alvo do cleanup precisa ser o `mktemp` validado
  e o cleanup deve ser idempotente.

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

Ao vencer o prazo, leia o worktree antes de decidir — `git status --porcelain` e
`git log -1` dizem se ha trabalho a salvar. So entao escolha: cobrar entrega
parcial com sequencia numerada minima, redespachar do zero, ou assumir o card.
Nunca deixe a decisao para a proxima vez que o usuario perguntar.

## Progresso e continuidade

Ao final de cada ciclo relate somente: `done_delta`, cards fechados, blocker
medido, cards despachados e proximo gate. Se a sessao terminar, deixe handoff do
ai-memory com o resultado de `pipeline.sh`, fila `ready`, cards tocados e o
proximo comando verificavel. O agente seguinte deve rerodar o pipeline antes de
confiar no handoff.

Nao emita promessa de conclusao enquanto houver backlog, ready, doing, review,
validating ou blocker de especificacao/infraestrutura.

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
