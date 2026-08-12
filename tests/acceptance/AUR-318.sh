#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-318'
readonly scenario='AC-001'
selector="${1:-AC-001}"

fail() {
  printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2
  exit 1
}

red_absent() {
  printf '%s/%s: Congelar taskmaster como histórico ausente\n' "$card" "$scenario" >&2
  exit 1
}

infra() {
  printf '%s/%s/inconclusive/%s\n' "$card" "$scenario" "$1" >&2
  exit 79
}

case "$selector" in
  AC-001|TestAUR318|ContractAUR318|IntegrationAUR318|E2EAUR318) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

for tool in awk chmod cmp cp find go grep mktemp mv sed sha256sum sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root

readonly fixture_rel='tests/characterization/legacy/taskmaster'
readonly snapshot_rel="$fixture_rel/snapshot"
readonly expected_rel="$fixture_rel/expected.tsv"
readonly unit_rel='tests/unit/AUR-318.go'
readonly contract_rel='tests/contracts/AUR-318.go'
readonly integration_rel='tests/integration/AUR-318.go'
readonly e2e_rel='tests/e2e/AUR-318.sh'

for path in go.mod go.sum "$fixture_rel/fixture.go" "$expected_rel" \
  "$snapshot_rel/tasks/tasks.json" "$snapshot_rel/CLAUDE.md" \
  '.taskmaster/tasks/tasks.json' '.taskmaster/CLAUDE.md' \
  "$unit_rel" "$contract_rel" "$integration_rel" "$e2e_rel"; do
  [[ -f "$repo_root/$path" && ! -L "$repo_root/$path" && -r "$repo_root/$path" ]] || fail "entrypoint_missing:$path"
done
[[ -x "$repo_root/$e2e_rel" ]] || fail 'e2e_not_executable'

# The audit copy is bounded and must remain byte-identical to the live frozen
# tree: same relative file set, same bytes, before and after every layer runs.
verify_taskmaster_frozen() {
  local phase="$1" live="$repo_root/.taskmaster" snap="$repo_root/$snapshot_rel"
  local live_list snap_list rel size count=0 total=0
  live_list="$(cd -- "$live" && find . -type f | LC_ALL=C sort)" || infra taskmaster_list
  snap_list="$(cd -- "$snap" && find . -type f | LC_ALL=C sort)" || infra snapshot_list
  [[ -n "$snap_list" ]] || fail "snapshot_empty:$phase"
  [[ "$live_list" == "$snap_list" ]] || fail "taskmaster_bytes_changed:$phase"
  while IFS= read -r rel; do
    rel="${rel#./}"
    cmp -s -- "$live/$rel" "$snap/$rel" || fail "taskmaster_bytes_changed:$phase"
    count=$((count + 1))
    size="$(wc -c <"$snap/$rel")" || infra snapshot_size
    total=$((total + size))
  done <<<"$snap_list"
  (( count >= 1 && count <= 64 )) || fail "snapshot_unbounded:files:$count"
  (( total >= 1 && total <= 1048576 )) || fail "snapshot_unbounded:bytes:$total"
}

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a318.XXXXXX")" || infra mktemp
cleanup() {
  chmod -R u+w -- "$run_dir" 2>/dev/null || true
  rm -rf -- "$run_dir" 2>/dev/null || true
}
trap cleanup EXIT INT TERM HUP
export AUR318_GOCACHE="${AUR318_GOCACHE:-$run_dir/go-cache}"
mkdir -p "$AUR318_GOCACHE" "$run_dir/go-tmp" || infra cache_setup

contains_canary() {
  local file="$1" canary="${AURUM_SECRET_CANARY:-}"
  [[ -z "$canary" ]] || ! grep -Fq -- "$canary" "$file" || fail secret_canary_exposed
}

prepare_layer() {
  local root="$1" layer_source="$2" package_dir="$3" package_name="$4" function_name="$5"
  mkdir -p "$root/$fixture_rel" "$root/$package_dir" || infra layer_setup
  cp -- "$repo_root/go.mod" "$root/go.mod" || infra layer_setup
  cp -- "$repo_root/go.sum" "$root/go.sum" || infra layer_setup
  cp -- "$repo_root/$fixture_rel/fixture.go" "$root/$fixture_rel/fixture.go" || infra layer_setup
  cp -- "$repo_root/$expected_rel" "$root/$expected_rel" || infra layer_setup
  cp -R -- "$repo_root/$snapshot_rel" "$root/$fixture_rel/snapshot" || infra layer_setup
  chmod -R u+w -- "$root" || infra layer_setup
  cp -- "$repo_root/$layer_source" "$root/$layer_source" || infra layer_setup
  printf 'package %s\nimport "testing"\nfunc TestAUR318Bridge(t *testing.T) { %s(t) }\n' "$package_name" "$function_name" \
    >"$root/$package_dir/aur318_bridge_test.go" || infra layer_setup
}

