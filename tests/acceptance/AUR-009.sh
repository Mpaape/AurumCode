#!/usr/bin/env bash
# AUR-009 / AC-001 -- prove that the single redaction filter is applied to
# stdout, stderr, log, finding, diff, cache and evidence, so canaries in URL,
# header, body, diff, error and tool result never appear outside an
# authorized sink. This program emits observations only; it never writes
# evidence, issues a verdict or asserts approval.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-009'
readonly scenario='AC-001'
selector="${1:-AC-001}"

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

infra() {
  printf '%s/%s/inconclusive/%s\n' "$card" "$scenario" "$1" >&2
  exit 79
}

case "$selector" in
  AC-001|TestAUR009|IntegrationAUR009|E2EAUR009) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

for tool in awk cp go grep head mkdir mktemp sed sha256sum sort tr wc find; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root

readonly filter_rel='internal/security/redaction/redaction.go'
readonly suite_rel='tests/security/redactionsuite/harness.go'
readonly cases_rel='tests/specs/AUR-009/cases.yaml'
readonly unit_rel='tests/unit/AUR-009.go'
readonly integration_rel='tests/integration/AUR-009.go'
readonly e2e_rel='tests/e2e/AUR-009.sh'
readonly docs_rel='docs/specs/AUR-009.md'

for path in "$filter_rel" "$suite_rel" "$cases_rel" "$unit_rel" \
  "$integration_rel" "$e2e_rel" "$docs_rel" go.mod go.sum; do
  [[ -f "$repo_root/$path" && ! -L "$repo_root/$path" && -r "$repo_root/$path" ]] || fail "entrypoint_missing:$path"
done

# The documented example is the same sealed vector the sweep executes, so the
# doc cannot drift from the behavior without failing this acceptance.
grep -Fq 'GET https://api.example.test/v1/repos?[REDACTED] HTTP/1.1' "$repo_root/$docs_rel" || fail docs_example_missing
grep -Fq 'Authorization: [REDACTED]' "$repo_root/$docs_rel" || fail docs_example_missing

vector_count=0
deadline_seconds=30

# Bash-side schema gate: the same bounds the Go parser enforces (64 vectors,
# 4 MiB expanded bytes, deadline at most 30 s, all three vector kinds).
validate_cases() {
  local f="$repo_root/$cases_rel"
  local bytes count total
  bytes="$(wc -c <"$f")" || fail cases_unreadable
  [[ "$bytes" =~ ^[0-9]+$ ]] || fail cases_unreadable
  (( bytes > 0 && bytes <= 1048576 )) || fail cases_file_size
  head -n 1 "$f" | grep -qx 'schema: aurum.redaction-cases' || fail cases_schema
  sed -n '2p' "$f" | grep -qx 'version: 1' || fail cases_version
  grep -qx '  max_vectors: 64' "$f" || fail cases_limit_vectors
  grep -qx '  max_total_bytes: 4194304' "$f" || fail cases_limit_bytes
  grep -Eqx '  deadline_seconds: ([1-9]|[12][0-9]|30)' "$f" || fail cases_limit_deadline
  deadline_seconds="$(sed -n 's/^  deadline_seconds: //p' "$f")"
  [[ "$deadline_seconds" =~ ^[0-9]+$ ]] || fail cases_limit_deadline
  count="$(grep -c '^  - name: ' "$f" || true)"
  [[ "$count" =~ ^[0-9]+$ ]] || fail cases_vector_count
  (( count >= 3 && count <= 64 )) || fail cases_vector_count
  grep -qx '    kind: nominal' "$f" || fail cases_missing_nominal
  grep -qx '    kind: invalid' "$f" || fail cases_missing_invalid
  grep -qx '    kind: boundary' "$f" || fail cases_missing_boundary
  total="$(awk '
    /^    input: "/ { line = $0; sub(/^    input: "/, "", line); sub(/"$/, "", line); total += length(line) }
    /^    repeat_count: / { line = $0; sub(/^    repeat_count: /, "", line); total += line + 0 }
    END { print total + 0 }
  ' "$f")" || fail cases_total_bytes
  [[ "$total" =~ ^[0-9]+$ ]] || fail cases_total_bytes
  (( total <= 4194304 )) || fail cases_total_bytes
  vector_count="$count"
}
validate_cases

# One canary value per top-level run; the e2e child inherits it so every
# layer proves absence of the same secret.
if [[ -z "${AURUM_SECRET_CANARY:-}" ]]; then
  export AURUM_SECRET_CANARY="aur009-canary-$$-${RANDOM}${RANDOM}${RANDOM}"
fi

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a009.XXXXXX")" || infra mktemp
cleanup() { rm -rf -- "$run_dir" 2>/dev/null || true; }
trap cleanup EXIT INT TERM HUP

