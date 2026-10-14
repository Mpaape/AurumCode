#!/usr/bin/env bash
# Tutorial executavel: plugin externo na validacao agendada (AUR-622).
# Veja docs/tutorials/plugin-agendado.md.
#
#   run.sh ingest|fora-do-envio|workflow|degradado|cobertura-perdida
#   run.sh all | --check | limpar
#
# O plugin de exemplo (plugin-exemplo/plugin.sh) roda no host e faz o papel do
# harness sem modelo e sem rede: escreve um findings.json e um SARIF do
# harness sinteticos. O aurumcode (list e ingest) roda SEMPRE na imagem do
# produto, rede none.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(ingest fora-do-envio workflow degradado cobertura-perdida)

# roda_plugin CENARIO: o plugin escreve security-scan/ na arvore do caso.
roda_plugin() {
  echo "\$ plugin-exemplo/plugin.sh $1 <arvore>"
  set +e
  "$HERE/plugin-exemplo/plugin.sh" "$1" "$TUT_WORK" >"$STATE/plugin-$1.log" 2>&1
  PLUGIN_RC=$?
  set -e
  echo "plugin terminou com exit $PLUGIN_RC (saida guardada fora do log)"
}

ingere() {
  mkdir -p "$TUT_WORK/.saida"
  rm -f "$TUT_WORK/.saida/plugin-vvah.sarif"
  aurum plugin ingest --name vvah --format vvah-findings --input /work/security-scan/findings.json \
    --repo /work --plugin-exit "$PLUGIN_RC" --sarif /work/.saida/plugin-vvah.sarif
}

# o stdout do ingest nao cita titulo, arquivo nem texto de achado
sem_texto_de_achado() {
  if printf '%s\n' "$LAST_OUT" | grep -qE 'Titulo sintetico|app\.py|descricao sintetica|valor-sintetico'; then
    echo "ERRO: a saida do ingest cita texto de achado"; return 1
  fi
  echo "saida do ingest: so contagens, nenhum titulo, arquivo ou descricao"
}

# resume o SARIF gravado (lido no host com python3)
mostra_sarif() {
  python3 - "$TUT_WORK/.saida/plugin-vvah.sarif" <<'PY'
import json, sys
raw = open(sys.argv[1]).read()
run = json.loads(raw)["runs"][0]
print("sarif: version 2.1.0, ferramenta %s" % run["tool"]["driver"]["name"])
print("sarif: automationDetails.id %s" % run["automationDetails"]["id"])
print("sarif: %d resultado(s)" % len(run["results"]))
for r in run["results"]:
    fp = r["partialFingerprints"]["aurumcodePlugin/v1"]
    print("sarif: %s security-severity %s fingerprint %s..." % (r["ruleId"], r["properties"]["security-severity"], fp[:10]))
print("sarif: segredo do texto redigido: %s" % ("sim" if "valor-sintetico" not in raw and "[REDACTED]" in raw else "NAO"))
PY
}

sem_sarif() {
  if [ -e "$TUT_WORK/.saida/plugin-vvah.sarif" ]; then echo "ERRO: SARIF gravado"; return 1; fi
  echo "nenhum SARIF gravado: os alertas abertos ficam como estao"
}

# 1. O plugin acha 3 casos (2 verdadeiros positivos, 1 falso positivo).
caso_ingest() {
  tut_repo ingest repo-exemplo
  aurum plugin list --json --repo /work
  expect_rc 0 "plugin list le scheduled.plugins da configuracao confiavel"
  roda_plugin tres-achados
  ingere
  expect_rc 0 "ingest conclusivo: SARIF gravado com os 2 verdadeiros positivos"
  sem_texto_de_achado
  mostra_sarif
}

# 2. Falso positivo, canal posture e achado sem veredito ficam fora do envio.
caso_fora_do_envio() {
  tut_repo fora-do-envio repo-exemplo
  roda_plugin misto
  ingere
  expect_rc 0 "so o verdadeiro positivo do canal principal vai para a aba Security; os outros so contam"
  sem_texto_de_achado
  mostra_sarif
}

# 3. O job agendado do review.yml: conferencia estatica, nao execucao.
caso_workflow() {
  local wf="$REPO_ROOT/.github/workflows/review.yml" bloco
  bloco="$(awk '/^  plugins-list:$/{p=1} p&&/^  [a-z-]+:$/&&!/^  plugins(-list)?:$/{p=0} p' "$wf")"
  [ -n "$bloco" ] || { echo "ERRO: jobs plugins-list/plugins ausentes"; return 1; }
  grep -qF "if: github.event_name == 'schedule' || (github.event_name == 'workflow_dispatch' && github.ref_name == github.event.repository.default_branch)" <<<"$bloco" \
    && echo "gatilho: schedule, ou workflow_dispatch na branch padrao"
  grep -qF 'schedule|workflow_dispatch) ;;' <<<"$bloco" && echo "outro evento (pull_request incluso): o primeiro passo sai com exit 1"
  if grep -qE 'upload-artifact|GITHUB_STEP_SUMMARY' <<<"$bloco"; then echo "ERRO: canal publico no job"; return 1; fi
  echo "sem upload-artifact e sem resumo do job"
  grep -qF '>"${RUNNER_TEMP}/plugin-${PLUGIN_NAME}.log" 2>&1' <<<"$bloco" && echo "saida do plugin: arquivo em RUNNER_TEMP, nunca impresso"
  grep -qE 'uses: github/codeql-action/upload-sarif@[0-9a-f]{40}' <<<"$bloco" && echo "envio: upload-sarif fixado por SHA"
  grep -qF 'category: aurumcode/plugin/${{ matrix.plugin.name }}' <<<"$bloco" && echo "categoria: aurumcode/plugin/<nome>"
  grep -qF 'alertas enviados para a aba Security (visível só para quem tem escrita).' <<<"$bloco" && echo "log: so a contagem de alertas"
  echo "RESULTADO: o job plugins so publica na aba Security"
  echo "NAO EXECUTADO AQUI: o envio ao code scanning (nao ha runner nem GitHub neste ambiente)"
}

# Falha 1: o harness conclui (exit 0) mas se declara degradado.
caso_degradado() {
  tut_repo degradado repo-exemplo
  roda_plugin degradado
  ingere
  expect_rc 1 "plugin degradado: inconclusivo, exit 1"
  sem_sarif
}

# Falha 2: exit 0, findings.json sem achado e degraded false, mas o SARIF do
# harness diz coverageDegraded (unidades de analise falharam): zero falso.
caso_cobertura_perdida() {
  tut_repo cobertura-perdida repo-exemplo
  roda_plugin cobertura-perdida
  ingere
  expect_rc 1 "cobertura perdida com zero achados: inconclusivo, nunca zero falso"
  sem_sarif
}

tut_main "$@"
