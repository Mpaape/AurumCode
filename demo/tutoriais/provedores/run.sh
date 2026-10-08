#!/usr/bin/env bash
# Tutorial executavel: provedores de LLM por configuracao (AUR-596) e
# provedores reserva quando o principal cai (AUR-605). Veja
# ../README.md e docs/tutorials/provedores.md.
#
#   run.sh sem-perfil|azure-openai|anthropic|catalogo-do-operador|reserva-assume|duas-reservas|falha-fora-do-schema|falha-perfil-desconhecido|falha-todas-caem|falha-reserva-sem-chave
#   run.sh all | --check | limpar
#
# O provedor e um servidor FALSO (provedor-falso.py) que roda DENTRO do
# container do produto, em 127.0.0.1:8080 (dentro.sh), registra o que cada
# dialeto recebeu (sem a chave) e responde uma revisao fixa. Rede do container:
# none; nenhuma chamada sai da maquina e nenhuma credencial real existe.
set -Eeuo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=../_lib/tutorial.sh
. "$HERE/../_lib/tutorial.sh"

CASOS=(sem-perfil azure-openai anthropic catalogo-do-operador reserva-assume duas-reservas falha-fora-do-schema falha-perfil-desconhecido falha-todas-caem falha-reserva-sem-chave)

# aurum_prov ROTA VAR=valor... -- ARGS: como `aurum`, sem fixture, com o
# provedor falso em LLM_BASE_URL=http://127.0.0.1:8080/ROTA. Imprime as
# variaveis do operador (a chave e a URL ficam fora da linha de comando).
aurum_prov() {
  # A chave falsa entra pelo nome da variavel (como em sast/run.sh): nenhuma
  # atribuicao literal de chave no texto do tutorial.
  local rota="$1" envs=() shown=() chave=LLM_API_KEY; shift
  while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do envs+=(-e "$1"); shown+=("$1"); shift; done
  shift
  if [ "${#shown[@]}" -gt 0 ]; then printf '$ %s aurumcode %s\n' "${shown[*]}" "$*"; else printf '$ aurumcode %s\n' "$*"; fi
  set +e
  LAST_OUT="$(docker run --rm --network none --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e "LLM_BASE_URL=http://127.0.0.1:8080/$rota" -e "$chave=chave-falsa-local-0123456789abcdef" \
    "${envs[@]}" -v "$HERE:/fixtures:ro" -v "$TUT_WORK:/work" -w /work \
    --entrypoint /fixtures/dentro.sh "$TUT_IMAGE" "$@" 2>&1)"
  LAST_RC=$?
  set -e
  [ -z "$LAST_OUT" ] || printf '%s\n' "$LAST_OUT"
  echo "exit_code=$LAST_RC"
}

# 1. Sem LLM_PROVIDER: o dialeto de sempre (bearer, response_format json_object).
caso_sem_perfil() {
  tut_repo sem-perfil repo-exemplo/base repo-exemplo/mudanca
  aurum_prov legado -- review --base main
  expect_rc 0 "sem LLM_PROVIDER a requisicao e a de sempre e a revisao conclui"
}

# 2. Azure OpenAI: chave no header api-key, nunca Authorization; api-version na
#    query. (A revisao nao envia limite de resposta; quando um chamador envia,
#    o perfil usa max_completion_tokens: provado nos testes de contrato.)
caso_azure_openai() {
  tut_repo azure-openai repo-exemplo/base repo-exemplo/mudanca
  aurum_prov azure LLM_PROVIDER=azure-openai -- review --base main
  expect_rc 0 "azure-openai: api-key e api-version, sem Authorization, e a revisao conclui"
}

# 3. Anthropic: a camada OpenAI-compativel ignora response_format; o produto
#    nao o envia e confere a resposta contra o schema.
caso_anthropic() {
  tut_repo anthropic repo-exemplo/base repo-exemplo/mudanca
  aurum_prov anthropic LLM_PROVIDER=anthropic -- review --base main
  expect_rc 0 "anthropic: sem response_format, resposta dentro do schema, revisao conclui"
}

