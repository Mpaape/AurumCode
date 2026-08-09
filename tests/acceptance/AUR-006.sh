#!/usr/bin/env bash
# AUR-006 / AC-001 -- validate the bootstrap profile without invoking an OCI
# engine. The Go contract owns the behavioral matrix; this wrapper dispatches
# every declared test layer and treats missing toolchain/dependencies as infra.
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-006'
readonly scenario='AC-001'
selector="${1:-AC-001}"

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

require_file() {
  local path="$1"
  [[ -f "$path" && ! -L "$path" && -r "$path" ]] || fail "entrypoint_missing:$path"
}

for path in \
  .board/schemas/container-profile.schema.json \
  internal/sandbox/profile/schema.go \
  tests/contracts/sandbox-profile/contract_test.go \
  tests/specs/AUR-006/cases.yaml \
  tests/specs/AUR-006/go.mod \
  tests/specs/AUR-006/go.sum; do
  require_file "$path"
done

case "$selector" in
  AC-001|TestAUR006|ContractAUR006|IntegrationAUR006|E2EAUR006) ;;
  AC-001-MATRIX)
    [[ "${AURUM_A006_INTERNAL:-}" == 1 ]] || {
      printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2
      exit 64
    }
    ;;
  *)
    printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2
    exit 64
    ;;
esac

command -v go >/dev/null 2>&1 || infra missing_tool:go

run_dir=''
output=''
cleanup() {
  rm -f -- "$output"
  if [[ -n "$run_dir" ]]; then
    rm -rf -- "$run_dir"
  fi
}
trap cleanup EXIT INT TERM HUP

go_test_infrastructure() {
  local file="$1"
  grep -Eiq -- \
    'module lookup disabled|missing go\.sum|no required module provides|cannot find module|could not download|toolchain.*(unavailable|not found)' \
    "$file"
}

run_go_test() {
  local label="$1"
  shift
  output="$(mktemp "${TMPDIR:-/tmp}/aurum-a006-go.XXXXXX")" || infra mktemp_failed
  set +e
  (
    cd -- "$run_dir" || exit 70
    mkdir -p -- "$run_dir/.go-cache" "$run_dir/.go-tmp" || exit 70
    AURUMCODE_ROOT="$run_dir" GOTOOLCHAIN=local GOPROXY=off \
      GOCACHE="$run_dir/.go-cache" GOTMPDIR="$run_dir/.go-tmp" go test -vet=off "$@"
  ) >"$output" 2>&1
  local test_exit=$?
  set -e
  if (( test_exit != 0 )); then
    if go_test_infrastructure "$output"; then
      infra "go_test_unavailable:${label}:${test_exit}"
    fi
    sed -n '1,80p' "$output" >&2
    fail "go_test_failed:${label}:${test_exit}"
  fi
  rm -f -- "$output"
  output=''
}

prepare_go_tree() {
  local entrypoint="${1:-}"
  run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a006-tree.XXXXXX")" || infra mktemp_failed
  mkdir -p -- \
    "$run_dir/internal/sandbox/profile" \
    "$run_dir/.board/schemas" \
    "$run_dir/tests/specs/AUR-006"
  cp -- \
    "$repo_root/internal/sandbox/profile/schema.go" \
    "$run_dir/internal/sandbox/profile/schema.go"
  cp -- \
    "$repo_root/.board/schemas/container-profile.schema.json" \
    "$run_dir/.board/schemas/container-profile.schema.json"
  cp -- \
    "$repo_root/tests/specs/AUR-006/cases.yaml" \
    "$run_dir/tests/specs/AUR-006/cases.yaml"
  cp -- \
    "$repo_root/tests/specs/AUR-006/go.mod" \
    "$run_dir/go.mod"
  cp -- \
    "$repo_root/tests/specs/AUR-006/go.sum" \
    "$run_dir/go.sum"
  if [[ -n "$entrypoint" ]]; then
    mkdir -p -- "$run_dir/$(dirname -- "$entrypoint")"
    cp -- "$repo_root/$entrypoint" "$run_dir/$entrypoint"
  fi
}

run_go_entrypoint() {
  local entrypoint="$1"
  local package_path="$2"
  local function_name="$3"
  require_file "$entrypoint"
  grep -Eq -- "^[[:space:]]*func[[:space:]]+${function_name}[[:space:]]*\\(" "$entrypoint" ||
    fail "entrypoint_missing:${entrypoint}::${function_name}"
  prepare_go_tree "$entrypoint"
  printf '%s\n' \
    'package bridge' \
    '' \
    'import (' \
    '  "testing"' \
    "  target \"github.com/Mpaape/AurumCode/${package_path}\"" \
    ')' \
    '' \
    'func TestAUR006Bridge(t *testing.T) {' \
    "  target.${function_name}(t)" \
    '}' >"$run_dir/bridge_test.go"
  run_go_test "$function_name" -run '^TestAUR006Bridge$' -count=1 -v
  rm -rf -- "$run_dir"
  run_dir=''
}

run_matrix() {
  require_file tests/contracts/sandbox-profile/contract_test.go
  prepare_go_tree tests/contracts/sandbox-profile/contract_test.go
  run_go_test AUR006ContractSuite ./tests/contracts/sandbox-profile -run '^(TestContractAUR006|TestAUR006Rejects.*)$' -count=1 -v
  rm -rf -- "$run_dir"
  run_dir=''
}

run_e2e() {
  require_file tests/e2e/AUR-006.sh
  bash tests/e2e/AUR-006.sh E2EAUR006 >/dev/null
}

emit_result() {
  printf '{"card":"%s","scenario":"%s","selector":"%s","result":"valid","cases":11,"engine_invocations":0}\n' \
    "$card" "$scenario" "$selector"
}

case "$selector" in
  AC-001)
    run_go_entrypoint tests/unit/AUR-006.go tests/unit TestAUR006
    run_go_entrypoint tests/contracts/AUR-006.go tests/contracts ContractAUR006
    run_go_entrypoint tests/integration/AUR-006.go tests/integration IntegrationAUR006
    run_e2e
    ;;
  TestAUR006)
    run_go_entrypoint tests/unit/AUR-006.go tests/unit TestAUR006
    ;;
  ContractAUR006)
    run_go_entrypoint tests/contracts/AUR-006.go tests/contracts ContractAUR006
    ;;
  IntegrationAUR006)
    run_go_entrypoint tests/integration/AUR-006.go tests/integration IntegrationAUR006
    ;;
  E2EAUR006)
    require_file tests/e2e/AUR-006.sh
    bash tests/e2e/AUR-006.sh E2EAUR006
    ;;
  AC-001-MATRIX)
    run_matrix
    ;;
esac

emit_result
