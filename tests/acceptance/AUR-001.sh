#!/usr/bin/env bash
set -euo pipefail

ledger='.board/research/legacy-disposition.md'
[[ -s "$ledger" ]] || { printf 'AUR-001/AC-001: legacy ledger absent\n' >&2; exit 1; }
for surface in 'go.mod' 'Dockerfile' 'action.yml' 'cmd/regenerate-docs' \
  'internal/documentation' 'internal/pipeline' 'internal/llm' 'pkg/types' \
  '.taskmaster' 'CLAUDE.md'; do
  grep -Fq "\`$surface" "$ledger" || {
    printf 'AUR-001/AC-001: unmapped legacy surface: %s\n' "$surface" >&2
    exit 1
  }
done
printf '{"card":"AUR-001","scenario":"AC-001","result":"pass"}\n'
