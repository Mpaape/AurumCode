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
- O checkout coordenador pode permanecer sujo. Toda acceptance, review e
  validacao final rodam em worktree limpo do SHA candidato.
- Use a maior frota segura que a fila e os recursos realmente suportam; nao
  encha slots com cards que falharam preflight ou possuem dependencia ausente.

## Preflight que nao pode ser pulado

Antes de qualquer builder, reviewer ou validator, leia o card completo e rode o
preflight no worktree limpo. Ele exige:

- paths e read_paths canonicos; paths de candidato ativo existem e sao tracked;
- `validation: none|tested|skeptical` declarado explicitamente antes de `ready`;
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
- card com camada ou comando Go materializa `go.mod` e `go.sum` em
  `paths`/`read_paths`, sem modulo substituto criado pelo acceptance;
- worktree limpo e, com `PREFLIGHT_RUN=1`, exit real do acceptance.

Para `ready`, o preflight de builder valida contrato, dependencias, posse,
profile, lock, imagem e runtime base, mas nao exige ainda os artifacts de
`paths` nem executa acceptance ausente. Reviewer/validator continuam exigindo
todos os paths tracked, acceptance executavel e exit real. Essa distinção evita
que cards novos fiquem impossíveis de despachar por ainda não terem sido
construídos.

Quando `PREFLIGHT_RUN=1` executa a acceptance nominal, somente exit `0` é
verde. Exit `1` é falha do card, não RED aceitável; apenas códigos explícitos
de infraestrutura podem ser classificados como indisponibilidade.

Uma imagem Go sem `bash` nao passa: o runner executa a acceptance com `bash`.
Uma imagem Bash sem Go nao passa para acceptance que chama Go. AUR-006 mostrou
que detectar somente o nome da imagem ou somente o host nao e suficiente.

Nao crie profile dentro de feature para contornar o DAG. AUR-402 e dono do
registry; AUR-403 e dono do profile Go. Se o owner estiver bloqueado, registre o
bloqueio e nao fabrique um profile local.

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

1. Test designer prova RED pelo comportamento esperado. Falha de ambiente nao e
   RED. Para caracterizacao, prova GREEN, mutacao RED e restore GREEN.
2. Builder implementa somente `paths`, executa baseline, mutacao e restore, e
   cria commit com identidade humana configurada, sem atribuicao de IA.
3. Reviewer independente revisa o mesmo SHA imutavel ainda fora da fila ativa,
   antes da integracao; depois da aprovacao o coordenador integra exatamente
   esse SHA e move o card para `review`/`validating`,
   cada hunk, o contrato,
   schema/parser, acceptance, paths, exits e fronteiras de seguranca.
   Cada selector Unit, Contract, Integration e E2E declarado deve executar uma
   assercao real. Exit 0 com `[no test files]`, zero testes, branch vazia ou
   selector que retorna sem chamar a camada e veto, mesmo se o nominal agregado
   estiver verde.
4. Validator executa o acceptance e as camadas declaradas no mesmo SHA e em
   worktree limpo. Exit 0 e evidencia; exit 69/79 e inconclusivo; exit 1 so e
   RED depois que o programa realmente iniciou.
5. Coordenador integra apenas o candidato aprovado, grava evidencia sanitizada,
   atualiza Delivery record e roda `bash .board/pipeline.sh`.
6. So entao move para `done`. Nenhum agente pode substituir review, mutacao ou
   evidencia por prosa.

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

## Progresso e continuidade

Ao final de cada ciclo relate somente: `done_delta`, cards fechados, blocker
medido, cards despachados e proximo gate. Se a sessao terminar, deixe handoff do
ai-memory com o resultado de `pipeline.sh`, fila `ready`, cards tocados e o
proximo comando verificavel. O agente seguinte deve rerodar o pipeline antes de
confiar no handoff.

Nao emita promessa de conclusao enquanto houver backlog, ready, doing, review,
validating ou blocker de especificacao/infraestrutura.
