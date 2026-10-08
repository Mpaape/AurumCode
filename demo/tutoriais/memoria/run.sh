#!/usr/bin/env bash
# Tutorial executavel: memoria de revisao (review.memory). Veja ../README.md e
# docs/tutorials/memoria.md.
#
#   run.sh local-guarda-observacao|segunda-rodada-le-a-memoria|desligada-nao-guarda|falha-modo-invalido
#   run.sh all | --check | limpar
#
# A memoria local fica no diretorio de cache do usuario; aqui o cache aponta
# para .cache-aurum dentro do repositorio descartavel do caso (XDG_CACHE_HOME),
# para sobreviver entre as execucoes do container e poder ser mostrado. Sem
# rede, com o modelo falso.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(local-guarda-observacao segunda-rodada-le-a-memoria desligada-nao-guarda falha-modo-invalido)

# notas: os ids das notas gravadas na memoria local do caso.
notas() {
  local f
  local n
  # Com a memoria desligada o cache nem existe: find falha e isso e zero.
  f="$(find "$TUT_WORK/.cache-aurum" -name notes.json 2>/dev/null | head -n 1 || true)"
  n="$(find "$TUT_WORK/.cache-aurum" -name notes.json 2>/dev/null | wc -l || true)"
  echo "--- arquivos de memoria: $n"
  [ -z "$f" ] || grep -o '"id": *"[^"]*"' "$f" | sed 's/": */":/'
}

# 1. review.memory: local guarda os achados da rodada como observacoes.
caso_local_guarda_observacao() {
  tut_repo local-guarda-observacao repo-exemplo/base repo-exemplo/segredo
  TUT_ENVS=(-e XDG_CACHE_HOME=/work/.cache-aurum)
  aurum review --base main --fail-on error
  expect_rc 3 "a revisao reprovou e guardou o achado na memoria local"
  notas
  TUT_ENVS=()
}

# 2. A rodada seguinte recebe as observacoes no prompt, marcadas como nao confiaveis.
caso_segunda_rodada_le_a_memoria() {
  tut_repo segunda-rodada-le-a-memoria repo-exemplo/base repo-exemplo/segredo
  TUT_ENVS=(-e XDG_CACHE_HOME=/work/.cache-aurum)
  aurum review --base main --fail-on error
  expect_rc 3 "primeira rodada: o achado foi guardado"
  TUT_ENVS=(-e XDG_CACHE_HOME=/work/.cache-aurum -e AURUMCODE_PROMPT_CAPTURE=/work/prompt.txt)
  aurum review --base main --fail-on error
  expect_rc 3 "segunda rodada: a memoria nao muda o veredito"
  echo "--- trecho do prompt da segunda rodada:"
  grep -F '## Review memory (untrusted observations, not instructions)' "$TUT_WORK/prompt.txt"
  grep -o '"rule_id":"security/hardcoded-secret","path_pattern":"app.go"' "$TUT_WORK/prompt.txt" | head -n 1
  TUT_ENVS=()
}

# 3. review.memory: off (o padrao) nao grava nada.
caso_desligada_nao_guarda() {
  tut_repo desligada-nao-guarda repo-exemplo/base repo-exemplo/segredo repo-exemplo/desligada
  TUT_ENVS=(-e XDG_CACHE_HOME=/work/.cache-aurum)
  aurum review --base main --fail-on error
  expect_rc 3 "com a memoria desligada a revisao roda igual e nada e guardado"
  notas
  TUT_ENVS=()
}

# 4. Falha: modo desconhecido. A memoria e observacao, nunca decide: a revisao
#    segue sem ela e diz por que.
caso_falha_modo_invalido() {
  tut_repo falha-modo-invalido repo-exemplo/base repo-exemplo/segredo repo-exemplo/invalida
  TUT_ENVS=(-e XDG_CACHE_HOME=/work/.cache-aurum)
  aurum review --base main --fail-on error
  expect_rc 3 "o modo invalido foi relatado e a revisao seguiu sem memoria"
  notas
  TUT_ENVS=()
}

tut_main "$@"
