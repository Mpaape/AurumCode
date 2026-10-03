#!/usr/bin/env bash
# Auxiliares dos tutoriais da cadeia de suprimentos (AUR-563). Carregue DEPOIS
# de _lib/tutorial.sh:  . "$HERE/../_lib/cadeia.sh"
#
# As imagens de Trivy, Cosign, Dependency-Track e PostgreSQL sao as MESMAS do
# guia corporativo (AUR-554): este arquivo le o images.lock dele, sem copiar
# digest algum. Semgrep roda dentro da imagem do produto (versao igual a
# .board/bootstrap/locks/scanners.yml).
: "${REPO_ROOT:?carregue _lib/tutorial.sh antes}"
CAD_LOCK="$REPO_ROOT/demo/gate-corporativo/images.lock"

# cad_lock NOME: a referencia por digest de NOME em images.lock.
cad_lock() { sed -n "s/^$1=//p" "$CAD_LOCK" | head -n1; }

# cad_pull NOME: garante a imagem fixada por digest (baixa so se faltar).
cad_pull() {
  local ref; ref="$(cad_lock "$1")"
  case "$ref" in *@sha256:*) ;; *) echo "ERRO: $1 sem digest em images.lock" >&2; return 1 ;; esac
  docker image inspect "$ref" >/dev/null 2>&1 || docker pull -q "$ref" >/dev/null
}

# cad_bins: extrai os binarios estaticos do Trivy e do Cosign das imagens
# fixadas para $STATE/bin (idempotente). Os caminhos dentro das imagens sao os
# mesmos que demo/gate-corporativo/run.sh usa.
cad_bins() {
  mkdir -p "$STATE/bin" "$STATE/trivy-cache"
  local c
  if [ ! -x "$STATE/bin/trivy" ]; then
    cad_pull trivy
    c="$(docker create "$(cad_lock trivy)")"; docker cp "$c:/usr/local/bin/trivy" "$STATE/bin/trivy"; docker rm "$c" >/dev/null
  fi
  if [ ! -x "$STATE/bin/cosign" ]; then
    cad_pull cosign
    c="$(docker create "$(cad_lock cosign)")"; docker cp "$c:/ko-app/cosign" "$STATE/bin/cosign"; docker rm "$c" >/dev/null
  fi
  chmod +x "$STATE/bin/"*
}

# cad_run [opcoes docker ...] -- ARGV...: roda ARGV na imagem do produto (qualquer
# entrypoint), como o usuario do host, com o repositorio do caso em /work, o
# diretorio do tutorial em /fixtures (somente leitura) e, se existirem, os
# binarios em /demo/bin. Rede: nenhuma (--network none), salvo CAD_NET=<rede>.
cad_run() {
  local opts=()
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do opts+=("$1"); shift; done
  [ "${1:-}" = "--" ] && shift
  local binmount=()
  [ ! -d "$STATE/bin" ] || binmount=(-v "$STATE/bin:/demo/bin:ro")
  docker run --rm --network "${CAD_NET:-none}" --user "$(id -u):$(id -g)" -e HOME=/tmp \
    "${binmount[@]}" "${opts[@]}" \
    -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work "$TUT_IMAGE" "$@"
}
