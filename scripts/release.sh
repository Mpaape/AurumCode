#!/usr/bin/env bash
# Release oficial do AurumCode (AUR-510). Documentacao: docs/releases.md.
#
#   scripts/release.sh preparar --sha <SHA> [--versao X.Y.Z] [--main <ref>]
#   scripts/release.sh publicar --sha <SHA> --versao X.Y.Z --evidencia-consumidor <dir>
#   scripts/release.sh fixar-exemplos --versao X.Y.Z
#
# preparar (padrao, so leitura): confere que o SHA esta na main, deriva a
# versao e as notas da primeira secao "## X.Y.Z" do CHANGELOG.md daquele SHA
# e recusa SHA fora da main, versao regressiva, tag existente com outro SHA,
# changelog incoerente e exemplo de instalacao que nao fixa a versao.
# publicar: repete o preparar, exige a evidencia do QA no consumidor
# (AUR-512) para o mesmo SHA e so entao cria a tag anotada e a release, com a
# identidade git e as credenciais do gh JA configuradas por quem roda (nunca
# define nem inventa uma). Repetir e idempotente; nada e sobrescrito.
# fixar-exemplos: troca a referencia dos exemplos para @vX.Y.Z (edita os
# arquivos locais para a PR de release; nao commita).
#
# Exit: 0 ok; 1 recusado; 2 uso; 79 pre-requisito ausente.
set -Eeuo pipefail

readonly exemplos=(.github/workflows/examples/code-review.yml docs/site/workflow.yml)
readonly guia=docs/getting-started.md
readonly secoes_notas=('### Entregue' '### Limites' '### Migração do v1')
readonly max_linhas_notas=120

recusa() { echo "release: recusado: $*" >&2; exit 1; }
uso() { echo "release: $*" >&2; exit 2; }
falta() { echo "release: pre-requisito ausente: $*" >&2; exit 79; }

semver_ok() { [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; }

# maior_tag: a maior versao vX.Y.Z ja marcada (vazio quando nao ha).
maior_tag() {
  git tag -l 'v[0-9]*.[0-9]*.[0-9]*' | sed 's/^v//' | while IFS= read -r v; do
    if semver_ok "$v"; then printf '%s\n' "$v"; fi
  done | sort -V | tail -n 1
}

# secao_versao CHANGELOG: a primeira "## X.Y.Z" (com data opcional) depois de
# "## Unreleased", sem o "v".
secao_versao() {
  awk '
    /^## / { h = $0; sub(/^## +/, "", h); sub(/ +- .*$/, "", h); gsub(/[][]/, "", h)
             if (h == "Unreleased") { seen = 1; next }
             if (seen && h ~ /^[0-9]+\.[0-9]+\.[0-9]+$/) { print h; exit } }
  ' <<<"$1"
}

# notas CHANGELOG VERSAO: o corpo da secao da versao.
notas() {
  awk -v v="$2" '
    /^## / { h = $0; sub(/^## +/, "", h); sub(/ +- .*$/, "", h); gsub(/[][]/, "", h)
             if (on) exit; if (h == v) { on = 1; next } }
    on { print }
  ' <<<"$1"
}

# confere SHA VERSAO MAIN: todas as recusas do preparar; imprime as notas.
confere() {
  local sha="$1" versao="$2" main="$3" changelog derivada corpo linhas maior marcado secao ex conteudo primeiro=''
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || uso "--sha precisa ser o SHA completo"
  git cat-file -e "$sha^{commit}" 2>/dev/null || recusa "SHA $sha nao existe neste clone"
  git rev-parse -q --verify "$main^{commit}" >/dev/null || falta "ref $main (faca git fetch)"
  git merge-base --is-ancestor "$sha" "$main" || recusa "SHA $sha fora de $main"
  changelog="$(git show "$sha:CHANGELOG.md" 2>/dev/null)" || recusa "CHANGELOG.md ausente em $sha"
  grep -q '^## Unreleased' <<<"$changelog" || recusa "CHANGELOG.md sem a secao ## Unreleased"
  derivada="$(secao_versao "$changelog")"
  [ -n "$derivada" ] || recusa "CHANGELOG.md sem secao de versao depois de ## Unreleased"
  if [ -z "$versao" ]; then versao="$derivada"; fi
  semver_ok "$versao" || uso "versao $versao nao e X.Y.Z"
  [ "$versao" = "$derivada" ] || recusa "a secao mais nova do CHANGELOG.md e $derivada, nao $versao"
  corpo="$(notas "$changelog" "$versao")"
  [ -n "$(tr -d '[:space:]' <<<"$corpo")" ] || recusa "notas de $versao vazias"
  for secao in "${secoes_notas[@]}"; do
    grep -Fxq -- "$secao" <<<"$corpo" || recusa "notas de $versao sem '$secao'"
  done
  linhas="$(wc -l <<<"$corpo")"
  [ "$linhas" -le "$max_linhas_notas" ] || recusa "notas de $versao com $linhas linhas (limite $max_linhas_notas)"
  if git rev-parse -q --verify "refs/tags/v$versao" >/dev/null; then
    marcado="$(git rev-list -n 1 "v$versao")"
    [ "$marcado" = "$sha" ] || recusa "tag v$versao ja existe em $marcado, nao em $sha"
  else
    maior="$(maior_tag)"
    if [ -n "$maior" ]; then
      [ "$(printf '%s\n%s\n' "$maior" "$versao" | sort -V | tail -n 1)" = "$versao" ] && [ "$versao" != "$maior" ] ||
        recusa "versao $versao nao e maior que v$maior"
    fi
  fi
  for ex in "${exemplos[@]}"; do
    conteudo="$(git show "$sha:$ex" 2>/dev/null)" || recusa "exemplo $ex ausente em $sha"
    grep -q "@v$versao\b" <<<"$conteudo" || recusa "exemplo $ex nao fixa @v$versao"
    if grep -Eq '@(main|v[0-9]+\.[0-9]+\.[0-9]+)\b' <<<"$(grep -v "@v$versao\b" <<<"$conteudo")"; then
      recusa "exemplo $ex referencia outra versao"
    fi
    if [ -z "$primeiro" ]; then primeiro="$conteudo"; elif [ "$conteudo" != "$primeiro" ]; then
      recusa "exemplos ${exemplos[*]} diferem em $sha"
    fi
  done
  git show "$sha:$guia" 2>/dev/null | grep -q "@v$versao\b" || recusa "$guia nao mostra a instalacao de @v$versao"
  printf 'release: v%s em %s\n' "$versao" "$sha"
  printf '%s\n' "$corpo"
}

