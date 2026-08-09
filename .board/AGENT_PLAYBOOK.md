# Caderno de Orientacoes do Escritorio

Este caderno e o ponto de partida de qualquer agente que receba um card. O
monitor de 20 minutos e `.board/office-cycle.sh`; o gate estrutural e
`bash .board/pipeline.sh`; o preflight de um card e
`PREFLIGHT_RUN=1 bash .board/card-preflight.sh AUR-NNN /caminho/do/worktree`.
O `.board/validate.sh` e legado congelado: nao o execute neste fluxo e nao use
seu exit code como autorizacao.

## Regra de nao-reaprendizado

Uma sessao curta deve conseguir falhar fechado sem conhecer este historico. O
pipeline bloqueia candidatos `review`/`validating` com paths ausentes, acceptance
nao executavel, profile sem lock ou accept divergente. O preflight repete essas
checagens, exige worktree limpo e faz um smoke test real da imagem pinada. O
`oci-run` repete o bloqueio imediatamente antes de materializar qualquer
container. Se os resultados divergirem, o resultado e bloqueio, nunca GREEN.

## O que deu errado na onda anterior

- **AUR-001:** o builder validou uma arvore imutavel, mas a integracao foi
  testada num checkout compartilhado que havia recebido arquivos de outros
  cards. A seal/inventory estatica ficou stale e gerou ciclos de correcoes.
  Regra: a validacao final sempre roda num worktree limpo do commit integrado;
  nunca no checkout coordenador sujo.
- **AUR-006:** o card foi despachado com `validation: tested` sem confirmar a
  capacidade do ambiente. O aceite terminou em exit 69 por falta de Go, e o
  profile `bootstrap-readonly-v1` e Bash-only apesar de o acceptance exigir Go.
  Exit 69 e bloqueio de infraestrutura, nunca GREEN e nunca `done`. Mesmo
  depois de recuperar os paths declarados, o preflight continuou bloqueando.
  O erro metodologico foi tratar isso como espera: o coordenador deve reparar o
  DAG/write-set e executar o owner do profile. AUR-403 materializa de forma
  checked-in o profile Go, seu schema, lock e registry; se nenhum owner existir,
  crie ou corrija esse contrato antes de despachar o consumidor.
- **AUR-002:** a acceptance hashava fixtures e metadata, mas nao executava o
  comportamento de `cmd/regenerate-docs`; uma mutacao real ficou verde. Uma
  acceptance que nao executa o entrypoint e um teste falso.
- **AUR-003:** a acceptance fazia parse/grep e o schema era mais fraco que o
  parser Go. O runner de unidade passou com `[no test files]`. Schema, parser e
  acceptance precisam compartilhar casos, e o runner deve executar uma funcao
  ou teste real.
- **AUR-014:** o caminho que funcionou: artefato de dados fechado, acceptance
  com mutacoes executaveis, aceite OCI verde, review independente e validacao
  no candidato imutavel.

## Preflight obrigatorio

1. Leia o card inteiro e confirme `paths`, `read_paths`, `forbidden_paths`,
   `validation` e o comando de acceptance. Antes de mover para `ready`, declare
   explicitamente `validation: none|tested|skeptical`; nao invente uma regra de
   validacao depois do commit.
2. Confirme que todas as dependencias estao em `done` e que nenhum builder ja
   possui os mesmos paths. O card deve ser a unica posse de seus paths.
3. Crie um worktree limpo a partir do HEAD atual. Nao use o checkout do
   coordenador para construir ou validar. Todas as lanes, inclusive vazias,
   precisam existir nesse clone por arquivo rastreado; diretorio presente apenas
   no checkout sujo nao prova que o pipeline e reproduzivel. Execute `oci-run`
   com o cwd nesse worktree: chamar o script por path absoluto nao seleciona o
   candidato, pois o runner resolve a raiz com `git rev-parse` no cwd.
4. Rode `bash .board/pipeline.sh` e
   `PREFLIGHT_RUN=1 bash .board/card-preflight.sh AUR-NNN /caminho/do/worktree`.
   O preflight inspeciona a imagem digest-pinada e executa um smoke test dentro
   dela. A imagem precisa conter `bash` porque o runner chama `bash`; se o
   acceptance usa Go, a mesma imagem precisa conter `go`. Falta de runtime e
   contrato invalido, nao um RED que possa ser empurrado ao validator.
   Em card owner de profile, o candidato completo também prova cada profile que
   publica, mesmo que o acceptance rode no bootstrap: Bash é sempre obrigatório
   e Go é obrigatório quando command/cache declara Go. JSON, schema e lock
   coerentes não substituem esse probe executável.
   Card que declara camada ou comando Go deve materializar `go.mod` e `go.sum`
   em `paths`/`read_paths`; gerar modulo substituto no acceptance nao corrige o
   contrato de leitura.
   Todo arquivo tracked nesses roots passa pelo mesmo scanner de credential
   shapes do `oci-run`. Tokens sinteticos de teste devem ser montados em runtime
   ou divididos no source; nunca afrouxe o scanner para aceitar um literal.
   Um card `ready` com todos os `paths` já rastreados é tratado automaticamente
   como candidato completo: acceptance executável, paths e exit nominal passam
   a ser obrigatórios antes do review.
