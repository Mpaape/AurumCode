#!/usr/bin/env bash
# AUR-422 AC-001: the loader resolves bootstrap-readonly-v1 in the registry and
# derives the plan oci-run would execute before any engine; only the nominal
# documents derive a hardened plan, and every declared mutant is refused with a
# stable code and zero engine invocations.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-422 scenario=AC-001
selector="${1:-AC-001}"
fail(){ printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra(){ printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }
case "$selector" in
  AC-001|TestAUR422|ContractAUR422|IntegrationAUR422|E2EAUR422|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

inputs=(
  .board/oci/profiles/bootstrap-readonly-v1.json
  .board/locks/oci/bootstrap-readonly-v1.lock.json
  .board/oci/profiles/registry.v1.json
  .board/schemas/container-profile.schema.json
  .board/bin/oci-run
  go.mod go.sum
  tests/unit/AUR-422.go tests/contracts/AUR-422.go tests/integration/AUR-422.go
  tests/e2e/AUR-422.sh
)
for p in "${inputs[@]}"; do [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"; done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a422.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
root="$run_dir/root"
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"
for p in "${inputs[@]}"; do mkdir -p "$root/$(dirname "$p")"; cp "$repo_root/$p" "$root/$p"; done
printf 'package unit\nimport "testing"\nfunc TestAUR422Bridge(t *testing.T){ TestAUR422(t) }\n' >"$root/tests/unit/aur422_bridge_test.go"
printf 'package contracts\nimport "testing"\nfunc TestAUR422Bridge(t *testing.T){ ContractAUR422(t) }\n' >"$root/tests/contracts/aur422_bridge_test.go"
printf 'package integration\nimport "testing"\nfunc TestAUR422Bridge(t *testing.T){ IntegrationAUR422(t) }\n' >"$root/tests/integration/aur422_bridge_test.go"

go_lanes(){
  # One offline invocation compiles and executes the unit, contract and
  # integration assertions against the materialized documents.
  local mutation="${1:-}" out rc pkgs=("${@:2}")
  (( ${#pkgs[@]} > 0 )) || pkgs=(./tests/unit ./tests/contracts ./tests/integration)
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" AURUM_A422_MUTATION="$mutation" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -v -vet=off -p 1 -count=1 "${pkgs[@]}" -run '^TestAUR422Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    rejection="$(grep -om1 'AUR-422/AC-001/[A-Za-z0-9/_-]*' <<<"$out" || true)"
    [[ -z "$rejection" ]] || printf '%s\n' "$rejection" >&2
    fail "selector-exit:$rc"
  fi
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  (( ok_count == ${#pkgs[@]} )) || fail "zero-tests:${ok_count}-of-${#pkgs[@]}"
  ! grep -Fq '[no test files]' <<<"$out" || fail 'no-test-files'
  if [[ -z "$mutation" ]]; then
    case " ${pkgs[*]} " in
      *' ./tests/unit '*) grep -Fq 'plan image=' <<<"$out" || fail 'plan-line-missing' ;;
    esac
  fi
}

mutation_case(){
  # The mutant is constructed from the nominal documents in memory; the loader
  # must return the exact stable code, with zero engine invocations.
  local mutation="$1" expected="$2" out rc
  set +e
  out="$(cd "$root" && AURUMCODE_ROOT="$root" AURUM_A422_MUTATION="$mutation" \
    HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=readonly \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go test -v -vet=off -p 1 -count=1 ./tests/unit -run '^TestAUR422Bridge$' 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "mutation-not-enforced:$mutation"
  grep -Fq "mutation=$mutation code=$expected engine_invocations=0" <<<"$out" ||
    fail "mutation-code-missing:$mutation"
}

e2e_case(){ (cd "$root" && bash tests/e2e/AUR-422.sh E2EAUR422) || fail selector:E2EAUR422; }

case "$selector" in
  AC-001)
    go_lanes ''
    e2e_case
    mutation_case MUT-001 lock-mismatch
    mutation_case MUT-002 mutable-image
    mutation_case MUT-003 privileged-plan
    printf '{"card":"%s","scenario":"%s","result":"valid","engine_invocations":0}\n' "$card" "$scenario"
    ;;
  TestAUR422) go_lanes '' ./tests/unit ;;
  ContractAUR422) go_lanes '' ./tests/contracts ;;
  IntegrationAUR422) go_lanes '' ./tests/integration ;;
  E2EAUR422) e2e_case ;;
  AC-001-MUT-001) mutation_case MUT-001 lock-mismatch ;;
  AC-001-MUT-002) mutation_case MUT-002 mutable-image ;;
  AC-001-MUT-003) mutation_case MUT-003 privileged-plan ;;
esac
