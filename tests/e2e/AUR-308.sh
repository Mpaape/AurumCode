#!/usr/bin/env bash
set -euo pipefail

case "${1:-E2EAUR308}" in
  E2EAUR308) ;;
  *) printf 'AUR-308/AC-001/unknown-selector\n' >&2; exit 64 ;;
esac

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"

command -v go >/dev/null 2>&1 || { printf 'AUR-308/AC-001/infrastructure/missing-go\n' >&2; exit 69; }
(cd "$repo_root" && go test -p 1 ./tests/characterization/legacy/documentation -run '^TestLegacyPackageMatrix$' -count=1)
