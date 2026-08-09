#!/usr/bin/env bash
set -euo pipefail

readonly selector="${1:-E2EAUR006}"
case "$selector" in
  E2EAUR006) ;;
  *) printf 'AUR-006/AC-001/unknown-selector\n' >&2; exit 64 ;;
esac

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)"
exec env AURUM_A006_INTERNAL=1 bash "$repo_root/tests/acceptance/AUR-006.sh" AC-001-MATRIX
