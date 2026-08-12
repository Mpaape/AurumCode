#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR405}" == E2EAUR405 ]] || { printf 'AUR-405/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/fake-provider-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/fake-provider-v1.json"
test -s "$repo_root/.board/locks/oci/fake-provider-v1.lock.json"
grep -Fqx '    "key": "fake-provider-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/fake-provider-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/fake-provider-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/fake-provider-v1.lock.json"
# The adapter can only reach the loopback fake, carries no API key, and every
# credential and name-resolution path out of the namespace is denied.
grep -Fqx '"provider_endpoint": "http://127.0.0.1:8080/v1",' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
for denied in dns egress credential_sources; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/fake-provider-v1.json"
done
grep -Fqx '"api_key": "absent",' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
grep -Fqx '"response_scripts": "digest-pinned",' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
grep -Fqx '"network": "none",' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
# Response and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"timeout_seconds": 60,' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
grep -Fqx '"request_timeout_seconds": 5,' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
grep -Fqx '"max_response_bytes": 65536,' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
grep -Fqx '"max_responses": 64,' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/fake-provider-v1.lock.json"
# No document this card publishes may carry an outbound provider host.
! grep -Fq 'api.openai.com' "$repo_root/.board/oci/profiles/fake-provider-v1.json"
printf 'AUR-405/AC-001/E2EAUR405/ok\n'
