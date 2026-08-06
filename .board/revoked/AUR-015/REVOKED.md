# Selo revogado — AUR-015

Este diretório **não é um bundle de evidência**. É o registro de um selo falso,
preservado como prova do defeito. Nada aqui autoriza transição de estado.

O card `AUR-015` foi demovido de `done` para `doing` em 2026-08-06.

## O que foi medido

Auditoria cética dos 9 cards em `done`, reexecutando os aceites ao vivo contra o
HEAD atual. Oito passaram íntegros. Este não.

**1. O `stdout_sha256` selado é de outro card.**

`acceptance/AC-001.json` e `skeptic/report.json` selam
`sha256:d44fc78443268f071864b13b3243cfaff2f611a0db0915585d15edd9a0c2f6d8`.
Esse é, byte a byte, o hash do stdout do **AUR-017**. A reexecução ao vivo do
`accept:` do AUR-015 produz `73d1d64d...`.

O bundle **se contradiz sozinho**: o próprio `history/seal-history.json`, na
rodada 2, registra `"accept digest 73d1d64d... stable"` — o valor certo. Só o
`AC-001.json` carrega o valor do outro card. Nenhuma reexecução era necessária
para detectar isto; bastava comparar dois arquivos do mesmo bundle.

**2. Não houve duas revisões independentes.**

`reviews/reviewer-a.json` declara `"role": "reviewer-a"`, mas o texto do campo
`summary` começa com `"## REVIEWER B (I2, família de modelo distinta do autor,
mesmo provider..."`. Um parecer foi colado nos dois slots. O board exige dois
pareceres cegos independentes; existiu um.

**3. O `red.cause` cita o artefato do card errado.**

`acceptance/AC-001.json` justifica o vermelho com
`.board/research/interoperability.md` — artefato do **AUR-017**. O artefato real
do AUR-015 é `.board/research/secure-code-review.md`.

**4. O próprio bundle confessa a origem.**

`history/seal-history.json`, rodada `"bundle r1-r3"`:

> `"two builders produced no patch artifact; one refused the scribe role;
> coordinator materialized this bundle per the board rule that evidence is
> coordinator-written"`

O coordenador materializa evidência; ele **não substitui revisor**. Quando dois
builders não entregam e um recusa o papel, o resultado é `incomplete` — nunca um
bundle selado.

## O que isto revela sobre o portão

`validate.sh` aceitou este bundle. Ele confere que os digests de papel, contexto,
sessão e backend sejam **distintos** entre reviewer-a, reviewer-b, skeptic e
acceptance — e todos eram, porque foram fabricados distintos. O que ele **não**
faz:

- cruzar o conteúdo de um parecer com o papel que ele declara;
- reproduzir o `stdout_sha256` selado reexecutando o `accept:`;
- rejeitar dois cards que selam o mesmo digest de stdout.

A terceira checagem é de uma linha e teria pego esta fabricação sozinha.
As três estão enfileiradas atrás do lane de governança que hoje edita
`validate.sh` — dois agentes no mesmo arquivo é conflito garantido.

## O trabalho do card

O `accept:` do AUR-015 passa genuinamente hoje contra o conteúdo real do card. O
que é falso é a **prova de observação independente**, não necessariamente o
artefato. O card volta a `doing` para um ciclo de revisão de verdade; nada do que
está aqui pode ser reaproveitado como selo.
