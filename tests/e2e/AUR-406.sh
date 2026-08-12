#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR406}" == E2EAUR406 ]] || { printf 'AUR-406/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/parser-worker-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/parser-worker-v1.json"
test -s "$repo_root/.board/locks/oci/parser-worker-v1.lock.json"
grep -Fqx '    "key": "parser-worker-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/parser-worker-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/parser-worker-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/parser-worker-v1.lock.json"
# Worker and grammar set are admitted only content-addressed, and both live in the
# bounded tmpfs rather than in the checkout.
grep -Fqx '"worker": "digest-pinned",' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"grammar_set": "digest-pinned",' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"worker_root": "/tmp/aurum-parser-worker",' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"grammar_set_root": "/tmp/aurum-parser-grammars",' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
# Code execution, the host filesystem and any subprocess are denied in the published
# bytes, and so is every host-reaching container capability.
for denied in code_execution host_filesystem subprocess; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/parser-worker-v1.json"
done
for none_key in network mounts devices sockets cap_add; do
  grep -Fqx "\"$none_key\": \"none\"," "$repo_root/.board/oci/profiles/parser-worker-v1.json"
done
grep -Fqx '"privileged": false,' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"user": "65534:65534",' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
# Blob and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"blob_source": "stdin",' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"timeout_seconds": 60,' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"parse_timeout_seconds": 5,' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"max_blob_bytes": 1048576,' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
grep -Fqx '"max_blobs": 256,' "$repo_root/.board/oci/profiles/parser-worker-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/parser-worker-v1.lock.json"
printf 'AUR-406/AC-001/E2EAUR406/ok\n'
