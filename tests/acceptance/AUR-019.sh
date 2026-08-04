#!/usr/bin/env bash
set -euo pipefail

f='.board/research/providers.md'
[[ -s "$f" ]] || { printf 'AUR-019/AC-001: provider baseline absent\n' >&2; exit 1; }
for needle in 'OpenAI' 'LiteLLM' 'Anthropic' 'Ollama' 'Azure' 'Gemini' 'Bedrock'; do
  grep -Fq "$needle" "$f" || exit 1
done
printf '{"card":"AUR-019","scenario":"AC-001","result":"pass"}\n'
