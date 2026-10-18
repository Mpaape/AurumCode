#!/bin/sh
# Busca so o historico de que a PR precisa, nunca o do repositorio inteiro.
# Rodado por .github/workflows/review.yml e changelog.yml logo depois do
# checkout raso da PR (fetch-depth: 1). $1 e o checkout da PR, $2 e $3 os
# SHAs de base e head do evento. Busca a base com --depth=1 e aprofunda em
# rodadas (--deepen=32, 64, 128...) ate o ancestral comum existir e nenhum
# commit de fronteira rasa (o arquivo shallow) cair dentro de
# `git rev-list HEAD --not BASE`, o mesmo criterio com que o Gitleaks aceita
# um clone raso. Nao ha teto de rodadas: o pior caso e o historico inteiro,
# o comportamento antigo. Sem ancestral comum com o historico inteiro, falha.
# GH_TOKEN (opcional) autentica so os comandos git deste script, pela
# configuracao de ambiente do git (GIT_CONFIG_*): nunca gravado no
# .git/config, nunca impresso, fora da linha de comando.
set -eu
dir="${1:-}"
base="${2:-}"
head="${3:-}"
title="Histórico da PR"

fail() {
  echo "::error title=$title::$1" >&2
  exit 1
}

[ -n "$dir" ] && [ -n "$base" ] && [ -n "$head" ] ||
  fail "uso: fetch-pr-range.sh DIR BASE_SHA HEAD_SHA (checkout, BASE_SHA ou HEAD_SHA ausente)"

# remote_git roda git no checkout com o token, quando houver, so neste
# processo.
remote_git() {
  if [ -z "${GH_TOKEN:-}" ]; then
    git -C "$dir" "$@"
    return
  fi
  server="${GITHUB_SERVER_URL:-https://github.com}"
  auth="$(printf '%s' "x-access-token:$GH_TOKEN" | base64 | tr -d '\n')"
  GIT_CONFIG_COUNT=1 \
    GIT_CONFIG_KEY_0="http.$server/.extraheader" \
    GIT_CONFIG_VALUE_0="AUTHORIZATION: basic $auth" \
    git -C "$dir" "$@"
}

# range_complete: o ancestral comum existe e nenhuma fronteira rasa esta
# dentro do intervalo da PR.
range_complete() {
  git -C "$dir" merge-base "$base" "$head" >/dev/null 2>&1 || return 1
  range="$(git -C "$dir" rev-list "$head" --not "$base")" || return 1
  shallow="$(git -C "$dir" rev-parse --git-path shallow)" || return 1
  case "$shallow" in
    /*) ;;
    *) shallow="$dir/$shallow" ;;
  esac
  [ -s "$shallow" ] || return 0
  [ -z "$range" ] && return 0
  ! printf '%s\n' "$range" | grep -qxF -f "$shallow"
}

is_shallow() {
  [ "$(git -C "$dir" rev-parse --is-shallow-repository)" = "true" ]
}

git -C "$dir" cat-file -e "$head^{commit}" 2>/dev/null ||
  fail "o commit head $head não está no checkout da PR"
if ! git -C "$dir" cat-file -e "$base^{commit}" 2>/dev/null; then
  remote_git fetch -q --no-tags --depth=1 origin "$base" ||
    fail "não foi possível buscar o commit base $base (git fetch falhou)"
fi

deepen=32
while ! range_complete; do
  if ! is_shallow; then
    fail "o repositório já tem o histórico inteiro e $base e $head não têm ancestral comum: a PR não tem intervalo para revisar"
  fi
  before="$(git -C "$dir" rev-list --count --all)"
  remote_git fetch -q --no-tags --deepen="$deepen" origin "$base" "$head" ||
    fail "git fetch --deepen=$deepen falhou ao buscar o histórico da PR"
  if [ "$(git -C "$dir" rev-list --count --all)" = "$before" ] && is_shallow; then
    # Nada novo chegou: busca o resto do historico de uma vez.
    remote_git fetch -q --no-tags --unshallow origin "$base" "$head" ||
      fail "git fetch --unshallow falhou ao buscar o histórico da PR"
  fi
  deepen=$((deepen * 2))
done
echo "histórico da PR completo: $(git -C "$dir" rev-list --count "$head" --not "$base") commit(s) da PR, $(git -C "$dir" rev-list --count --all) no clone"
