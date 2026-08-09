#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
readonly card=AUR-403 scenario=AC-001
selector="${1:-AC-001}"
fail(){ printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra(){ printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }
case "$selector" in AC-001|TestAUR403|ContractAUR403|IntegrationAUR403|E2EAUR403|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;; *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;; esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a403.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
copy(){ local root="$1"; shift; local p; for p in "$@"; do mkdir -p "$root/$(dirname "$p")"; cp "$repo_root/$p" "$root/$p"; done; }
common(){ copy "$1" .board/schemas/go-unit-offline-profile.schema.json .board/schemas/container-profile.schema.json .board/oci/profiles/registry.v1.json .board/oci/profiles/go-unit-offline-v1.json .board/locks/oci/go-unit-offline-v1.lock.json .board/locks/oci/registry-v1.lock.json go.mod go.sum; }
go_case(){ local label="$1" pkg="$2" fn="$3" src="$4" root="$run_dir/root-$label" out; common "$root"; copy "$root" "$src"; printf 'package %s\nimport "testing"\nfunc TestAUR403Bridge(t *testing.T){ %s(t) }\n' "${pkg##*/}" "$fn" >"$root/$pkg/aur403_bridge_test.go"; set +e; out="$(cd "$root" && AURUMCODE_ROOT="$root" GOPROXY=off GOSUMDB=off GONOSUMDB='*' GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" go test -mod=readonly -p 1 "./$pkg" -run '^TestAUR403Bridge$' -count=1 2>&1)"; local rc=$?; set -e; printf '%s\n' "$out"; ((rc==0)) || fail "selector:$label:exit:$rc"; grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$label:zero-tests"; rm -rf -- "$root"; }
e2e_case(){ local root="$run_dir/root-e2e"; common "$root"; copy "$root" tests/e2e/AUR-403.sh; (cd "$root" && bash tests/e2e/AUR-403.sh E2EAUR403) || fail selector:E2EAUR403; rm -rf -- "$root"; }
aur402_case(){ (cd "$repo_root" && bash tests/acceptance/AUR-402.sh AC-001) || fail compatibility:AUR402; }
mutation_case(){ local mutation="$1" root="$run_dir/root-mut-$mutation" out; common "$root"; copy "$root" tests/unit/AUR-403.go; printf 'package unit\nimport "testing"\nfunc TestAUR403Bridge(t *testing.T){ TestAUR403(t) }\n' >"$root/tests/unit/aur403_bridge_test.go"; set +e; out="$(cd "$root" && AURUMCODE_ROOT="$root" AURUM_A403_MUTATION="$mutation" GOPROXY=off GOSUMDB=off GONOSUMDB='*' GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" go test -mod=readonly -p 1 ./tests/unit -run '^TestAUR403Bridge$' -count=1 2>&1)"; local rc=$?; set -e; printf '%s\n' "$out"; ((rc==0)) || fail "mutation:$mutation:exit:$rc"; grep -Fq "mutation=$mutation code=$mutation" <<<"$out" || fail "mutation:$mutation:not-rejected"; rm -rf -- "$root"; }
run_all(){ go_case unit tests/unit TestAUR403 tests/unit/AUR-403.go; go_case contract tests/contracts ContractAUR403 tests/contracts/AUR-403.go; go_case integration tests/integration IntegrationAUR403 tests/integration/AUR-403.go; e2e_case; aur402_case; mutation_case duplicate-profile; mutation_case mutable-input; mutation_case unsafe-plan; printf '{"card":"%s","scenario":"%s","result":"valid","engine_invocations":0}\n' "$card" "$scenario"; }
case "$selector" in
 AC-001) run_all;;
 TestAUR403) go_case unit tests/unit TestAUR403 tests/unit/AUR-403.go;;
 ContractAUR403) go_case contract tests/contracts ContractAUR403 tests/contracts/AUR-403.go;;
 IntegrationAUR403) go_case integration tests/integration IntegrationAUR403 tests/integration/AUR-403.go;;
 E2EAUR403) e2e_case;;
 AC-001-MUT-001) mutation_case duplicate-profile;;
 AC-001-MUT-002) mutation_case mutable-input;;
 AC-001-MUT-003) mutation_case unsafe-plan;;
esac
