#!/usr/bin/env bash
# Tutorial executavel: dados de analise (AUR-563). Veja docs/tutorials/dados-de-analise.md.
#
#   run.sh declarado-ou-nao|vencido|cache|workflow-agendado|adulterado|indisponivel
#   run.sh all | --check | limpar
#
# O review consulta o endereco de AURUMCODE_GITHUB_API_URL (padrao
# https://api.github.com). Para provar cada desfecho com o binario real, o container
# do produto roda, no MESMO container, um servidor simples em loopback
# (servidor-local.py, http://127.0.0.1:8080) e a variavel aponta para ele: sem
# DNS, sem CA, sem TLS de demonstracao. Rede do container: none.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(declarado-ou-nao vencido cache workflow-agendado adulterado indisponivel)

# aurum_srv MODO ARGS...: como `aurum`, mas com o servidor local (MODO) apontado pela variavel.
# SRV_CACHE=<dir do host> monta o cache de dados de analise (/tmp/.cache) para
# persistir entre execucoes; sem ele cada execucao comeca sem cache.
aurum_srv() {
  local modo="$1"; shift
  local cache=()
  [ -z "${SRV_CACHE:-}" ] || { mkdir -p "$SRV_CACHE"; cache=(-v "$SRV_CACHE:/tmp/.cache"); }
  printf '$ aurumcode %s\n' "$*"
  set +e
  LAST_OUT="$(docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e AURUMCODE_GITHUB_API_URL=http://127.0.0.1:8080 \
    -e AURUMCODE_LLM_FIXTURE=/fixtures/fixture-llm.json "${cache[@]}" \
    -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /fixtures/dentro.sh "$TUT_IMAGE" "$modo" "$@" 2>&1)"
  LAST_RC=$?
  set -e
  [ -z "$LAST_OUT" ] || printf '%s\n' "$LAST_OUT"
  echo "exit_code=$LAST_RC"
}

# mostra o bloco analysis_data da auditoria (lido no host com python3).
mostra_auditoria() {
  python3 - "$TUT_WORK/audit.json" <<'PY'
import json, sys
a = json.load(open(sys.argv[1])).get("analysis_data")
if a is None:
    print("auditoria: sem campo analysis_data")
else:
    for k in ("source", "tag", "generated_at", "digest"):
        print("auditoria analysis_data.%s: %s" % (k, a.get(k)))
PY
}

# repositorio do caso: base + mudanca + configuracao (config/<nome>)
repo() { tut_repo "$1" repo-exemplo/base repo-exemplo/mudanca "config/$2"; }

# 1. analysis_data nao declarado: zero rede. Declarado: consulta e registra na auditoria.
caso_declarado_ou_nao() {
  echo "--- A. nao declarado (o servidor local esta no ar, mas ninguem o consulta)"
  repo declarado-ou-nao-a sem-declarar
  aurum_srv valido review --base main --auditoria audit.json
  expect_rc 0 "sem analysis_data declarado o review nao fez nenhuma requisicao e nao imprimiu linha analysis_data"
  printf 'linhas citando analysis_data na saida do review: %s\n' "$(printf '%s\n' "$LAST_OUT" | grep -c 'analysis_data' || true)"
  mostra_auditoria
  echo "--- B. declarado, artefato valido de 1 dia (max_age_days: 7)"
  repo declarado-ou-nao-b bloqueia
  aurum_srv valido review --base main --auditoria audit.json
  expect_rc 0 "artefato valido: o review aprovou e registrou digest e data na auditoria"
  mostra_auditoria
}

# 2. vencido: 30 dias contra max_age_days: 7. block reprova, warn so retem a aprovacao.
caso_vencido() {
  echo "--- gate.inconclusive: block"
  repo vencido-block bloqueia
  aurum_srv vencido review --base main
  expect_rc 1 "artefato vencido com block: o review falha (analysis_data_stale)"
  echo "--- gate.inconclusive: warn"
  repo vencido-warn avisa
  aurum_srv vencido review --base main
  expect_rc 0 "artefato vencido com warn: exit 0, mas a linha inconclusiva continua no parecer"
}

