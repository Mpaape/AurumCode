#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-E2EAUR403}" == E2EAUR403 ]] || { printf 'AUR-403/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/oci/profiles/go-unit-offline-v1.json"
test -s "$repo_root/.board/locks/oci/go-unit-offline-v1.lock.json"
test -s "$repo_root/.board/schemas/go-unit-offline-profile.schema.json"
grep -Fqx '    "key": "go-unit-offline-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/go-unit-offline-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/go-unit-offline-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
