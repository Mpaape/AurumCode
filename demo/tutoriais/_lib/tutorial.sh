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
#   run.sh --check     confere out/.imagem (arvore e imagem) e compara out/<caso>.log
#                      normalizado com expected/<caso>.txt (nao exige docker)
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
NORMALIZA="$TUT_LIB/normaliza.sed"
IMAGEM_REG="$OUT/.imagem"
LAST_RC=0
LAST_OUT=

# ---------------------------------------------------------------- imagem
# Identidade da arvore: sha256 de go.mod, go.sum e dos arquivos de producao (sem
# *_test.go) de cmd, internal e pkg, que e o que o `docker build` do produto
# compila. Usa so sha256 de arquivos (sem git e sem docker): o mesmo valor no
# host e no container selado dos aceites, onde o Dockerfile nao e materializado.
# O Dockerfile entra a parte (TUT_DOCKERFILE, vazio quando ausente): e conferido
# sempre que existe e participa da tag da imagem.
tut_sha256() { if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi; }
tut_tree_id() {
  (
    cd "$REPO_ROOT"
    { printf '%s\n' go.mod go.sum
      find cmd internal pkg -type f ! -name '*_test.go' 2>/dev/null || true
    } | LC_ALL=C sort | while IFS= read -r f; do
      if [ -f "$f" ]; then printf '%s  %s\n' "$(tut_sha256 < "$f" | awk '{print $1}')" "$f"; fi
    done | tut_sha256 | awk '{print $1}'
  )
}
tut_dockerfile_id() {
  if [ -f "$REPO_ROOT/Dockerfile" ]; then tut_sha256 < "$REPO_ROOT/Dockerfile" | awk '{print $1}'; fi
}
TUT_TREE="$(tut_tree_id)"
TUT_DOCKERFILE="$(tut_dockerfile_id)"

# A tag deriva da identidade da arvore e do Dockerfile: uma imagem de outra arvore
# nunca e reaproveitada. AURUMCODE_TUT_IMAGE continua sobrescrevendo a tag.
TUT_TAG="$(printf '%s%s' "$TUT_TREE" "$TUT_DOCKERFILE" | tut_sha256 | awk '{print substr($1,1,12)}')"
TUT_IMAGE_PADRAO="${AURUMCODE_TUT_IMAGE:-aurum-tutoriais:$TUT_TAG}"

# Build args do docker build (palavras CHAVE=valor separadas por espaco). Um
# tutorial que precisa de outra compilacao do produto (ex.: a engine de exemplo,
# GO_TAGS=aurum_exemplo) declara TUT_BUILD_ARGS no run.sh ANTES de carregar este
# arquivo; AURUMCODE_TUT_BUILD_ARGS sobrescreve. Com build args a tag ganha o
# sufixo -<12 hex dos args>: a imagem padrao (TUT_IMAGE_PADRAO), usada pelos
# outros tutoriais, nunca e sobrescrita por uma compilacao diferente.
TUT_BUILD_ARGS="${AURUMCODE_TUT_BUILD_ARGS:-${TUT_BUILD_ARGS:-}}"
if [ -n "$TUT_BUILD_ARGS" ]; then
  TUT_IMAGE="$TUT_IMAGE_PADRAO-$(printf '%s' "$TUT_BUILD_ARGS" | tut_sha256 | awk '{print substr($1,1,12)}')"
else
  TUT_IMAGE="$TUT_IMAGE_PADRAO"
fi

# tut_build IMAGEM [ARGS]: docker build do Dockerfile da raiz com os build args,
# uma vez por arvore (AURUMCODE_TUT_REBUILD=1 forca).
tut_build() {
  local img="$1" args=() a
  for a in ${2:-}; do args+=(--build-arg "$a"); done
  if [ "${AURUMCODE_TUT_REBUILD:-0}" != "1" ] && docker image inspect "$img" >/dev/null 2>&1; then
    return 0
  fi
  echo "(construindo a imagem do produto $img a partir do Dockerfile${2:+ com $2})" >&2
  docker build -q "${args[@]}" -t "$img" "$REPO_ROOT" >/dev/null
}

# Constroi a imagem do tutorial (com TUT_BUILD_ARGS, quando declarados).
tut_image() { tut_build "$TUT_IMAGE" "$TUT_BUILD_ARGS"; }

