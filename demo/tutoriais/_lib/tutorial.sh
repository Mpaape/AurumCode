#!/usr/bin/env bash
# Framework comum dos tutoriais executaveis (AUR-561).
#
# Um tutorial e um diretorio demo/tutoriais/<nome>/ com um run.sh que faz
# `source` deste arquivo, declara os casos e chama `tut_main "$@"`:
#
#   HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
#   . "$HERE/../_lib/tutorial.sh"
#   CASOS=(primeira-revisao sem-provedor)
#   caso_primeira_revisao() { ... }        # um caso = uma funcao caso_<nome com _>
#   tut_main "$@"
#
# Uso (o run.sh de cada tutorial expoe o mesmo):
#   run.sh <caso>      executa um caso e grava out/<caso>.log
#   run.sh all         executa todos os casos, na ordem
#   run.sh --check     compara out/<caso>.log com expected/<caso>.txt (sem docker)
#   run.sh limpar      remove .estado/ e a imagem local do produto, se pedido
#
# O aurumcode roda SEMPRE na imagem do produto (docker build do Dockerfile da
# raiz), sem rede (--network none) e com o provedor de modelo falso e
# deterministico (AURUMCODE_LLM_FIXTURE). Nunca ha credencial real. No host
# so existem bash, git, docker e python3.
set -Eeuo pipefail

: "${HERE:?o run.sh deve definir HERE antes de carregar _lib/tutorial.sh}"
TUT_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd "$TUT_LIB/../../.." && pwd -P)"
OUT="$HERE/out"
EXPECTED="$HERE/expected"
STATE="$HERE/.estado"
TUT_IMAGE="${AURUMCODE_TUT_IMAGE:-aurum-tutoriais:local}"
LAST_RC=0

# ---------------------------------------------------------------- imagem
# Constroi a imagem do produto uma vez (AURUMCODE_TUT_REBUILD=1 forca).
tut_image() {
  if [ "${AURUMCODE_TUT_REBUILD:-0}" != "1" ] && docker image inspect "$TUT_IMAGE" >/dev/null 2>&1; then
    return 0
  fi
  echo "(construindo a imagem do produto $TUT_IMAGE a partir do Dockerfile)" >&2
  docker build -q -t "$TUT_IMAGE" "$REPO_ROOT" >/dev/null
}

# ---------------------------------------------------------------- execucao
# aurum_raw [-e VAR=valor ...] -- args...: roda /app/aurumcode na imagem.
#   cwd        /work   (o repositorio do caso, gravavel)
#   /fixtures  o diretorio do tutorial, somente leitura (fixture, politica, ...)
#   AURUMCODE_LLM_FIXTURE aponta para TUT_FIXTURE (padrao fixture-llm.json);
#   TUT_FIXTURE=none remove o provedor (caso "sem provedor").
#   TUT_POLICY=<dir> monta <dir> do tutorial em /policy (somente leitura).
aurum_raw() {
  local envs=() fx="${TUT_FIXTURE:-fixture-llm.json}"
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do envs+=("$1" "$2"); shift 2; done
  [ "${1:-}" = "--" ] && shift
  [ "$fx" = none ] || envs+=(-e "AURUMCODE_LLM_FIXTURE=/fixtures/$fx")
  # TUT_POLICY=<dir do tutorial>: monta a politica central, somente leitura, em
  # /policy (fora da arvore revisada); passe --politica /policy ao aurumcode.
  [ -z "${TUT_POLICY:-}" ] || envs+=(-v "$HERE/$TUT_POLICY:/policy:ro")
  docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \
    "${envs[@]}" \
    -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /app/aurumcode "$TUT_IMAGE" "$@"
}

# aurum args...: imprime "$ aurumcode args", executa, imprime "exit_code=N" e
# guarda N em LAST_RC. Nao aborta a fase: cada caso afirma o exit esperado
# com expect_rc.
aurum() {
  printf '$ aurumcode %s\n' "$*"
  set +e
  aurum_raw "${TUT_ENVS[@]}" -- "$@" 2>&1
  LAST_RC=$?
  set -e
  echo "exit_code=$LAST_RC"
}
TUT_ENVS=()

