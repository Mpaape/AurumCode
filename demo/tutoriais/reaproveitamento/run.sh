#!/usr/bin/env bash
# Tutorial executavel: reaproveitamento de revisao e cache (AUR-562). Veja ../README.md e docs/tutorials/reaproveitamento.md.
#
#   run.sh reuso-por-arquivo|veredito-base-e-pr|modelo-mudou|politica-mudou|cache-degradado|falha-sem-cache
#   run.sh all | --check | limpar
#
# O cache fica em /work/.cache-aurum (AURUMCODE_CACHE_DIR), dentro do repositorio descartavel do caso.
# O GITHUB_SHA e fixo. O "modelo" e o JSON da fixture: trocar o conteudo muda a identidade do modelo.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"
# shellcheck source=../_lib/pr.sh
. "$HERE/../_lib/pr.sh"

CASOS=(reuso-por-arquivo veredito-base-e-pr modelo-mudou politica-mudou cache-degradado falha-sem-cache)

SHA=2222222222222222222222222222222222222222
CACHE=(-e AURUMCODE_CACHE_DIR=/work/.cache-aurum -e "GITHUB_SHA=$SHA")

# entradas: quantas entradas de veredito (verdict:*) e de arquivo ha no cache.
entradas() {
  local v a
  v="$(find "$TUT_WORK/.cache-aurum" -maxdepth 1 -name 'verdict:*' 2>/dev/null | wc -l)"
  a="$(find "$TUT_WORK/.cache-aurum" -maxdepth 1 -name '*.json' ! -name 'verdict:*' 2>/dev/null | wc -l)"
  echo "cache: $v entrada(s) de veredito, $a entrada(s) por arquivo"
}

# 1. Mesmo conteudo, mesmo modelo, mesma politica: o arquivo nao e reenviado ao modelo.
caso_reuso_por_arquivo() {
  tut_repo reuso-por-arquivo repo-exemplo/base repo-exemplo/segredo
  TUT_POLICY=politica-medium; TUT_FIXTURE=fixture-aviso.json; TUT_ENVS=("${CACHE[@]}")
  echo "--- primeira execucao: cache vazio"
  aurum review --base main --politica /policy
  expect_rc 3 "o achado reprova"
  entradas
  echo "--- segunda execucao: mesmo SHA, politica e modelo"
  aurum review --base main --politica /policy
  expect_rc 3 "o mesmo veredito, sem reenviar o arquivo ao modelo"
  entradas
}

# 2. --base e --pr compartilham o veredito? Tentativa contra o GitHub falso local.
caso_veredito_base_e_pr() {
  tut_repo veredito-base-e-pr repo-exemplo/base repo-exemplo/segredo
  tgit remote add origin https://git.example.com/OWNER/REPO.git
  TUT_POLICY=politica-medium; TUT_FIXTURE=fixture-aviso.json; TUT_ENVS=("${CACHE[@]}")
  tut_pr_servidor veredito-base-e-pr
  trap tut_pr_parar RETURN
  aurum review --base main --politica /policy
  entradas
  aurum_pr review --pr 7 --repo OWNER/REPO --publicar --modo-publicacao review --politica /policy
  expect_rc 3 "o --pr chega ao mesmo veredito"
  entradas
  echo "NAO DEMONSTRADO: o compartilhamento da mesma chave entre --base e --pr; neste experimento (diff do GitHub falso) cada caminho gravou a sua"
}

# 3. Modelo mudou: a identidade do modelo faz parte da chave; nada e reaproveitado.
caso_modelo_mudou() {
  tut_repo modelo-mudou repo-exemplo/base repo-exemplo/segredo
  TUT_POLICY=politica-medium; TUT_FIXTURE=none
  TUT_ENVS=("${CACHE[@]}" -e AURUMCODE_LLM_FIXTURE=/work/modelo.json)
  cp "$HERE/fixture-aviso.json" "$TUT_WORK/modelo.json"
  echo "--- modelo A (a resposta tem um achado)"
  aurum review --base main --politica /policy
  expect_rc 3 "modelo A reprova"
  echo "--- modelo B (outra resposta; mesmo SHA, politica e arquivos)"
  cp "$HERE/fixture-vazia.json" "$TUT_WORK/modelo.json"
  aurum review --base main --politica /policy
  expect_rc 0 "com outro modelo nada e reaproveitado: a resposta nova vale"
  entradas
}

# 4. Politica mudou: o veredito nao atravessa politicas diferentes.
caso_politica_mudou() {
  tut_repo politica-mudou repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-aviso.json; TUT_ENVS=("${CACHE[@]}")
  TUT_POLICY=politica-high
  aurum review --base main --politica /policy
  expect_rc 0 "politica high: o warning passa"
  entradas
  TUT_POLICY=politica-medium
  aurum review --base main --politica /policy
  expect_rc 3 "politica medium: o mesmo warning reprova; a politica nova gera outra entrada de veredito (a resposta por arquivo segue reaproveitada)"
  entradas
}

# 5. Cache degradado: entradas corrompidas viram revisao nova, nunca aprovacao.
caso_cache_degradado() {
  tut_repo cache-degradado repo-exemplo/base repo-exemplo/segredo
  TUT_POLICY=politica-medium; TUT_FIXTURE=fixture-aviso.json; TUT_ENVS=("${CACHE[@]}")
  aurum review --base main --politica /policy
  expect_rc 3 "primeira execucao reprova e grava o cache"
  local f
  for f in "$TUT_WORK"/.cache-aurum/*; do printf '{isto nao e json' > "$f"; done
  echo "--- todas as entradas do cache corrompidas"
  aurum review --base main --politica /policy
  expect_rc 3 "cache corrompido: revisao nova, o achado continua reprovando"
  if grep -q 'reused' <<<"$LAST_OUT"; then echo "ERRO: reaproveitou cache corrompido"; return 1; fi
  echo "nenhuma linha 'reused': nada foi reaproveitado do cache corrompido"
}

# Falha: sem AURUMCODE_CACHE_DIR nao ha reaproveitamento, e o produto diz isso; inconclusivo nao grava.
caso_falha_sem_cache() {
  tut_repo falha-sem-cache repo-exemplo/base repo-exemplo/segredo
  TUT_POLICY=politica-medium; TUT_FIXTURE=fixture-aviso.json; TUT_ENVS=()
  echo "--- sem AURUMCODE_CACHE_DIR"
  aurum review --base main --politica /policy
  expect_rc 3 "sem cache a revisao funciona, mas o veredito nao e compartilhado"
  echo "--- revisao inconclusiva nunca grava veredito"
  tut_repo falha-sem-cache-inconclusivo repo-exemplo/base repo-exemplo/gerado
  TUT_POLICY=politica-bloqueia; TUT_FIXTURE=fixture-vazia.json; TUT_ENVS=("${CACHE[@]}")
  aurum review --base main --politica /policy
  expect_rc 1 "inconclusivo em block reprova"
  entradas
}

tut_main "$@"
