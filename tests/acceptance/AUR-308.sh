#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly card=AUR-308 scenario=AC-001
selector="${1:-AC-001}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  AC-001|TestAUR308|ContractAUR308|IntegrationAUR308|E2EAUR308|AC-001-MUT-001|AC-001-MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"
[[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo-root
command -v go >/dev/null 2>&1 || infra missing-go

for path in \
  go.mod go.sum internal/documentation \
  tests/characterization/legacy/documentation/classification.tsv \
  tests/characterization/legacy/documentation/characterization.go \
  tests/characterization/legacy/documentation/characterization_test.go \
  tests/unit/AUR-308.go tests/contracts/AUR-308.go tests/integration/AUR-308.go \
  tests/e2e/AUR-308.sh; do
  [[ -e "$repo_root/$path" && ! -L "$repo_root/$path" ]] || fail "entrypoint-missing:$path"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a308.XXXXXX")" || infra mktemp
cleanup() {
  chmod -R u+rwX -- "$run_dir" 2>/dev/null || true
  rm -rf -- "$run_dir" 2>/dev/null || true
}
trap cleanup EXIT INT TERM HUP

work="$run_dir/work"
mkdir -p "$work/internal" "$work/tests/characterization/legacy" "$work/tests/unit" "$work/tests/contracts" "$work/tests/integration" "$work/tests/e2e" \
  "$run_dir/go-cache" "$run_dir/go-tmp" || infra mkdir
cp "$repo_root/go.mod" "$repo_root/go.sum" "$work/" || infra copy-module
cp -R "$repo_root/internal/documentation" "$work/internal/" || infra copy-documentation
cp -R "$repo_root/tests/characterization/legacy/documentation" "$work/tests/characterization/legacy/" || infra copy-characterization
cp "$repo_root/tests/unit/AUR-308.go" "$work/tests/unit/AUR-308.go" || infra copy-unit
cp "$repo_root/tests/contracts/AUR-308.go" "$work/tests/contracts/AUR-308.go" || infra copy-contract
cp "$repo_root/tests/integration/AUR-308.go" "$work/tests/integration/AUR-308.go" || infra copy-integration
cp "$repo_root/tests/e2e/AUR-308.sh" "$work/tests/e2e/AUR-308.sh" || infra copy-e2e
chmod +x "$work/tests/e2e/AUR-308.sh"

cat >"$work/tests/unit/aur308_bridge_test.go" <<'EOF'
package unit
import "testing"
func TestAUR308Bridge(t *testing.T) { TestAUR308(t) }
EOF
cat >"$work/tests/contracts/aur308_bridge_test.go" <<'EOF'
package contracts
import "testing"
func TestAUR308ContractBridge(t *testing.T) { ContractAUR308(t) }
EOF
cat >"$work/tests/integration/aur308_bridge_test.go" <<'EOF'
package integration
import "testing"
func TestAUR308IntegrationBridge(t *testing.T) { IntegrationAUR308(t) }
EOF

export AURUMCODE_ROOT="$work"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=1
export GOCACHE="$run_dir/go-cache" GOTMPDIR="$run_dir/go-tmp"

module_path="$(cd "$work" && go list -m)" || infra module-path
package_list="$run_dir/packages.txt"
(cd "$work" && go list \
  ./internal/documentation/extractors/... \
  ./internal/documentation/incremental \
  ./internal/documentation/normalizer \
  ./internal/documentation/site) | sed "s#^${module_path}/##" | sort >"$package_list" || infra package-inventory
export AURUM_A308_PACKAGE_LIST="$package_list"

run_characterization() {
  local log="$run_dir/characterization.json" test
  (cd "$work" && go test -json -p 1 ./tests/characterization/legacy/documentation -count=1) >"$log" || fail characterization-suite
  while IFS=$'\t' read -r _ _ test _; do
    [[ "$test" == test ]] && continue
    awk -v wanted="$test" 'index($0, "\"Action\":\"pass\"") && index($0, "\"Test\":\"" wanted "\"") { found=1 } END { exit !found }' "$log" \
      || fail "unexecuted-characterization:$test"
  done <"$work/tests/characterization/legacy/documentation/classification.tsv"
}

run_unit() {
  (cd "$work" && go test -p 1 ./tests/unit -run '^TestAUR308Bridge$' -count=1)
}
run_contract() {
  (cd "$work" && go test -p 1 ./tests/contracts -run '^TestAUR308ContractBridge$' -count=1)
}
run_integration() {
  (cd "$work" && go test -p 1 ./tests/integration -run '^TestAUR308IntegrationBridge$' -count=1)
}
run_e2e() {
  bash "$work/tests/e2e/AUR-308.sh" E2EAUR308
}

mutation_one() {
  local mutated="$run_dir/classification-mut1.tsv" log="$run_dir/mut1.log" status
  awk 'BEGIN { FS=OFS="\t" } NR==2 { $3="TestAUR308UnexecutedMutation" } { print }' \
    "$work/tests/characterization/legacy/documentation/classification.tsv" >"$mutated"
  set +e
  (cd "$work" && AURUM_A308_MANIFEST="$mutated" go test -p 1 ./tests/unit -run '^TestAUR308Bridge$' -count=1) >"$log" 2>&1
  status=$?
  set -e
  [[ "$status" -eq 1 ]] || fail MUT-001
  grep -Fq 'references unexecuted test TestAUR308UnexecutedMutation' "$log" || fail MUT-001
  printf '%s/%s/MUT-001 raw_exit=%d\n' "$card" "$scenario" "$status"
  unset AURUM_A308_MANIFEST
  run_unit >/dev/null || fail MUT-001-restore
  printf '%s/%s/MUT-001-restore raw_exit=0\n' "$card" "$scenario"
}

mutation_two() {
  local stage="$run_dir/mut2-stage" outside="$run_dir/mut2-outside.json" output log="$run_dir/mut2.log" status
  mkdir -p "$stage"
  printf 'outside-sentinel\n' >"$outside"
  output="$stage/AC-001.json"
  ln -s "$outside" "$output"
  set +e
  (cd "$work" && AURUM_A308_ARTIFACT_STAGE="$stage" AURUM_A308_ARTIFACT="$output" AURUM_A308_INPUT_DIGEST='sha256:mutation' \
    go test -p 1 ./tests/integration -run '^TestAUR308IntegrationBridge$' -count=1) >"$log" 2>&1
  status=$?
  set -e
  [[ "$status" -eq 1 ]] || fail MUT-002
  grep -Fq 'artifact output is a symlink' "$log" || fail MUT-002
  [[ "$(cat "$outside")" == outside-sentinel ]] || fail MUT-002
  printf '%s/%s/MUT-002 raw_exit=%d\n' "$card" "$scenario" "$status"
  rm "$output"
  AURUM_A308_ARTIFACT_STAGE="$stage" AURUM_A308_ARTIFACT="$output" AURUM_A308_INPUT_DIGEST='sha256:mutation' run_integration >/dev/null \
    || fail MUT-002-restore
  printf '%s/%s/MUT-002-restore raw_exit=0\n' "$card" "$scenario"
}

emit_artifact() {
  local stage="$run_dir/final-stage" output input_digest
  mkdir -p "$stage"
  output="$stage/AC-001.json"
  input_digest="$(sed -n 's/.*"input_manifest_digest":"\([^"]*\)".*/\1/p' "$repo_root/.aurum-bootstrap/context.json" 2>/dev/null || true)"
  [[ "$input_digest" == sha256:* ]] || input_digest='sha256:standalone'
  AURUM_A308_ARTIFACT_STAGE="$stage" AURUM_A308_ARTIFACT="$output" AURUM_A308_INPUT_DIGEST="$input_digest" run_integration >/dev/null \
    || fail artifact-emission
  cat "$output"
}

run_nominal() {
  run_characterization
  printf '%s/%s/characterization raw_exit=0\n' "$card" "$scenario"
  run_unit
  printf '%s/%s/TestAUR308 raw_exit=0\n' "$card" "$scenario"
  run_contract
  printf '%s/%s/ContractAUR308 raw_exit=0\n' "$card" "$scenario"
  run_integration
  printf '%s/%s/IntegrationAUR308 raw_exit=0\n' "$card" "$scenario"
  run_e2e
  printf '%s/%s/E2EAUR308 raw_exit=0\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_nominal; mutation_one; mutation_two; emit_artifact ;;
  TestAUR308) run_unit ;;
  ContractAUR308) run_contract ;;
  IntegrationAUR308) run_integration ;;
  E2EAUR308) run_e2e ;;
  AC-001-MUT-001) mutation_one ;;
  AC-001-MUT-002) mutation_two ;;
esac
