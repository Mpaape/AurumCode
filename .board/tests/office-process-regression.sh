#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

# The fixture proves the process independently at both gates: preflight and
# oci-run must reject Go when the locked image only provides Bash.

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
fixture="$(mktemp -d "${TMPDIR:-/tmp}/aurum-office-preflight.XXXXXX")"
cleanup() { rm -rf -- "$fixture"; }
trap cleanup EXIT INT TERM HUP

mkdir -p "$fixture/.board/cards/doing" "$fixture/.board/cards/ready" "$fixture/.board/oci/profiles" \
  "$fixture/.board/locks/oci" "$fixture/.board/bin" "$fixture/tests/acceptance"
cp -- "$root/.board/card-preflight.sh" "$fixture/.board/card-preflight.sh"
cp -- "$root/.board/bin/oci-run" "$fixture/.board/bin/oci-run"
chmod +x "$fixture/.board/card-preflight.sh" "$fixture/.board/bin/oci-run"

cat >"$fixture/.board/locks/oci/bootstrap-readonly-v1.lock.json" <<'EOF'
{
"schema": "aurum.oci-image-lock",
"version": 1,
"profile": "bootstrap-readonly-v1",
"image": "bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831"
}
EOF
fixture_lock_digest="sha256:$(sha256sum -- "$fixture/.board/locks/oci/bootstrap-readonly-v1.lock.json" | awk '{print $1}')"
cat >"$fixture/.board/oci/profiles/bootstrap-readonly-v1.json" <<EOF
{
"schema": "aurum.container-profile",
"version": 1,
"profile": "bootstrap-readonly-v1",
"lock": ".board/locks/oci/bootstrap-readonly-v1.lock.json",
"lock_digest": "$fixture_lock_digest",
"network": "none",
"user": "65534:65534",
"cap_drop": "ALL",
"cap_add": "none",
"mounts": "none",
"devices": "none",
"pull": "never",
"tmpfs": "rw,nosuid,nodev",
"read_only_rootfs": true,
"no_new_privileges": true,
"privileged": false,
"timeout_seconds": 120,
"memory_mb": 256,
"cpu_millis": 1000,
"pids_limit": 128,
"tmpfs_mb": 128,
"stdout_limit_bytes": 65536,
"stderr_limit_bytes": 65536,
"max_input_files": 10000,
"max_input_bytes": 67108864
}
EOF
cat >"$fixture/.board/cards/doing/AUR-900.md" <<'EOF'
---
id: AUR-900
version: 1
title: Runtime preflight regression fixture
status: doing
validation: tested
office: O00-governance
depends_on: []
paths: [tests/acceptance/AUR-900.sh]
forbidden_paths: [.git, .env, secrets]
---

## Acceptance

container_profile: `bootstrap-readonly-v1`
accept: `./.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-900`

## Skeptical mutations

### MUT-001

- Change: remove the promised runtime behavior.
EOF
cat >"$fixture/tests/acceptance/AUR-900.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'nominal\n'
EOF
chmod +x "$fixture/tests/acceptance/AUR-900.sh"

cat >"$fixture/.board/cards/ready/AUR-901.md" <<'EOF'
---
id: AUR-901
version: 1
title: Complete ready candidate regression fixture
status: ready
validation: tested
office: O00-governance
depends_on: []
paths: [tests/acceptance/AUR-901.sh]
forbidden_paths: [.git, .env, secrets]
---

## Acceptance

container_profile: `bootstrap-readonly-v1`
accept: `./.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-901`

## Skeptical mutations

### MUT-001

- Change: remove the candidate behavior.
EOF
cat >"$fixture/tests/acceptance/AUR-901.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'complete-ready-candidate\n'
EOF
chmod 0644 "$fixture/tests/acceptance/AUR-901.sh"

git -C "$fixture" init -q
git -C "$fixture" add .
git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid \
  commit -q -m fixture

positive_output="$(bash "$fixture/.board/card-preflight.sh" AUR-900 "$fixture")"
[[ "$positive_output" == *'runtime=bash'* ]] || {
  printf 'regression error: nominal Bash profile did not pass preflight\n' >&2
  exit 1
}

