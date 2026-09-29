#!/usr/bin/env bash
# Host-only orchestration. Git stays here; Go runs only through the sealed runner.
set -euo pipefail
root="$(cd "${BASH_SOURCE[0]%/*}/../.." && pwd -P)"
fixture="$(mktemp -d "${TMPDIR:-/tmp}/aurum-go-profile.XXXXXX")"
trap 'rm -rf -- "$fixture"' EXIT
fail() { printf 'go-profile regression: %s\n' "$1" >&2; exit 1; }

author_name="$(git -C "$root" config user.name)"
author_email="$(git -C "$root" config user.email)"
[[ -n "$author_name" && -n "$author_email" ]] || fail 'configured human Git identity is required'
fixture_commit() {
  git -C "$fixture" add .
  GIT_AUTHOR_NAME="$author_name" GIT_AUTHOR_EMAIL="$author_email" \
  GIT_COMMITTER_NAME="$author_name" GIT_COMMITTER_EMAIL="$author_email" \
    git -C "$fixture" commit -q -m "$1"
}
run_runner() { (cd "$fixture" && ./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-999); }
run_preflight() { bash "$fixture/.board/card-preflight.sh" AUR-999 "$fixture"; }

mkdir -p "$fixture/.board/bin" "$fixture/.board/cards/ready" \
  "$fixture/.board/oci/profiles" "$fixture/.board/locks/oci" \
  "$fixture/tests/acceptance" "$fixture/fixture"
cp "$root/.board/bin/oci-run" "$fixture/.board/bin/oci-run"
cp "$root/.board/card-preflight.sh" "$fixture/.board/card-preflight.sh"
cp "$root/.board/oci/profiles/go-unit-offline-v1.json" "$fixture/.board/oci/profiles/go-unit-offline-v1.json"
cp "$root/.board/locks/oci/go-unit-offline-v1.lock.json" "$fixture/.board/locks/oci/go-unit-offline-v1.lock.json"
cat > "$fixture/.board/cards/ready/AUR-999.md" <<'CARD'
---
id: AUR-999
version: 1
title: Sealed Go regression fixture
status: ready
validation: tested
paths: [tests/acceptance/AUR-999.sh, fixture/value.go, fixture/value_test.go]
read_paths: [go.mod, go.sum]
forbidden_paths: [.git, .env, credentials, secrets]
---

## Acceptance
container_profile: `go-unit-offline-v1`
accept: `./.board/bin/oci-run --profile go-unit-offline-v1 --card AUR-999`

## Skeptical mutation
MUT-001 changes Value to return the wrong number.
CARD
cp "$root/go.mod" "$fixture/go.mod"
cp "$root/go.sum" "$fixture/go.sum"
cat > "$fixture/fixture/value.go" <<'GO'
package fixture

func Value() int { return 7 }
GO
cat > "$fixture/fixture/value_test.go" <<'GO'
package fixture

import (
    "testing"
    "gopkg.in/yaml.v3"
)

func TestValue(t *testing.T) {
    if got := Value(); got != 7 { t.Fatalf("Value() = %d, want 7", got) }
    var decoded struct { Value int `yaml:"value"` }
    if err := yaml.Unmarshal([]byte("value: 7"), &decoded); err != nil || decoded.Value != 7 {
        t.Fatalf("cached yaml.v3 dependency failed offline: %v, value=%d", err, decoded.Value)
    }
}
GO
cat > "$fixture/tests/acceptance/AUR-999.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[[ "$GOPROXY" == off && "$GOSUMDB" == off && "$GOTOOLCHAIN" == local ]]
[[ "$GOCACHE" == /tmp/* && "$GOTMPDIR" == /tmp && -w /tmp ]]
[[ ! -w /workspace ]]
printf 'declared Go parallelism: %s %s\n' "$GOMAXPROCS" "$GOFLAGS"
test_status=0
result="$(go test -mod=readonly -count=1 -v ./fixture 2>&1)" || test_status=$?
printf '%s\n' "$result"
(( test_status == 0 )) || exit "$test_status"
[[ "$result" == *'--- PASS: TestValue'* ]]
SH
chmod 0755 "$fixture/tests/acceptance/AUR-999.sh" "$fixture/.board/bin/oci-run" "$fixture/.board/card-preflight.sh"
git -C "$fixture" init -q
fixture_commit fixture-green

run_preflight >/dev/null || fail 'canonical Go profile failed preflight'
green_output="$(run_runner 2>&1)" || { printf '%s\n' "$green_output" >&2; fail 'real Go assertion did not pass in OCI'; }
[[ "$green_output" == *'--- PASS: TestValue'* ]] || fail 'runner was green without executing TestValue'
[[ "$green_output" == *'declared Go parallelism: 2 -buildvcs=false -p=2'* ]] || fail 'Go parallelism did not follow the declared CPU'

sed -i 's/"cpu_millis": 2000/"cpu_millis": 1000/' "$fixture/.board/oci/profiles/go-unit-offline-v1.json"
fixture_commit lower-declared-cpu
cpu_output="$(run_runner 2>&1)" || { printf '%s\n' "$cpu_output" >&2; fail 'lower CPU profile failed Go assertion'; }
[[ "$cpu_output" == *'declared Go parallelism: 1 -buildvcs=false -p=1'* && "$cpu_output" == *'--- PASS: TestValue'* ]] || fail 'runtime retained stale Go parallelism'

sed -i 's/return 7/return 8/' "$fixture/fixture/value.go"
fixture_commit fixture-red
red_rc=0
red_output="$(run_runner 2>&1)" || red_rc=$?
[[ "$red_rc" == 1 && "$red_output" == *'--- FAIL: TestValue'* ]] || {
  printf '%s\n' "$red_output" >&2
  fail "mutated Go assertion did not fail through the same runner (exit $red_rc)"
}

printf '\n' >> "$fixture/.board/locks/oci/go-unit-offline-v1.lock.json"
fixture_commit divergent-lock
lock_rc=0
lock_output="$(run_preflight 2>&1)" || lock_rc=$?
[[ "$lock_rc" == 1 && "$lock_output" == *'lock_digest does not match lock bytes'* ]] || fail 'preflight accepted divergent lock'
lock_rc=0
lock_output="$(run_runner 2>&1)" || lock_rc=$?
[[ "$lock_rc" == 65 && "$lock_output" == *'different lock content digest'* ]] || fail 'runner accepted divergent lock'

cp "$root/.board/locks/oci/go-unit-offline-v1.lock.json" "$fixture/.board/locks/oci/go-unit-offline-v1.lock.json"
sed -i 's/"privileged": false/"privileged": true/' "$fixture/.board/oci/profiles/go-unit-offline-v1.json"
fixture_commit forbidden-privilege
priv_rc=0
priv_output="$(run_preflight 2>&1)" || priv_rc=$?
[[ "$priv_rc" == 1 && "$priv_output" == *'violates runner hardening'* ]] || fail 'preflight accepted privileged profile'
priv_rc=0
priv_output="$(run_runner 2>&1)" || priv_rc=$?
[[ "$priv_rc" == 65 && "$priv_output" == *'must declare privileged as false'* ]] || fail 'runner accepted privileged profile'

cp "$root/.board/oci/profiles/go-unit-offline-v1.json" "$fixture/.board/oci/profiles/go-unit-offline-v1.json"
bash_image='bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831'
sed -i "s|^\"image\":.*|\"image\": \"$bash_image\"|" "$fixture/.board/locks/oci/go-unit-offline-v1.lock.json"
new_digest="sha256:$(sha256sum "$fixture/.board/locks/oci/go-unit-offline-v1.lock.json" | awk '{print $1}')"
sed -i "s|^\"lock_digest\":.*|\"lock_digest\": \"$new_digest\",|" "$fixture/.board/oci/profiles/go-unit-offline-v1.json"
fixture_commit bash-only-runtime
runtime_rc=0
runtime_output="$(run_preflight 2>&1)" || runtime_rc=$?
[[ "$runtime_rc" == 1 && "$runtime_output" == *'lacks required runtime (bash,go)'* ]] || fail 'preflight accepted Bash-only runtime for Go'
runtime_rc=0
runtime_output="$(run_runner 2>&1)" || runtime_rc=$?
[[ "$runtime_rc" == 65 && "$runtime_output" == *'lacks required acceptance runtime: bash,go'* ]] || fail 'runner accepted Bash-only runtime for Go'

printf 'go-profile regression: real Go PASS, mutated Go RED, lock/privilege/runtime rejected\n'
