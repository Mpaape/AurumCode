#!/usr/bin/env bash
set -euo pipefail

readonly manifest='tests/characterization/legacy/routing-skills/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || {
  printf 'AUR-340/AC-001: routing-skill manifest absent\n' >&2
  exit 1
}
for skill in ai-memory-durable-pages ai-memory-handoff \
  ai-memory-learning-maintenance ai-memory-retrieval \
  ai-memory-routing-install; do
  grep -Fq ".agents/skills/$skill/SKILL.md" "$manifest" || {
    printf 'AUR-340/AC-001: agents routing skill unmapped\n' >&2
    exit 1
  }
  grep -Fq ".claude/skills/$skill/SKILL.md" "$manifest" || {
    printf 'AUR-340/AC-001: claude routing skill unmapped\n' >&2
    exit 1
  }
done
grep -Eq $'\tbyte_equivalent=true\tuntrusted=true$' "$manifest" || {
  printf 'AUR-340/AC-001: equivalence or trust proof absent\n' >&2
  exit 1
}
printf '{"card":"AUR-340","scenario":"AC-001","result":"pass"}\n'
