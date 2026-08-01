#!/usr/bin/env bash
set -euo pipefail

f='.board/research/mcp.md'
[[ -s "$f" ]] || { printf 'AUR-021/AC-001: MCP baseline absent\n' >&2; exit 1; }
for needle in 'stdio' 'read-only' 'OAuth' 'injection' 'capability'; do grep -Fiq "$needle" "$f" || exit 1; done
printf '{"card":"AUR-021","scenario":"AC-001","result":"pass"}\n'
