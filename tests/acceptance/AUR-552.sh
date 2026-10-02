#!/usr/bin/env bash
# AUR-552 acceptance: `aurumcode xbom --type build|cbom` generates CycloneDX
# 1.6 BOMs whose every component cites file and line, drops components
# without verifiable evidence, and documents the remaining types.
#
# Selectors:
#   all             every behavior test below, then the mutation
#   AC-001          Build BOM and CBOM validate and cite file and line
#   AC-002          a component without evidence does not enter the BOM
#   AC-003          aibom/saasbom/netbom exit 2 pointing at the doc section,
#                   and the doc defines format and delivery of each
#   AC-002-MUT-001  removing the evidence check (lineCitesToken -> true) in a
#                   copy must turn AC-002's tests RED
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-552'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-002-MUT-001) ;;
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
  cmd/aurumcode/aur552.go \
  internal/xbom/evidence.go \
  internal/xbom/catalog/build.yml \
  internal/xbom/catalog/cbom.yml; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in cmd/aurumcode/aur552_test.go internal/xbom/xbom_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a552.XXXXXX")" || infra mktemp
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

run_go_test() {
  local pkgs="$1" pattern="$2" log="$3"
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$pattern") >"$log" 2>&1
  local status=$?
  set -e
  cat "$log" >&2
  return $status
}

need_pass() {
  local log="$1"; shift
  local name
  for name in "$@"; do
    grep -q -- "^--- PASS: $name " "$log" || fail "missing-pass:$name"
  done
}

# AC-002-MUT-001: lineCitesToken is one anchored line. Replacing its body
# with "return true" accepts any component whatever the cited line says --
# the defect AC-002 exists to refuse.
apply_mutation() {
  local target="$run_dir/root/internal/xbom/evidence.go"
  local anchor='AUR-552 AC-002: evidence check'
  grep -Fq -- "$anchor" "$target" || infra mutation-anchor-missing
  [[ "$(grep -Fc -- "$anchor" "$target")" == "1" ]] || infra mutation-anchor-not-unique
  local line
  line="$(grep -Fn -- "$anchor" "$target" | head -1 | cut -d: -f1)"
  sed -i "${line}s/.*/\treturn true \/\/ AUR-552 MUT-001: evidence check removed/" "$target"
  sed -n "${line}p" "$target" | grep -Fq 'MUT-001: evidence check removed' || infra mutation-not-applied
}

ac001_cmd='^TestAUR552BuildAndCBOMValidateAndCiteFileAndLine$'
ac002_cmd='^TestAUR552ComponentWithoutEvidenceDoesNotEnterBOM$'
ac002_unit='^(TestVerifyDropsComponentWithoutEvidence|TestLLMEnrichesButCannotInvent|TestLLMAdditionalTokenIsDerivedFromName|TestLLMAdditionalSameKeyCannotAddFalseOccurrence|TestVerifyWithRelativeRoot)$'
ac001_names=(TestAUR552BuildAndCBOMValidateAndCiteFileAndLine)
ac002_names=(TestAUR552ComponentWithoutEvidenceDoesNotEnterBOM TestVerifyDropsComponentWithoutEvidence TestLLMEnrichesButCannotInvent)

check_ac003_docs() {
  local doc="$repo_root/docs/configuration.md" spec="$repo_root/docs/specs/AUR-552.md"
  [[ -f "$doc" && -f "$spec" ]] || infra missing-docs
  local t
  for t in aibom saasbom netbom; do
    grep -Fq "id=\"xbom-$t\"" "$doc" || fail "doc-anchor-missing:xbom-$t"
  done
  grep -Fq 'CycloneDX 1.6' "$doc" || fail 'doc-missing-cyclonedx-1.6'
  grep -Fq '/api/v1/bom' "$doc" || fail 'doc-missing-delivery'
  grep -Fq 'machine-learning-model' "$doc" || fail 'doc-missing-aibom-format'
  grep -Fq 'services' "$doc" || fail 'doc-missing-services-format'
  grep -Fq 'x-trust-boundary' "$doc" || fail 'doc-missing-netbom-format'
}

