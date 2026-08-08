# Caderno de Orientacoes do Escritorio

Este caderno e o ponto de partida de qualquer agente que receba um card. O
gate leve e `bash .board/pipeline.sh`; o preflight de um card e
`bash .board/card-preflight.sh AUR-NNN /caminho/do/worktree`. O
`.board/validate.sh` legado nao faz parte deste fluxo.

## O que deu errado na onda anterior

- **AUR-001:** o builder validou uma arvore imutavel, mas a integracao foi
  testada num checkout compartilhado que havia recebido arquivos de outros
  cards. A seal/inventory estatica ficou stale e gerou ciclos de correcoes.
  Regra: a validacao final sempre roda num worktree limpo do commit integrado;
  nunca no checkout coordenador sujo.
- **AUR-006:** o card foi despachado com `validation: tested` sem confirmar a
  capacidade do ambiente. O aceite terminou em exit 69 por falta de Go. Exit
  69 e bloqueio de infraestrutura, nunca GREEN e nunca `done`.
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
   `validation` e o comando de acceptance. Nao invente uma regra de validacao
   depois do commit.
2. Confirme que todas as dependencias estao em `done` e que nenhum builder ja
   possui os mesmos paths. O card deve ser a unica posse de seus paths.
3. Crie um worktree limpo a partir do HEAD atual. Nao use o checkout do
   coordenador para construir ou validar.
4. Rode `bash .board/pipeline.sh` e
   `PREFLIGHT_RUN=1 bash .board/card-preflight.sh AUR-NNN /caminho/do/worktree`.
   Se Go faltar mas o card tiver aceite OCI, o host preflight apenas avisa; o
   aceite OCI canonico precisa ser executado e sair 0.
5. Rode a acceptance nominal antes de alterar codigo. Registre o exit real.
6. Execute pelo menos uma mutacao que altere o comportamento prometido. A
   mutacao deve produzir RED; restaurar o arquivo deve produzir GREEN. Hash,
   grep, contagem ou leitura de metadata sem executar o entrypoint nao contam.
7. Confirme que cada selector declarado chama um teste real. `go test` com
   `[no test files]`, um arquivo `Test...` que nunca e executado, ou um selector
   aceito mas ignorado e falha de validacao.
8. Compare schema e parser: cada campo obrigatorio, limite, enum, path seguro,
   erro e regra de disjointness deve existir nos dois lados. Um schema fraco nao
   e compensado por um parser forte.
9. Verifique as ferramentas antes de despachar: Go, OCI, rede none, imagem e
   dependencias em cache. Falta de ferramenta vira `validating`/bloqueio de
   infraestrutura, nao `done`.

## Fluxo de entrega

- Builder: implementa somente paths do card, executa baseline/mutacao e cria
  um commit humano completo.
- Reviewer: revisa o SHA imutavel, executa acceptance e procura testes falsos,
  escopo fora dos paths, schema/parser divergente e saida que engole exit code.
- Validator: roda a acceptance e os testes contra o mesmo commit em worktree
  limpo. Exit 0 e evidencia; exit 69 e inconclusivo; exit 1 e RED.
- Coordenador: integra o commit, grava `validated.json`, escreve no card
  `commit`, `review: approved` e `validation: passed`, e so entao move para
  `done`.

## Evidencia minima

Cards com `validation: tested|skeptical` em `done` precisam de
`.board/evidence/AUR-NNN/validated.json` contendo o card, o SHA completo, o
review aprovado, a validacao passada e pelo menos um `exit_code: 0`. O pipeline
recusa `done` sem essa correspondencia.

## Regra dos 20 minutos

Cada ciclo de monitoramento dura 20 minutos. Se dois ciclos nao aumentarem a
contagem de `done`, pare a abordagem atual. Nao reenvie o mesmo prompt: classifique
o bloqueio como especificacao, prova falsa, conflito de paths ou infraestrutura;
corrija o processo ou troque de card.

## Estado ao escrever este caderno

- `done`: AUR-001, AUR-014, AUR-018, AUR-019 e AUR-233, com evidencias leves.
- `validating`: AUR-006, aguardando Go/OCI executavel; nao e verde.
- `doing`: AUR-002 e AUR-003, ambos retornaram para correcao apos review.
- `ready`: vazio.