if [[ -n "${AURUM_A009_GOCACHE:-}" && -d "${AURUM_A009_GOCACHE:-}" && -w "${AURUM_A009_GOCACHE:-}" ]]; then
  gocache="$AURUM_A009_GOCACHE"
else
  gocache="$run_dir/go-cache"
  mkdir -p "$gocache" || infra cache_setup
  export AURUM_A009_GOCACHE="$gocache"
fi
mkdir -p "$run_dir/go-tmp" "$run_dir/home" "$run_dir/gopath" || infra cache_setup

contains_canary() {
  local file="$1"
  if grep -Fq -- "$AURUM_SECRET_CANARY" "$file"; then
    fail secret_canary_exposed
  fi
}

prepare_module() {
  local root="$1"
  mkdir -p "$root/internal/security/redaction" "$root/tests/security/redactionsuite" \
    "$root/tests/specs/AUR-009" || infra module_setup
  cp "$repo_root/go.mod" "$root/go.mod" || infra module_setup
  cp "$repo_root/go.sum" "$root/go.sum" || infra module_setup
  cp "$repo_root/$filter_rel" "$root/$filter_rel" || infra module_setup
  cp "$repo_root/$suite_rel" "$root/$suite_rel" || infra module_setup
  cp "$repo_root/$cases_rel" "$root/$cases_rel" || infra module_setup
}

run_go_layer() {
  local name="$1" layer_rel="$2" function_name="$3" package_dir="$4" package_name="$5"
  local root="$run_dir/$name" log_file="$run_dir/$name.log" rc bytes
  prepare_module "$root"
  mkdir -p "$root/$package_dir" || infra layer_setup
  cp "$repo_root/$layer_rel" "$root/$layer_rel" || infra layer_setup
  printf 'package %s\nimport "testing"\nfunc TestAUR009Bridge(t *testing.T) { %s(t) }\n' \
    "$package_name" "$function_name" >"$root/$package_dir/aur009_bridge_test.go" || infra layer_setup
  set +e
  (
    cd -- "$root" &&
      HOME="$run_dir/home" GOPATH="$run_dir/gopath" GOCACHE="$gocache" \
      GOTMPDIR="$run_dir/go-tmp" GOFLAGS='-mod=readonly' GOPROXY=off GOSUMDB=off \
      GOTOOLCHAIN=local GOMAXPROCS=1 \
      AURUM_A009_CASES="$root/$cases_rel" \
      go test -tags aur009_redaction -p 1 -count=1 -v \
        -timeout "${deadline_seconds}s" -run '^TestAUR009Bridge$' "./$package_dir"
  ) >"$log_file" 2>&1
  rc=$?
  set -e
  contains_canary "$log_file"
  bytes="$(wc -c <"$log_file")" || infra layer_output
  [[ "$bytes" =~ ^[0-9]+$ ]] || infra layer_output
  (( bytes > 0 && bytes <= 32768 )) || fail "${name}_output_unbounded"
  layer_rc="$rc"
}

classify_integration_failure() {
  local log_file="$1" leaks mismatches
  if ! grep -Eq '^AUR009_(LEAK|MISMATCH)\b' "$log_file"; then
    # The sweep never reached the behavior (compile, loader or harness
    # failure). That is not a valid behavioral RED and not a mutation catch.
    sed -n '1,60p' "$log_file" >&2
    fail "integration_exit:$layer_rc"
  fi
  leaks="$(awk -F '\t' '$1 == "AUR009_LEAK" { print $2 }' "$log_file" | sort -u | tr '\n' ' ')"
  mismatches="$(awk -F '\t' '$1 == "AUR009_MISMATCH" { print $2 }' "$log_file" | sort -u | tr '\n' ' ')"
  if [[ "$leaks" == 'error ' && ( -z "$mismatches" || "$mismatches" == 'error ' ) ]]; then
    fail MUT-002
  fi
  if [[ -z "$leaks" && "$mismatches" == 'url ' ]]; then
    fail MUT-001
  fi
  fail behavior-missing
}

run_integration() {
  local log_file="$run_dir/integration.log" observed observed_leaks
  run_go_layer integration "$integration_rel" IntegrationAUR009 tests/integration integration
  if [[ "$layer_rc" -ne 0 ]]; then
    classify_integration_failure "$log_file"
  fi
  observed="$(grep -c '^AUR009_OBSERVATION' "$log_file" || true)"
  [[ "$observed" =~ ^[0-9]+$ ]] || fail observation_count
  (( observed == vector_count )) || fail observation_count
  awk -F '\t' '$1 == "AUR009_DIGEST" { found = 1 } END { exit found ? 0 : 1 }' "$log_file" || fail sweep_digest_missing
  awk -F '\t' '$1 == "AUR009_OBSERVATION" && NF == 6 { print $2 "\t" $3 "\t" $4 "\t" $5 "\t" $6 }' \
    "$log_file" >"$run_dir/observations.tsv"
  [[ "$(wc -l <"$run_dir/observations.tsv")" == "$vector_count" ]] || fail observation_count
  awk -F '\t' '$1 == "AUR009_DIGEST" { print $2 }' "$log_file" | head -n 1 >"$run_dir/sweep.digest"
  # Observed leak count, not an assumption: the sweep reports one AUR009_LEAK
  # line per vector whose sink output still carried the canary.
  observed_leaks="$(grep -c '^AUR009_LEAK' "$log_file" || true)"
  [[ "$observed_leaks" =~ ^[0-9]+$ ]] || fail leak_count
  printf '%s\n' "$observed_leaks" >"$run_dir/leaks.count"
  printf '%s/IntegrationAUR009: pass\n' "$card"
}

