#!/usr/bin/env bash
# Bootstrap-only data proof. The real Go assertion and red mutation are run
# separately by .board/tests/go-profile-regression.sh on the host via oci-run.
set -euo pipefail
selector="${1:-all}"
case "$selector" in all|AC-001|AC-003) ;; *) printf 'AUR-508/unknown-selector\n' >&2; exit 64 ;; esac
root="$(cd "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
profile="$root/.board/oci/profiles/go-unit-offline-v1.json"
lock="$root/.board/locks/oci/go-unit-offline-v1.lock.json"
schema="$root/.board/schemas/go-unit-offline-profile.schema.json"
registry="$root/.board/oci/profiles/registry.v1.json"
runner="$root/.board/bin/oci-run"
fail() { printf 'AUR-508/%s/%s\n' "$selector" "$1" >&2; exit 1; }
registry_digest() {
  awk -v target="$1" '
    /"key": "go-unit-offline-v1"/ { found=1; next }
    found && index($0, "\"" target "\":") {
      sub(/^.*: "/, ""); sub(/".*$/, ""); print; exit
    }
  ' "$registry"
}
check_profile_data() {
  local file="$1" lock_file="$2" lock_hash
  lock_hash="sha256:$(sha256sum "$lock_file" | awk '{print $1}')"
  grep -Fxq '"schema": "aurum.container-profile",' "$file" &&
    grep -Fxq '"profile": "go-unit-offline-v1",' "$file" &&
    grep -Fxq "\"lock_digest\": \"$lock_hash\"," "$file" &&
    grep -Fxq '"network": "none",' "$file" &&
    grep -Fxq '"cap_drop": "ALL",' "$file" &&
    grep -Fxq '"cap_add": "none",' "$file" &&
    grep -Fxq '"mounts": "none",' "$file" &&
    grep -Fxq '"devices": "none",' "$file" &&
    grep -Fxq '"pull": "never",' "$file" &&
    grep -Fxq '"tmpfs": "rw,nosuid,nodev",' "$file" &&
    grep -Fxq '"read_only_rootfs": true,' "$file" &&
    grep -Fxq '"no_new_privileges": true,' "$file" &&
    grep -Fxq '"privileged": false,' "$file" &&
    grep -Fxq '"timeout_seconds": 600,' "$file" &&
    grep -Fxq '"memory_mb": 2048,' "$file" &&
    grep -Fxq '"cpu_millis": 2000,' "$file" &&
    grep -Fxq '"pids_limit": 512,' "$file" &&
    grep -Fxq '"tmpfs_mb": 512,' "$file"
}
ac001() {
  [[ -f "$profile" && -f "$lock" && -f "$schema" && -f "$registry" ]] || fail missing-input
  check_profile_data "$profile" "$lock" || fail incompatible-profile-data
  [[ "$(registry_digest schema_digest)" == "sha256:$(sha256sum "$schema" | awk '{print $1}')" ]] || fail schema-registry-drift
  [[ "$(registry_digest lock_digest)" == "sha256:$(sha256sum "$lock" | awk '{print $1}')" ]] || fail lock-registry-drift
  grep -Fq "require_literal profile_doc \"\$profile_label\" schema s 'aurum.container-profile'" "$runner" || fail runner-schema-drift
  grep -Fq 'go-unit-offline-v1' "$root/.board/profile-owners.tsv" || fail missing-owner
  grep -Fq $'go-unit-offline-v1\tAUR-403' "$root/.board/profile-owners.tsv" || fail owner-drift

  local scratch
  scratch="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a508.XXXXXX")" || fail mktemp
  cp "$profile" "$scratch/profile.json"
  cp "$lock" "$scratch/lock.json"
  printf '\n' >> "$scratch/lock.json"
  if check_profile_data "$scratch/profile.json" "$scratch/lock.json"; then
    rm -rf -- "$scratch"
    fail divergent-lock-accepted
  fi
  sed 's/"privileged": false/"privileged": true/' "$profile" > "$scratch/profile.json"
  if check_profile_data "$scratch/profile.json" "$lock"; then
    rm -rf -- "$scratch"
    fail privilege-accepted
  fi
  rm -rf -- "$scratch"
}
ac003() {
  grep -Fq '"image": "aurum-bootstrap-go-bash@sha256:' "$lock" || fail wrong-runtime-lock
  grep -Fq 'run_tmpfs="/tmp:exec,' "$runner" || fail nonexecutable-scratch
  grep -Fq -- '--env=GOPROXY=off' "$runner" || fail missing-offline-env
  grep -Fq -- '--env=GOCACHE=/tmp/go-build' "$runner" || fail missing-scratch-cache
  grep -Fq -- '--env=CGO_ENABLED=0' "$runner" || fail missing-cgo-bound
  grep -Fq -- '"${go_profile_env[@]}"' "$runner" || fail env-not-passed-to-worker
  ! grep -Eq '"\$engine" exec "\$name" git[[:space:]]+config' "$root/.board/bin/go-shared" || fail container-git-config
  bash -n "$runner" || fail runner-syntax
  bash -n "$root/.board/card-preflight.sh" || fail preflight-syntax
  bash -n "$root/.board/bin/go-shared" || fail shared-syntax
}
case "$selector" in
  all) ac001; ac003 ;;
  AC-001) ac001 ;;
  AC-003) ac003 ;;
esac
printf 'AUR-508/%s/pass (profile data only; host Go regression still required)\n' "$selector"
