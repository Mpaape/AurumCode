#!/usr/bin/env bash
# AUR-422 E2E: the checked-in bootstrap-readonly-v1 documents are registered,
# digest-bound, hardened, and the loader source performs no engine call.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR422}" == E2EAUR422 ]] || { printf 'AUR-422/AC-001/unknown-selector\n' >&2; exit 64; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)"
command -v sha256sum >/dev/null 2>&1 || { printf 'AUR-422/AC-001/infrastructure/missing_sha256sum\n' >&2; exit 69; }

profile="$repo_root/.board/oci/profiles/bootstrap-readonly-v1.json"
lock="$repo_root/.board/locks/oci/bootstrap-readonly-v1.lock.json"
registry="$repo_root/.board/oci/profiles/registry.v1.json"
schema="$repo_root/.board/schemas/container-profile.schema.json"
runner="$repo_root/.board/bin/oci-run"

test -s "$profile"
test -s "$lock"
test -s "$registry"
test -s "$schema"
test -s "$runner"

# The key is registered and the profile pins its own lock path.
grep -Fq '"key": "bootstrap-readonly-v1"' "$registry"
grep -Fqx '"lock": ".board/locks/oci/bootstrap-readonly-v1.lock.json",' "$profile"

# Hardening literals of the materialized profile.
grep -Fqx '"network": "none",' "$profile"
grep -Fqx '"user": "65534:65534",' "$profile"
grep -Fqx '"cap_drop": "ALL",' "$profile"
grep -Fqx '"read_only_rootfs": true,' "$profile"
grep -Fqx '"no_new_privileges": true,' "$profile"
grep -Fqx '"privileged": false,' "$profile"
grep -Fqx '"pull": "never",' "$profile"

# Declared ceilings: wall-clock, memory, CPU, PIDs, stdout/stderr bytes.
grep -Fqx '"timeout_seconds": 120,' "$profile"
grep -Fqx '"memory_mb": 256,' "$profile"
grep -Fqx '"cpu_millis": 1000,' "$profile"
grep -Fqx '"pids_limit": 128,' "$profile"
grep -Fqx '"stdout_limit_bytes": 65536,' "$profile"
grep -Fqx '"stderr_limit_bytes": 65536,' "$profile"

# The lock pins the image by immutable digest, never a mutable tag.
grep -Eq '^"image": "[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}"$' "$lock"

# Content-addressed binding: the digest declared by the profile is the digest
# of the lock bytes on disk.
declared_digest="$(grep -o '"lock_digest": "sha256:[0-9a-f]\{64\}"' "$profile" | grep -o 'sha256:[0-9a-f]*')"
actual_digest="sha256:$(sha256sum "$lock" | awk '{print $1}')"
[[ "$declared_digest" == "$actual_digest" ]] || { printf 'AUR-422/AC-001/MUT-001/lock-mismatch\n' >&2; exit 1; }

# The runner keeps its die 78 diagnostics for unregistered profiles.
grep -Fq 'die 78 "profile is not registered' "$runner"
grep -Fq 'has no lock at .board/locks/oci' "$runner"

# The characterization loader performs zero engine calls: no exec import.
if grep -Fq 'os/exec' "$repo_root/tests/unit/AUR-422.go"; then
  printf 'AUR-422/AC-001/MUT-003/privileged-plan\n' >&2
  exit 1
fi

printf 'e2e=ok profile=bootstrap-readonly-v1 lock_digest=%s engine_invocations=0\n' "$actual_digest"
