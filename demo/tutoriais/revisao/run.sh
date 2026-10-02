#!/usr/bin/env bash
# Tutorial executavel: revisao (AUR-561). Veja ../README.md e docs/tutorials/revisao.md.
#
#   run.sh primeira-revisao|sem-provedor|com-provedor|fix|pr-workflow|falha-nao-revisado
#   run.sh all | --check | limpar
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(primeira-revisao sem-provedor com-provedor fix pr-workflow falha-nao-revisado)

# 1. Primeira revisao local: uma troca de mensagem, modelo (fixture) sem achados.
caso_primeira_revisao() {
  tut_repo primeira-revisao repo-exemplo/base repo-exemplo/limpa
  TUT_FIXTURE=fixture-vazia.json
  aurum review --base main
  expect_rc 0 "primeira revisao concluida sem achados"
}

# 2. Sem provedor: so a analise deterministica, declarada como tal.
caso_sem_provedor() {
  tut_repo sem-provedor repo-exemplo/base repo-exemplo/segredo
  TUT_FIXTURE=none
  aurum review --base main --seguranca --fail-on error
  expect_rc 3 "sem provedor, a analise deterministica achou o segredo e reprovou"
}

# 3. Com provedor (fixture): o achado cita a regra do catalogo.
caso_com_provedor() {
  tut_repo com-provedor repo-exemplo/base repo-exemplo/segredo
  aurum review --base main --fail-on error
  expect_rc 3 "achado do modelo citando security/hardcoded-secret reprovou"
}

# 4. fix: aplica uma sugestao como patch.
caso_fix() {
  tut_repo fix repo-exemplo/base repo-exemplo/segredo
  echo "--- antes"; sed -n '6p' "$TUT_WORK/app.go"
  cp "$HERE/fix/sugestoes.json" "$TUT_WORK/sugestoes.json"
  printf '$ aurumcode fix --file sugestoes.json > fix.patch\n'
  set +e; aurum_raw -- fix --file sugestoes.json > "$TUT_WORK/fix.patch"; LAST_RC=$?; set -e
  echo "exit_code=$LAST_RC"
  expect_rc 0 "fix imprimiu o patch (nada foi escrito no repositorio)"
  cat "$TUT_WORK/fix.patch"
  git -C "$TUT_WORK" apply --unidiff-zero --check fix.patch && echo "git apply --unidiff-zero --check: o patch aplica"
  git -C "$TUT_WORK" apply --unidiff-zero fix.patch
  echo "--- depois"; sed -n '6p' "$TUT_WORK/app.go"
  # a mesma sugestao agora esta velha (o current_code ja nao existe): exit 1, nenhum patch
  aurum fix --file sugestoes.json
  expect_rc 1 "sugestao velha recusada, sem patch"
}

# 5. Revisao de PR pelo workflow reutilizavel. O que roda aqui e a conferencia do
# workflow do chamador contra as entradas reais do reutilizavel; a revisao de
# PR em si so acontece no GitHub (veja o tutorial).
caso_pr_workflow() {
  local wf="$HERE/workflow-chamador.yml" reuse="$REPO_ROOT/.github/workflows/review.yml" k
  echo "workflow do chamador: workflow-chamador.yml"
  grep -q '^  pull_request:' "$wf" && echo "gatilho: pull_request"
  grep -qE 'uses: OWNER/AurumCode/\.github/workflows/review\.yml@[0-9a-f]{40}$' "$wf" && echo "uses: workflow reutilizavel fixado em SHA"
  # cada entrada em with: existe em workflow_call.inputs; cada secret existe em workflow_call.secrets
  for k in $(awk '/^    with:/{w=1;next} w&&/^    [a-z]/{w=0} w&&/^      [a-z_]+:/{sub(":","",$1);print $1}' "$wf"); do
    grep -qE "^      $k:\$" "$reuse" || { echo "ERRO: input $k nao existe em review.yml"; return 1; }
    echo "input $k: existe em review.yml"
  done
  for k in $(awk '/^    secrets:/{w=1;next} w&&/^[^ ]/{w=0} w&&/^      [A-Z_]+:/{sub(":","",$1);print $1}' "$wf"); do
    grep -qE "^      $k:\$" "$reuse" || { echo "ERRO: secret $k nao existe em review.yml"; return 1; }
    echo "secret $k: existe em review.yml"
  done
  for k in "contents: read" "pull-requests: write" "statuses: write" "checks: read"; do
    grep -qE "^      $k\$" "$reuse" && grep -qE "^  $k\$" "$wf" || { echo "ERRO: permissao '$k' nao coberta pelo chamador"; return 1; }
    echo "permissao $k: o chamador concede o que o reutilizavel declara"
  done
}

# Falha: arquivo nao revisado (binario) nunca conta como aprovado.
caso_falha_nao_revisado() {
  tut_repo falha-nao-revisado repo-exemplo/base repo-exemplo/binario
  TUT_FIXTURE=fixture-vazia.json
  echo "--- sem gate: veredito 'Comment', nunca 'Approve'"
  aurum review --base main
  expect_rc 0 "sem gate, exit 0 mas o veredito nao e Approve"
  echo "--- com gate.inconclusive: block"
  tut_repo falha-nao-revisado-gate repo-exemplo/base repo-exemplo/gate-estrito repo-exemplo/binario
  aurum review --base main
  expect_rc 1 "com gate.inconclusive: block, cobertura parcial reprova"
}

tut_main "$@"
