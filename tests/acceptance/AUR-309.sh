#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-309'
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
  AC-001|TestAUR309|ContractAUR309|IntegrationAUR309|E2EAUR309) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

for tool in awk cmp cp find go grep mktemp mv sed sha256sum sort wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *) script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root

readonly fixture_rel='tests/characterization/legacy/pipeline'
readonly expected_rel="$fixture_rel/expected.tsv"
readonly source_rel='internal/pipeline/extractor_pipeline.go'
readonly unit_rel='tests/unit/AUR-309.go'
readonly contract_rel='tests/contracts/AUR-309.go'
readonly integration_rel='tests/integration/AUR-309.go'
readonly e2e_rel='tests/e2e/AUR-309.sh'

for path in "$source_rel" go.mod go.sum "$fixture_rel/fixture.go" "$expected_rel" \
  "$fixture_rel/stubs/documentation/extractors/extractors.go" \
  "$fixture_rel/stubs/documentation/incremental/incremental.go" \
  "$fixture_rel/stubs/documentation/normalizer/normalizer.go" \
  "$fixture_rel/stubs/documentation/site/site.go" \
  "$fixture_rel/stubs/documentation/welcome/welcome.go" \
  "$fixture_rel/stubs/llm/llm.go" "$fixture_rel/stubs/yaml/yaml.go" \
  "$fixture_rel/stubs/yaml/modules.txt" "$unit_rel" "$contract_rel" \
  "$integration_rel" "$e2e_rel"; do
  [[ -f "$repo_root/$path" && ! -L "$repo_root/$path" && -r "$repo_root/$path" ]] || fail "entrypoint_missing:$path"
done
[[ -x "$repo_root/$e2e_rel" ]] || fail 'e2e_not_executable'

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a309.XXXXXX")" || infra mktemp
cleanup() { rm -rf -- "$run_dir"; }
trap cleanup EXIT INT TERM HUP
mkdir -p "$run_dir/go-cache" "$run_dir/go-tmp" || infra cache_setup

contains_canary() {
  local file="$1" canary="${AURUM_SECRET_CANARY:-}"
  [[ -z "$canary" ]] || ! grep -Fq -- "$canary" "$file" || fail secret_canary_exposed
}

prepare_layer() {
  local root="$1" layer_source="$2" function_name="$3" package_dir="$4" package_name="$5"
  mkdir -p "$root/internal/pipeline" "$root/internal/documentation/extractors" \
    "$root/internal/documentation/incremental" "$root/internal/documentation/normalizer" \
    "$root/internal/documentation/site" "$root/internal/documentation/welcome" "$root/internal/llm" \
    "$root/vendor/gopkg.in/yaml.v3" "$root/$fixture_rel" "$root/$package_dir" || infra layer_setup
  cp "$repo_root/go.mod" "$root/go.mod" || infra layer_setup
  cp "$repo_root/go.sum" "$root/go.sum" || infra layer_setup
  cp "$repo_root/$source_rel" "$root/internal/pipeline/extractor_pipeline.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/documentation/extractors/extractors.go" "$root/internal/documentation/extractors/extractors.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/documentation/incremental/incremental.go" "$root/internal/documentation/incremental/incremental.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/documentation/normalizer/normalizer.go" "$root/internal/documentation/normalizer/normalizer.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/documentation/site/site.go" "$root/internal/documentation/site/site.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/documentation/welcome/welcome.go" "$root/internal/documentation/welcome/welcome.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/llm/llm.go" "$root/internal/llm/llm.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/yaml/yaml.go" "$root/vendor/gopkg.in/yaml.v3/yaml.go" || infra layer_setup
  cp "$repo_root/$fixture_rel/stubs/yaml/modules.txt" "$root/vendor/modules.txt" || infra layer_setup
  cp "$repo_root/$fixture_rel/fixture.go" "$root/$fixture_rel/fixture.go" || infra layer_setup
  cp "$repo_root/$layer_source" "$root/$layer_source" || infra layer_setup
  printf 'package %s\nimport "testing"\nfunc TestAUR309Bridge(t *testing.T) { %s(t) }\n' "$package_name" "$function_name" \
    >"$root/$package_dir/aur309_bridge_test.go" || infra layer_setup
}

