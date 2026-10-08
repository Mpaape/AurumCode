#!/bin/sh
# Ciclo de realimentacao (AUR-532), rodado por
# .github/workflows/realimentacao.yml dentro da imagem golang alpine pinada,
# a partir do checkout do AurumCode. POSIX sh: a imagem nao tem bash.
# SIGNALS_TOKEN le a org; POLICY_TOKEN (o token do job) grava so no
# repositorio da politica. Nenhuma identidade git e configurada: a escrita e
# pela API de conteudo, com o token do workflow.
set -eu
if [ -z "${POLICY_REPO:-}" ] || { [ -z "${ORG:-}" ] && [ -z "${REPOS:-}" ]; }; then
  echo "realimentacao: POLICY_REPO e ORG ou REPOS sao obrigatorios" >&2
  exit 2
fi
export GOFLAGS=-buildvcs=false
go build -o /tmp/aurumcode ./cmd/aurumcode
set -- --repo-politica "$POLICY_REPO" --publicar
if [ -n "${ORG:-}" ]; then
  set -- "$@" --org "$ORG"
fi
if [ -n "${REPOS:-}" ]; then
  set -- "$@" --repos "$REPOS"
fi
AURUMCODE_SIGNALS_TOKEN="${SIGNALS_TOKEN:-}" GITHUB_TOKEN="${POLICY_TOKEN:-}" /tmp/aurumcode realimentacao "$@"
