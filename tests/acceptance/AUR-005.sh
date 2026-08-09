#!/usr/bin/env bash
# AUR-005 / AC-001 -- execute the content-addressed evidence manifest contract
# and both card-declared Go selectors in the pinned Go+Bash OCI profile.
set -Eeuo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-005'
readonly scenario='AC-001'
readonly max_vectors=64
readonly max_bytes=$((4 * 1024 * 1024))
readonly max_deadline=30

selector="${1:-AC-001}"
case "$selector" in
  AC-001|TestAUR005|IntegrationAUR005) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

infra() {
  printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2
  exit 69
}

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root_unresolved
cd -- "$repo_root" || infra repo_root_unreachable

readonly schema='.board/schemas/evidence-bundle.schema.json'
readonly implementation='internal/evidence/manifest.go'
readonly package_test='internal/evidence/manifest_test.go'
readonly vectors='tests/specs/AUR-005/cases.yaml'
readonly unit_selector='tests/unit/AUR-005.go'
readonly integration_selector='tests/integration/AUR-005.go'
readonly documentation='docs/specs/AUR-005.md'

for path in "$schema" "$implementation" "$package_test" "$vectors" "$unit_selector" "$integration_selector" "$documentation" go.mod go.sum; do
  [[ -f "$path" && ! -L "$path" && -r "$path" ]] || fail "declared-path-absent:$path"
done
[[ -x tests/acceptance/AUR-005.sh ]] || fail acceptance-not-executable

for tool in cut go grep mktemp rm sha256sum wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing-tool:$tool"
done

vector_bytes="$(wc -c <"$vectors")" || infra vector_size_unavailable
(( vector_bytes <= max_bytes )) || fail vectors-limit-exceeded
vector_count="$(grep -c '^      "id": ' "$vectors")" || infra vector_count_unavailable
(( vector_count > 0 && vector_count <= max_vectors )) || fail vectors-count-invalid
grep -Fqx '    "deadline_seconds": 30' "$vectors" || fail vectors-deadline-invalid
grep -Fq '"id": "nominal"' "$vectors" || fail vectors-nominal-absent
grep -Fq '"id": "invalid"' "$vectors" || fail vectors-invalid-absent
grep -Fq '"id": "boundary"' "$vectors" || fail vectors-boundary-absent
grep -Fq '"id": "boundary-overflow"' "$vectors" || fail vectors-boundary-overflow-absent

run_root="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a005-go.XXXXXX")" || infra temporary_directory_unavailable
output="$run_root/go-test.out"
mkdir -p -- "$run_root/cache" "$run_root/tmp" || infra go_scratch_unavailable
cleanup() {
  rm -rf -- "$run_root"
}
trap cleanup EXIT INT TERM HUP

classify_failure() {
  if grep -Fq 'AUR-005/AC-001/behavior-missing' "$output"; then
    fail behavior-missing
  fi
  if grep -Fq 'manifest_digest: digest_mismatch' "$output"; then
    fail MUT-001
  fi
  if grep -Fq 'authority: authority_denied' "$output"; then
    fail MUT-002
  fi
  if grep -Eiq '(^|[[:space:]])(build failed|setup failed|undefined:|syntax error|cannot find package|no required module provides|module lookup disabled|missing go\.sum)|creating work dir' "$output"; then
    sed -n '1,80p' "$output" >&2
    infra go_loader_or_compilation_failed
  fi
  sed -n '1,80p' "$output" >&2
  fail assertion-failed
}

run_test() {
  local go_selector="$1"
  local assertion_marker="$2"
  : >"$output"
  set +e
  AURUMCODE_ROOT="$repo_root" GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off \
    GOCACHE="$run_root/cache" GOTMPDIR="$run_root/tmp" \
    go test -vet=off ./internal/evidence -run "^${go_selector}$" -count=1 -v >"$output" 2>&1
  local test_exit=$?
  set -e
  (( test_exit == 0 )) || classify_failure
  grep -Fqx "=== RUN   ${go_selector}" "$output" || fail "selector-not-executed:${go_selector}"
  grep -Eq "^--- PASS: ${go_selector} \(" "$output" || fail "selector-not-asserted:${go_selector}"
  grep -Fq -e "$assertion_marker" "$output" || fail "assertion-marker-absent:${go_selector}"
  if grep -Eiq '\[no test files\]|no tests to run|warning: no tests' "$output"; then
    fail "empty-selector:${go_selector}"
  fi
}

assertions=0
case "$selector" in
  AC-001)
    run_test TestManifestCanonicalReplayAndTamperRejection '--- PASS: TestManifestCanonicalReplayAndTamperRejection'
    ((assertions += 1))
    run_test TestAUR005 'assertion executed: tests/unit/AUR-005.go::TestAUR005'
    ((assertions += 1))
    run_test TestIntegrationAUR005 'assertion executed: tests/integration/AUR-005.go::IntegrationAUR005'
    ((assertions += 1))
    ;;
  TestAUR005)
    run_test TestAUR005 'assertion executed: tests/unit/AUR-005.go::TestAUR005'
    ((assertions += 1))
    ;;
  IntegrationAUR005)
    run_test TestIntegrationAUR005 'assertion executed: tests/integration/AUR-005.go::IntegrationAUR005'
    ((assertions += 1))
    ;;
esac

schema_digest="sha256:$(sha256sum -- "$schema" | cut -d' ' -f1)" || infra schema_digest_unavailable
vectors_digest="sha256:$(sha256sum -- "$vectors" | cut -d' ' -f1)" || infra vectors_digest_unavailable
printf '{"card":"%s","scenario":"%s","selector":"%s","assertions":%d,"vectors":%d,"max_vectors":%d,"max_bytes":%d,"deadline_seconds":%d,"schema_digest":"%s","vectors_digest":"%s","effects":0,"result":"pass"}\n' \
  "$card" "$scenario" "$selector" "$assertions" "$vector_count" "$max_vectors" "$max_bytes" "$max_deadline" "$schema_digest" "$vectors_digest"