ac003_run() {
  local log="$run_dir/ac003.log"
  run_go_test ./cmd/aurumcode/... '^TestAUR552DocumentedAndUnknownTypesAndNoPartialFile$' "$log" \
    || fail "go-test-exit"
  need_pass "$log" TestAUR552DocumentedAndUnknownTypesAndNoPartialFile
  check_ac003_docs
}

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test ./cmd/aurumcode/... "$ac001_cmd" "$log" || fail go-test-exit
    need_pass "$log" "${ac001_names[@]}"
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test ./cmd/aurumcode/... "$ac002_cmd" "$log" || fail go-test-exit
    run_go_test ./internal/xbom/... "$ac002_unit" "$run_dir/unit.log" || fail go-test-exit-unit
    cat "$run_dir/unit.log" >>"$log"
    need_pass "$log" "${ac002_names[@]}"
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    ac003_run
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002-MUT-001)
    apply_mutation
    log="$run_dir/mutation.log"
    run_go_test ./cmd/aurumcode/... "$ac002_cmd" "$log" || true
    run_go_test ./internal/xbom/... "$ac002_unit" "$run_dir/mutation_unit.log" || true
    cat "$run_dir/mutation_unit.log" >>"$log"
    if grep -Eq 'build failed|cannot use|undefined:|syntax error|\[build failed\]' "$log"; then
      fail mutation-build-failure-not-behavioral
    fi
    grep -Eq -- '^--- FAIL: TestAUR552ComponentWithoutEvidenceDoesNotEnterBOM' "$log" || fail 'mutation-survived:e2e'
    grep -Eq -- '^--- FAIL: TestVerifyDropsComponentWithoutEvidence' "$log" || fail 'mutation-survived:unit'
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test './internal/xbom/... ./cmd/aurumcode/...' '^(TestAUR552|TestEmbeddedCatalogsAreValid|TestInvalidCatalogRefused|TestBuildExtractionWithEvidence|TestCBOMExtraction|TestVerify|TestLLM|TestCatalogPrecedence|TestPromptPrecedence|TestValidateBOM)' "$log" || fail go-test-exit
    need_pass "$log" \
      TestAUR552BuildAndCBOMValidateAndCiteFileAndLine TestAUR552ComponentWithoutEvidenceDoesNotEnterBOM \
      TestAUR552DocumentedAndUnknownTypesAndNoPartialFile TestAUR552PolicyCatalogOverridesRepository \
      TestEmbeddedCatalogsAreValid TestInvalidCatalogRefused TestBuildExtractionWithEvidence TestCBOMExtraction \
      TestVerifyDropsComponentWithoutEvidence TestLLMEnrichesButCannotInvent TestLLMFailureKeepsDeterministicBOM \
      TestLLMKeepFalseExcludes TestLLMAdditionalTokenIsDerivedFromName TestLLMAdditionalSameKeyCannotAddFalseOccurrence TestLLMCannotRewriteCandidateIdentity TestCatalogPrecedence TestPromptPrecedence TestValidateBOMRejectsEvidencelessComponent \
      TestVerifyWithRelativeRoot
    check_ac003_docs

    seed_root
    apply_mutation
    mlog="$run_dir/mutation.log"
    run_go_test ./cmd/aurumcode/... "$ac002_cmd" "$mlog" || true
    run_go_test ./internal/xbom/... "$ac002_unit" "$run_dir/mutation_unit.log" || true
    cat "$run_dir/mutation_unit.log" >>"$mlog"
    if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$mlog"; then
      fail mutation-build-failure-not-behavioral
    fi
    grep -Eq -- '^--- FAIL: TestAUR552ComponentWithoutEvidenceDoesNotEnterBOM' "$mlog" || fail 'mutation-survived:e2e'
    grep -Eq -- '^--- FAIL: TestVerifyDropsComponentWithoutEvidence' "$mlog" || fail 'mutation-survived:unit'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
