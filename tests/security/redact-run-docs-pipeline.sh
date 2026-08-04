#!/usr/bin/env bash
set -euo pipefail

readonly target='RUN_DOCS_PIPELINE.md'
readonly placeholder='${OPENAI_API_KEY}'

[[ -f "$target" && ! -L "$target" ]] || {
  printf 'AUR-333 remediation: target missing or symlinked\n' >&2
  exit 1
}

# Count and replace in-process. Neither branch writes the matched value to a
# stream. Refuse ambiguity so an expanded rewrite cannot happen silently.
matches="$(perl -ne '$n += () = /sk-(?:proj-)?[A-Za-z0-9_-]{20,}/g; END { print $n + 0 }' "$target")"
case "$matches" in
  1)
    perl -pi -e 's/sk-(?:proj-)?[A-Za-z0-9_-]{20,}/\${OPENAI_API_KEY}/g' "$target"
    ;;
  0)
    grep -Fq "$placeholder" "$target" || {
      printf 'AUR-333 remediation: neither credential pattern nor placeholder found\n' >&2
      exit 1
    }
    ;;
  *)
    printf 'AUR-333 remediation: ambiguous credential-pattern count\n' >&2
    exit 1
    ;;
esac

printf 'AUR-333 remediation: placeholder verified; credential bytes not emitted\n'