# Constroi a imagem padrao, sem build args: o binario que todo usuario recebe.
# Um caso a usa com TUT_RUN_IMAGE="$TUT_IMAGE_PADRAO".
tut_image_padrao() { tut_build "$TUT_IMAGE_PADRAO" ""; }

# Registra em out/.imagem a imagem que gerou out/ e a arvore que a construiu.
tut_grava_imagem() {
  mkdir -p "$OUT"
  printf 'tree=%s\ndockerfile=%s\nbuild_args=%s\nimage=%s\n' "$TUT_TREE" "$TUT_DOCKERFILE" "$TUT_BUILD_ARGS" "$(docker image inspect --format '{{.Id}}' "$TUT_IMAGE")" > "$IMAGEM_REG"
}

# Confere out/.imagem: ausente ou de outra arvore reprova. Com docker e a imagem
# local, o digest tambem precisa bater; sem eles, so a arvore e conferida e a
# saida diz "imagem nao conferida (sem docker)".
tut_check_imagem() {
  local tree img atual dfile bargs
  [ -f "$IMAGEM_REG" ] || { echo "DIVERGENCIA: out/.imagem ausente: out/ nao foi gravado por run.sh all com a imagem desta arvore (rode run.sh all)"; return 1; }
  tree="$(sed -n 's/^tree=//p' "$IMAGEM_REG")"; img="$(sed -n 's/^image=//p' "$IMAGEM_REG")"
  dfile="$(sed -n 's/^dockerfile=//p' "$IMAGEM_REG")"; bargs="$(sed -n 's/^build_args=//p' "$IMAGEM_REG")"
  if [ "$tree" != "$TUT_TREE" ]; then
    echo "DIVERGENCIA: out/ foi gravado por uma imagem de outra arvore (gravada ${tree:0:12}, atual ${TUT_TREE:0:12}); rode run.sh all"
    return 1
  fi
  if [ -n "$TUT_DOCKERFILE" ] && [ "$dfile" != "$TUT_DOCKERFILE" ]; then
    echo "DIVERGENCIA: out/ foi gravado com outro Dockerfile (gravado ${dfile:0:12}, atual ${TUT_DOCKERFILE:0:12}); rode run.sh all"
    return 1
  fi
  if [ "$bargs" != "$TUT_BUILD_ARGS" ]; then
    echo "DIVERGENCIA: out/ foi gravado com outros build args (gravados '$bargs', atuais '$TUT_BUILD_ARGS'); rode run.sh all"
    return 1
  fi
  if command -v docker >/dev/null 2>&1 && atual="$(docker image inspect --format '{{.Id}}' "$TUT_IMAGE" 2>/dev/null)"; then
    if [ "$atual" != "$img" ]; then
      echo "DIVERGENCIA: a imagem local $TUT_IMAGE ($atual) nao e a que gravou out/ ($img); rode run.sh all"
      return 1
    fi
    echo "imagem conferida: $TUT_IMAGE $img (arvore ${TUT_TREE:0:12})"
  else
    echo "imagem nao conferida (sem docker): arvore ${TUT_TREE:0:12} confere com out/.imagem"
  fi
}

# ---------------------------------------------------------------- execucao
# aurum_raw [-e VAR=valor ...] -- args...: roda /app/aurumcode na imagem.
#   cwd        /work   (o repositorio do caso, gravavel)
#   /fixtures  o diretorio do tutorial, somente leitura (fixture, politica, ...)
#   AURUMCODE_LLM_FIXTURE aponta para TUT_FIXTURE (padrao fixture-llm.json);
#   TUT_FIXTURE=none remove o provedor (caso "sem provedor").
#   TUT_POLICY=<dir> monta <dir> do tutorial em /policy (somente leitura).
#   TUT_RUN_IMAGE=<imagem> roda outra imagem (padrao: TUT_IMAGE).
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
    --entrypoint /app/aurumcode "${TUT_RUN_IMAGE:-$TUT_IMAGE}" "$@"
}

