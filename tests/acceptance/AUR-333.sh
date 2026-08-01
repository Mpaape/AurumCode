#!/usr/bin/env bash
set -euo pipefail

readonly target='RUN_DOCS_PIPELINE.md'
readonly placeholder='${OPENAI_API_KEY}'

[[ -f "$target" && ! -L "$target" ]] || {
  printf 'AUR-333/AC-001: documentation input missing or symlinked\n' >&2
  exit 1
}

# These checks intentionally report only the pattern class. Matched bytes are
# never printed, copied to evidence, or passed as command arguments.
if grep -Eq 'sk-(proj-)?[A-Za-z0-9_-]{20,}' "$target"; then
  printf 'AUR-333/AC-001: credential-shaped value present\n' >&2
  exit 1
fi
if grep -Eq '(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,}|gh[pousr]_[A-Za-z0-9]{20,}|gsk_[A-Za-z0-9]{20,})' "$target"; then
  printf 'AUR-333/AC-001: credential-shaped value present\n' >&2
  exit 1
fi
grep -Fq "$placeholder" "$target" || {
  printf 'AUR-333/AC-001: non-secret placeholder absent\n' >&2
  exit 1
}

printf '{"card":"AUR-333","scenario":"AC-001","result":"pass","secret_bytes_emitted":false}\n'