# evidencia_ok DIR SHA: QA do consumidor (AUR-512) para este SHA, verificado
# pelo verificador do repositorio no container compartilhado.
evidencia_ok() {
  local dir="$1" sha="$2" f
  [ -d "$dir" ] || recusa "sem evidencia do consumidor em $dir"
  compgen -G "$dir/*.json" >/dev/null || recusa "sem evidencia do consumidor em $dir"
  command -v jq >/dev/null 2>&1 || falta jq
  for f in "$dir"/*.json; do
    jq -e --arg s "$sha" '.aurumcode_sha == $s' "$f" >/dev/null || recusa "evidencia $f nao e do SHA $sha"
  done
  [ -x .board/bin/go-shared ] || falta ".board/bin/go-shared (o verificador roda em container)"
  AURUMCODE_QA_EVIDENCIA="$dir" .board/bin/go-shared go test ./tests/consumer -count=1 \
    -run '^TestAUR512EvidenceIdentifiesTheRunAndInfraIsNotMeasured$' >/dev/null || recusa "o QA do consumidor reprova $sha"
}

publicar() {
  local sha="$1" versao="$2" main="$3" dir="$4" notas_arq
  [ -n "$versao" ] || uso "publicar exige --versao"
  [ -n "$(git config user.name || true)" ] && [ -n "$(git config user.email || true)" ] ||
    falta "identidade git (configure a sua; este script nunca a define)"
  command -v gh >/dev/null 2>&1 || falta gh
  gh auth status >/dev/null 2>&1 || falta "gh autenticado na conta de quem publica"
  notas_arq="$(mktemp)"
  trap 'rm -f -- "$notas_arq"' RETURN
  confere "$sha" "$versao" "$main" | tail -n +2 >"$notas_arq"
  evidencia_ok "$dir" "$sha"
  if git rev-parse -q --verify "refs/tags/v$versao" >/dev/null; then
    echo "release: tag v$versao ja aponta para $sha; nada a criar"
  else
    git tag -a "v$versao" "$sha" -m "AurumCode v$versao"
  fi
  git push origin "refs/tags/v$versao" || recusa "push da tag v$versao recusado (nunca forcado)"
  if gh release view "v$versao" >/dev/null 2>&1; then
    echo "release: v$versao ja publicada; nada sobrescrito"
  else
    gh release create "v$versao" --verify-tag --title "AurumCode v$versao" --notes-file "$notas_arq"
  fi
  [ "$(git rev-list -n 1 "v$versao")" = "$sha" ] || recusa "tag v$versao nao aponta para $sha"
}

fixar_exemplos() {
  local versao="$1" f
  semver_ok "$versao" || uso "--versao X.Y.Z"
  for f in "${exemplos[@]}" "$guia"; do
    sed -i -E "s#(Mpaape/AurumCode(/\.github/workflows/[a-z-]+\.yml)?)@(main|v[0-9]+\.[0-9]+\.[0-9]+)#\1@v$versao#g" "$f"
  done
  echo "release: exemplos fixados em @v$versao; revise e abra a PR de release"
}

acao="${1:-}"
[ "$#" -gt 0 ] && shift
sha='' versao='' main='origin/main' dir=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --sha)
      sha="${2:-}"; shift 2 ;;
    --versao)
      versao="${2:-}"; shift 2 ;;
    --main)
      main="${2:-}"; shift 2 ;;
    --evidencia-consumidor)
      dir="${2:-}"; shift 2 ;;
    *)
      uso "opcao desconhecida: $1" ;;
  esac
done
command -v git >/dev/null 2>&1 || falta git
case "$acao" in
  preparar)
    confere "$sha" "$versao" "$main" ;;
  publicar)
    publicar "$sha" "$versao" "$main" "$dir" ;;
  fixar-exemplos)
    fixar_exemplos "$versao" ;;
  *)
    uso "uso: release.sh preparar|publicar|fixar-exemplos (veja o cabecalho)" ;;
esac