run_unit() {
  run_go_layer unit "$unit_rel" TestAUR009 tests/unit unit
  if [[ "$layer_rc" -ne 0 ]]; then
    sed -n '1,60p' "$run_dir/unit.log" >&2
    fail "unit_exit:$layer_rc"
  fi
  printf '%s/TestAUR009: pass\n' "$card"
}

run_e2e() {
  bash "$repo_root/$e2e_rel" E2EAUR009
}

tree_digest() {
  local target
  (
    cd -- "$repo_root" || exit 1
    for target in "$@"; do
      if [[ -d "$target" ]]; then
        find "$target" -type f -print
      else
        printf '%s\n' "$target"
      fi
    done | LC_ALL=C sort -u | while IFS= read -r target; do
      digest_line="$(sha256sum -- "$target")" || exit 1
      printf '%s\t%s\n' "${digest_line%% *}" "$target"
    done
  ) | sha256sum | awk '{print $1}'
}

# measure_leaks sums every observed appearance of the canary outside an
# authorized sink: the sweep's own leak observations plus any line of a
# captured layer log carrying the canary. It reports a count; it never assumes
# one.
measure_leaks() {
  local total log extra
  total="$(head -n 1 "$run_dir/leaks.count")" || return 1
  [[ "$total" =~ ^[0-9]+$ ]] || return 1
  for log in "$run_dir"/*.log; do
    [[ -f "$log" ]] || continue
    extra="$(grep -Fc -- "$AURUM_SECRET_CANARY" "$log" || true)"
    [[ "$extra" =~ ^[0-9]+$ ]] || return 1
    total=$(( total + extra ))
  done
  printf '%s\n' "$total"
}

emit_observation() {
  local filter_digest tests_digest materialized_digest sweep_digest vectors
  local leak_total outside
  leak_total="$(measure_leaks)" || fail leak_count
  if (( leak_total > 0 )); then
    outside=true
  else
    outside=false
  fi
  [[ "$outside" == false ]] || fail secret_outside_authorized_sink
  filter_digest="$(tree_digest internal/security/redaction)" || infra filter_digest
  tests_digest="$(tree_digest tests/security/redactionsuite "$cases_rel" "$unit_rel" \
    "$integration_rel" "$e2e_rel" tests/acceptance/AUR-009.sh)" || infra tests_digest
  materialized_digest="$(tree_digest internal/security/redaction tests/security/redactionsuite \
    "$cases_rel" "$unit_rel" "$integration_rel" "$e2e_rel" tests/acceptance/AUR-009.sh \
    "$docs_rel" go.mod go.sum)" || infra materialized_digest
  sweep_digest="$(head -n 1 "$run_dir/sweep.digest")" || infra sweep_digest
  [[ "$sweep_digest" =~ ^[0-9a-f]{64}$ ]] || fail sweep_digest_missing
  vectors="$(awk -F '\t' '
    BEGIN { printf "[" }
    NF == 5 {
      if (count++) printf ","
      printf "{\"name\":\"%s\",\"kind\":\"%s\",\"result\":\"%s\",\"effects\":%d,\"artifact_sha256\":\"sha256:%s\"}", $1, $2, $3, $4 + 0, $5
    }
    END { printf "]" }
  ' "$run_dir/observations.tsv")" || infra vectors_json
  printf '{"schema":"aurum.redaction-acceptance","version":1,"card":"%s","scenario":"%s","candidate_identity_v1":{"materialized_tree_sha256":"sha256:%s","filter_tree_sha256":"sha256:%s","tests_tree_sha256":"sha256:%s"},"sweep_sha256":"sha256:%s","vector_count":%d,"vectors":%s,"observed_leak_count":%d,"secret_outside_authorized_sink":%s,"result":"pass"}\n' \
    "$card" "$scenario" "$materialized_digest" "$filter_digest" "$tests_digest" \
    "$sweep_digest" "$vector_count" "$vectors" "$leak_total" "$outside"
}

case "$selector" in
  TestAUR009) run_unit ;;
  IntegrationAUR009) run_integration ;;
  E2EAUR009) run_e2e ;;
  AC-001)
    run_integration
    run_unit
    run_e2e
    emit_observation
    ;;
esac
