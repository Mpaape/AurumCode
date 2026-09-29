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
scratch=''
cleanup_scratch() {
  if [[ -n "$scratch" && "$scratch" == "${TMPDIR:-/tmp}/aurum-a508."* && -d "$scratch" ]]; then
    rm -rf -- "$scratch"
  fi
}
trap cleanup_scratch EXIT
registry_digest() {
  awk -v target="$1" '
    /"key": "go-unit-offline-v1"/ { found=1; next }
    found && index($0, "\"" target "\":") {
      sub(/^.*: "/, ""); sub(/".*$/, ""); print; exit
    }
  ' "$registry"
}
check_range() {
  local file="$1" key="$2" minimum="$3" maximum="$4" value
  value="$(awk -v sought="\"$key\":" '$1 == sought { gsub(/,/, "", $2); print $2; exit }' "$file")"
  [[ "$value" =~ ^(0|[1-9][0-9]*)$ ]] || return 1
  (( value >= minimum && value <= maximum ))
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
    check_range "$file" timeout_seconds 1 900 &&
    check_range "$file" memory_mb 64 4096 &&
    check_range "$file" cpu_millis 100 4000 &&
    check_range "$file" pids_limit 16 4096 &&
    check_range "$file" tmpfs_mb 8 1024 &&
    check_range "$file" stdout_limit_bytes 4096 1048576 &&
    check_range "$file" stderr_limit_bytes 4096 1048576 &&
    check_range "$file" max_input_files 1 100000 &&
    check_range "$file" max_input_bytes 1 268435456
}
ac001() {
  [[ -f "$profile" && -f "$lock" && -f "$schema" && -f "$registry" ]] || fail missing-input
  check_profile_data "$profile" "$lock" || fail incompatible-profile-data
  [[ "$(registry_digest schema_digest)" == "sha256:$(sha256sum "$schema" | awk '{print $1}')" ]] || fail schema-registry-drift
  [[ "$(registry_digest lock_digest)" == "sha256:$(sha256sum "$lock" | awk '{print $1}')" ]] || fail lock-registry-drift
  grep -Fq "require_literal profile_doc \"\$profile_label\" schema s 'aurum.container-profile'" "$runner" || fail runner-schema-drift
  grep -Fq 'go-unit-offline-v1' "$root/.board/profile-owners.tsv" || fail missing-owner
  grep -Fq $'go-unit-offline-v1\tAUR-403' "$root/.board/profile-owners.tsv" || fail owner-drift

  scratch="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a508.XXXXXX")" || fail mktemp
  cp "$profile" "$scratch/profile.json"
  cp "$lock" "$scratch/lock.json"
  chmod u+w "$scratch/profile.json" "$scratch/lock.json"
  printf '\n' >> "$scratch/lock.json"
  if check_profile_data "$scratch/profile.json" "$scratch/lock.json"; then
    fail divergent-lock-accepted
  fi
  sed 's/"privileged": false/"privileged": true/' "$profile" > "$scratch/profile.json"
  if check_profile_data "$scratch/profile.json" "$lock"; then
    fail privilege-accepted
  fi
  sed 's/"cpu_millis": [0-9][0-9]*/"cpu_millis": 0/' "$profile" > "$scratch/profile.json"
  if check_profile_data "$scratch/profile.json" "$lock"; then
    fail out-of-range-cpu-accepted
  fi
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
