#!/usr/bin/env bash
set -euo pipefail

[[ -L AGENTS.md && "$(readlink AGENTS.md)" == 'CLAUDE.md' ]] || {
  printf 'AUR-010/AC-001: canonical instruction symlink absent\n' >&2
  exit 1
}
grep -Fq 'Never add AI authorship' CLAUDE.md || {
  printf 'AUR-010/AC-001: no-AI-attribution rule absent\n' >&2
  exit 1
}
printf '{"card":"AUR-010","scenario":"AC-001","result":"pass"}\n'