run_layer() {
  local name="$1" source="$2" function_name="$3" package_dir="$4" package_name="$5"
  local root="$run_dir/$name" log_file="$run_dir/$name.log" bytes rc
  prepare_layer "$root" "$source" "$function_name" "$package_dir" "$package_name"
  set +e
  (
    cd -- "$root" &&
      GOFLAGS='-mod=vendor' GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=1 \
      GOCACHE="$run_dir/go-cache" GOTMPDIR="$run_dir/go-tmp" \
      go test -tags aur309_characterization -p 1 "./$package_dir" -run '^TestAUR309Bridge$' -count=1 -v
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
  printf 'entrypoint\tclassification\teffect\n' >"$actual"
  awk -F '\t' '$1 == "AUR309_OBSERVATION" && NF == 4 { print $2 "\t" $3 "\t" $4 }' "$log_file" >>"$actual"
  [[ "$(wc -l <"$actual")" == 14 ]] || fail report_row_count
  {
    printf 'entrypoint\tclassification\teffect\n'
    sed '1d' "$actual" | LC_ALL=C sort -t $'\t' -k1,1
  } >"$run_dir/actual.sorted.tsv"
  mv "$run_dir/actual.sorted.tsv" "$actual"
  if ! cmp -s "$repo_root/$expected_rel" "$actual"; then
    expected_deploy="$(awk -F '\t' '$1 == "deployToGHPages" { print $2 }' "$repo_root/$expected_rel")"
    actual_deploy="$(awk -F '\t' '$1 == "deployToGHPages" { print $2 }' "$actual")"
    if [[ "$expected_deploy" == reachable && "$actual_deploy" == 'stub/unreachable' ]]; then
      fail MUT-001
    fi
    fail characterization_mismatch
  fi
}

run_unit() {
  run_layer unit "$unit_rel" TestAUR309 tests/unit unit
  printf '%s/TestAUR309: pass\n' "$card"
}

run_contract() {
  run_layer contract "$contract_rel" ContractAUR309 tests/contracts contracts
  printf '%s/ContractAUR309: pass\n' "$card"
}

run_integration() {
  run_layer integration "$integration_rel" IntegrationAUR309 tests/integration integration
  extract_report
  printf '%s/IntegrationAUR309: pass\n' "$card"
}

run_e2e() {
  bash "$repo_root/$e2e_rel" E2EAUR309
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
  local materialized_digest pipeline_digest tests_digest report_digest observations
  pipeline_digest="$(tree_digest internal/pipeline)" || infra pipeline_digest
  tests_digest="$(tree_digest "$fixture_rel" "$unit_rel" "$contract_rel" "$integration_rel" "$e2e_rel" tests/acceptance/AUR-309.sh)" || infra tests_digest
  materialized_digest="$(tree_digest internal/pipeline go.mod go.sum "$fixture_rel" "$unit_rel" "$contract_rel" "$integration_rel" "$e2e_rel" tests/acceptance/AUR-309.sh docs/specs/AUR-309.md)" || infra materialized_digest
  report_digest="$(sha256sum -- "$run_dir/actual.tsv")" || infra report_digest
  report_digest="${report_digest%% *}"
  observations="$(awk -F '\t' '
    NR == 1 { next }
    BEGIN { printf "[" }
    { if (count++) printf ","; printf "{\"entrypoint\":\"%s\",\"classification\":\"%s\",\"effect\":\"%s\"}", $1, $2, $3 }
    END { printf "]" }
  ' "$run_dir/actual.tsv")" || infra report_json
  printf '{"schema":"aurum.pipeline-characterization","version":1,"card":"%s","scenario":"%s","candidate_identity_v1":{"materialized_tree_sha256":"sha256:%s","pipeline_tree_sha256":"sha256:%s","tests_tree_sha256":"sha256:%s"},"report_sha256":"sha256:%s","entrypoints":%s,"external_indexing":false,"external_deploy":false,"result":"pass"}\n' \
    "$card" "$scenario" "$materialized_digest" "$pipeline_digest" "$tests_digest" "$report_digest" "$observations"
}

case "$selector" in
  TestAUR309) run_unit ;;
  ContractAUR309) run_contract ;;
  IntegrationAUR309) run_integration ;;
  E2EAUR309) run_e2e ;;
  AC-001)
    run_unit
    run_contract
    run_integration
    run_e2e
    emit_observation
    ;;
esac
