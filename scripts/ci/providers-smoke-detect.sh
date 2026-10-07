#!/usr/bin/env bash
# Detect step of .github/workflows/providers-smoke.yml: which LLM providers
# have every secret they need (received through the step's env). Writes the
# summary table and the `configurados` JSON list to GITHUB_OUTPUT; with no
# verifiable provider the run proves nothing and fails.
set -euo pipefail
declare -A precisa=(
  [openai]="OPENAI_API_KEY"
  [azure-openai]="AZURE_OPENAI_API_KEY AZURE_OPENAI_RESOURCE AZURE_OPENAI_DEPLOYMENT"
  [anthropic]="ANTHROPIC_API_KEY"
  [google]="GEMINI_API_KEY"
  [bedrock]="AWS_BEARER_TOKEN_BEDROCK AWS_REGION"
  [openrouter]="OPENROUTER_API_KEY"
  [opencode]="OPENCODE_API_KEY"
  [litellm]="LITELLM_API_KEY LITELLM_BASE_URL"
)
configurados=()
{
  echo "## Smoke de provedores"
  echo
  echo "| Provedor | Estado |"
  echo "|---|---|"
} >> "$GITHUB_STEP_SUMMARY"
for p in openai azure-openai anthropic google bedrock openrouter opencode litellm; do
  ok=1
  for v in ${precisa[$p]}; do [ -n "${!v:-}" ] || ok=0; done
  if [ "$ok" = 1 ]; then
    configurados+=("\"$p\"")
    echo "| $p | smoke ao vivo no job smoke ($p) |" >> "$GITHUB_STEP_SUMMARY"
  else
    echo "::warning::$p: nao verificado (secrets ausentes: ${precisa[$p]})"
    echo "| $p | nao verificado (sem secrets) |" >> "$GITHUB_STEP_SUMMARY"
  fi
done
echo "| ollama | nao verificado (exige servidor local) |" >> "$GITHUB_STEP_SUMMARY"
lista="[$(IFS=,; echo "${configurados[*]:-}")]"
echo "configurados=$lista" >> "$GITHUB_OUTPUT"
echo "provedores com secrets: $lista"
if [ "${#configurados[@]}" -eq 0 ]; then
  # Nenhum provedor verificavel: a execucao nao prova nada e nao
  # pode terminar verde.
  echo "::error::nenhum provedor com secrets; todos nao verificados"
  exit 1
fi
