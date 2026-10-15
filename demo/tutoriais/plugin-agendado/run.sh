#!/usr/bin/env bash
# Tutorial executavel: plugin externo na validacao agendada (AUR-622) e o
# destino issues de repositorio privado sem Code Security (AUR-623).
# Veja docs/tutorials/plugin-agendado.md.
#
#   run.sh ingest|fora-do-envio|nao-verificado|workflow|issues|degradado|cobertura-perdida|issues-publico
#   run.sh all | --check | limpar
#
# O plugin de exemplo (plugin-exemplo/plugin.sh) roda no host e faz o papel do
# harness sem modelo e sem rede: escreve um findings.json e um SARIF do
# harness sinteticos. O aurumcode (list e ingest) roda SEMPRE na imagem do
# produto, rede none; `plugin issues` fala so com um GitHub falso em
# 127.0.0.1 (github-falso-issues.py), nunca com o GitHub.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"
# shellcheck source=../_lib/pr.sh
. "$TUT_LIB/pr.sh"

CASOS=(ingest fora-do-envio nao-verificado workflow issues degradado cobertura-perdida issues-publico)

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
    print("sarif: %s %s security-severity %s fingerprint %s..." % (r["ruleId"], r["level"], r["properties"]["security-severity"], fp[:10]))
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

# 3. Duvida nunca some: o harness descartou um candidato como INCONCLUSIVE e o
# SARIF dele diz coverageDegraded so por s6_undetermined (verificacoes
# indeterminadas). O candidato vira alerta de aviso "nao verificado" e o envio
# segue com os 4 verdadeiros positivos.
caso_nao_verificado() {
  tut_repo nao-verificado repo-exemplo
  roda_plugin nao-verificado
  ingere
  expect_rc 0 "cobertura degradada so por s6_undetermined: envia os 4 achados e o nao verificado como aviso"
  sem_texto_de_achado
  mostra_sarif
}

# 4. O job agendado do review.yml: conferencia estatica, nao execucao.
caso_workflow() {
  local wf="$REPO_ROOT/.github/workflows/review.yml" bloco
  bloco="$(awk '/^  plugins-list:$/{p=1} p&&/^  [a-z-]+:$/&&!/^  plugins(-list)?:$/{p=0} p' "$wf")"
  [ -n "$bloco" ] || { echo "ERRO: jobs plugins-list/plugins ausentes"; return 1; }
  grep -qF "if: github.event_name == 'schedule' || (github.event_name == 'workflow_dispatch' && github.ref_name == github.event.repository.default_branch)" <<<"$bloco" \
    && echo "gatilho: schedule, ou workflow_dispatch na branch padrao"
  if grep -E '^    if:' <<<"$bloco" | grep -q pull_request; then echo "ERRO: o if de um job cita pull_request"; return 1; fi
  echo "pull_request: os jobs sao pulados pelo if (nenhum passo roda)"
  grep -qF 'schedule|workflow_dispatch) ;;' <<<"$bloco" && echo "defesa em profundidade: se rodasse em outro evento, o primeiro passo sairia com exit 1"
  grep -qF 'timeout-minutes: ${{ matrix.plugin.job_timeout_minutes }}' <<<"$bloco" && echo "timeout do job: o do plugin mais a folga fixa; rc 124 vira uma linha no log"
  if grep -qE 'upload-artifact|GITHUB_STEP_SUMMARY' <<<"$bloco"; then echo "ERRO: canal publico no job"; return 1; fi
  echo "sem upload-artifact e sem resumo do job"
  grep -qF '>"${RUNNER_TEMP}/plugin-${PLUGIN_NAME}.log" 2>&1' <<<"$bloco" && echo "saida do plugin: arquivo em RUNNER_TEMP, nunca impresso"
  grep -qE 'uses: github/codeql-action/upload-sarif@[0-9a-f]{40}' <<<"$bloco" && echo "envio: upload-sarif fixado por SHA"
  grep -qF 'category: aurumcode/plugin/${{ matrix.plugin.name }}' <<<"$bloco" && echo "categoria: aurumcode/plugin/<nome>"
  grep -qF 'alertas enviados para a aba Security (visível só para quem tem escrita).' <<<"$bloco" && echo "log: so a contagem de alertas"
  grep -qF "if: success() && steps.destination.outputs.destino == 'security'" <<<"$bloco" && echo "upload-sarif: so no destino security"
  grep -qF "if: success() && steps.destination.outputs.destino == 'issues'" <<<"$bloco" \
    && grep -qF 'aurumcode-review plugin issues --name "${PLUGIN_NAME}" --repo "${REPOSITORY}"' <<<"$bloco" \
    && echo "destino issues: plugin issues depois do ingest conclusivo, sem upload-sarif"
  grep -qF 'plus `issues: write` in a private repository' "$wf" && echo "chamador: issues: write documentado"
  echo "RESULTADO: o job plugins publica na aba Security, ou em issues do proprio repositorio privado sem Code Security"
  echo "NAO EXECUTADO AQUI: o envio ao code scanning e as issues reais (nao ha runner nem GitHub neste ambiente)"
}

