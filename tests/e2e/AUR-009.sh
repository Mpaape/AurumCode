#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

case "${1:-E2EAUR009}" in
  E2EAUR009) ;;
  *) printf 'AUR-009/AC-001/unknown-selector\n' >&2; exit 64 ;;
esac

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || {
  printf 'AUR-009/AC-001/inconclusive/repo_root\n' >&2
  exit 79
}

capture="$(mktemp "${TMPDIR:-/tmp}/aurum-a009-e2e.XXXXXX")" || {
  printf 'AUR-009/AC-001/inconclusive/mktemp\n' >&2
  exit 79
}
trap 'rm -f -- "$capture"' EXIT INT TERM HUP

set +e
bash "$repo_root/tests/acceptance/AUR-009.sh" IntegrationAUR009 >"$capture" 2>&1
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
  printf 'AUR-009/AC-001/e2e_child_exit:%s\n' "$rc" >&2
  exit 1
fi
bytes="$(wc -c <"$capture")" || exit 79
if [[ ! "$bytes" =~ ^[0-9]+$ ]] || (( bytes == 0 || bytes > 32768 )); then
  printf 'AUR-009/AC-001/e2e_output_unbounded\n' >&2
  exit 1
fi
if [[ -n "${AURUM_SECRET_CANARY:-}" ]] && grep -Fq -- "$AURUM_SECRET_CANARY" "$capture"; then
  printf 'AUR-009/AC-001/e2e_secret_canary_exposed\n' >&2
  exit 1
fi
grep -Fqx 'AUR-009/IntegrationAUR009: pass' "$capture" || {
  printf 'AUR-009/AC-001/e2e_effect_absent\n' >&2
  exit 1
}
printf 'AUR-009/E2EAUR009: pass\n'
