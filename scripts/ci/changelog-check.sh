#!/bin/sh
# Changelog obrigatorio (AUR-509), rodado pelo CI dentro da imagem golang
# alpine pinada (.github/workflows/changelog.yml). POSIX sh: a imagem nao tem
# bash nem git; o git e instalado porque o checkout chega empacotado.
# BASE_SHA vem do evento da PR; sem ele o check falha (nunca verde por falta
# de dado). O modo e lido do config.yml da base pelo proprio comando.
set -eu
if [ -z "${BASE_SHA:-}" ]; then
  echo "changelog: indeterminado: BASE_SHA ausente" >&2
  exit 1
fi
if ! command -v git >/dev/null 2>&1; then
  apk add --no-cache git >/dev/null
fi
git config --global --add safe.directory "$(pwd)"
export GOFLAGS=-buildvcs=false
go run ./cmd/aurumcode changelog --base "$BASE_SHA" --head HEAD --repo .
