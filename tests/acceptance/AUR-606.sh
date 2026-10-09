#!/usr/bin/env bash
# AUR-606 acceptance: the guide "Instalar com ajuda da IA" lets a person ask
# an AI agent to install and configure AurumCode by conversation.
#
# Selectors:
#   all      AC-001..AC-003, then MUT-001
#   AC-001   the guide exists and the documentation index links it
#   AC-002   it carries the request to paste and the six steps of the
#            agent's script (inspect, one question at a time, minimum
#            configuration, only what was chosen, verify, summary)
#   AC-003   the request never asks for keys in the chat and the agent is
#            told not to weaken a rule to get a green result
#   MUT-001  a copy of the docs without the index link must fail AC-001
# Unknown selectors exit 64.
set -euo pipefail

repo_root="$(cd -- "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
guide="docs/tutorials/instalacao-ia.md"

fail() { printf 'AUR-606/%s\n' "$1" >&2; exit 1; }

# ac001 ROOT: the guide exists and docs/README.md links it.
ac001() {
  [[ -s "$1/$guide" ]] || fail "AC-001/guia-ausente"
  grep -qF '(tutorials/instalacao-ia.md)' "$1/docs/README.md" || fail "AC-001/indice-sem-link"
}

ac002() {
  grep -qx '## Copie este pedido' "$repo_root/$guide" || fail "AC-002/pedido-ausente"
  local n
  for n in 1 2 3 4 5 6; do
    grep -qE "^### $n\. " "$repo_root/$guide" || fail "AC-002/passo-$n-ausente"
  done
  grep -qF 'uma pergunta por vez' "$repo_root/$guide" || fail "AC-002/uma-pergunta-por-vez"
}

ac003() {
  grep -qF 'Não peça chaves no chat' "$repo_root/$guide" || fail "AC-003/chave-no-chat"
  grep -qF 'não remova a regra para fazer o teste passar' "$repo_root/$guide" || fail "AC-003/afrouxar-regra"
}

mut001() {
  local copy
  copy="$(mktemp -d)"
  trap 'rm -rf -- "$copy"' RETURN
  mkdir -p "$copy/docs/tutorials"
  cp "$repo_root/$guide" "$copy/$guide"
  grep -vF '(tutorials/instalacao-ia.md)' "$repo_root/docs/README.md" > "$copy/docs/README.md" || true
  if ( ac001 "$copy" ) 2>/dev/null; then
    fail "MUT-001/indice-sem-link-passou"
  fi
  printf 'AUR-606/MUT-001/rejected\n'
}

case "${1:-all}" in
  all) ac001 "$repo_root"; ac002; ac003; mut001; printf 'AUR-606/all/pass\n' ;;
  AC-001) ac001 "$repo_root"; printf 'AUR-606/AC-001/pass\n' ;;
  AC-002) ac002; printf 'AUR-606/AC-002/pass\n' ;;
  AC-003) ac003; printf 'AUR-606/AC-003/pass\n' ;;
  MUT-001) mut001 ;;
  *) printf 'AUR-606/%s/unknown-selector\n' "$1" >&2; exit 64 ;;
esac
