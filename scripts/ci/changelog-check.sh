#!/bin/sh
# Changelog obrigatorio (AUR-509), rodado por .github/workflows/changelog.yml
# dentro da imagem golang alpine pinada, a partir do checkout da ferramenta.
# POSIX sh: a imagem nao tem bash nem git; o git e instalado porque o clone
# da PR chega empacotado. $1 e o checkout da PR (so dado, nunca executado).
# BASE_SHA e HEAD_SHA vem do evento; sem eles o check falha (nunca verde por
# falta de dado). O modo e lido do config.yml da base pelo proprio comando.
# Quando reprova, o comando imprime a entrada sugerida e, com
# GITHUB_STEP_SUMMARY montado pelo workflow, grava-a no resumo do job.
# PR_AUTHOR_LOGIN e PR_AUTHOR_TYPE (opcionais, do evento) viram --autor e
# --tipo-autor: um bot recebe changelog_check.bots. Sem eles o autor e
# humano e vale o modo declarado (AUR-610).
set -eu
target="${1:-}"
if [ -z "$target" ] || [ -z "${BASE_SHA:-}" ] || [ -z "${HEAD_SHA:-}" ]; then
  echo "changelog: indeterminado: checkout da PR, BASE_SHA ou HEAD_SHA ausente" >&2
  exit 1
fi
if ! command -v git >/dev/null 2>&1; then
  apk add --no-cache git >/dev/null
fi
git config --global --add safe.directory "$target"
export GOFLAGS=-buildvcs=false
bin="$(mktemp -d)/aurumcode"
go build -o "$bin" ./cmd/aurumcode
set -- --base "$BASE_SHA" --head "$HEAD_SHA" --repo "$target"
if [ -n "${PR_AUTHOR_LOGIN:-}" ]; then
  set -- "$@" --autor "$PR_AUTHOR_LOGIN"
fi
if [ -n "${PR_AUTHOR_TYPE:-}" ]; then
  set -- "$@" --tipo-autor "$PR_AUTHOR_TYPE"
fi
if [ -n "${POLICY_DIR:-}" ]; then
  "$bin" changelog "$@" --politica "$POLICY_DIR"
else
  AURUMCODE_POLICY='' "$bin" changelog "$@"
fi
