#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR407}" == E2EAUR407 ]] || { printf 'AUR-407/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/sqlite-offline-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
test -s "$repo_root/.board/locks/oci/sqlite-offline-v1.lock.json"
grep -Fqx '    "key": "sqlite-offline-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/sqlite-offline-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/sqlite-offline-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/sqlite-offline-v1.lock.json"
# The driver is admitted only content-addressed and only cgo-free, so no native SQLite
# library and no `sqlite3` binary is ever required by the plan.
grep -Fqx '"sqlite_driver": "digest-pinned",' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"sqlite_driver_cgo": false,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
# The database is ephemeral and lives in the bounded tmpfs, never in the checkout.
grep -Fqx '"database_mode": "ephemeral",' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"database_root": "/tmp/aurum-sqlite-state",' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
# Host database, extension loading, remote URI, implicit persistence and ATTACH are
# denied in the published bytes, and so is every host-reaching container capability.
for denied in host_database extension_loading remote_uri implicit_persistence attach_database; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
done
for none_key in network mounts devices sockets cap_add; do
  grep -Fqx "\"$none_key\": \"none\"," "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
done
grep -Fqx '"privileged": false,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"user": "65534:65534",' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
# Journal, storage and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"journal_mode": "memory",' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"tmpfs_mb": 128,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"max_database_bytes": 33554432,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"max_open_connections": 4,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"busy_timeout_ms": 2000,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"timeout_seconds": 60,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
grep -Fqx '"query_timeout_seconds": 5,' "$repo_root/.board/oci/profiles/sqlite-offline-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/sqlite-offline-v1.lock.json"
printf 'AUR-407/AC-001/E2EAUR407/ok\n'
