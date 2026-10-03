#!/usr/bin/env bash
# Tutorial executavel: gate de politica (AUR-562). Veja ../README.md e docs/tutorials/gate.md.
#
#   run.sh fail-on-severidade|inconclusivo-provedor|inconclusivo-cobertura|inconclusivo-sast|
#          inconclusivo-analysis-data|fontes|status-pr|repo-afrouxa|falha-fonte-invalida
#   run.sh all | --check | limpar
#
# Os casos do SAST usam o Semgrep REAL da imagem com uma regra local (sem rede) ou, para
# simular falha, um `semgrep` falso que a demonstracao poe na frente do PATH. O caso
# status-pr usa um GitHub FALSO em 127.0.0.1 (../_lib/pr.sh): prova o texto que o produto
# PUBLICA, nao o comportamento do GitHub nem de um runner.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"
# shellcheck source=../_lib/pr.sh
. "$HERE/../_lib/pr.sh"

CASOS=(fail-on-severidade inconclusivo-provedor inconclusivo-cobertura inconclusivo-sast inconclusivo-analysis-data fontes status-pr repo-afrouxa falha-fonte-invalida)

PATH_BASE=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# origens: imprime as origens dos achados que reprovaram, lidas da auditoria.
origens() {
  printf 'origens na auditoria: '
  python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(", ".join(sorted({b.get("origin","?") for b in d["blocking_findings"]})) or "(nenhuma)")' "$TUT_WORK/$1"
}

# 1. fail_on por severidade: high e error sao o mesmo limiar; medium inclui warning.
caso_fail_on_severidade() {
  tut_repo fail-on-severidade repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-aviso.json
  echo "--- fail_on [high], achado warning"
  TUT_POLICY=politica-high; aurum review --base main --politica /policy
  expect_rc 0 "warning abaixo do limiar high nao reprova"
  echo "--- fail_on [medium], o mesmo achado warning"
  TUT_POLICY=politica-medium; aurum review --base main --politica /policy
  expect_rc 3 "warning no limiar medium reprova (exit 3)"
  echo "--- fail_on [error], achado error"
  TUT_FIXTURE=fixture-erro.json
  TUT_POLICY=politica-error; aurum review --base main --politica /policy
  expect_rc 3 "error reprova com fail_on error"
  echo "--- fail_on [high], achado error: high e error sao o mesmo limiar"
  TUT_POLICY=politica-high; aurum review --base main --politica /policy
  expect_rc 3 "high e error sao o mesmo limiar: o achado error reprova"
}

# 2. Inconclusivo: provedor ausente.
caso_inconclusivo_provedor() {
  tut_repo inconclusivo-provedor repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=none
  echo "--- inconclusive: block, sem provedor"
  TUT_POLICY=politica-bloqueia; aurum review --base main --politica /policy
  expect_rc 1 "block: provedor ausente reprova, motivo quality_skipped"
  echo "--- inconclusive: warn, sem provedor"
  TUT_POLICY=politica-alerta; aurum review --base main --politica /policy
  expect_rc 0 "warn: provedor ausente alerta e sai 0, nunca aprovado"
  echo "--- inconclusive: warn com --exigir-qualidade"
  aurum review --base main --politica /policy --exigir-qualidade
  expect_rc 1 "--exigir-qualidade reprova mesmo com warn (provider_failure)"
}

# 3. Inconclusivo: cobertura parcial (arquivo binario).
caso_inconclusivo_cobertura() {
  tut_repo inconclusivo-cobertura repo-exemplo/base repo-exemplo/binario
  TUT_FIXTURE=fixture-vazia.json
  echo "--- inconclusive: block"
  TUT_POLICY=politica-bloqueia; aurum review --base main --politica /policy
  expect_rc 1 "block: cobertura parcial reprova"
  echo "--- inconclusive: warn"
  TUT_POLICY=politica-alerta; aurum review --base main --politica /policy
  expect_rc 0 "warn: cobertura parcial alerta e sai 0"
}

