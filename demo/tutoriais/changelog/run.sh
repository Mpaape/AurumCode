#!/usr/bin/env bash
# Tutorial executavel: changelog obrigatorio (AUR-509). Veja ../README.md e
# docs/tutorials/changelog.md.
#
#   run.sh entrada-valida|consolidar-release|sugestao-separada|pr-desliga-o-modo|log-de-agente|falha-entrada-ausente|sugestao-da-entrada
#   run.sh all | --check | limpar
#
# O veredito de `aurumcode changelog` e deterministico: compara o CHANGELOG.md
# da base (main) com o da PR (feature). Quando reprova, o check tambem
# imprime a entrada sugerida (pelo modelo configurado ou, sem ele, pelos
# assuntos dos commits). Roda na imagem do produto, sem rede, com o modelo
# falso.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(entrada-valida consolidar-release sugestao-separada pr-desliga-o-modo log-de-agente falha-entrada-ausente sugestao-da-entrada)

# 1. A PR muda o codigo e acrescenta uma linha em Unreleased: aprovado.
caso_entrada_valida() {
  tut_repo entrada-valida repo-exemplo/base repo-exemplo/mudanca repo-exemplo/entrada
  aurum changelog --base main
  expect_rc 0 "a PR acrescentou uma entrada util em Unreleased e o check aprovou"
}

# 2. PR de release: as linhas de Unreleased vao para a secao da versao com um
#    resumo. Linha so movida nao conta; o resumo conta.
caso_consolidar_release() {
  tut_repo consolidar-release repo-exemplo/base repo-exemplo/release
  aurum changelog --base main
  expect_rc 0 "a consolidacao da release com resumo novo foi aprovada"
}

# 3. A sugestao do review continua separada: ela propoe uma versao e um texto,
#    mas nao aprova nem reprova o merge.
caso_sugestao_separada() {
  tut_repo sugestao-separada repo-exemplo/base repo-exemplo/mudanca
  aurum review --base main --changelog
  expect_rc 0 "o review publicou a sugestao de changelog sem decidir o merge"
  aurum changelog --base main
  expect_rc 1 "a sugestao nao substitui a entrada: o check obrigatorio reprovou"
}

# 4. A PR tenta desligar o modo no proprio config.yml: o modo vem da base.
caso_pr_desliga_o_modo() {
  tut_repo pr-desliga-o-modo repo-exemplo/base repo-exemplo/mudanca repo-exemplo/desliga
  aurum changelog --base main
  expect_rc 1 "o modo foi lido da base; a PR que o desliga continua sujeita ao check"
}

# 5. Log de agente colado no changelog nao e entrada para quem usa o produto.
caso_log_de_agente() {
  tut_repo log-de-agente repo-exemplo/base repo-exemplo/mudanca repo-exemplo/log-de-agente
  aurum changelog --base main
  expect_rc 1 "a linha com log de agente foi recusada"
}

# 6. Falha: a PR muda o codigo e nao toca no CHANGELOG.md.
caso_falha_entrada_ausente() {
  tut_repo falha-entrada-ausente repo-exemplo/base repo-exemplo/mudanca
  aurum changelog --base main
  expect_rc 1 "sem entrada no CHANGELOG.md o check reprova"
}

# 7. A PR sem entrada continua reprovada, mas o check traz a entrada sugerida
#    pelo modelo, pronta para colar em Unreleased (o humano cola e ajusta).
caso_sugestao_da_entrada() {
  tut_repo sugestao-da-entrada repo-exemplo/base repo-exemplo/mudanca
  TUT_FIXTURE=fixture-sugestao.json
  aurum changelog --base main
  expect_rc 1 "o check reprovou e trouxe a entrada sugerida pelo modelo, pronta para colar"
}

tut_main "$@"
