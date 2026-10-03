#!/usr/bin/env bash
# Tutorial executavel: politica central (AUR-561). Veja ../README.md e docs/tutorials/politica-central.md.
#
#   run.sh politica-local|policy-repository|precedencia-por-secao|analysis-data|repo-afrouxa|falha-politica-invalida
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(politica-local policy-repository precedencia-por-secao analysis-data repo-afrouxa falha-politica-invalida)

# 1. --politica local: o mesmo repositorio, sem e com a politica.
caso_politica_local() {
  tut_repo politica-local repo-exemplo/base-sem-config repo-exemplo/segredo
  TUT_FIXTURE=fixture-politica.json
  echo "--- sem politica: nenhum gate"
  aurum review --base main
  expect_rc 0 "sem politica nao ha gate"
  echo "--- com --politica: a politica impoe o gate e o idioma"
  TUT_POLICY=politica
  aurum review --base main --politica /policy
  expect_rc 3 "a politica reprova o achado que cita a skill dela"
}

# 2. policy_repository no workflow: o chamador e coerente com o reutilizavel.
caso_policy_repository() {
  local wf="$HERE/workflow-organizacao.yml" reuse="$REPO_ROOT/.github/workflows/review.yml" k
  echo "workflow da organizacao: workflow-organizacao.yml"
  for k in policy_repository policy_ref security; do
    grep -qE "^      $k:\$" "$reuse" || { echo "ERRO: input $k nao existe em review.yml"; return 1; }
    echo "input $k: existe em review.yml"
  done
  grep -qE '^      policy_ref: [0-9a-f]{40}$' "$wf" && echo "policy_ref: SHA fixa, nao um branch"
  if grep -qE '^      policy_path:' "$reuse"; then echo "ERRO: review.yml tem policy_path"; return 1; fi
  echo "review.yml nao tem input policy_path: so policy_repository pode declarar a politica"
  grep -q 'repository: ${{ inputs.policy_repository }}' "$reuse" && echo "review.yml: faz o checkout de policy_repository"
  grep -q 'path: .aurumcode-policy' "$reuse" && echo "review.yml: o checkout vai para .aurumcode-policy"
  grep -q 'persist-credentials: false' "$reuse" && echo "review.yml: checkout sem persistir credenciais"
  grep -qF 'args+=(--politica /github/policy)' "$reuse" && echo "review.yml: passa --politica /github/policy ao aurumcode"
  echo "equivalente local: o caso politica-local roda o mesmo --politica contra um diretorio de politica"
}

# 3. Precedencia por secao: a politica governa so o que declara.
caso_precedencia_por_secao() {
  tut_repo precedencia-por-secao repo-exemplo/base-precedencia repo-exemplo/segredo
  TUT_POLICY=politica-precedencia
  TUT_FIXTURE=fixture-llm.json
  aurum review --base main --politica /policy
  expect_rc 0 "revisao feita sob politica"
  if grep -q 'supply_chain' <<<"$LAST_OUT"; then
    echo "ERRO: avisou sobre supply_chain, que a politica nao declara"; return 1
  fi
  echo "supply_chain: a politica nao declara, entao nao ha aviso e vale o do repositorio"
}

# 3b. analysis_data tem a mesma regra. Sem rede (a demonstracao roda com --network none)
# o artefato fica indisponivel: o texto de rede e omitido do registro porque varia por ambiente.
caso_analysis_data() {
  tut_repo analysis-data repo-exemplo/base-analysis-data repo-exemplo/segredo
  TUT_POLICY=politica-analysis-data
  TUT_FIXTURE=fixture-llm.json
  TUT_SED='s/(analysis_data_unavailable\)): .*/\1: <detalhe de rede omitido do registro>/'
  aurum review --base main --politica /policy
  expect_rc 0 "analysis_data da politica indisponivel e inconclusivo (warn), nunca aprovado"
}

# 4. O repositorio tenta afrouxar fail_on e nao consegue.
caso_repo_afrouxa() {
  tut_repo repo-afrouxa repo-exemplo/base-afrouxa repo-exemplo/segredo
  TUT_FIXTURE=fixture-politica.json
  echo "--- so o repositorio: fail_on [high] deixa o achado warning passar"
  aurum review --base main
  expect_rc 0 "o gate do proprio repositorio aceita o warning"
  echo "--- com a politica: fail_on [warning] vale, o do repositorio e ignorado"
  TUT_POLICY=politica
  aurum review --base main --politica /policy
  expect_rc 3 "o repositorio nao consegue afrouxar o gate da politica"
}

# Falha: politica invalida (chave desconhecida) e erro de carga.
caso_falha_politica_invalida() {
  tut_repo falha-politica-invalida repo-exemplo/base-sem-config repo-exemplo/segredo
  TUT_POLICY=politica-invalida
  TUT_FIXTURE=fixture-politica.json
  aurum review --base main --politica /policy
  expect_rc 1 "politica invalida falha antes de qualquer chamada ao modelo"
}

tut_main "$@"