# 4. Inconclusivo: SAST falhando (semgrep falso na frente do PATH) e SAST normal (Semgrep real).
caso_inconclusivo_sast() {
  tut_repo inconclusivo-sast repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-vazia.json
  echo "--- semgrep falso que falha (exit 2), inconclusive: block"
  TUT_ENVS=(-e "PATH=/fixtures/fakebin/erro:$PATH_BASE")
  TUT_POLICY=politica-sast; aurum review --base main --politica /policy
  expect_rc 1 "block: SAST que falhou nunca vira 'sem achados' (sast_execution_error)"
  echo "--- o mesmo SAST falho, inconclusive: warn"
  TUT_POLICY=politica-sast-alerta; aurum review --base main --politica /policy
  expect_rc 0 "warn: SAST falho alerta e sai 0"
  echo "--- semgrep falso com saida que nao e relatorio, inconclusive: block"
  TUT_ENVS=(-e "PATH=/fixtures/fakebin/invalido:$PATH_BASE")
  TUT_POLICY=politica-sast; aurum review --base main --politica /policy
  expect_rc 1 "block: saida invalida do SAST e inconclusiva (sast_invalid_output)"
  echo "--- Semgrep REAL (regra local): a mesma politica conclui e o achado SAST reprova"
  TUT_ENVS=()
  aurum review --base main --politica /policy
  expect_rc 3 "com o Semgrep real o SAST conclui e o achado reprova (nao e inconclusivo)"
}

# 5. Inconclusivo: analysis_data. Sem rede, o artefato fica indisponivel. Artefato vencido e digest divergente
# exigem um servidor de releases e NAO sao executados aqui.
caso_inconclusivo_analysis_data() {
  tut_repo inconclusivo-analysis-data repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-vazia.json
  TUT_POLICY=politica-analysis-data
  TUT_SED='s/(analysis_data_unavailable\)): .*/\1: <detalhe de rede omitido do registro>/; s/(analysis_data_unavailable): .*/\1: <detalhe de rede omitido do registro>/'
  aurum review --base main --politica /policy
  expect_rc 1 "block: artefato de analise indisponivel reprova (analysis_data_unavailable)"
}

# 6. gate.sources: um achado de cada origem, e o que cada politica conta.
caso_fontes() {
  tut_repo fontes repo-exemplo/base repo-exemplo/achados
  TUT_FIXTURE=fixture-erro.json
  local p
  for p in todas skills analysis sast; do
    echo "--- gate.sources: $p"
    TUT_POLICY="politica-fonte-$p"
    aurum review --base main --politica /policy --auditoria "/work/auditoria-$p.json"
    expect_rc 3 "sources $p: o gate reprova"
    origens "auditoria-$p.json"
  done
}

# 7. O texto que o produto publica no commit status aurumcode/policy-gate (GitHub falso local).
caso_status_pr() {
  local alvo
  for alvo in aprovado reprovado inconclusivo; do
    case "$alvo" in
      aprovado)     tut_repo "status-pr-$alvo" repo-exemplo/base repo-exemplo/segredo; TUT_FIXTURE=fixture-aviso.json; TUT_POLICY=politica-high ;;
      reprovado)    tut_repo "status-pr-$alvo" repo-exemplo/base repo-exemplo/segredo; TUT_FIXTURE=fixture-erro.json; TUT_POLICY=politica-high ;;
      inconclusivo) tut_repo "status-pr-$alvo" repo-exemplo/base repo-exemplo/binario; TUT_FIXTURE=fixture-vazia.json; TUT_POLICY=politica-bloqueia ;;
    esac
    echo "--- $alvo"
    tut_pr_servidor "status-pr-$alvo"
    aurum_pr review --pr 7 --repo OWNER/REPO --publicar --check --modo-publicacao review --politica /policy
    tut_pr_log
    tut_pr_parar
  done
  expect_rc 1 "o ultimo caso (inconclusivo em block) sai 1 e publica status failure"
}

# 8. O repositorio tenta afrouxar a politica: inconclusive warn e sources [skills] sao ignorados.
caso_repo_afrouxa() {
  tut_repo repo-afrouxa repo-exemplo/base-afrouxa repo-exemplo/binario
  TUT_FIXTURE=fixture-vazia.json
  echo "--- so o repositorio: inconclusive warn"
  aurum review --base main
  expect_rc 0 "o proprio repositorio aceita cobertura parcial como alerta"
  echo "--- com a politica (inconclusive block): o gate do repositorio e ignorado"
  TUT_POLICY=politica-bloqueia
  aurum review --base main --politica /policy
  expect_rc 1 "o repositorio nao consegue afrouxar o inconclusive da politica"
}

# Falha: valor desconhecido em gate.sources e erro de carga.
caso_falha_fonte_invalida() {
  tut_repo falha-fonte-invalida repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=fixture-aviso.json
  TUT_POLICY=politica-invalida
  aurum review --base main --politica /policy
  expect_rc 1 "gate.sources com valor desconhecido falha antes de qualquer chamada ao modelo"
}

tut_main "$@"
