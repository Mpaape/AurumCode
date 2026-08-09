#!/usr/bin/env bash
set -euo pipefail
case "${1:-E2EAUR402}" in E2EAUR402) ;; *) printf 'AUR-402/AC-001/unknown-selector\n' >&2; exit 64 ;; esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/oci/profiles/registry.v1.json"
test -s "$repo_root/.board/locks/oci/registry-v1.lock.json"
