#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
readonly card=AUR-411 scenario=AC-001
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
case "$selector" in AC-001|TestAUR411|ContractAUR411|IntegrationAUR411|E2EAUR411|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;; *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;; esac
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
required_paths=(.board/schemas/polyglot-toolchain-profile.schema.json .board/schemas/container-profile.schema.json .board/oci/profiles/registry.v1.json .board/oci/profiles/polyglot-toolchain-v1.json .board/locks/oci/polyglot-toolchain-v1.lock.json .board/locks/oci/registry-v1.lock.json go.mod go.sum tests/unit/AUR-411.go tests/contracts/AUR-411.go tests/integration/AUR-411.go tests/e2e/AUR-411.sh)
for p in "${required_paths[@]}"; do [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"; done
run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a411.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/bin" "$run_dir/probe-bin"
readonly pristine_path="$PATH"

# ---------------------------------------------------------------------------------------
# Measurement 1: which declared runtimes actually resolve through PATH in this image.
#
# This is taken FIRST, on the pristine PATH, before a single shim exists -- otherwise a
# shim named `cargo` or `dotnet` would answer the probe and the measurement would report a
# toolchain that is not there. What it measures, and all it measures, is whether the
# runtime command resolves through PATH here; it says nothing about a runtime reachable
# only by absolute path, and the Go layer makes no claim beyond that.
#
# Two controls make the probe falsifiable in both directions: one name is placed on PATH
# immediately before measuring and must come back `present`, and one name that cannot exist
# must come back `absent`. A probe hard-wired to either answer fails on one of them.
# ---------------------------------------------------------------------------------------
availability_log="$run_dir/toolchain-availability.log"
export AURUM_A411_AVAILABILITY_LOG="$availability_log"
printf '#!/usr/bin/env bash\nexit 0\n' >"$run_dir/probe-bin/aurum-control-present-411"
chmod 0700 "$run_dir/probe-bin/aurum-control-present-411"
: >"$availability_log"
measure_availability(){
  local probe verdict
  PATH="$run_dir/probe-bin:$pristine_path"
  hash -r 2>/dev/null || true
  for probe in bash cargo cc dotnet node pwsh python3 tsc aurum-control-present-411 aurum-control-absent-411; do
    if command -v -- "$probe" >/dev/null 2>&1; then verdict=present; else verdict=absent; fi
    printf '%s %s\n' "$probe" "$verdict" >>"$availability_log"
  done
  PATH="$pristine_path"
  hash -r 2>/dev/null || true
  grep -Fqx 'aurum-control-present-411 present' "$availability_log" || fail availability-probe-inert:control-present
  grep -Fqx 'aurum-control-absent-411 absent' "$availability_log" || fail availability-probe-inert:control-absent
  printf 'toolchain_availability_measured=%s\n' "$(grep -c '[^[:space:]]' "$availability_log")"
  sed 's/^/toolchain_availability /' "$availability_log"
}

# ---------------------------------------------------------------------------------------
# Measurement 2: `package_installs: 0` is a claim made by a JSON document; the ledger below
# is the measurement that has to agree with it. Every package-manager and toolchain-
# installer entrypoint is shadowed by a shim placed ahead of PATH; a shim records its own
# name and refuses to run, so any attempt to install a toolchain or a package from anywhere
# in this acceptance -- the Go layers, the E2E program, the compatibility regression or a
# mutant -- is written down and observable. It observes an invocation resolved through
# PATH; a caller reaching a package manager by absolute path would not appear in it, and
# nothing here claims otherwise.
# ---------------------------------------------------------------------------------------
install_log="$run_dir/package-installs.log"
export AURUM_A411_INSTALL_LOG="$install_log"
: >"$install_log"
for installer_binary in apk apt apt-get pip pip3 pipx poetry npm npx yarn pnpm corepack bun deno tsc dotnet nuget cargo rustup gem bundler conda brew curl wget; do
  {
    printf '#!/usr/bin/env bash\n'
    printf 'printf "%%s\\n" "%s" >>"$AURUM_A411_INSTALL_LOG"\n' "$installer_binary"
    printf 'exit 70\n'
  } >"$run_dir/bin/$installer_binary"
  chmod 0700 "$run_dir/bin/$installer_binary"
done
install_ledger_count(){ local n; n="$(grep -c '[^[:space:]]' "$install_log" 2>/dev/null || true)"; printf '%s' "${n:-0}"; }
# The shims only measure anything once they are ahead of PATH. This is deliberately not
# done before `measure_availability` has run: a shim named `cargo`, `dotnet` or `tsc` would
# answer the availability probe and report a toolchain that is not in the image.
install_shim_path(){ PATH="$run_dir/bin:$pristine_path"; export PATH; hash -r 2>/dev/null || true; }
# The ledger is proved load-bearing before it is trusted as evidence of absence: a shim is
# invoked once on purpose and has to appear. A ledger that records nothing would make the
# final assertion below pass without doing any work.
install_ledger_selftest(){
  local observed
  install_shim_path
  npm install >/dev/null 2>&1 || true
  observed="$(install_ledger_count)"
  [[ "$observed" == 1 ]] || fail "install-ledger-inert:$observed"
  : >"$install_log"
}
install_ledger_assert_zero(){
  local observed
  observed="$(install_ledger_count)"
  [[ "$observed" == 0 ]] || fail "install-ledger-nonzero:$observed"
}

copy(){ local root="$1"; shift; local p; for p in "$@"; do mkdir -p "$root/$(dirname "$p")"; cp "$repo_root/$p" "$root/$p"; done; }
common(){ copy "$1" .board/schemas/polyglot-toolchain-profile.schema.json .board/schemas/container-profile.schema.json .board/oci/profiles/registry.v1.json .board/oci/profiles/polyglot-toolchain-v1.json .board/locks/oci/polyglot-toolchain-v1.lock.json .board/locks/oci/registry-v1.lock.json go.mod go.sum; }
gorun(){ local root="$1" pkg="$2" mutation="${3:-}"; (cd "$root" && AURUMCODE_ROOT="$root" AURUM_A411_MUTATION="$mutation" AURUM_A411_INSTALL_LOG="$install_log" AURUM_A411_AVAILABILITY_LOG="$availability_log" GOPROXY=off GOSUMDB=off GONOSUMDB='*' GOTOOLCHAIN=local GOMAXPROCS=1 GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" go test -v -mod=readonly -p 1 "./$pkg" -run '^TestAUR411Bridge$' -count=1 2>&1); }
go_case(){
  local label="$1" pkg="$2" fn="$3" src="$4" out rc code
  local root="$run_dir/root-$label"
  common "$root"; copy "$root" "$src"
  printf 'package %s\nimport "testing"\nfunc TestAUR411Bridge(t *testing.T){ %s(t) }\n' "${pkg##*/}" "$fn" >"$root/$pkg/aur411_bridge_test.go"
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
  grep -Eq '^=== RUN[[:space:]]+TestAUR411Bridge$' <<<"$out" || fail "selector:$label:not-executed"
  grep -Eq '^--- PASS:[[:space:]]+TestAUR411Bridge[[:space:]]' <<<"$out" || fail "selector:$label:not-passed"
  rm -rf -- "$root"
}
e2e_case(){ local root="$run_dir/root-e2e"; common "$root"; copy "$root" tests/e2e/AUR-411.sh; (cd "$root" && bash tests/e2e/AUR-411.sh E2EAUR411) || fail selector:E2EAUR411; rm -rf -- "$root"; }
# Compatibility regression. Registering an eleventh key must leave the canonical registry
# loader delivered by AUR-402 green, so its acceptance is re-executed here against the
# extended registry. The AUR-403 to AUR-410 loaders are extended in the same commit and
# re-proved by their own acceptances, whose profile documents this card does not
# materialize.
aur402_case(){ (cd "$repo_root" && AURUM_A402_EMBEDDED_LAYERS=1 AURUM_A402_CACHE_ROOT="$run_dir/cache" AURUM_A402_TMP_ROOT="$run_dir/gotmp" bash tests/acceptance/AUR-402.sh AC-001) || fail compatibility:AUR402; }
mutation_case(){
  local mutation="$1" expected="${2:-$1}" out rc
  local root="$run_dir/root-mut-$mutation"
  common "$root"; copy "$root" tests/unit/AUR-411.go
  printf 'package unit\nimport "testing"\nfunc TestAUR411Bridge(t *testing.T){ TestAUR411(t) }\n' >"$root/tests/unit/aur411_bridge_test.go"
  set +e; out="$(gorun "$root" tests/unit "$mutation")"; rc=$?; set -e
  printf '%s\n' "$out"
  (( rc == 0 )) || fail "mutation:$mutation:exit:$rc"
  grep -Fq "mutation=$mutation code=$expected" <<<"$out" || fail "mutation:$mutation:not-rejected"
  rm -rf -- "$root"
}
run_all(){
  measure_availability
  install_ledger_selftest
  go_case unit tests/unit TestAUR411 tests/unit/AUR-411.go
  go_case contract tests/contracts ContractAUR411 tests/contracts/AUR-411.go
  go_case integration tests/integration IntegrationAUR411 tests/integration/AUR-411.go
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
  mutation_case toolchain-install-allowed unsafe-plan
  mutation_case toolchain-bootstrap-allowed unsafe-plan
  mutation_case materialization-on-demand unsafe-plan
  mutation_case package-install-allowed unsafe-plan
  mutation_case package-registry-allowed unsafe-plan
  mutation_case package-installs-nonzero unsafe-plan
  mutation_case telemetry-enabled unsafe-plan
  mutation_case partition-telemetry-enabled unsafe-plan
  mutation_case cache-writable unsafe-plan
  mutation_case partition-cache-writable unsafe-plan
  mutation_case host-toolchain-allowed unsafe-plan
  mutation_case host-cache-allowed unsafe-plan
  mutation_case host-filesystem-allowed unsafe-plan
  mutation_case subprocess-allowed unsafe-plan
  mutation_case unregistered-language-allowed unsafe-plan
  mutation_case language-list-widened unsafe-plan
  mutation_case language-list-shrunk unsafe-plan
  mutation_case language-list-unsorted unsafe-plan
  mutation_case language-list-duplicated unsafe-plan
  mutation_case language-list-empty unsafe-plan
  mutation_case language-partition-mismatch unsafe-plan
  mutation_case shared-runtime unsafe-plan
  mutation_case shared-cache-root unsafe-plan
  mutation_case runtime-remapped unsafe-plan
  mutation_case polyglot-root-host unsafe-plan
  mutation_case polyglot-root-traversal unsafe-plan
  mutation_case polyglot-root-trailing-slash unsafe-plan
  mutation_case polyglot-root-lookalike unsafe-plan
  mutation_case polyglot-root-newline unsafe-plan
  mutation_case polyglot-root-trailing-newline unsafe-plan
  mutation_case polyglot-root-leading-space unsafe-plan
  mutation_case polyglot-root-uppercase unsafe-plan
  mutation_case polyglot-root-homoglyph unsafe-plan
  mutation_case polyglot-root-empty unsafe-plan
  mutation_case cache-root-escape unsafe-plan
  mutation_case cache-root-traversal unsafe-plan
  mutation_case cache-root-trailing-slash unsafe-plan
  mutation_case cache-root-lookalike unsafe-plan
  mutation_case cache-root-newline unsafe-plan
  mutation_case cache-root-trailing-newline unsafe-plan
  mutation_case cache-root-leading-space unsafe-plan
  mutation_case cache-root-uppercase unsafe-plan
  mutation_case cache-root-homoglyph unsafe-plan
  mutation_case cache-root-empty unsafe-plan
  mutation_case cache-root-outside-polyglot-root unsafe-plan
  mutation_case unbounded-cache unsafe-plan
  mutation_case oversized-cache unsafe-plan
  mutation_case aggregate-cache-overflow unsafe-plan
  mutation_case oversized-package-set unsafe-plan
  mutation_case unbounded-package-bytes unsafe-plan
  mutation_case unbounded-resolve-timeout unsafe-plan
  mutation_case resolve-timeout-exceeds-run unsafe-plan
  mutation_case max-languages-lowered unsafe-plan
  mutation_case unlimited-tmpfs unsafe-plan
  mutation_case pip-index-reopened unsafe-plan
  mutation_case npm-offline-disabled unsafe-plan
  mutation_case npm-registry-reopened unsafe-plan
  mutation_case nuget-cache-escape unsafe-plan
  mutation_case dotnet-telemetry-reopened unsafe-plan
  mutation_case cargo-offline-disabled unsafe-plan
  mutation_case cargo-home-escape unsafe-plan
  mutation_case powershell-telemetry-reopened unsafe-plan
  mutation_case powershell-updatecheck-reopened unsafe-plan
  mutation_case polyglot-root-environment unsafe-plan
  mutation_case proxy-environment unsafe-plan
  mutation_case no-proxy-environment unsafe-plan
  mutation_case cgo-environment unsafe-plan
  mutation_case unpinned-toolchain-source digest-invalid
  mutation_case mutable-toolchain-source digest-invalid
  mutation_case unpinned-partition-toolchain digest-invalid
  mutation_case missing-pids-limit schema-invalid
  mutation_case missing-resolve-timeout schema-invalid
  mutation_case missing-languages schema-invalid
  mutation_case missing-toolchains schema-invalid
  mutation_case real-credential credential-present
  install_ledger_assert_zero
  printf '{"card":"%s","scenario":"%s","result":"valid","package_installs":0,"package_installs_measured":%s,"toolchain_install":"denied","languages":8}\n' "$card" "$scenario" "$(install_ledger_count)"
}
case "$selector" in
 AC-001) run_all;;
 TestAUR411) measure_availability; install_ledger_selftest; go_case unit tests/unit TestAUR411 tests/unit/AUR-411.go; install_ledger_assert_zero;;
 ContractAUR411) go_case contract tests/contracts ContractAUR411 tests/contracts/AUR-411.go;;
 IntegrationAUR411) go_case integration tests/integration IntegrationAUR411 tests/integration/AUR-411.go;;
 E2EAUR411) e2e_case;;
 AC-001-MUT-001) mutation_case duplicate-profile;;
 AC-001-MUT-002) mutation_case mutable-input;;
 AC-001-MUT-003) mutation_case unsafe-plan;;
esac
