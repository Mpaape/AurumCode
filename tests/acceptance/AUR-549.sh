#!/usr/bin/env bash
# AUR-549 acceptance: `aurumcode sbom` generates an OWASP CycloneDX SBOM
# with Trivy (`trivy fs --format cyclonedx --output <file> <repo>`, and
# `trivy image --format cyclonedx --output <file> <image>` when an image is
# given), validates the result (JSON, bomFormat=CycloneDX, specVersion AT
# LEAST the configured one, same major -- card v3: the digest-pinned Trivy
# 0.73.0 emits CycloneDX 1.7 with no flag to request an older spec version,
# so spec_version in config is a floor, never an exact match) before ever
# writing output_file, and routes any failure/absence of Trivy through the
# AUR-519 policy gate's gate.inconclusive, never accepting an empty or
# invalid SBOM. See docs/specs/AUR-549.md for the full account.
#
# Selectors:
#   all             run every behavior test below, then apply the AC-002
#                   mutation and confirm AC-002's own tests go RED
#   AC-001          fake trivy producing the REAL pinned Trivy's output
#                   (CycloneDX 1.7, not the configured minimum 1.6) writes
#                   the configured, validated file, invoked as
#                   `trivy fs --format cyclonedx --output <file> <repo>`
#   AC-002          output that is not CycloneDX, or whose specVersion is
#                   below the configured minimum or has a different major,
#                   is refused and no file is left at output_file (covers:
#                   specVersion below the floor, a different major, wrong
#                   bomFormat, non-JSON output, empty output)
#   AC-003          trivy absent or erroring follows gate.inconclusive:
#                   no gate/`block` fails closed (exitQualityNotReviewed),
#                   `warn` publishes the reason and exits 0
#   AC-004          with --imagem given, the image SBOM is generated into
#                   its own separate file from the repository's
#   AC-002-MUT-001  bypass the bomFormat/specVersion check in
#                   internal/sbom/validator.go (the exact defect AC-002
#                   exists to catch: accepting a non-CycloneDX/too-old/
#                   different-major output); AC-002's own tests must go RED
# A build failure during the mutation run is infrastructure, never a
# silently-passing mutation. Unknown selectors exit 64; infrastructure
# failures exit 79; behavioral failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-549'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for source in \
  cmd/aurumcode/aur549.go \
  cmd/aurumcode/policygate.go \
  internal/sbom/generator.go \
  internal/sbom/validator.go \
  internal/sbom/pipeline.go \
  internal/sbom/config.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in cmd/aurumcode/aur549_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a549.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
seed_root() {
  rm -rf "$run_dir/root"
  mkdir -p "$run_dir/root"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$run_dir/root/$source"
  done
  chmod -R u+w -- "$run_dir/root"
}
seed_root

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# AC-002-MUT-001: bypass the one line that decides bomFormat/specVersion
# (formatAndVersionOK, internal/sbom/validator.go) -- the exact defect this
# card's validator exists to catch. Anchored on the single, unique
# decision line (card v3: a minimum-version comparison via
# specVersionAtLeast, not an exact match); sed replaces it with an
# unconditional "return true".
apply_mutation_validator() {
  local target="$run_dir/root/internal/sbom/validator.go"
  local anchor='return bom.BOMFormat == cycloneDXFormat && specVersionAtLeast(bom.SpecVersion, wantSpecVersion)'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor}|return true|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  grep -Fq 'return true' "$target" || infra mutation-not-applied
  return 0
}

run_go_test() {
  local pattern="$1" log="$2"
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./cmd/aurumcode/... -run "$pattern") >"$log" 2>&1
  local status=$?
  set -e
  cat "$log" >&2
  return $status
}

check_mutation_red() {
  local log="$1" name="$2"
  grep -Eq -- "^--- FAIL: TestAUR549${name}" "$log" || fail 'mutation-survived'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

ac001_pattern='^TestAUR549TrivyGeneratesValidatedSBOM$'
ac002_pattern='^TestAUR549NonCycloneDXOutputRejected$'
ac003_pattern='^TestAUR549MissingTrivyIsInconclusive$'
ac004_pattern='^TestAUR549ImageProducesSeparateFile$'

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test "$ac001_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR549TrivyGeneratesValidatedSBOM " "$log" || fail 'missing-pass:TrivyGeneratesValidatedSBOM'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test "$ac002_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR549NonCycloneDXOutputRejected " "$log" || fail 'missing-pass:NonCycloneDXOutputRejected'
    for sub in badversion othermajor wrongformat notjson empty; do
      grep -q "^    --- PASS: TestAUR549NonCycloneDXOutputRejected/${sub} " "$log" || fail "missing-pass:${sub}"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    log="$run_dir/test.log"
    run_go_test "$ac003_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR549MissingTrivyIsInconclusive " "$log" || fail 'missing-pass:MissingTrivyIsInconclusive'
    for sub in NoGateFailsClosed GateBlockFailsClosed GateWarnNeverBlocks; do
      grep -q "^    --- PASS: TestAUR549MissingTrivyIsInconclusive/${sub} " "$log" || fail "missing-pass:${sub}"
    done
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-004)
    log="$run_dir/test.log"
    run_go_test "$ac004_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR549ImageProducesSeparateFile " "$log" || fail 'missing-pass:ImageProducesSeparateFile'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002-MUT-001)
    log="$run_dir/mutation.log"
    apply_mutation_validator
    run_go_test "$ac002_pattern" "$log" || true
    check_mutation_red "$log" 'NonCycloneDXOutputRejected'
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test '^TestAUR549' "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in \
      TrivyGeneratesValidatedSBOM NonCycloneDXOutputRejected \
      MissingTrivyIsInconclusive ImageProducesSeparateFile; do
      grep -q "^--- PASS: TestAUR549$name " "$log" || fail "missing-pass:$name"
    done

    mutlog="$run_dir/mutation.log"
    apply_mutation_validator
    run_go_test "$ac002_pattern" "$mutlog" || true
    check_mutation_red "$mutlog" 'NonCycloneDXOutputRejected'

    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
