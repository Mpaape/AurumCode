#!/usr/bin/env bash
set -euo pipefail

f='.board/decisions/ADR-0001-optional-local-memory.md'
for needle in 'off' 'ephemeral' 'local' '.git/aurumcode/memory.sqlite3' \
  'SQLite' 'FTS5' 'no vector database' 'typed status'; do
  grep -Fiq "$needle" "$f" || { printf 'AUR-020/AC-001: missing %s\n' "$needle" >&2; exit 1; }
done
printf '{"card":"AUR-020","scenario":"AC-001","result":"pass"}\n'
