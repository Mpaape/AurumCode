#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR409}" == E2EAUR409 ]] || { printf 'AUR-409/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/fake-scm-offline-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
test -s "$repo_root/.board/locks/oci/fake-scm-offline-v1.lock.json"
grep -Fqx '    "key": "fake-scm-offline-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/fake-scm-offline-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/fake-scm-offline-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest. It is the bootstrap image,
# which carries no git, no git-upload-pack, no git-receive-pack, no ssh and no curl.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/fake-scm-offline-v1.lock.json"
# The SCM is the local fake, never a real version-control binary, and the fake engine,
# the event log and the response set are admitted only content-addressed.
grep -Fqx '"scm_backend": "fake-local",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"real_scm_binary": "denied",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"fake_scm": "digest-pinned",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"fake_scm_cgo": false,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"event_set": "digest-pinned",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"response_set": "digest-pinned",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
# The fake engine, the pinned events and the simulated repositories live under three
# distinct bounded tmpfs roots and never in the checkout; the repositories are ephemeral.
grep -Fqx '"fake_scm_root": "/tmp/aurum-fake-scm-engine",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"event_root": "/tmp/aurum-fake-scm-events",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"repository_mode": "ephemeral",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"repository_root": "/tmp/aurum-fake-scm-repos",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
# There is no origin and no token, and the transport allowlist is closed and published
# verbatim: the in-process fake transport is the only one admitted.
grep -Fqx '"remote_origin": "absent",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"token": "absent",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"remote_protocols": ["fake"],' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
# A custom transport, a credential helper, an askpass program, hooks, submodules, URL
# rewriting, publication, an external destination, a credential source, the host
# checkout, the host filesystem and a subprocess are denied in the published bytes, and
# so is every host-reaching container capability.
for denied in custom_transport credential_helper askpass hooks submodules url_rewriting publication external_destination credential_sources host_checkout host_filesystem subprocess; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
done
for none_key in network mounts devices sockets cap_add; do
  grep -Fqx "\"$none_key\": \"none\"," "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
done
grep -Fqx '"privileged": false,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"user": "65534:65534",' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
# Every configuration channel a version-control client could read a credential helper, an
# askpass program, an `insteadOf` rewrite or an `ext::` transport from is closed too.
grep -Fq '"GIT_CONFIG_NOSYSTEM": "1"' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fq '"GIT_CONFIG_GLOBAL": "/dev/null"' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fq '"GIT_CONFIG_SYSTEM": "/dev/null"' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fq '"GIT_TERMINAL_PROMPT": "0"' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fq '"GIT_ASKPASS": "/bin/false"' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fq '"GIT_ALLOW_PROTOCOL": "none"' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
# Replay, repository and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"tmpfs_mb": 128,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"max_events": 1024,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"max_event_bytes": 65536,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"max_responses": 64,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"max_response_bytes": 65536,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"max_repository_bytes": 33554432,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"timeout_seconds": 60,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
grep -Fqx '"scm_timeout_seconds": 5,' "$repo_root/.board/oci/profiles/fake-scm-offline-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/fake-scm-offline-v1.lock.json"
printf 'AUR-409/AC-001/E2EAUR409/ok\n'
