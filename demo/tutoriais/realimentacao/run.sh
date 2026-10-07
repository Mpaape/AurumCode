#!/usr/bin/env bash
# Tutorial executavel: realimentacao da politica (AUR-532). Veja ../README.md e
# docs/tutorials/realimentacao.md.
#
#   run.sh plano-sem-publicar|publica-uma-pr|rodada-sem-novidade|medir-melhora|medir-regressao|falha-sem-modelo
#   run.sh all | --check | limpar
#
# O GitHub e um servidor falso local (github-falso.py, em 127.0.0.1): o
# container usa --network host so para alcanca-lo, com um token falso. O
# modelo e o provedor falso (fixture-propostas.json). A medicao le relatorios
# do corpus gravados em medicao/, sem rede.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(plano-sem-publicar publica-uma-pr rodada-sem-novidade medir-melhora medir-regressao falha-sem-modelo)
TUT_FIXTURE=fixture-propostas.json
GH_PID=
GH_PORT=
GH_LOG=

# gh_servidor CASO: sobe o GitHub falso com a politica de politica/ em main.
gh_servidor() {
  GH_PORT=$((20000 + RANDOM % 20000))
  GH_LOG="$STATE/$1.github.log"; : > "$GH_LOG"
  python3 "$HERE/github-falso.py" "$GH_PORT" "$HERE" "$GH_LOG" &
  GH_PID=$!
  trap gh_parar EXIT
  local i
  for i in $(seq 1 50); do
    (: > "/dev/tcp/127.0.0.1/$GH_PORT") 2>/dev/null && return 0
    sleep 0.1
  done
  echo "ERRO: servidor falso nao subiu" >&2; return 1
}

gh_parar() { [ -z "$GH_PID" ] || { kill "$GH_PID" 2>/dev/null || true; wait "$GH_PID" 2>/dev/null || true; GH_PID=; }; }

# aurum_gh ARGS...: como `aurum`, com o container na rede do host (so o
# loopback: o servidor falso) e a API do GitHub apontando para ele.
aurum_gh() {
  printf '$ aurumcode %s\n' "$*"
  local envs=()
  [ "$TUT_FIXTURE" = none ] || envs+=(-e "AURUMCODE_LLM_FIXTURE=/fixtures/$TUT_FIXTURE")
  set +e
  LAST_OUT="$(docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e "AURUMCODE_GITHUB_API_URL=http://127.0.0.1:$GH_PORT" -e GITHUB_TOKEN=token-falso-local \
    "${envs[@]}" -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /app/aurumcode "${TUT_RUN_IMAGE:-$TUT_IMAGE}" "$@" 2>&1)"
  LAST_RC=$?
  set -e
  [ -z "$LAST_OUT" ] || printf '%s\n' "$LAST_OUT"
  echo "exit_code=$LAST_RC"
}

# escritas: o que o produto gravou no GitHub falso, uma linha por escrita.
escritas() {
  echo "--- escritas no GitHub falso: $(wc -l < "$GH_LOG")"
  cat "$GH_LOG"
}

vazio() { TUT_WORK="$STATE/$1"; rm -rf "$TUT_WORK"; mkdir -p "$TUT_WORK"; }

# 1. Sem --publicar: coleta os sinais, pede as propostas e so imprime o plano.
caso_plano_sem_publicar() {
  vazio plano-sem-publicar
  gh_servidor plano-sem-publicar
  aurum_gh realimentacao --repos exemplo/app --repo-politica exemplo/politica
  expect_rc 0 "o plano cita os sinais e descarta a proposta sem sinal"
  escritas
}

# 2. Com --publicar: uma unica PR na politica, na branch aurum/realimentacao.
caso_publica_uma_pr() {
  vazio publica-uma-pr
  gh_servidor publica-uma-pr
  aurum_gh realimentacao --repos exemplo/app --repo-politica exemplo/politica --publicar
  expect_rc 0 "a rodada abriu uma PR na politica, sem tocar na main"
  escritas
}

# 3. Rodar de novo sem sinal novo: nada e aberto nem gravado.
caso_rodada_sem_novidade() {
  vazio rodada-sem-novidade
  gh_servidor rodada-sem-novidade
  aurum_gh realimentacao --repos exemplo/app --repo-politica exemplo/politica --publicar
  expect_rc 0 "a primeira rodada abriu a PR"
  aurum_gh realimentacao --repos exemplo/app --repo-politica exemplo/politica --publicar
  expect_rc 0 "sem sinal novo a segunda rodada nao abriu nem gravou nada"
  escritas
}

# 4. Medicao: a politica proposta melhora o corpus.
caso_medir_melhora() {
  vazio medir-melhora
  aurum realimentacao --medir --medicao-antes /fixtures/medicao/antes.json --medicao-depois /fixtures/medicao/depois-melhor.json
  expect_rc 0 "antes e depois medidos, sem regressao"
}

# 5. Medicao: "aprovado com defeito" sobe e a regressao e destacada.
caso_medir_regressao() {
  vazio medir-regressao
  aurum realimentacao --medir --medicao-antes /fixtures/medicao/antes.json --medicao-depois /fixtures/medicao/depois-pior.json
  expect_rc 1 "o aumento de aprovado com defeito foi destacado como regressao"
}

# 6. Falha: ha sinal novo e nenhum modelo configurado.
caso_falha_sem_modelo() {
  vazio falha-sem-modelo
  gh_servidor falha-sem-modelo
  TUT_FIXTURE=none
  aurum_gh realimentacao --repos exemplo/app --repo-politica exemplo/politica --publicar
  expect_rc 1 "sem modelo nao ha proposta e nenhuma PR foi aberta"
  TUT_FIXTURE=fixture-propostas.json
  escritas
}

tut_main "$@"
