#!/usr/bin/env bash
# Extensao do framework (AUR-562): `review --pr` contra um GitHub FALSO e local
# (github-falso.py, em 127.0.0.1). Nao e um runner nem o GitHub: serve para provar o que o
# produto PUBLICA (status, review) sem rede externa. Requer tutorial.sh ja carregado.
#
#   tut_pr_servidor CASO   inicia o servidor para o diff main..feature do repo do caso
#   aurum_pr ARGS...       como `aurum`, com o container em --network host (so loopback do
#                          host: o servidor falso) e AURUMCODE_GITHUB_API_URL apontando para ele
#   tut_pr_log             imprime o que o produto publicou (status e reviews), uma linha JSON
#   tut_pr_parar           encerra o servidor
TUT_PR_PID=
TUT_PR_PORT=
TUT_PR_SHA=2222222222222222222222222222222222222222
TUT_PR_LOG=

tut_pr_servidor() {
  local caso="$1"
  TUT_PR_PORT=$((20000 + RANDOM % 20000))
  TUT_PR_LOG="$STATE/$caso.pr.log"; : > "$TUT_PR_LOG"
  git -C "$TUT_WORK" diff main feature | grep -v "^index " > "$STATE/$caso.pr.diff"
  python3 "$TUT_LIB/github-falso.py" "$TUT_PR_PORT" "$STATE/$caso.pr.diff" "$TUT_PR_LOG" "$TUT_PR_SHA" &
  TUT_PR_PID=$!
  local i
  for i in $(seq 1 50); do
    (: > "/dev/tcp/127.0.0.1/$TUT_PR_PORT") 2>/dev/null && return 0
    sleep 0.1
  done
  echo "ERRO: servidor falso nao subiu" >&2; return 1
}

# tut_pr_servidor_recusa CASO: o GitHub falso recusa o diff do PR por tamanho (406
# too_large) e informa o head e o base REAIS do repo do caso (feature e main): a
# revisao so pode ler o diff do checkout verificado.
tut_pr_servidor_recusa() {
  local caso="$1" base
  TUT_PR_PORT=$((20000 + RANDOM % 20000))
  TUT_PR_LOG="$STATE/$caso.pr.log"; : > "$TUT_PR_LOG"
  TUT_PR_SHA="$(tgit rev-parse feature)"; base="$(tgit rev-parse main)"
  python3 "$TUT_LIB/github-falso.py" "$TUT_PR_PORT" 406 "$TUT_PR_LOG" "$TUT_PR_SHA" "$base" &
  TUT_PR_PID=$!
  local i
  for i in $(seq 1 50); do
    (: > "/dev/tcp/127.0.0.1/$TUT_PR_PORT") 2>/dev/null && return 0
    sleep 0.1
  done
  echo "ERRO: servidor falso nao subiu" >&2; return 1
}

tut_pr_parar() { [ -z "$TUT_PR_PID" ] || { kill "$TUT_PR_PID" 2>/dev/null || true; wait "$TUT_PR_PID" 2>/dev/null || true; TUT_PR_PID=; }; }

aurum_pr() {
  printf '$ aurumcode %s\n' "$*"
  local fx="${TUT_FIXTURE:-fixture-llm.json}" envs=() pol=()
  [ "$fx" = none ] || envs+=(-e "AURUMCODE_LLM_FIXTURE=/fixtures/$fx")
  [ -z "${TUT_POLICY:-}" ] || pol=(-v "$HERE/$TUT_POLICY:/policy:ro")
  set +e
  LAST_OUT="$(docker run --rm --network host --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e "AURUMCODE_GITHUB_API_URL=http://127.0.0.1:$TUT_PR_PORT" -e GITHUB_TOKEN=token-falso-local \
    -e AURUMCODE_PR_PERMISSION_MODE=endpoint -e "GITHUB_SHA=$TUT_PR_SHA" -e AURUMCODE_BASE_SHA=base \
    "${envs[@]}" "${TUT_ENVS[@]}" "${pol[@]}" \
    -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /app/aurumcode "$TUT_IMAGE" "$@" 2>&1)"
  LAST_RC=$?
  set -e
  [ -z "$LAST_OUT" ] || printf '%s\n' "$LAST_OUT"
  echo "exit_code=$LAST_RC"
}

# tut_pr_log: o que o produto publicou, na ordem. Um status vira uma linha;
# um comentario na linha, uma linha com o caminho; o parecer (review ou
# comentario com o marcador) e impresso inteiro, como o leitor o ve no PR.
tut_pr_log() { [ ! -s "$TUT_PR_LOG" ] || python3 -c '
import json,sys
for l in open(sys.argv[1]):
    d=json.loads(l); c=d["corpo"]; metodo="POST" if "POST" in d else "PATCH"; rota=d[metodo]
    if "/statuses/" in rota:
        print("status publicado: context=%s state=%s" % (c.get("context"), c.get("state")))
        print("  description: %s" % c.get("description"))
        continue
    corpo=c.get("body","")
    if "<!-- aurumcode-review -->" in corpo:
        print("parecer %s em %s:" % ("editado" if metodo=="PATCH" else "publicado", rota.split("/pulls/")[-1].split("/issues/")[-1]))
        print(corpo.rstrip())
        for com in c.get("comments",[]) or []:
            print("comentario na linha %s:%s: %s" % (com.get("path"), com.get("line"), (com.get("body","").split("\n")[0])))
    elif "path" in c:
        print("comentario na linha %s:%s: %s" % (c.get("path"), c.get("line"), corpo.split("\n")[0]))
    elif metodo == "PATCH":
        print("comentario anterior editado (%s): %s" % (rota.rsplit("/", 1)[-1], corpo.split("\n")[0]))
    else:
        print("publicado em %s" % rota.split("/pulls/")[-1].split("/issues/")[-1])
' "$TUT_PR_LOG"; }
