# Tutoriais executáveis

Cada tutorial de `docs/tutorials/` tem uma demonstração aqui, no padrão do
guia corporativo (`demo/gate-corporativo`): os blocos de configuração do
documento **são** os arquivos da demonstração, cada caso de uso roda como uma
fase, a saída da última execução real é versionada em `out/`, e
`run.sh --check` compara essa saída com o que o documento promete.

| Tutorial | Demonstração | Casos |
|---|---|---|
| [revisao](../../docs/tutorials/revisao.md) | `revisao/` | primeira-revisao, sem-provedor, com-provedor, fix, pr-workflow, falha-nao-revisado |
| [deliberacao](../../docs/tutorials/deliberacao.md) | `deliberacao/` | diff-grande-pede-semgrep, diff-pequeno-nao-pede, estoura-rodadas, falha-semgrep-ausente |
| [skills](../../docs/tutorials/skills.md) | `skills/` | ver `skills/run.sh` |
| [politica-central](../../docs/tutorials/politica-central.md) | `politica-central/` | ver `politica-central/run.sh` |
| [qualquer-linguagem](../../docs/tutorials/qualquer-linguagem.md) | `qualquer-linguagem/` | repo-poliglota, arquivo-sem-gramatica, binario-e-gerado, politica-terraform, apelidos-e-instrucoes, falha-extensao-desconhecida |
| [benchmark](../../docs/tutorials/benchmark.md) | `benchmark/` | rodar-corpus, ler-relatorio, adicionar-caso, aprovado-com-defeito, falha-caso-sem-manifest (via `go-shared`, sem o produto na imagem) |
| [operacao](../../docs/tutorials/operacao.md) | `operacao/` | ambiente-go-shared, aceite-selado, profiles-e-locks, dependencia-e-repin, scanners-por-digest, entrega-e-evidencia, falha-evidencia-ausente |
| [gate](../../docs/tutorials/gate.md) | `gate/` | ver `gate/run.sh` (usa `_lib/pr.sh`: GitHub falso em 127.0.0.1) |
| [excecoes](../../docs/tutorials/excecoes.md) | `excecoes/` | ver `excecoes/run.sh` |
| [auditoria-sarif](../../docs/tutorials/auditoria-sarif.md) | `auditoria-sarif/` | ver `auditoria-sarif/run.sh` |
| [reaproveitamento](../../docs/tutorials/reaproveitamento.md) | `reaproveitamento/` | ver `reaproveitamento/run.sh` |
| [segredos](../../docs/tutorials/segredos.md) | `segredos/` | segredo-no-diff, segredo-so-no-historico, allow-sob-politica, ignore-sob-politica, binario-ausente |
| [sast](../../docs/tutorials/sast.md) | `sast/` | regra-local, registry-sem-rede, nosemgrep-e-semgrepignore, origem-sast, semgrep-falha |
| [sbom-dependency-track](../../docs/tutorials/sbom-dependency-track.md) | `sbom-dependency-track/` | up, sbom-versao-minima, upload-e-metricas, limiares, violacao-de-politica, secret-ausente, timeout, projeto-por-microservico, down |
| [assinatura](../../docs/tutorials/assinatura.md) | `assinatura/` | chave-efemera, verificacao-por-terceiro, keyless-actions, bundle-artefato, falha-cosign, falha-sem-bundle, falha-imagem-sem-digest |
| [xbom](../../docs/tutorials/xbom.md) | `xbom/` | build-bom, cbom, evidencia, catalogo, enriquecimento, tipos-documentados, falha-catalogo-invalido |
| [extensao](../../docs/tutorials/extensao.md) | `extensao/` | engine-no-gate, skill-no-prompt, ferramenta-pedida, falha-binario-padrao (imagem com `TUT_BUILD_ARGS="GO_TAGS=aurum_exemplo"`; a falha usa a imagem padrao) |
| [agente](../../docs/tutorials/agente.md) | `agente/` | gate-consultado, skill-do-repo, hook-pre-commit, gate-inconclusivo (cliente MCP de teste em `cliente-mcp.py`, python3 no host) |
| [dados-de-analise](../../docs/tutorials/dados-de-analise.md) | `dados-de-analise/` | declarado-ou-nao, vencido, cache, workflow-agendado, adulterado, indisponivel |
| [provedores](../../docs/tutorials/provedores.md) | `provedores/` | sem-perfil, azure-openai, anthropic, catalogo-do-operador, falha-fora-do-schema, falha-perfil-desconhecido (provedor falso dentro do container, rede none) |
| [changelog](../../docs/tutorials/changelog.md) | `changelog/` | entrada-valida, consolidar-release, sugestao-separada, pr-desliga-o-modo, log-de-agente, falha-entrada-ausente |

