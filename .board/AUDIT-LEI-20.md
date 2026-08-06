# Auditoria da lei 20 — path declarado que ninguém lê

Data: 2026-08-06. Método: **teste decisivo**, não proxy — para cada path declarado,
removê-lo da árvore, rodar o `accept:` pelo `oci-run` real, restaurar e provar a
restauração. Cinco auditores adversariais independentes, em worktrees isolados.

Este arquivo é **registro de defeito**: não é evidência, não é autorização, não move card.

## Por que a auditoria existe

`validate.sh:1863` exige que todo path declarado **exista** na árvore. Um `grep` pelo
conceito de leitura no validador inteiro retorna **zero**: nada exige que o aceite leia o
que o card declara possuir. A lei 20 nasceu quando um cético removeu dois paths do
`AUR-015` e o aceite selado continuou em exit 0 com stdout byte a byte idêntico.

Um levantamento por `grep` apontou 29 de 84 suspeitos. `grep` é proxy — um path pode ser
lido por variável ou por varredura de diretório. Esta auditoria rodou o teste que decide.

## Resultado: 49 paths testados, 18 sobreviveram à remoção

Sobreviver à remoção **não é uma só coisa**. A auditoria separou duas causas, e confundi-las
seria repetir o erro que ela existe para achar.

### A. Path nunca lido — 15 casos

O aceite não toca o arquivo em nenhum caminho de execução.

| card | lane | path | o card **afirma** que lê |
|---|---|---|---|
| AUR-016 | done | `docs/specs/AUR-016.md` | **sim** |
| AUR-021 | done | `docs/specs/AUR-021.md` | **sim** |
| AUR-363 | done | `go.mod` | **sim** |
| AUR-363 | done | `go.sum` | **sim** |
| AUR-017 | done | `docs/specs/AUR-017.md` | não |
| AUR-020 | done | `docs/specs/AUR-020.md` | não |
| AUR-020 | done | `tests/integration/AUR-020_test.go` | não |
| AUR-021 | done | `tests/integration/AUR-021_test.go` | não |
| AUR-359 | done | `docs/specs/AUR-359.md` | não |
| AUR-360 | done | `docs/specs/AUR-360.md` | não |
| AUR-360 | done | `Makefile` | não |
| AUR-360 | done | `Dockerfile` | não |
| AUR-362 | done | `docs/specs/AUR-362.md` | não |
| AUR-363 | done | `docs/specs/AUR-363.md` | não |
| AUR-015 | doing | `tests/integration/AUR-015_test.go` | não |

### B. Path lido, mas ausência é indistinguível de limpo — 3 casos, todos `AUR-362`

`.github/workflows`, `Dockerfile` e `.docker/docs.Dockerfile`. A remoção pura dá exit 0,
**mas mutar o conteúdo derruba o aceite**: injetar a string `bandit` produz exit 1
`scanner-unlocked`. O `grep` real existe (`tests/acceptance/AUR-362.sh:311-329`).

Estes paths **são lidos**. O defeito é outro e é a **lei 12**: o aceite varre em busca de
scanner não travado, e "arquivo ausente" produz o mesmo resultado que "arquivo limpo".
Chamá-los de decorativos seria impreciso; deixá-los sem asserção de contagem seria a Lei
12 sobrevivendo num card selado.

### Inconclusivos — 14 paths

Diretórios e scripts de aceite cuja remoção faz o runner recusar antes de executar
(exit 65/66/69/79). **Falha de infraestrutura não prova leitura nem decoratividade —
lei 4.** Detalhe por lote em `/tmp/aurum-reviews/lei20-lote*.md`.

Um caso merece nota: em `AUR-360`, remover `go.mod` e `go.sum` produz exit 79 com
`gomod_unreadable` e `gosum_unreadable` — mensagens **dedicadas**, o que mostra que o
script tenta lê-los antes de falhar. O auditor recusou classificar nos dois sentidos, e
está certo.

## As duas gravidades

**Path esquecido** — está em `paths:` e ninguém o lê. Defeito de declaração: ou o aceite
passa a lê-lo, ou ele sai de `paths:`.

**Afirmação falsa** — o card diz por escrito que o aceite lê ou executa aquele arquivo, e
não lê. **É a classe que fez este board revogar o selo do `AUR-015` hoje.** Quatro casos,
**todos em cards `done`**: `AUR-016` e `AUR-021` (`docs/specs`), `AUR-363` (`go.mod`,
`go.sum`).

## O que isto **não** é

Não é o defeito do `AUR-015`. Lá o bundle era **fabricado**: o `stdout_sha256` selado era
byte a byte o de outro card, um parecer de reviewer-b estava colado no slot do
reviewer-a, e o próprio `seal-history` registrava que o coordenador materializara o bundle
porque dois builders não entregaram.

Aqui as evidências são genuínas — três papéis independentes, observações reais. O que está
errado é uma **afirmação no texto do card**, não a prova. O remédio é proporcional:
**corrigir a afirmação**, não revogar o selo — a menos que se demonstre que algum revisor
aprovou *porque* acreditou que o path era lido.

## O custo do remédio, declarado

Corrigir o texto de um card muda o `spec_digest`, que entra na `CandidateIdentityV1`. Os
cards `done` afetados precisam ser **re-selados**, não apenas editados. Não existe
correção barata aqui, e fingir que existe seria repetir o erro que originou a auditoria.

O card de governança `GOV-second-reader`, em revisão, passará a exigir que todo card em
`review`/`done` carregue ao menos uma citação concreta executável pelo segundo leitor — o
que força a revisitar exatamente estes cards. As correções devem ser dobradas nessa
passagem, e **nunca em silêncio**: quem reabrir um bundle `done` registra o motivo.

## O buraco do portão que permite tudo isto

`validate.sh` confere existência de path declarado e não confere leitura. A checagem que
faltaria é a que esta auditoria executou à mão: remover e observar. Cara para rodar a cada
execução do validador, barata como **mutação por card**, no mesmo lugar onde as mutações
céticas já vivem — e com o cuidado que o caso B ensina: a mutação certa nem sempre é
remover o arquivo; às vezes é mudar o conteúdo dele.