# aurum args...: imprime "$ aurumcode args", executa, imprime "exit_code=N" e
# guarda N em LAST_RC e a saida em LAST_OUT. Nao aborta a fase: cada caso afirma o exit esperado
# com expect_rc.
aurum() {
  printf '$ aurumcode %s\n' "$*"
  set +e
  LAST_OUT="$(aurum_raw "${TUT_ENVS[@]}" -- "$@" 2>&1)"
  LAST_RC=$?
  set -e
  # TUT_SED: filtro sed -E opcional sobre a saida (so para texto que varia por ambiente; documente-o no caso).
  [ -z "${TUT_SED:-}" ] || LAST_OUT="$(printf '%s\n' "$LAST_OUT" | sed -E "$TUT_SED")"
  [ -z "$LAST_OUT" ] || printf '%s\n' "$LAST_OUT"
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
  tut_grava_imagem
  set +e
  ( set -Ee; echo "== $c"; TUT_FIXTURE="${TUT_FIXTURE:-fixture-llm.json}"; "$fn" ) 2>&1 | tee "$OUT/$c.log"
  local rc="${PIPESTATUS[0]}"
  set -e
  return "$rc"
}

# --check: cada linha de expected/<caso>.txt precisa aparecer, como trecho literal, em
# out/<caso>.log normalizado por _lib/normaliza.sed, TANTAS VEZES quantas o
# expected/ a repete (duas linhas `exit_code=3` exigem dois `exit_code=3`), e as
# linhas `RESULTADO:` (a conclusao de cada passo) precisam aparecer na ordem do
# expected/. Linhas vazias e '#' sao ignoradas. Sai 1 na primeira divergencia. Por fim confere out/.imagem
# (tut_check_imagem): a prova so vale se out/ veio da imagem desta arvore.
#
# Notacao de forma (valores volateis): expected/ e os blocos de docs/tutorials/
# escrevem a forma em vez do valor: `board valid: <N> atomic cards`,
# `analysis-data/<timestamp>`, `<timestamp>`, `is <duracao> days old`. O out/ e
# normalizado pelas mesmas regras antes do grep, entao outro N ou outra data
# passa e um valor fora da forma reprova. Lista completa em _lib/normaliza.sed.
# TUT_CONFERE_AWK le expected/ (-v esperado=) e o out/ normalizado (stdin) e
# imprime a primeira divergencia: trecho com menos ocorrencias que no
# expected/, ou linha RESULTADO: fora de ordem. awk POSIX (roda no busybox).
TUT_CONFERE_AWK='
BEGIN {
  while ((getline l < esperado) > 0) {
    if (l == "" || substr(l, 1, 1) == "#") continue
    if (!(l in quer)) distinta[++n] = l
    quer[l]++
    if (index(l, "RESULTADO:") == 1) resultado[++r] = l
  }
}
{ o[++m] = $0 }
END {
  for (i = 1; i <= n; i++) {
    l = distinta[i]; tem = 0
    for (j = 1; j <= m && tem < quer[l]; j++) {
      resto = o[j]
      while ((p = index(resto, l)) > 0) { tem++; resto = substr(resto, p + length(l)) }
    }
    if (tem == 0) { printf "trecho esperado ausente: %s\n", l; exit 1 }
    if (tem < quer[l]) { printf "trecho esperado %d vez(es), encontrado %d: %s\n", quer[l], tem, l; exit 1 }
  }
  j = 1
  for (i = 1; i <= r; i++) {
    while (j <= m && index(o[j], resultado[i]) == 0) j++
    if (j > m) { printf "RESULTADO fora de ordem: %s\n", resultado[i]; exit 1 }
    j++
  }
}'

tut_check() {
  local c line norm
  for c in "${CASOS[@]}"; do
    [ -f "$OUT/$c.log" ] || { echo "DIVERGENCIA caso=$c: out/$c.log ausente"; return 1; }
    [ -f "$EXPECTED/$c.txt" ] || { echo "DIVERGENCIA caso=$c: expected/$c.txt ausente"; return 1; }
    # sem o arquivo de regras (copia parcial) compara o out/ cru: mais estrito, nunca mais frouxo
    if [ -f "$NORMALIZA" ]; then norm="$(sed -E -f "$NORMALIZA" "$OUT/$c.log")"; else norm="$(cat "$OUT/$c.log")"; fi
    if ! line="$(printf '%s\n' "$norm" | awk -v esperado="$EXPECTED/$c.txt" "$TUT_CONFERE_AWK")"; then
      echo "DIVERGENCIA caso=$c: $line"
      return 1
    fi
    echo "caso $c: ok"
  done
  tut_check_imagem || return 1
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
