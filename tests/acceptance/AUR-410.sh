#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
readonly card=AUR-410 scenario=AC-001
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
case "$selector" in AC-001|TestAUR410|ContractAUR410|IntegrationAUR410|E2EAUR410|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;; *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;; esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
required_paths=(.board/schemas/oci-conformance-profile.schema.json .board/schemas/container-profile.schema.json .board/oci/profiles/registry.v1.json .board/oci/profiles/oci-conformance-v1.json .board/locks/oci/oci-conformance-v1.lock.json .board/locks/oci/registry-v1.lock.json go.mod go.sum tests/unit/AUR-410.go tests/contracts/AUR-410.go tests/integration/AUR-410.go tests/e2e/AUR-410.sh)
for p in "${required_paths[@]}"; do [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"; done
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a410.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/bin"

# `engine_invocations: 0` is a claim made by a JSON document. The ledger below is the
# measurement that has to agree with it. Every container-engine entrypoint is shadowed by
# a shim placed ahead of PATH; a shim records its own name and refuses to run, so any
# attempt to reach an engine from anywhere in this acceptance -- the Go layers, the E2E
# program, the compatibility regression or a mutant -- is written down and observable.
engine_log="$run_dir/engine-invocations.log"
export AURUM_A410_ENGINE_LOG="$engine_log"
: >"$engine_log"
for engine_binary in docker podman nerdctl ctr crictl runc buildah skopeo docker-compose podman-remote; do
  {
    printf '#!/usr/bin/env bash\n'
    printf 'printf "%%s\\n" "%s" >>"$AURUM_A410_ENGINE_LOG"\n' "$engine_binary"
    printf 'exit 70\n'
  } >"$run_dir/bin/$engine_binary"
  chmod 0700 "$run_dir/bin/$engine_binary"
done
PATH="$run_dir/bin:$PATH"
export PATH
hash -r 2>/dev/null || true
engine_ledger_count(){ local n; n="$(grep -c '[^[:space:]]' "$engine_log" 2>/dev/null || true)"; printf '%s' "${n:-0}"; }
# The ledger is proved load-bearing before it is trusted as evidence of absence: the shim
# is invoked once on purpose and has to appear. A ledger that records nothing would make
# the final assertion below pass without doing any work.
engine_ledger_selftest(){
  local observed
  docker version >/dev/null 2>&1 || true
  observed="$(engine_ledger_count)"
  [[ "$observed" == 1 ]] || fail "engine-ledger-inert:$observed"
  : >"$engine_log"
}
engine_ledger_assert_zero(){
  local observed
  observed="$(engine_ledger_count)"
  [[ "$observed" == 0 ]] || fail "engine-ledger-nonzero:$observed"
}

copy(){ local root="$1"; shift; local p; for p in "$@"; do mkdir -p "$root/$(dirname "$p")"; cp "$repo_root/$p" "$root/$p"; done; }
common(){ copy "$1" .board/schemas/oci-conformance-profile.schema.json .board/schemas/container-profile.schema.json .board/oci/profiles/registry.v1.json .board/oci/profiles/oci-conformance-v1.json .board/locks/oci/oci-conformance-v1.lock.json .board/locks/oci/registry-v1.lock.json go.mod go.sum; }
gorun(){ local root="$1" pkg="$2" mutation="${3:-}"; (cd "$root" && AURUMCODE_ROOT="$root" AURUM_A410_MUTATION="$mutation" AURUM_A410_ENGINE_LOG="$engine_log" GOPROXY=off GOSUMDB=off GONOSUMDB='*' GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" go test -v -mod=readonly -p 1 "./$pkg" -run '^TestAUR410Bridge$' -count=1 2>&1); }
go_case(){
  local label="$1" pkg="$2" fn="$3" src="$4" out rc code
  local root="$run_dir/root-$label"
  common "$root"; copy "$root" "$src"
  printf 'package %s\nimport "testing"\nfunc TestAUR410Bridge(t *testing.T){ %s(t) }\n' "${pkg##*/}" "$fn" >"$root/$pkg/aur410_bridge_test.go"
  set +e; out="$(gorun "$root" "$pkg")"; rc=$?; set -e
  printf '%s\n' "$out"
  if (( rc != 0 )); then
    code="$(sed -n 's/.*registry_code=\([a-z-]\{1,\}\).*/\1/p' <<<"$out" | head -1)"
    if [[ -n "$code" && "$code" != valid ]]; then loader_fail "$code"; fi
    fail "selector:$label:exit:$rc"
  fi
  # `ok <pkg> 0.00s [no tests to run]` also starts with ok, so a package that compiled
  # but executed nothing would pass an `ok`-only check. Demand the RUN line and the PASS
  # line for this exact bridge as well.
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail "selector:$label:zero-tests"
  grep -Eq '^=== RUN[[:space:]]+TestAUR410Bridge$' <<<"$out" || fail "selector:$label:not-executed"
  grep -Eq '^--- PASS:[[:space:]]+TestAUR410Bridge[[:space:]]' <<<"$out" || fail "selector:$label:not-passed"
  rm -rf -- "$root"
}
e2e_case(){ local root="$run_dir/root-e2e"; common "$root"; copy "$root" tests/e2e/AUR-410.sh; (cd "$root" && bash tests/e2e/AUR-410.sh E2EAUR410) || fail selector:E2EAUR410; rm -rf -- "$root"; }
# Compatibility regression. Registering a tenth key must leave the canonical registry
# loader delivered by AUR-402 green, so its acceptance is re-executed here against the
# extended registry. The AUR-403 to AUR-409 loaders are extended in the same commit and
# re-proved by their own acceptances, whose profile documents this card does not
# materialize.
aur402_case(){ (cd "$repo_root" && AURUM_A402_EMBEDDED_LAYERS=1 AURUM_A402_CACHE_ROOT="$run_dir/cache" AURUM_A402_TMP_ROOT="$run_dir/gotmp" bash tests/acceptance/AUR-402.sh AC-001) || fail compatibility:AUR402; }
mutation_case(){
  local mutation="$1" expected="${2:-$1}" out rc
  local root="$run_dir/root-mut-$mutation"
  common "$root"; copy "$root" tests/unit/AUR-410.go
  printf 'package unit\nimport "testing"\nfunc TestAUR410Bridge(t *testing.T){ TestAUR410(t) }\n' >"$root/tests/unit/aur410_bridge_test.go"
  set +e; out="$(gorun "$root" tests/unit "$mutation")"; rc=$?; set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "mutation:$mutation:exit:$rc"
  grep -Fq "mutation=$mutation code=$expected" <<<"$out" || fail "mutation:$mutation:not-rejected"
  rm -rf -- "$root"
}
run_all(){
  engine_ledger_selftest
  go_case unit tests/unit TestAUR410 tests/unit/AUR-410.go
  go_case contract tests/contracts ContractAUR410 tests/contracts/AUR-410.go
  go_case integration tests/integration IntegrationAUR410 tests/integration/AUR-410.go
  e2e_case
  aur402_case
  mutation_case duplicate-profile
  mutation_case mutable-input
  mutation_case unsafe-plan
  mutation_case mount-enabled unsafe-plan
  mutation_case socket-enabled unsafe-plan
  mutation_case device-enabled unsafe-plan
  mutation_case root-user unsafe-plan
  mutation_case privileged-enabled unsafe-plan
  mutation_case capability-added unsafe-plan
  mutation_case capability-kept unsafe-plan
  mutation_case writable-rootfs unsafe-plan
  mutation_case new-privileges-allowed unsafe-plan
  mutation_case writable-checkout unsafe-plan
  mutation_case egress-allowed unsafe-plan
  mutation_case probe-output-authoritative unsafe-plan
  mutation_case probe-output-trusted unsafe-plan
  mutation_case probe-verdict-advisory unsafe-plan
  mutation_case verdict-authority-probe unsafe-plan
  mutation_case probe-trusted unsafe-plan
  mutation_case orchestrator-untrusted unsafe-plan
  mutation_case probe-writes-orchestrator-root unsafe-plan
  mutation_case probe-writes-report-root unsafe-plan
  mutation_case probe-writes-parent-root unsafe-plan
  mutation_case probe-writable-roots-empty unsafe-plan
  mutation_case engine-list-widened unsafe-plan
  mutation_case engine-list-unsupported unsafe-plan
  mutation_case engine-list-empty unsafe-plan
  mutation_case engine-list-duplicated unsafe-plan
  mutation_case unsupported-engine-allowed unsafe-plan
  mutation_case engine-invoked unsafe-plan
  mutation_case engine-socket-allowed unsafe-plan
  mutation_case engine-api-allowed unsafe-plan
  mutation_case host-engine-allowed unsafe-plan
  mutation_case host-checkout-allowed unsafe-plan
  mutation_case host-filesystem-allowed unsafe-plan
  mutation_case subprocess-allowed unsafe-plan
  mutation_case cgo-probe unsafe-plan
  mutation_case persistent-report unsafe-plan
  mutation_case orchestrator-root-host unsafe-plan
  mutation_case probe-root-traversal unsafe-plan
  mutation_case probe-root-trailing-slash unsafe-plan
  mutation_case probe-root-lookalike unsafe-plan
  mutation_case probe-root-newline unsafe-plan
  mutation_case probe-root-trailing-newline unsafe-plan
  mutation_case probe-root-leading-space unsafe-plan
  mutation_case probe-root-uppercase unsafe-plan
  mutation_case probe-root-homoglyph unsafe-plan
  mutation_case probe-root-empty unsafe-plan
  mutation_case report-root-escape unsafe-plan
  mutation_case unbounded-report unsafe-plan
  mutation_case oversized-report unsafe-plan
  mutation_case oversized-probe-set unsafe-plan
  mutation_case unbounded-probe-bytes unsafe-plan
  mutation_case unbounded-probe-timeout unsafe-plan
  mutation_case unlimited-tmpfs unsafe-plan
  mutation_case engine-socket-environment unsafe-plan
  mutation_case container-host-environment unsafe-plan
  mutation_case engine-config-environment unsafe-plan
  mutation_case proxy-environment unsafe-plan
  mutation_case no-proxy-environment unsafe-plan
  mutation_case cgo-environment unsafe-plan
  mutation_case unpinned-probe-image-set digest-invalid
  mutation_case mutable-probe-image-set digest-invalid
  mutation_case missing-pids-limit schema-invalid
  mutation_case missing-probe-timeout schema-invalid
  mutation_case missing-supported-engines schema-invalid
  mutation_case real-credential credential-present
  engine_ledger_assert_zero
  printf '{"card":"%s","scenario":"%s","result":"valid","engine_invocations":0,"engine_invocations_measured":%s,"probe_verdict_authority":"denied"}\n' "$card" "$scenario" "$(engine_ledger_count)"
}
case "$selector" in
 AC-001) run_all;;
 TestAUR410) engine_ledger_selftest; go_case unit tests/unit TestAUR410 tests/unit/AUR-410.go; engine_ledger_assert_zero;;
 ContractAUR410) go_case contract tests/contracts ContractAUR410 tests/contracts/AUR-410.go;;
 IntegrationAUR410) go_case integration tests/integration IntegrationAUR410 tests/integration/AUR-410.go;;
 E2EAUR410) e2e_case;;
 AC-001-MUT-001) mutation_case duplicate-profile;;
 AC-001-MUT-002) mutation_case mutable-input;;
 AC-001-MUT-003) mutation_case unsafe-plan;;
esac