# ---- destino issues (AUR-623): GitHub falso local, estado em memoria
GH_ESTADO=
gh_falso() {
  local caso="$1" visibilidade="$2" i
  TUT_PR_PORT=$((20000 + RANDOM % 20000))
  TUT_PR_LOG="$STATE/$caso.gh.log"; : > "$TUT_PR_LOG"
  GH_ESTADO="$STATE/$caso.gh.json"
  python3 "$HERE/github-falso-issues.py" "$TUT_PR_PORT" "$visibilidade" "$TUT_PR_LOG" "$GH_ESTADO" &
  TUT_PR_PID=$!
  trap 'tut_pr_parar' EXIT
  for i in $(seq 1 50); do
    (: > "/dev/tcp/127.0.0.1/$TUT_PR_PORT") 2>/dev/null && return 0
    sleep 0.1
  done
  echo "ERRO: GitHub falso nao subiu" >&2; return 1
}

sincroniza() {
  aurum_pr plugin issues --name vvah --sarif /work/.saida/plugin-vvah.sarif --repo "$1"
}

# o que o GitHub falso guardou: uma linha por issue, sem o texto do achado
mostra_issues() {
  python3 - "$GH_ESTADO" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
for i in d["issues"]:
    marcador = i["body"].split("\n", 1)[0]
    print("issue #%d %s | %s" % (i["number"], i["state"], i["title"]))
    print("  rotulos %s | marcador %s" % (",".join(i["labels"]), marcador))
    print("  comentarios %d | segredo redigido: %s" % (len(d["comentarios"].get(str(i["number"]), [])), "sim" if "valor-sintetico" not in i["body"] else "NAO"))
PY
}

escritas() { grep -cvE '^GET ' "$TUT_PR_LOG" || true; }

# 5. Repositorio privado sem Code Security: tres varreduras seguidas.
caso_issues() {
  tut_repo issues repo-exemplo
  gh_falso issues private
  echo "-- varredura 1: dois verdadeiros positivos"
  roda_plugin tres-achados; ingere
  sincroniza dono/privado
  expect_rc 0 "abre uma issue por achado, com marcador oculto e rotulo"
  sem_texto_de_achado
  mostra_issues
  echo "-- varredura 2: so um dos dois continua"
  roda_plugin misto; ingere
  sincroniza dono/privado
  expect_rc 0 "mantem a que continua sem duplicar e fecha a que sumiu, com comentario"
  mostra_issues
  echo "-- varredura 3: o achado fechado volta, com outros novos"
  roda_plugin nao-verificado; ingere
  sincroniza dono/privado
  expect_rc 0 "reabre a issue cujo achado voltou e abre as novas"
  sem_texto_de_achado
  mostra_issues
  echo "-- uma pessoa fecha a issue #4 (decisao de dispensar); varredura 4 com o achado ainda presente"
  pessoa_fecha 4
  sincroniza dono/privado
  expect_rc 0 "issue fechada por pessoa fica fechada e conta como dispensada"
  mostra_issues
  tut_pr_parar
}

# pessoa_fecha N: fecha a issue no GitHub falso como uma pessoa (sem o
# comentario-marcador do plugin).
pessoa_fecha() {
  python3 - "$TUT_PR_PORT" "$1" <<'PY'
import json, sys, urllib.request
req = urllib.request.Request("http://127.0.0.1:%s/repos/dono/privado/issues/%s" % (sys.argv[1], sys.argv[2]),
                             data=json.dumps({"state": "closed", "state_reason": "completed"}).encode(), method="PATCH")
urllib.request.urlopen(req).read()
PY
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
# harness diz coverageDegraded por zero_analysis (unidades de analise
# falharam): zero falso.
caso_cobertura_perdida() {
  tut_repo cobertura-perdida repo-exemplo
  roda_plugin cobertura-perdida
  ingere
  expect_rc 1 "cobertura perdida com zero achados: inconclusivo, nunca zero falso"
  sem_sarif
}

# Falha 3: repositorio publico nunca recebe issue; varredura inconclusiva nao
# toca em nenhuma.
caso_issues_publico() {
  tut_repo issues-publico repo-exemplo
  gh_falso issues-publico public
  roda_plugin tres-achados; ingere
  sincroniza dono/publico
  expect_rc 1 "repositorio publico: recusa antes de qualquer escrita"
  echo "escritas no GitHub falso: $(escritas)"
  tut_pr_parar
  gh_falso issues-inconclusivo private
  roda_plugin degradado; ingere
  sincroniza dono/privado
  expect_rc 1 "varredura inconclusiva: nenhuma chamada ao GitHub"
  echo "requisicoes ao GitHub falso: $(wc -l < "$TUT_PR_LOG" | tr -d ' ')"
  tut_pr_parar
}

tut_main "$@"
