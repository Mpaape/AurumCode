#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
readonly card=AUR-409 scenario=AC-001
selector="${1:-AC-001}"
fail(){ printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra(){ printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }
# A loader rejection is reported with the skeptical-mutation id the card declares for
# that code, so the mutant and the nominal path speak the same language.
loader_fail(){
  local code="$1" prefix=''
  case "$code" in
    duplicate-profile) prefix='MUT-001/' ;;
    mutable-input) prefix='MUT-002/' ;;
    unsafe-plan) prefix='MUT-003/' ;;
  esac
  printf '%s/%s/%s%s\n' "$card" "$scenario" "$prefix" "$code" >&2
  exit 1
}
case "$selector" in AC-001|TestAUR409|ContractAUR409|IntegrationAUR409|E2EAUR409|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;; *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;; esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a409.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
copy(){ local root="$1"; shift; local p; for p in "$@"; do mkdir -p "$root/$(dirname "$p")"; cp "$repo_root/$p" "$root/$p"; done; }
common(){ copy "$1" .board/schemas/fake-scm-offline-profile.schema.json .board/schemas/container-profile.schema.json .board/oci/profiles/registry.v1.json .board/oci/profiles/fake-scm-offline-v1.json .board/locks/oci/fake-scm-offline-v1.lock.json .board/locks/oci/registry-v1.lock.json go.mod go.sum; }
gorun(){ local root="$1" pkg="$2" mutation="${3:-}"; (cd "$root" && AURUMCODE_ROOT="$root" AURUM_A409_MUTATION="$mutation" GOPROXY=off GOSUMDB=off GONOSUMDB='*' GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" go test -v -mod=readonly -p 1 "./$pkg" -run '^TestAUR409Bridge$' -count=1 2>&1); }
go_case(){
  local label="$1" pkg="$2" fn="$3" src="$4" out rc code
  local root="$run_dir/root-$label"
  common "$root"; copy "$root" "$src"
  printf 'package %s\nimport "testing"\nfunc TestAUR409Bridge(t *testing.T){ %s(t) }\n' "${pkg##*/}" "$fn" >"$root/$pkg/aur409_bridge_test.go"
  set +e; out="$(gorun "$root" "$pkg")"; rc=$?; set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    code="$(sed -n 's/.*registry_code=\([a-z-]\{1,\}\).*/\1/p' <<<"$out" | head -1)"
    if [[ -n "$code" && "$code" != valid ]]; then loader_fail "$code"; fi
    fail "selector:$label:exit:$rc"
  fi
  # `ok <pkg> 0.00s [no tests to run]` also starts with ok, so a package that compiled
  # but executed nothing would pass an `ok`-only check. Demand the RUN line as well.
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$label:zero-tests"
  grep -Eq '^=== RUN[[:space:]]+TestAUR409Bridge$' <<<"$out" || fail "selector:$label:not-executed"
  grep -Eq '^--- PASS:[[:space:]]+TestAUR409Bridge[[:space:]]' <<<"$out" || fail "selector:$label:not-passed"
  rm -rf -- "$root"
}
e2e_case(){ local root="$run_dir/root-e2e"; common "$root"; copy "$root" tests/e2e/AUR-409.sh; (cd "$root" && bash tests/e2e/AUR-409.sh E2EAUR409) || fail selector:E2EAUR409; rm -rf -- "$root"; }
# Compatibility regression. Registering a ninth key must leave the canonical registry
# loader delivered by AUR-402 green, so its acceptance is re-executed here against the
# extended registry. The AUR-403, AUR-404, AUR-405, AUR-406, AUR-407 and AUR-408 loaders
# are extended in the same commit and re-proved by their own acceptances, whose profile
# documents this card does not materialize.
aur402_case(){ (cd "$repo_root" && AURUM_A402_EMBEDDED_LAYERS=1 AURUM_A402_CACHE_ROOT="$run_dir/cache" AURUM_A402_TMP_ROOT="$run_dir/gotmp" bash tests/acceptance/AUR-402.sh AC-001) || fail compatibility:AUR402; }
mutation_case(){
  local mutation="$1" expected="${2:-$1}" out rc
  local root="$run_dir/root-mut-$mutation"
  common "$root"; copy "$root" tests/unit/AUR-409.go
  printf 'package unit\nimport "testing"\nfunc TestAUR409Bridge(t *testing.T){ TestAUR409(t) }\n' >"$root/tests/unit/aur409_bridge_test.go"
  set +e; out="$(gorun "$root" tests/unit "$mutation")"; rc=$?; set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "mutation:$mutation:exit:$rc"
  grep -Fq "mutation=$mutation code=$expected" <<<"$out" || fail "mutation:$mutation:not-rejected"
  rm -rf -- "$root"
}
run_all(){
  go_case unit tests/unit TestAUR409 tests/unit/AUR-409.go
  go_case contract tests/contracts ContractAUR409 tests/contracts/AUR-409.go
  go_case integration tests/integration IntegrationAUR409 tests/integration/AUR-409.go
  e2e_case
  aur402_case
  mutation_case duplicate-profile
  mutation_case mutable-input
  mutation_case unsafe-plan
  mutation_case mount-enabled unsafe-plan
  mutation_case socket-enabled unsafe-plan
  mutation_case device-enabled unsafe-plan
  mutation_case root-user unsafe-plan
  mutation_case capability-added unsafe-plan
  mutation_case real-scm-backend unsafe-plan
  mutation_case real-scm-binary-allowed unsafe-plan
  mutation_case remote-origin-https unsafe-plan
  mutation_case remote-origin-scp unsafe-plan
  mutation_case remote-origin-ssh unsafe-plan
  mutation_case remote-origin-file unsafe-plan
  mutation_case remote-origin-ext unsafe-plan
  mutation_case remote-origin-absent-prefix unsafe-plan
  mutation_case protocol-allowlist-widened unsafe-plan
  mutation_case protocol-allowlist-ext unsafe-plan
  mutation_case custom-transport-allowed unsafe-plan
  mutation_case credential-helper-allowed unsafe-plan
  mutation_case askpass-allowed unsafe-plan
  mutation_case hooks-allowed unsafe-plan
  mutation_case submodule-allowed unsafe-plan
  mutation_case url-rewriting-allowed unsafe-plan
  mutation_case publication-allowed unsafe-plan
  mutation_case external-destination-allowed unsafe-plan
  mutation_case token-present unsafe-plan
  mutation_case credential-source unsafe-plan
  mutation_case host-checkout-allowed unsafe-plan
  mutation_case host-filesystem-allowed unsafe-plan
  mutation_case subprocess-allowed unsafe-plan
  mutation_case cgo-fake-scm unsafe-plan
  mutation_case persistent-repository unsafe-plan
  mutation_case repository-root-host unsafe-plan
  mutation_case repository-root-traversal unsafe-plan
  mutation_case repository-root-trailing-slash unsafe-plan
  mutation_case repository-root-lookalike unsafe-plan
  mutation_case event-root-escape unsafe-plan
  mutation_case fake-scm-root-escape unsafe-plan
  mutation_case unbounded-repository unsafe-plan
  mutation_case oversized-repository unsafe-plan
  mutation_case oversized-event-log unsafe-plan
  mutation_case unbounded-response unsafe-plan
  mutation_case unlimited-tmpfs unsafe-plan
  mutation_case askpass-environment unsafe-plan
  mutation_case protocol-environment unsafe-plan
  mutation_case system-config-environment unsafe-plan
  mutation_case global-config-environment unsafe-plan
  mutation_case terminal-prompt-environment unsafe-plan
  mutation_case unpinned-fake-scm digest-invalid
  mutation_case unpinned-event-set digest-invalid
  mutation_case mutable-response-set digest-invalid
  mutation_case missing-scm-timeout schema-invalid
  mutation_case real-credential credential-present
  printf '{"card":"%s","scenario":"%s","result":"valid","engine_invocations":0}\n' "$card" "$scenario"
}
case "$selector" in
 AC-001) run_all;;
 TestAUR409) go_case unit tests/unit TestAUR409 tests/unit/AUR-409.go;;
 ContractAUR409) go_case contract tests/contracts ContractAUR409 tests/contracts/AUR-409.go;;
 IntegrationAUR409) go_case integration tests/integration IntegrationAUR409 tests/integration/AUR-409.go;;
 E2EAUR409) e2e_case;;
 AC-001-MUT-001) mutation_case duplicate-profile;;
 AC-001-MUT-002) mutation_case mutable-input;;
 AC-001-MUT-003) mutation_case unsafe-plan;;
esac