## Como rodar

```bash
bash demo/tutoriais/<tutorial>/run.sh all       # todos os casos (docker)
bash demo/tutoriais/<tutorial>/run.sh <caso>    # um caso
bash demo/tutoriais/<tutorial>/run.sh --check   # out/.imagem e out/ contra expected/, sem docker
bash demo/tutoriais/<tutorial>/run.sh limpar    # apaga .estado/
```

No host só existem `bash`, `git`, `docker` e `python3`. O programa roda na
imagem do produto (`docker build` do `Dockerfile` da raiz, tag
`aurum-tutoriais:<12 hex>` derivada da identidade da árvore, construída na primeira execução; `AURUMCODE_TUT_REBUILD=1`
força, `AURUMCODE_TUT_IMAGE=` usa outra tag; um `run.sh` que declara
`TUT_BUILD_ARGS="CHAVE=valor ..."` antes de carregar `_lib/tutorial.sh`
constrói com esses `--build-arg`, numa tag com o sufixo `-<12 hex dos args>`,
e `AURUMCODE_TUT_BUILD_ARGS` sobrescreve), sem rede (`--network none`) e com
o provedor de modelo falso e determinístico (`AURUMCODE_LLM_FIXTURE`): nunca
há credencial real.

## Imagem registrada e valores voláteis

`run.sh all` (ou um caso) grava `out/.imagem` com a identidade da árvore
(sha256 do `Dockerfile`, `go.mod`, `go.sum` e dos arquivos de produção de `cmd`,
`internal` e `pkg`, sem `*_test.go`) e o digest da imagem usada. A tag da imagem
deriva dessa identidade, então uma imagem de outra árvore nunca é reaproveitada.
`run.sh --check` falha com o motivo quando `out/.imagem` falta ou foi gravado por
outra árvore, e imprime `imagem conferida` (com docker e a imagem local) ou
`imagem nao conferida (sem docker)` (só a árvore é conferida, como no container
selado).

`expected/` e os blocos de `docs/tutorials/` nunca pinam valor volátil. Eles
escrevem a forma, e o `--check` (e os aceites 563/564) normalizam o `out/` com
`_lib/normaliza.sed` antes da comparação literal:

| Valor volátil | Forma em `expected/` e no texto |
|---|---|
| `board valid: 585 atomic cards` | `board valid: <N> atomic cards` |
| `analysis-data/20261002T134230Z` | `analysis-data/<timestamp>` |
| `2026-10-02T13:42:30Z` | `<timestamp>` |
| `is 30.0 days old` / `idade: 30.0 dias` | `is <duracao> days old` / `idade: <duracao>` |

Outro número ou outra data passa; um valor fora da forma (`board valid: abc
atomic cards`) reprova. Para um novo valor volátil, acrescente a regra em
`_lib/normaliza.sed`.

## O framework (`_lib/tutorial.sh`)

Um `run.sh` mínimo:

```bash
#!/usr/bin/env bash
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
. "$HERE/../_lib/tutorial.sh"

CASOS=(primeiro-caso caso-de-falha)

caso_primeiro_caso() {
  tut_repo primeiro-caso repo-exemplo/base repo-exemplo/mudanca
  aurum review --base main
  expect_rc 0 "o que este caso prova"
}

tut_main "$@"
```

| Função | O que faz |
|---|---|
| `CASOS=(...)` | A ordem dos casos. O caso `a-b` é a função `caso_a_b`. |
| `tut_repo CASO BASE [OVERLAY...]` | Cria `.estado/CASO` como repositório git: copia `BASE` e comita em `main`; copia cada `OVERLAY` por cima, na branch `feature`, um commit cada. Define `TUT_WORK`, o diretório de trabalho do caso. |
| `aurum ARGS...` | Imprime `$ aurumcode ARGS`, roda na imagem (cwd `/work` = o repositório do caso; o diretório do tutorial em `/fixtures`, só leitura), imprime `exit_code=N` e guarda `N` em `LAST_RC`. Não aborta o caso. |
| `aurum_raw -- ARGS...` | Como `aurum`, sem eco nem `exit_code`; use para redirecionar a saída (`> arquivo`). |
| `expect_rc N "frase"` | Imprime `RESULTADO: frase` se `LAST_RC == N`, senão `ERRO: ...` e falha o caso. |
| `TUT_FIXTURE=arquivo.json` | Qual JSON do tutorial serve de modelo (padrão `fixture-llm.json`); `none` remove o provedor. `TUT_ENVS=(-e VAR=valor)` acrescenta variáveis ao container. |
| `tgit ARGS...` | `git -C "$TUT_WORK"` com identidade de demonstração, só nos repositórios descartáveis (a configuração git do usuário nunca é tocada). |