set +e
ready_mode_output="$(bash "$fixture/.board/card-preflight.sh" AUR-901 "$fixture" 2>&1)"
ready_mode_rc=$?
set -e
[[ "$ready_mode_rc" == 1 && "$ready_mode_output" == *'tested card lacks executable acceptance'* ]] || {
  printf 'regression error: complete ready candidate inherited relaxed builder checks\n' >&2
  printf '%s\n' "$ready_mode_output" >&2
  exit 1
}
chmod +x "$fixture/tests/acceptance/AUR-901.sh"
git -C "$fixture" add tests/acceptance/AUR-901.sh
git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid \
  commit -q -m executable-ready-candidate
ready_positive="$(PREFLIGHT_RUN=1 bash "$fixture/.board/card-preflight.sh" AUR-901 "$fixture")"
[[ "$ready_positive" == *'preflight ok: AUR-901'* ]] || {
  printf 'regression error: executable complete ready candidate did not pass\n' >&2
  exit 1
}

sed -i 's/^"lock_digest":.*$/"lock_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",/' \
  "$fixture/.board/oci/profiles/bootstrap-readonly-v1.json"
git -C "$fixture" add .board/oci/profiles/bootstrap-readonly-v1.json
git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid \
  commit -q -m bad-lock-binding
set +e
bad_lock_output="$(bash "$fixture/.board/card-preflight.sh" AUR-900 "$fixture" 2>&1)"
bad_lock_rc=$?
set -e
[[ "$bad_lock_rc" == 1 && "$bad_lock_output" == *'lock_digest does not match lock bytes'* ]] || {
  printf 'regression error: preflight accepted a forged lock binding\n' >&2
  printf '%s\n' "$bad_lock_output" >&2
  exit 1
}

sed -i "s|^\"lock_digest\":.*$|\"lock_digest\": \"$fixture_lock_digest\",|" \
  "$fixture/.board/oci/profiles/bootstrap-readonly-v1.json"
git -C "$fixture" add .board/oci/profiles/bootstrap-readonly-v1.json
git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid \
  commit -q -m restore-lock-binding

cat >"$fixture/tests/acceptance/AUR-900.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
go test ./...
EOF
chmod +x "$fixture/tests/acceptance/AUR-900.sh"
git -C "$fixture" add tests/acceptance/AUR-900.sh
git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid \
  commit -q -m go-runtime-mismatch

set +e
missing_module_output="$(bash "$fixture/.board/card-preflight.sh" AUR-900 "$fixture" 2>&1)"
missing_module_rc=$?
set -e
[[ "$missing_module_rc" == 1 && "$missing_module_output" == *'does not materialize go.mod'* ]] || {
  printf 'regression error: Go acceptance without module inputs passed preflight\n' >&2
  printf '%s\n' "$missing_module_output" >&2
  exit 1
}

printf 'module fixture\n' >"$fixture/go.mod"
: >"$fixture/go.sum"
sed -i '/^paths:/a read_paths: [go.mod, go.sum]' "$fixture/.board/cards/doing/AUR-900.md"
git -C "$fixture" add go.mod go.sum .board/cards/doing/AUR-900.md
git -C "$fixture" -c user.name=fixture -c user.email=fixture@example.invalid \
  commit -q -m materialize-go-module

set +e
negative_output="$(bash "$fixture/.board/card-preflight.sh" AUR-900 "$fixture" 2>&1)"
negative_rc=$?
runner_output="$(cd "$fixture" && ./.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-900 2>&1)"
runner_rc=$?
set -e
[[ "$negative_rc" == 1 && "$negative_output" == *'lacks required runtime (bash,go)'* ]] || {
  printf 'regression error: Go acceptance was not rejected by preflight\n' >&2
  printf '%s\n' "$negative_output" >&2
  exit 1
}
[[ "$runner_rc" == 65 && "$runner_output" == *'lacks required acceptance runtime: bash,go'* ]] || {
  printf 'regression error: oci-run did not reject the Go/runtime mismatch\n' >&2
  printf '%s\n' "$runner_output" >&2
  exit 1
}
printf 'office process regression: passed\n'
