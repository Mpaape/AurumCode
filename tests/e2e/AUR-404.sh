#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR404}" == E2EAUR404 ]] || { printf 'AUR-404/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/go-git-offline-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/go-git-offline-v1.json"
test -s "$repo_root/.board/locks/oci/go-git-offline-v1.lock.json"
grep -Fqx '    "key": "go-git-offline-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/go-git-offline-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/go-git-offline-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Git-capable image is admitted only by immutable digest, and the plan denies
# every host-reaching capability the fixture could otherwise inherit.
grep -Fqx '"image": "golang@sha256:4746d26432a9117a5f58e95cb9f954ddf0de128e9d5816886514199316e4a2fb"' "$repo_root/.board/locks/oci/go-git-offline-v1.lock.json"
for denied in host_checkout credential_helpers hooks signing; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/go-git-offline-v1.json"
done
grep -Fqx '"network": "none",' "$repo_root/.board/oci/profiles/go-git-offline-v1.json"
grep -Fqx '"git_fixture": "ephemeral",' "$repo_root/.board/oci/profiles/go-git-offline-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/go-git-offline-v1.lock.json"
printf 'AUR-404/AC-001/E2EAUR404/ok\n'
