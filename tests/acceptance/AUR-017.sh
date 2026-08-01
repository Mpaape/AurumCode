#!/usr/bin/env bash
set -euo pipefail

f='.board/research/interoperability.md'
[[ -s "$f" ]] || { printf 'AUR-017/AC-001: interoperability baseline absent\n' >&2; exit 1; }
for needle in 'SARIF 2.1.0' 'OpenAI' 'MCP'; do grep -Fq "$needle" "$f" || exit 1; done
printf '{"card":"AUR-017","scenario":"AC-001","result":"pass"}\n'
