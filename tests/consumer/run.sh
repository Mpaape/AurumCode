#!/usr/bin/env bash
# QA do AurumCode instalado num repositorio consumidor separado (AUR-512).
#
#   tests/consumer/run.sh --repo OWNER/CONSUMIDOR --sha <SHA do AurumCode> \
#     --evidencia <dir> [--manter] [cenario ...]
#
# Para cada cenario de cenarios.json: cria a branch qa/<cenario>-<sha7> a partir
# da main do consumidor, copia fixtures/<fixture> e instala
# workflows/<workflow> como .github/workflows/aurumcode.yml com o SHA sob teste,
# commita com a identidade git JA configurada (nunca configura nem inventa
# uma), abre a PR, espera o workflow, coleta status, checks, comentarios,
# review, sugestoes inline e o log de falha, e grava <dir>/<cenario>.json.
# Sem --manter, fecha a PR e apaga a branch no fim.
#
# Pre-requisitos no host: git, gh (autenticado na conta do dono), jq. A main do
# consumidor ja tem fixtures/base (este script nunca escreve na main) e os
# secrets LLM_API_KEY e LLM_BASE_URL. A verificacao roda depois, no container:
#
#   .board/bin/go-shared exec -w "$PWD" env AURUMCODE_QA_EVIDENCIA="$PWD/<dir>" go test ./tests/consumer -run TestAUR512
#
# Falha de infraestrutura (billing, run que nao comeca, API fora) vira
# evidencia "medido": false com a limitacao: nunca aprovacao.
# Exit: 0 evidencia gravada; 2 uso; 79 pre-requisito ausente.
set -Eeuo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo='' sha='' out='' manter=0
cenarios=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo)
      repo="${2:-}"; shift 2 ;;
    --sha)
      sha="${2:-}"; shift 2 ;;
    --evidencia)
      out="${2:-}"; shift 2 ;;
    --manter)
      manter=1; shift ;;
    -*)
      echo "opcao desconhecida: $1" >&2; exit 2 ;;
    *)
      cenarios+=("$1"); shift ;;
  esac
done
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "--repo OWNER/NOME obrigatorio" >&2; exit 2; }
[[ "$sha" =~ ^[0-9a-f]{40}$ ]] || { echo "--sha precisa ser o SHA completo do AurumCode sob teste" >&2; exit 2; }
[ -n "$out" ] || { echo "--evidencia <dir> obrigatorio" >&2; exit 2; }
for bin in git gh jq; do
  command -v "$bin" >/dev/null 2>&1 || { echo "pre-requisito ausente: $bin" >&2; exit 79; }
done
[ -n "$(git config user.name || true)" ] && [ -n "$(git config user.email || true)" ] || {
  echo "identidade git nao configurada: configure a sua; este script nunca a define" >&2; exit 79; }
mkdir -p "$out"
if [ "${#cenarios[@]}" -eq 0 ]; then
  mapfile -t cenarios < <(jq -r '.[] | select(.manual != true) | .id' "$here/cenarios.json")
fi

work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT
gh repo clone "$repo" "$work/consumidor" -- -q
cd "$work/consumidor"
grep -q 'changelog_check' .aurumcode/config.yml 2>/dev/null || {
  echo "a main de $repo nao tem tests/consumer/fixtures/base; prepare-a por PR antes do QA" >&2; exit 79; }

