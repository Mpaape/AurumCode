#!/usr/bin/env bash
# Smoke step of .github/workflows/providers-smoke.yml: a minimal live
# `aurumcode review` against the provider in PROVEDOR, with the model taken
# from the Actions variable SMOKE_MODEL_<PROVEDOR> (SMOKE_VARS) and the
# secrets received through the step's env. Exit 0 or 3 from the review is a
# verified provider; any other code fails the smoke.
set -euo pipefail
nome="SMOKE_MODEL_$(printf '%s' "$PROVEDOR" | tr 'a-z-' 'A-Z_')"
SMOKE_MODEL="$(printf '%s' "$SMOKE_VARS" | jq -r --arg n "$nome" '.[$n] // ""')"
[ -n "$SMOKE_MODEL" ] || { echo "defina a variavel de Actions $nome com o modelo do smoke" >&2; exit 1; }
repo="$RUNNER_TEMP/repo"
mkdir -p "$repo"
git -C "$repo" init -q -b main
printf 'def soma(a, b):\n    return a + b\n' > "$repo/app.py"
git -C "$repo" add app.py
git -C "$repo" -c user.name=smoke -c user.email=smoke@example.invalid commit -q -m base
printf 'def soma(a, b):\n    """Soma dois numeros."""\n    return a + b\n' > "$repo/app.py"
git -C "$repo" -c user.name=smoke -c user.email=smoke@example.invalid commit -qam doc
extra=()
if [ "$PROVEDOR" = litellm ]; then extra=(-e "LLM_BASE_URL=$LITELLM_BASE_URL"); fi
set +e
docker run --rm -v "$repo:/workspace" -w /workspace -e HOME=/tmp \
  -e "LLM_PROVIDER=$PROVEDOR" -e "LLM_MODEL=$SMOKE_MODEL" "${extra[@]}" \
  -e OPENAI_API_KEY -e AZURE_OPENAI_API_KEY -e AZURE_OPENAI_RESOURCE -e AZURE_OPENAI_DEPLOYMENT \
  -e ANTHROPIC_API_KEY -e GEMINI_API_KEY -e AWS_BEARER_TOKEN_BEDROCK -e AWS_REGION \
  -e OPENROUTER_API_KEY -e OPENCODE_API_KEY -e LITELLM_API_KEY \
  --entrypoint /app/aurumcode aurumcode-smoke review --base HEAD~1
rc=$?
set -e
echo "exit_code=$rc"
# 0 = revisao concluida sem bloqueio; 3 = concluida com achado que
# reprova. Qualquer outro codigo (1 = provedor falhou ou resposta
# inconclusiva) reprova o smoke.
case "$rc" in 0|3) echo "$PROVEDOR: verificado ao vivo" >> "$GITHUB_STEP_SUMMARY" ;; *) exit 1 ;; esac