run_layer() {
  local name="$1" source="$2" package_dir="$3" package_name="$4" function_name="$5"
  local root="$run_dir/$name" log_file="$run_dir/$name.log" bytes rc
  prepare_layer "$root" "$source" "$package_dir" "$package_name" "$function_name"
  set +e
  (
    cd -- "$root" &&
      GOFLAGS='-mod=mod' GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=1 \
      GOCACHE="$AUR318_GOCACHE" GOTMPDIR="$run_dir/go-tmp" \
      AUR318_FIXTURE_DIR="$root/$fixture_rel" \
      AUR318_TASKMASTER_DIR="$repo_root/.taskmaster" \
      go test -tags aur318_characterization -p 1 "./$package_dir" -run '^TestAUR318Bridge$' -count=1 -v
  ) >"$log_file" 2>&1
  rc=$?
  set -e
  contains_canary "$log_file"
  bytes="$(wc -c <"$log_file")" || infra layer_output
  [[ "$bytes" =~ ^[0-9]+$ ]] || infra layer_output
  if (( bytes == 0 || bytes > 32768 )); then
    fail "${name}_output_unbounded"
  fi
  if [[ "$rc" -ne 0 ]]; then
    sed -n '1,80p' "$log_file" >&2
    fail "${name}_exit:$rc"
  fi
}

extract_report() {
  local log_file="$run_dir/integration.log" actual="$run_dir/actual.tsv"
  local queue_actual promote_actual bytes_actual
  printf 'observation\tvalue\teffect\n' >"$actual"
  awk -F '\t' '$1 == "AUR318_OBSERVATION" && NF == 4 { print $2 "\t" $3 "\t" $4 }' "$log_file" \
    | LC_ALL=C sort -t $'\t' -k1,1 >>"$actual"
  [[ "$(wc -l <"$actual")" == 9 ]] || fail report_row_count
  if ! cmp -s "$repo_root/$expected_rel" "$actual"; then
    queue_actual="$(awk -F '\t' '$1 == "queue_taskmaster_items" { print $2 }' "$actual")"
    promote_actual="$(awk -F '\t' '$1 == "promote_taskmaster_done" { print $2 }' "$actual")"
    bytes_actual="$(awk -F '\t' '$1 == "taskmaster_bytes" { print $2 }' "$actual")"
    [[ -n "$queue_actual" && -n "$promote_actual" && -n "$bytes_actual" ]] || fail characterization_mismatch
    if [[ "$queue_actual" != 0 ]]; then
      red_absent
    fi
    if [[ "$promote_actual" != 'exit:1' ]]; then
      fail MUT-001
    fi
    [[ "$bytes_actual" == 'identical' ]] || fail 'taskmaster_bytes_changed:observed'
    fail characterization_mismatch
  fi
}

run_unit() {
  run_layer unit "$unit_rel" tests/unit unit TestAUR318
  printf '%s/TestAUR318: pass\n' "$card"
}

run_contract() {
  run_layer contract "$contract_rel" tests/contracts contracts ContractAUR318
  printf '%s/ContractAUR318: pass\n' "$card"
}

run_integration() {
  run_layer integration "$integration_rel" tests/integration integration IntegrationAUR318
  extract_report
  printf '%s/IntegrationAUR318: pass\n' "$card"
}

run_e2e() {
  bash "$repo_root/$e2e_rel" E2EAUR318
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

emit_observation() {
  local materialized_digest taskmaster_digest snapshot_digest tests_digest report_digest observations
  taskmaster_digest="$(tree_digest .taskmaster)" || infra taskmaster_digest
  snapshot_digest="$(tree_digest "$snapshot_rel")" || infra snapshot_digest
  tests_digest="$(tree_digest "$fixture_rel" "$unit_rel" "$contract_rel" "$integration_rel" "$e2e_rel" tests/acceptance/AUR-318.sh)" || infra tests_digest
  materialized_digest="$(tree_digest .taskmaster go.mod go.sum "$fixture_rel" "$unit_rel" "$contract_rel" "$integration_rel" "$e2e_rel" tests/acceptance/AUR-318.sh docs/specs/AUR-318.md)" || infra materialized_digest
  report_digest="$(sha256sum -- "$run_dir/actual.tsv")" || infra report_digest
  report_digest="${report_digest%% *}"
  observations="$(awk -F '\t' '
    NR == 1 { next }
    BEGIN { printf "[" }
    { if (count++) printf ","; printf "{\"observation\":\"%s\",\"value\":\"%s\",\"effect\":\"%s\"}", $1, $2, $3 }
    END { printf "]" }
  ' "$run_dir/actual.tsv")" || infra report_json
  printf '{"schema":"aurum.taskmaster-freeze-characterization","version":1,"card":"%s","scenario":"%s","candidate_identity_v1":{"materialized_tree_sha256":"sha256:%s","taskmaster_tree_sha256":"sha256:%s","snapshot_tree_sha256":"sha256:%s","tests_tree_sha256":"sha256:%s"},"report_sha256":"sha256:%s","observations":%s,"taskmaster_selection_source":false,"taskmaster_bytes_identical":true,"result":"pass"}\n' \
    "$card" "$scenario" "$materialized_digest" "$taskmaster_digest" "$snapshot_digest" "$tests_digest" "$report_digest" "$observations"
}

case "$selector" in
  TestAUR318) run_unit ;;
  ContractAUR318) run_contract ;;
  IntegrationAUR318)
    verify_taskmaster_frozen before
    run_integration
    verify_taskmaster_frozen after
    ;;
  E2EAUR318) run_e2e ;;
  AC-001)
    verify_taskmaster_frozen before
    run_integration
    run_unit
    run_contract
    run_e2e
    verify_taskmaster_frozen after
    emit_observation
    ;;
esac
