#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR411}" == E2EAUR411 ]] || { printf 'AUR-411/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
profile="$repo_root/.board/oci/profiles/polyglot-toolchain-v1.json"
test -s "$repo_root/.board/schemas/polyglot-toolchain-profile.schema.json"
test -s "$profile"
test -s "$repo_root/.board/locks/oci/polyglot-toolchain-v1.lock.json"
grep -Fqx '    "key": "polyglot-toolchain-v1",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "schema": ".board/schemas/polyglot-toolchain-profile.schema.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
grep -Fqx '    "lock": ".board/locks/oci/polyglot-toolchain-v1.lock.json",' "$repo_root/.board/oci/profiles/registry.v1.json"
# The Go-capable image is admitted only by immutable digest. It is the bootstrap image,
# which was probed under the profile's own flags: it carries bash and go and none of the
# eight declared runtimes beyond bash. That absence is what this plan denies rather than
# tolerates, because the plan can never install one.
grep -Fqx '"image": "aurum-bootstrap-go-bash@sha256:503ac356fa6bca4bad56fade87b5479ff371bd446ffb9c7db91f211323c7c73e"' "$repo_root/.board/locks/oci/polyglot-toolchain-v1.lock.json"
# Toolchains are pinned, pre-materialized and never created by the plan.
grep -Fqx '"polyglot_root": "/tmp/aurum-polyglot",' "$profile"
grep -Fqx '"toolchain_source": "digest-pinned",' "$profile"
grep -Fqx '"toolchain_materialization": "pre-materialized",' "$profile"
grep -Fqx '"cache_mode": "read-only",' "$profile"
grep -Fqx '"package_installs": 0,' "$profile"
# The eight language partitions are published verbatim, each with its own runtime, its own
# bounded read-only cache directory and telemetry denied.
grep -Fqx '"languages": ["bash", "c-cpp", "dotnet", "javascript", "powershell", "python", "rust", "typescript"],' "$profile"
grep -Fqx '"bash": {"runtime": "bash", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/bash", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"c-cpp": {"runtime": "cc", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/c-cpp", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"dotnet": {"runtime": "dotnet", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/dotnet", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"javascript": {"runtime": "node", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/javascript", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"powershell": {"runtime": "pwsh", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/powershell", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"python": {"runtime": "python3", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/python", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"rust": {"runtime": "cargo", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/rust", "cache_mode": "read-only", "telemetry": "denied"},' "$profile"
grep -Fqx '"typescript": {"runtime": "tsc", "toolchain": "digest-pinned", "cache_root": "/tmp/aurum-polyglot/typescript", "cache_mode": "read-only", "telemetry": "denied"}' "$profile"
# Install, bootstrap, package registry, telemetry, egress, the host toolchain, the host
# cache, the host filesystem, a subprocess and an unregistered language are denied in the
# published bytes, and so is every host-reaching container capability.
for denied in toolchain_install toolchain_bootstrap package_install package_registry telemetry egress host_toolchain host_cache host_filesystem subprocess unregistered_language; do
  grep -Fqx "\"$denied\": \"denied\"," "$profile"
done
for none_key in network mounts devices sockets cap_add; do
  grep -Fqx "\"$none_key\": \"none\"," "$profile"
done
grep -Fqx '"privileged": false,' "$profile"
grep -Fqx '"read_only_rootfs": true,' "$profile"
grep -Fqx '"no_new_privileges": true,' "$profile"
grep -Fqx '"checkout_readonly": true,' "$profile"
grep -Fqx '"cap_drop": "ALL",' "$profile"
grep -Fqx '"user": "65534:65534",' "$profile"
grep -Fqx '"pull": "never",' "$profile"
# Every channel a package manager could reach an index, a registry or a telemetry endpoint
# through is closed in the published environment too.
grep -Fq '"PIP_NO_INDEX": "1"' "$profile"
grep -Fq '"PIP_DISABLE_PIP_VERSION_CHECK": "1"' "$profile"
grep -Fq '"NPM_CONFIG_OFFLINE": "true"' "$profile"
grep -Fq '"NPM_CONFIG_REGISTRY": ""' "$profile"
grep -Fq '"NUGET_PACKAGES": "/tmp/aurum-polyglot/dotnet"' "$profile"
grep -Fq '"DOTNET_CLI_TELEMETRY_OPTOUT": "1"' "$profile"
grep -Fq '"DOTNET_NOLOGO": "1"' "$profile"
grep -Fq '"CARGO_HOME": "/tmp/aurum-polyglot/rust"' "$profile"
grep -Fq '"CARGO_NET_OFFLINE": "true"' "$profile"
grep -Fq '"POWERSHELL_TELEMETRY_OPTOUT": "1"' "$profile"
grep -Fq '"POWERSHELL_UPDATECHECK": "Off"' "$profile"
grep -Fq '"AURUM_POLYGLOT_ROOT": "/tmp/aurum-polyglot"' "$profile"
grep -Fq '"CGO_ENABLED": "0"' "$profile"
grep -Fq '"NO_PROXY": "*"' "$profile"
# Partition, package and wall-clock bounds are present and non-zero in the published bytes.
grep -Fqx '"tmpfs_mb": 128,' "$profile"
grep -Fqx '"pids_limit": 128,' "$profile"
grep -Fqx '"memory_mb": 256,' "$profile"
grep -Fqx '"cpu_millis": 1000,' "$profile"
grep -Fqx '"max_languages": 8,' "$profile"
grep -Fqx '"max_packages": 4096,' "$profile"
grep -Fqx '"max_package_bytes": 16384,' "$profile"
grep -Fqx '"max_cache_bytes": 8388608,' "$profile"
grep -Fqx '"timeout_seconds": 60,' "$profile"
grep -Fqx '"resolve_timeout_seconds": 5,' "$profile"
! grep -Fq 'latest' "$repo_root/.board/locks/oci/polyglot-toolchain-v1.lock.json"
printf 'AUR-411/AC-001/E2EAUR411/ok\n'