O `run.sh <caso>` grava `out/<caso>.log` (stdout+stderr, via `tee`) e sai com o
código do caso. `run.sh --check` lê `expected/<caso>.txt`: **cada linha é um
trecho literal** que precisa existir em `out/<caso>.log` normalizado, **tantas
vezes quantas** `expected/` a repete (duas linhas `exit_code=3` exigem dois
`exit_code=3` na saída; um só reprova com `trecho esperado 2 vez(es),
encontrado 1`), e as linhas `RESULTADO:` precisam aparecer **na ordem** de
`expected/` (senão `RESULTADO fora de ordem`). As demais linhas não têm ordem
imposta: um trecho pode ser só um pedaço de uma linha longa da saída. Linhas
vazias e iniciadas por `#` são ignoradas; sai 1 na primeira divergência
(`DIVERGENCIA caso=...`). Sem docker, sem rede: serve para o aceite offline.

## Criar um tutorial novo

1. Crie `demo/tutoriais/<nome>/` com o `run.sh` (modelo acima), `fixture-llm.json`
   (a forma exata aceita pelo motor: `issues[]` com `file`, `line`, `severity`,
   `rule_id` do catálogo, `message`, `impact`, `evidence`, `suggestion`,
   `verification`), `repo-exemplo/<base|mudanca>/...` e os demais arquivos de
   configuração que o texto mostra.
2. Escreva um caso por fase, com **um caso de falha**. Cada caso deve afirmar o
   exit esperado com `expect_rc` e imprimir uma linha `RESULTADO:` que diga o
   que foi provado.
3. Rode `run.sh all` de verdade; o `out/` gerado é o que se versiona. Copie para
   `expected/<caso>.txt` só as linhas que provam o caso (não o log inteiro).
4. Em `docs/tutorials/<nome>.md`, para cada arquivo de configuração use o
   marcador seguido do bloco, idêntico ao arquivo:

   ````markdown
   <!-- arquivo: demo/tutoriais/<nome>/caminho/do/arquivo.yml -->
   ```yaml
   (o conteúdo exato do arquivo)
   ```
   ````

   e para cada saída esperada use `<!-- saida: <caso> -->` seguido de um bloco
   cujas linhas existam, literalmente, em `out/<caso>.log`.
5. Comandos do produto, nos blocos `bash`, começam por `aurumcode` (com ou sem
   `$ `); o aceite confere cada subcomando e cada flag contra `--help`.
6. Use só `localhost`, `127.0.0.1`, `example.com/.org/.net`, `.invalid`, `.test` e
   placeholders (`OWNER/REPO`). Nunca endpoint ou nome interno.
7. Acrescente o tutorial ao nav do `mkdocs.yml` e ao índice
   `docs/tutorials/README.md`.

`tests/acceptance/AUR-561.sh` mostra como o aceite verifica um tutorial:
blocos idênticos aos arquivos, `--check` sobre o `out/` versionado, saídas do
texto presentes no `out/`, comandos e flags reais, domínios reservados.

## Tutoriais da cadeia de suprimentos (AUR-563)

Os cinco tutoriais da cadeia de suprimentos carregam, além do framework,
`_lib/cadeia.sh`: ele **lê o `images.lock` do guia corporativo**
(`demo/gate-corporativo/images.lock`, AUR-554) em vez de copiar digests.
Trivy, Cosign, Dependency-Track e PostgreSQL saem dessas imagens fixadas por
digest (`cad_lock`, `cad_pull`, `cad_bins`); o Semgrep é o da imagem do
produto (versão igual a `.board/bootstrap/locks/scanners.yml`). Nem todos
rodam `--network none`: o `sbom-dependency-track` usa `--network host` (o gate
só aceita `https` ou IP de loopback) e o `dados-de-analise` redireciona o nome
da API do GitHub para um servidor falso local, dentro do mesmo container.
Cada `run.sh` diz o que usa.
