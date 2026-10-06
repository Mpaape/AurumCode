#!/usr/bin/env bash
# Checks the documentation screenshots against their manifest without a
# browser or docker (bash, awk, sed and sha256sum only):
#   - the manifest pins the Playwright image of scripts/docs/playwright.lock;
#   - every manifest image exists and the digest of its input (out/ log or
#     page source) still matches: an out/ changed without regenerating fails;
#   - every capture referenced by a docs page exists and is in the manifest,
#     and no PNG under docs/assets/capturas is missing from it;
#   - every capability page has a desktop and a mobile capture;
#   - every tutorial has the "Como fica" section with a capture per case, and
#     docs/index.md and docs/extensao.md reference captures.
# Usage: capturas-check.sh [repo-root]. Exit 0 ok, 1 divergence, 2 missing input.
set -Eeuo pipefail
export LC_ALL=C
root="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$root"
readonly dir='docs/assets/capturas' manifesto='docs/assets/capturas/capturas.json'
readonly paginas_fixas='index configuration gate-corporativo architecture extensao'
erros=0
falha() { printf 'capturas-check: %s\n' "$1" >&2; erros=$((erros + 1)); }
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1"; else shasum -a 256 "$1"; fi | awk '{print $1}'; }
campo() { sed -n "s/.*\"$1\":\"\\([^\"]*\\)\".*/\\1/p" <<<"$2"; }

[[ -f "$manifesto" ]] || { echo "capturas-check: manifesto ausente: $manifesto (rode scripts/docs/capturas.sh)" >&2; exit 2; }
[[ -f scripts/docs/playwright.lock ]] || { echo "capturas-check: scripts/docs/playwright.lock ausente" >&2; exit 2; }
lock="$(sed -n '1p' scripts/docs/playwright.lock)"
[[ "$lock" =~ @sha256:[0-9a-f]{64}$ ]] || falha "playwright.lock nao pina a imagem por digest: $lock"

declare -A no_manifesto=()
n=0
while IFS= read -r linha; do
  img="$(campo imagem "$linha")"; insumo="$(campo insumo "$linha")"
  dig="$(campo insumo_sha256 "$linha")"; pw="$(campo playwright "$linha")"
  # O digest e gravado so com o prefixo sha256: (um hex nu depois de "insumo"
  # casa com a regra de token do gitleaks); valor sem o prefixo reprova.
  [[ -n "$img" && -n "$insumo" && "$dig" =~ ^sha256:[0-9a-f]{64}$ ]] || { falha "entrada invalida no manifesto: $linha"; continue; }
  dig="${dig#sha256:}"
  no_manifesto["$img"]=1; n=$((n + 1))
  [[ "$pw" == "$lock" ]] || falha "$img: imagem Playwright $pw difere de playwright.lock"
  [[ -f "$img" ]] || falha "$img: captura do manifesto ausente"
  if [[ ! -f "$insumo" ]]; then falha "$img: insumo ausente: $insumo"
  elif [[ "$(sha "$insumo")" != "$dig" ]]; then falha "$img: insumo $insumo mudou sem regenerar a captura (digest diverge do manifesto; rode scripts/docs/capturas.sh)"
  fi
done < <(grep '^ *{"imagem":' "$manifesto" || true)
(( n > 0 )) || falha "manifesto sem capturas"

# Every PNG on disk is in the manifest.
while IFS= read -r png; do
  [[ -n "${no_manifesto[$png]+x}" ]] || falha "$png: fora do manifesto"
done < <(find "$dir" -type f -name '*.png' | sort)

# Every capture referenced by a page exists and is in the manifest.
refs=0
while IFS= read -r ref; do
  img="docs/${ref}"; refs=$((refs + 1))
  [[ -f "$img" ]] || falha "$img: captura referenciada ausente"
  [[ -n "${no_manifesto[$img]+x}" ]] || falha "$img: captura referenciada fora do manifesto"
done < <(find docs -name '*.md' -type f -exec grep -ohE 'assets/capturas/[A-Za-z0-9._/-]+\.png' {} + | sort -u)

# Capability pages: desktop and mobile.
paginas="$paginas_fixas"
for d in demo/tutoriais/*/; do
  t="$(basename "$d")"
  [[ "$t" != _* && -d "$d/out" && -f "docs/tutorials/$t.md" ]] && paginas+=" tutorials-$t"
done
for p in $paginas; do
  for vista in desktop mobile; do
    [[ -n "${no_manifesto[$dir/paginas/$p-$vista.png]+x}" ]] || falha "pagina $p sem captura $vista"
  done
done

# Tutorials: "Como fica" with a capture per case.
for d in demo/tutoriais/*/; do
  t="$(basename "$d")"; doc="docs/tutorials/$t.md"
  [[ "$t" != _* && -d "$d/out" && -f "$doc" ]] || continue
  grep -q '^## Como fica$' "$doc" || falha "$doc sem a secao Como fica"
  for log in "$d"out/*.log; do
    [[ -f "$log" ]] || continue
    c="$(basename "$log" .log)"
    grep -qF "assets/capturas/$t/$c-" "$doc" || falha "$doc: caso $c sem captura em Como fica"
    [[ -n "${no_manifesto[$dir/$t/$c-terminal.png]+x}" ]] || falha "$t/$c sem captura do terminal no manifesto"
  done
done
for doc in docs/index.md docs/extensao.md; do
  grep -qE 'assets/capturas/[A-Za-z0-9._/-]+\.png' "$doc" || falha "$doc nao referencia capturas"
done

if (( erros > 0 )); then
  printf 'capturas-check: FALHOU (%d divergencias)\n' "$erros" >&2
  exit 1
fi
printf 'capturas-check: ok (%d capturas no manifesto, %d referencias)\n' "$n" "$refs"
