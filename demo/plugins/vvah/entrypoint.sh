#!/bin/sh
# Ponto de entrada do plugin de exemplo: o harness so descobre. Os parametros
# que o tornam seguro ficam aqui, nao na configuracao do repositorio:
# perfil fora da arvore (--config), parada antes de S10 (--stop-after s9),
# sem configuracao local (VVAHARNESS_NO_LOCAL_CONFIG=1) e sem rastreamento
# nem envio a hub (variaveis apagadas antes de executar).
set -eu
unset LANGSMITH_TRACING LANGSMITH_TRACING_V2 LANGSMITH_API_KEY LANGSMITH_ENDPOINT LANGSMITH_PROJECT \
  LANGCHAIN_TRACING LANGCHAIN_TRACING_V2 LANGCHAIN_API_KEY LANGCHAIN_ENDPOINT LANGCHAIN_PROJECT \
  VVAHARNESS_INGEST_URL VVAHARNESS_INGEST_TOKEN VVAHARNESS_INGEST_VERIFY EV_API_COLLECTION 2>/dev/null || true
export VVAHARNESS_NO_LOCAL_CONFIG=1
: "${VVAH_MODEL:?defina VVAH_MODEL (ex.: claude-haiku-5-5)}"
: "${ANTHROPIC_API_KEY:?defina ANTHROPIC_API_KEY}"
exec vvaharness scan --repo /alvo --config /cfg/descoberta.yaml --stop-after s9 "$@"