5. Rode a acceptance nominal antes de alterar codigo. Registre o exit real.
   Se o candidato for preparado em clone isolado, confirme que `user.name` e
   `user.email` existem antes do commit. Clone nao herda necessariamente a
   configuracao local do coordenador: copie somente a identidade humana ja
   configurada no checkout coordenador; identidade ausente e bloqueio de
   publicacao, nunca um valor a inventar.
6. Execute pelo menos uma mutacao que altere o comportamento prometido. A
   mutacao deve produzir RED; restaurar o arquivo deve produzir GREEN. Hash,
   grep, contagem ou leitura de metadata sem executar o entrypoint nao contam.
7. Confirme que cada selector declarado chama um teste real. `go test` com
   `[no test files]`, um arquivo `Test...` que nunca e executado, ou um selector
   aceito mas ignorado e falha de validacao.
8. Compare schema e parser: cada campo obrigatorio, limite, enum, path seguro,
   erro e regra de disjointness deve existir nos dois lados. Um schema fraco nao
   e compensado por um parser forte.
9. Compare o runtime declarado com a imagem do profile. Se a acceptance usa Go,
   a imagem precisa conter Go; `bootstrap-readonly-v1` nao pode executar esse
   card. Profile Bash-only + acceptance Go e erro de especificacao, nao um
   bloqueio para empurrar ao validator.
10. Verifique as ferramentas antes de despachar: Docker ou Podman, imagem local,
    `bash`, `go` quando exigido, rede none e dependencias em cache. Falta de
    ferramenta no host e exit 79; runtime ausente na imagem e erro de contrato.
    Nenhum dos dois pode virar `done`.
    Exit 125 antes do processo é infraestrutura; 126/127 porque o entrypoint ou
    uma ferramenta não existe dentro da imagem são erro do contrato do profile.
11. Profile OCI ausente ou incapaz nao autoriza espera nem pedido ao usuario.
    Localize e execute o owner de registry/schema/profile/lock; se o owner nao
    for viavel, corrija `paths` e dependencias do card apropriado, rode o
    pipeline e construa o profile checked-in. Nao use profile local/untracked.
    Nesta reconstrucao, AUR-402 possui o registry e AUR-403 possui o profile Go.

12. Ao alterar qualquer gate, runner ou skill do escritorio, rode
    `bash .board/tests/office-process-regression.sh`, `bash -n` nos scripts
    alterados, `python3 -B -m py_compile .board/bin/check-delivery-evidence.py`
    e `git diff --check`. Esse teste usa uma fixture Git temporaria e prova que
    o profile Bash-only passa para Bash, falha para Go e rejeita lock adulterado.
    Acceptance que copia inputs read-only para staging deve tornar somente seu
    `mktemp` gravavel no trap e ignorar cleanup idempotente; erro de remocao nao
    pode sobrescrever um nominal verde.

## Fluxo de entrega

- Builder: implementa somente paths do card, executa baseline/mutacao e cria
  um commit humano completo.
- Reviewer: revisa o SHA imutavel, executa acceptance e procura testes falsos,
  escopo fora dos paths, schema/parser divergente e saida que engole exit code.
- Validator: roda a acceptance e os testes contra o mesmo commit em worktree
  limpo. Exit 0 e evidencia; exit 69/79 e inconclusivo; exit 1 e RED somente
  quando o programa chegou ao comportamento. Falha de loader, runtime,
  imagem, engine ou dependencia nunca e RED.
- Coordenador: integra o commit, grava `validated.json`, escreve no card
  `commit`, `review: approved` e `validation: passed`, e so entao move para
  `done`.

## Evidencia minima

Cards com `validation: tested|skeptical` em `done` precisam de
`.board/evidence/AUR-NNN/validated.json` contendo o card, o SHA completo, o
review aprovado, a validacao passada e pelo menos um `exit_code: 0`. O pipeline
recusa `done` sem essa correspondencia.

## Regra dos 20 minutos

Cada ciclo de monitoramento dura 20 minutos. No inicio, rode
`.board/office-cycle.sh --start`; em cada revisao, rode
`.board/office-cycle.sh --review`. Se dois reviews nao aumentarem `done`, o
script sai com exit 75 e a abordagem esta encerrada. Nao reenvie o mesmo prompt:
classifique o bloqueio como especificacao, prova falsa, conflito de paths ou
infraestrutura; corrija o processo ou troque de card.

## Estado ao escrever este caderno (snapshot histórico; não usar para dispatch)

- `done`: AUR-001, AUR-014, AUR-018, AUR-019 e AUR-233, com evidencias leves.
- Os estados acima são históricos. Para dispatch, leia os diretórios vivos e
  rode `bash .board/pipeline.sh`; cards `ready` passam pelo preflight de
  builder antes da criação do worktree.