# 3. cache: a primeira execucao baixa e guarda; a segunda, com a listagem fora do ar, usa a copia.
caso_cache() {
  SRV_CACHE="$STATE/cache-cache"; rm -rf "$SRV_CACHE"
  echo "--- 1. servidor no ar: baixa, verifica e guarda no cache"
  repo cache bloqueia
  aurum_srv valido review --base main --auditoria audit.json
  expect_rc 0 "primeira execucao: artefato remoto verificado e guardado no cache"
  mostra_auditoria
  echo "--- 2. a listagem de releases cai (HTTP 503); a copia em cache, ainda dentro da idade, e usada"
  aurum_srv indisponivel review --base main --auditoria audit.json
  expect_rc 0 "listagem fora do ar com copia valida em cache: usa o cache e o declara"
  mostra_auditoria
  echo "--- 3. sem cache nenhum e a listagem fora do ar: inconclusivo"
  SRV_CACHE="$STATE/cache-vazio"; rm -rf "$SRV_CACHE"
  aurum_srv indisponivel review --base main
  expect_rc 1 "sem copia em cache e sem listagem: analysis_data_unavailable"
  SRV_CACHE=
}

# 4. workflow agendado que publica: conferencia estatica (nao executado aqui).
caso_workflow_agendado() {
  local wf="$REPO_ROOT/.github/workflows/analysis-data.yml" copia="$HERE/workflow-publicador.yml" pub="$REPO_ROOT/scripts/artifacts/publish.sh"
  echo "NAO EXECUTADO: o workflow roda so no GitHub Actions; aqui so a conferencia estatica do arquivo."
  diff -q "$wf" "$copia" >/dev/null && echo "workflow-publicador.yml: identico a .github/workflows/analysis-data.yml"
  grep -qF "cron: '17 3 * * *'" "$copia" && echo "gatilho: schedule diario (cron 17 3 * * *)"
  grep -q '^  workflow_dispatch:' "$copia" && echo "gatilho: workflow_dispatch (disparo manual)"
  grep -qE '^    needs: build$' "$copia" && echo "publish depende de build: teste falhando nao publica"
  grep -qE '^      contents: write$' "$copia" && echo "so o job publish recebe contents: write"
  grep -q 'analysis-data/\*' "$pub" && echo "publish.sh: so aceita tag analysis-data/*"
  grep -q -- '--draft --latest=false' "$pub" && grep -q -- '--draft=false --latest=false' "$pub" && echo "publish.sh: cria como rascunho e so depois publica (nunca fica visivel pela metade)"
  grep -q 'never touched' "$pub" && echo "publish.sh: tag existente nunca e tocada"
  echo "RESULTADO: o workflow tem o agendamento, o teste antes de publicar e a publicacao em duas etapas que o texto descreve (conferencia estatica; a imutabilidade do release depende de 'Immutable releases' no repositorio e NAO foi demonstrada)"
}

# Falha: arquivo ou manifesto adulterado.
caso_adulterado() {
  echo "--- A. scanners.yml servido diferente do digest do manifesto"
  repo adulterado-arquivo bloqueia
  aurum_srv adulterado-arquivo review --base main
  expect_rc 1 "arquivo adulterado: analysis_data_digest_mismatch reprova"
  echo "--- B. set_digest do manifesto nao confere com a lista de arquivos"
  repo adulterado-manifesto bloqueia
  aurum_srv adulterado-manifesto review --base main
  expect_rc 1 "manifesto adulterado: analysis_data_digest_mismatch reprova"
}

# Falha: fonte indisponivel.
caso_indisponivel() {
  echo "--- A. a listagem responde HTTP 503 e nao ha cache"
  repo indisponivel-503 bloqueia
  aurum_srv indisponivel review --base main
  expect_rc 1 "listagem 503 sem cache: analysis_data_unavailable reprova"
  echo "--- B. sem rede nenhuma (container sem rota, nada escuta)"
  repo indisponivel-rede bloqueia
  TUT_SED='s/(analysis_data_unavailable\)): .*/\1: <detalhe de rede omitido do registro>/'
  aurum review --base main
  TUT_SED=
  expect_rc 1 "sem rede e sem cache: analysis_data_unavailable reprova"
  echo "--- C. o mesmo com gate.inconclusive: warn"
  repo indisponivel-warn avisa
  TUT_SED='s/(analysis_data_unavailable\)): .*/\1: <detalhe de rede omitido do registro>/'
  aurum review --base main
  TUT_SED=
  expect_rc 0 "sem rede com warn: exit 0, mas nunca aprovado silenciosamente"
}

tut_main "$@"