expect_rc() {
  if [ "$LAST_RC" -eq "$1" ]; then
    echo "RESULTADO: $2"
  else
    echo "ERRO: esperado exit_code=$1, obtido $LAST_RC ($2)"
    return 1
  fi
}

# ---------------------------------------------------------------- repositorios
# tgit: git com identidade da demonstracao (apenas nos repositorios descartaveis
# de .estado/; a configuracao git do usuario nunca e tocada).
tgit() {
  git -C "$TUT_WORK" -c user.name=Demo -c user.email=demo@example.invalid \
    -c commit.gpgsign=false -c init.defaultBranch=main "$@"
}

# tut_repo CASO BASE [OVERLAY...]: cria .estado/CASO como repositorio git.
# BASE e copiado e comitado em `main`; cada OVERLAY e copiado por cima na
# branch `feature`, em um commit proprio cada. Define TUT_WORK.
tut_repo() {
  local caso="$1" base="$2"; shift 2
  TUT_WORK="$STATE/$caso"
  rm -rf "$TUT_WORK"; mkdir -p "$TUT_WORK"
  cp -R "$HERE/$base/." "$TUT_WORK/"
  tgit init -q -b main
  tgit add -A
  tgit commit -q -m "base"
  if [ "$#" -gt 0 ]; then
    tgit checkout -q -b feature
    local ov
    for ov in "$@"; do
      cp -R "$HERE/$ov/." "$TUT_WORK/"
      tgit add -A
      tgit commit -q -m "feature: $ov"
    done
  fi
}

# ---------------------------------------------------------------- fases
tut_fn() { printf 'caso_%s' "${1//-/_}"; }

tut_run_caso() {
  local c="$1" fn
  fn="$(tut_fn "$c")"
  declare -F "$fn" >/dev/null || { echo "caso desconhecido: $c" >&2; return 64; }
  mkdir -p "$OUT" "$STATE"
  tut_image
  set +e
  ( set -Ee; echo "== $c"; TUT_FIXTURE="${TUT_FIXTURE:-fixture-llm.json}"; "$fn" ) 2>&1 | tee "$OUT/$c.log"
  local rc="${PIPESTATUS[0]}"
  set -e
  return "$rc"
}

# --check: cada linha de expected/<caso>.txt precisa aparecer, como trecho
# literal (grep -F), em out/<caso>.log. Linhas vazias e '#' sao ignoradas.
# Sai 1 na primeira divergencia.
tut_check() {
  local c line
  for c in "${CASOS[@]}"; do
    [ -f "$OUT/$c.log" ] || { echo "DIVERGENCIA caso=$c: out/$c.log ausente"; return 1; }
    [ -f "$EXPECTED/$c.txt" ] || { echo "DIVERGENCIA caso=$c: expected/$c.txt ausente"; return 1; }
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in ''|'#'*) continue ;; esac
      if ! grep -qF -- "$line" "$OUT/$c.log"; then
        echo "DIVERGENCIA caso=$c: trecho esperado ausente: $line"
        return 1
      fi
    done < "$EXPECTED/$c.txt"
    echo "caso $c: ok"
  done
  echo "CHECK OK"
}

tut_limpar() {
  rm -rf "${STATE:?}"
  echo "limpo: $STATE removido"
  if [ "${AURUMCODE_TUT_RMI:-0}" = "1" ]; then docker rmi "$TUT_IMAGE" >/dev/null 2>&1 || true; echo "imagem $TUT_IMAGE removida"; fi
}

tut_main() {
  local c
  case "${1:-}" in
    --check) tut_check ;;
    all) for c in "${CASOS[@]}"; do tut_run_caso "$c"; done ;;
    limpar) tut_limpar ;;
    '') echo "uso: run.sh ${CASOS[*]}|all|--check|limpar" >&2; exit 64 ;;
    *)
      for c in "${CASOS[@]}"; do
        if [ "$c" = "$1" ]; then tut_run_caso "$c"; return; fi
      done
      echo "uso: run.sh ${CASOS[*]}|all|--check|limpar" >&2; exit 64 ;;
  esac
}
