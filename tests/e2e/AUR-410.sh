#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR410}" == E2EAUR410 ]] || { printf 'AUR-410/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
test -s "$repo_root/.board/schemas/oci-conformance-profile.schema.json"
test -s "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
test -s "$repo_root/.board/locks/oci/oci-conformance-v1.lock.json"
grep -Fqx '    "key": "oci-conformance-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/oci-conformance-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/oci-conformance-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest. It is the bootstrap image,
# which carries no container-engine CLI at all: no docker, podman, nerdctl, ctr, crictl,
# runc, buildah or skopeo, and no engine socket.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/oci-conformance-v1.lock.json"
# The trusted orchestrator and the untrusted probes are two different sides, and the
# verdict belongs to the trusted one. Probe output is an observation, never an authority.
grep -Fqx '"orchestrator": "trusted",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe": "untrusted",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_output": "untrusted-observation",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_verdict_authority": "denied",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"verdict_authority": "orchestrator",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_image_set": "digest-pinned",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_cgo": false,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
# The orchestrator, the probes and the rendered reports live under three distinct bounded
# tmpfs roots and never in the checkout, and the probe writes only into its own root.
grep -Fqx '"orchestrator_root": "/tmp/aurum-oci-orchestrator",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_root": "/tmp/aurum-oci-probes",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"report_root": "/tmp/aurum-oci-reports",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"report_mode": "ephemeral",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_writable_roots": ["/tmp/aurum-oci-probes"],' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
# The engine allowlist is closed and published verbatim, and this card reaches none of
# them: the declared invocation count is zero.
grep -Fqx '"supported_engines": ["docker", "podman"],' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"unsupported_engine": "denied",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"engine_invocations": 0,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
# The engine socket, the engine API, the host engine, egress, the host checkout, the host
# filesystem and a subprocess are denied in the published bytes, and so is every
# host-reaching container capability.
for denied in engine_socket engine_api host_engine egress host_checkout host_filesystem subprocess; do
  grep -Fqx "\"$denied\": \"denied\"," "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
done
for none_key in network mounts devices sockets cap_add; do
  grep -Fqx "\"$none_key\": \"none\"," "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
done
grep -Fqx '"privileged": false,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"read_only_rootfs": true,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"no_new_privileges": true,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"checkout_readonly": true,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"cap_drop": "ALL",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"user": "65534:65534",' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
# Every channel an engine client could be addressed through is closed in the published
# environment too: no endpoint, no client configuration directory, no proxy.
grep -Fq '"DOCKER_HOST": ""' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fq '"CONTAINER_HOST": ""' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fq '"DOCKER_CONFIG": "/dev/null"' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fq '"DOCKER_API_VERSION": ""' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fq '"CGO_ENABLED": "0"' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fq '"NO_PROXY": "*"' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
# Probe, report and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"tmpfs_mb": 128,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"pids_limit": 128,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"memory_mb": 256,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"cpu_millis": 1000,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"max_probes": 1024,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"max_probe_bytes": 65536,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"max_report_bytes": 1048576,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"timeout_seconds": 60,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
grep -Fqx '"probe_timeout_seconds": 5,' "$repo_root/.board/oci/profiles/oci-conformance-v1.json"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/oci-conformance-v1.lock.json"
printf 'AUR-410/AC-001/E2EAUR410/ok\n'