# 4. Catalogo do operador: LLM_PROVIDERS_FILE acrescenta um perfil proprio.
caso_catalogo_do_operador() {
  tut_repo catalogo-do-operador repo-exemplo/base repo-exemplo/mudanca
  aurum_prov gateway LLM_PROVIDER=meu-gateway LLM_PROVIDERS_FILE=/fixtures/meus-perfis.yml GATEWAY_KEY=chave-falsa-gw-0123456789 -- review --base main
  expect_rc 0 "perfil do operador: chave so no header x-gw-key, sem response_format, revisao conclui"
}

# 5. O principal cai (503) e a reserva 1 responde a mesma revisao, com a
#    propria URL e a propria chave; a troca aparece no stderr.
caso_reserva_assume() {
  tut_repo reserva-assume repo-exemplo/base repo-exemplo/mudanca
  aurum_prov fora-do-ar LLM_FALLBACK_1_BASE_URL=http://127.0.0.1:8080/reserva LLM_FALLBACK_1_API_KEY=reserva-falsa -- review --base main
  expect_rc 0 "principal fora do ar: a reserva 1 responde e a revisao conclui"
}

# 6. Duas reservas, de perfis diferentes: o principal e a reserva 1 caem, a
#    reserva 2 (perfil anthropic) responde. As reservas sao tentadas em ordem.
caso_duas_reservas() {
  tut_repo duas-reservas repo-exemplo/base repo-exemplo/mudanca
  aurum_prov fora-do-ar LLM_FALLBACK_1_BASE_URL=http://127.0.0.1:8080/fora-do-ar LLM_FALLBACK_1_API_KEY=reserva-falsa LLM_FALLBACK_2_PROVIDER=anthropic LLM_FALLBACK_2_BASE_URL=http://127.0.0.1:8080/anthropic LLM_FALLBACK_2_API_KEY=reserva-falsa -- review --base main
  expect_rc 0 "principal e reserva 1 fora do ar: a reserva 2 responde, na ordem"
}

# Falha 1: provedor sem saida estruturada respondendo fora do schema.
caso_falha_fora_do_schema() {
  tut_repo falha-fora-do-schema repo-exemplo/base repo-exemplo/mudanca
  aurum_prov fora-do-schema LLM_PROVIDER=anthropic -- review --base main
  expect_rc 1 "resposta fora do schema: revisao inconclusiva, nunca aprovada"
  echo "--- erro do provedor ecoando a chave: chega resumido, redigido e com o nome do provedor"
  aurum_prov erro LLM_PROVIDER=anthropic -- review --base main
  expect_rc 1 "erro 401 do provedor: nomeado, resumido e sem a chave"
}

# Falha 2: perfil desconhecido falha listando os validos; nada e enviado.
caso_falha_perfil_desconhecido() {
  tut_repo falha-perfil-desconhecido repo-exemplo/base repo-exemplo/mudanca
  aurum_prov legado LLM_PROVIDER=nao-existe -- review --base main
  expect_rc 1 "perfil desconhecido: erro com a lista de perfis validos"
}

# Falha 3: todos caem. A revisao falha fechada e o erro nomeia cada um.
caso_falha_todas_caem() {
  tut_repo falha-todas-caem repo-exemplo/base repo-exemplo/mudanca
  aurum_prov fora-do-ar LLM_FALLBACK_1_BASE_URL=http://127.0.0.1:8080/fora-do-ar LLM_FALLBACK_1_API_KEY=reserva-falsa -- review --base main
  expect_rc 1 "principal e reserva fora do ar: revisao falha, nunca aprovada"
}

# Falha 4: reserva configurada pela metade (URL sem chave). Erro antes de
# qualquer requisicao: a chave do principal nunca vai para a reserva.
caso_falha_reserva_sem_chave() {
  tut_repo falha-reserva-sem-chave repo-exemplo/base repo-exemplo/mudanca
  aurum_prov legado LLM_FALLBACK_1_BASE_URL=http://127.0.0.1:8080/reserva -- review --base main
  expect_rc 1 "reserva sem chave: erro nomeando LLM_FALLBACK_1, nada enviado"
}

tut_main "$@"