# espera_run BRANCH DEPOIS_DE: imprime "id url conclusao" do run mais novo da
# branch criado depois do instante dado, ou falha em 30 minutos.
espera_run() {
  local branch="$1" depois="$2" i linha
  for i in $(seq 1 180); do
    linha="$(gh run list --repo "$repo" --branch "$branch" --limit 5 --json databaseId,url,status,conclusion,createdAt \
      --jq "[.[] | select(.createdAt > \"$depois\")][0] | select(. != null) | \"\(.databaseId) \(.url) \(.status) \(.conclusion)\"" 2>/dev/null || true)"
    if [ -n "$linha" ] && [ "$(echo "$linha" | cut -d' ' -f3)" = completed ]; then
      echo "$linha" | cut -d' ' -f1,2,4
      return 0
    fi
    sleep 10
  done
  return 1
}

# coleta CENARIO PR RUN_ID RUN_URL CONCLUSAO: o JSON de evidencia de uma rodada.
coleta() {
  local cenario="$1" pr="$2" run_id="$3" run_url="$4" conclusao="$5" head status checks texto log idioma modo inline comentarios
  head="$(git rev-parse HEAD)"
  status="$(gh api "repos/$repo/commits/$head/status" --jq '[.statuses[] | {(.context): .state}] | add // {}')"
  checks="$(gh api "repos/$repo/commits/$head/check-runs" --jq '[.check_runs[] | {(.name): .conclusion}] | add // {}')"
  texto="$(gh api "repos/$repo/issues/$pr/comments" --jq '[.[] | select(.body | contains("<!-- aurumcode-review -->")) | .body] | join("\n")')"
  comentarios="$(gh api "repos/$repo/issues/$pr/comments" --jq '[.[] | select(.body | contains("<!-- aurumcode-review -->"))] | length')"
  modo=comments
  if [ "$(gh api "repos/$repo/pulls/$pr/reviews" --jq '[.[] | select(.body | contains("<!-- aurumcode-review -->"))] | length')" -gt 0 ]; then
    modo=review
    texto="$texto
$(gh api "repos/$repo/pulls/$pr/reviews" --jq '[.[] | .body] | join("\n")')"
  fi
  inline="$(gh api "repos/$repo/pulls/$pr/comments" --jq '[.[] | select(.body | contains("```suggestion"))] | length')"
  log="$(gh run view "$run_id" --repo "$repo" --log-failed 2>/dev/null | tail -n 200 || true)"
  idioma=''
  if printf '%s' "$texto" | grep -q 'Veredito'; then idioma=pt-BR; elif printf '%s' "$texto" | grep -q 'Verdict'; then idioma=en; fi
  jq -n --arg c "$cenario" --arg t "$sha" --arg r "$repo" --arg s "$head" --argjson id "$run_id" --arg url "$run_url" \
    --arg conc "$conclusao" --argjson st "$(jq -n --argjson a "$status" --argjson b "$checks" '$a + $b')" \
    --arg idioma "$idioma" --arg modo "$modo" --argjson inline "$inline" --argjson com "$comentarios" \
    --arg texto "$texto
$log" \
    '{cenario: $c, aurumcode_sha: $t, repo: $r, sha: $s, run_id: $id, run_url: $url, medido: true, limitacoes: [],
      conclusao: $conc, status: $st, idioma: $idioma, modo: $modo, sugestoes_inline: $inline,
      comentarios_aurum: $com, texto: $texto}'
}

nao_medido() {
  jq -n --arg c "$1" --arg t "$sha" --arg r "$repo" --arg why "$2" '{cenario: $c, aurumcode_sha: $t, repo: $r, medido: false, limitacoes: [$why]}' >"$out/$1.json"
  echo "$1: nao medido ($2)"
}

# instala FIXTURE WORKFLOW: copia a fixture e o workflow pinado e commita.
instala() {
  cp -R "$here/fixtures/$1/." .
  mkdir -p .github/workflows
  sed "s/AURUMCODE_SHA/$sha/g" "$here/workflows/$2" > .github/workflows/aurumcode.yml
  git add -A
  git commit -q -m "QA AUR-512: $1 com $2"
}

for cenario in "${cenarios[@]}"; do
  def="$(jq -c --arg id "$cenario" '.[] | select(.id == $id)' "$here/cenarios.json")"
  [ -n "$def" ] || { echo "cenario desconhecido: $cenario" >&2; exit 2; }
  fixture="$(jq -r '.fixture' <<<"$def")"; workflow="$(jq -r '.workflow' <<<"$def")"
  correcao="$(jq -r '.correcao // ""' <<<"$def")"; rodadas="$(jq -r '.rodadas // 1' <<<"$def")"
  branch="qa/$cenario-${sha:0:7}"
  git checkout -q main
  git checkout -q -B "$branch"
  instala "$fixture" "$workflow"
  inicio="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  git push -q -f origin "$branch" || { nao_medido "$cenario" "push recusado"; continue; }
  pr="$(gh pr create --repo "$repo" --head "$branch" --base main --title "QA AUR-512: $cenario" \
    --body "QA do AurumCode $sha no consumidor (AUR-512), cenário $cenario. Fechada pelo script." | sed 's#.*/##')" ||
    { nao_medido "$cenario" "PR nao aberta"; continue; }
  run="$(espera_run "$branch" "$inicio")" || { nao_medido "$cenario" "workflow nao concluiu em 30 min (billing ou fila)"; continue; }
  for ((i = 1; i < rodadas; i++)); do
    inicio="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    gh run rerun "${run%% *}" --repo "$repo" >/dev/null
    run="$(espera_run "$branch" "$inicio")" || run="$(gh run view "${run%% *}" --repo "$repo" --json databaseId,url,conclusion --jq '"\(.databaseId) \(.url) \(.conclusion)"')"
  done
  read -r run_id run_url conclusao <<<"$run"
  evid="$(coleta "$cenario" "$pr" "$run_id" "$run_url" "$conclusao")"
  if [ -n "$correcao" ]; then
    instala "$correcao" "$workflow"
    inicio="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    git push -q origin "$branch"
    if run="$(espera_run "$branch" "$inicio")"; then
      read -r run_id run_url conclusao <<<"$run"
      evid="$(jq --argjson d "$(coleta "$cenario" "$pr" "$run_id" "$run_url" "$conclusao")" '. + {apos_correcao: $d}' <<<"$evid")"
    fi
  fi
  printf '%s\n' "$evid" >"$out/$cenario.json"
  echo "$cenario: evidencia gravada (PR #$pr, run $run_id)"
  [ "$manter" = 1 ] || gh pr close "$pr" --repo "$repo" --delete-branch >/dev/null || true
done
