#!/usr/bin/env bash
# AUR-551 acceptance: `aurumcode sign` signs the SBOM (AUR-549's own
# output) and/or the artifact image with Sigstore/Cosign, shelling out to
# an injectable `cosign` binary (a local fake in every test below -- no
# network, no real cosign required to run this script). See
# docs/specs/AUR-551.md for the real cosign proof (container, ephemeral
# key, offline sign-blob/verify-blob) this script does not attempt to
# reproduce.
#
# Selectors:
#   all             run every behavior test below, then apply the
#                   skeptical mutation and confirm it turns AC-002 red
#   AC-001          with signing on, the SBOM is signed (a verifiable
#                   bundle is written) and the configured image is signed,
#                   in that order, with no gate.inconclusive softening
#   AC-002          a signing failure fails the command (non-zero exit)
#                   and names the artifact left unsigned on stderr
#   AC-003          with no quality_gates.supply_chain declared, the
#                   command is a no-op (exit 0, cosign never invoked)
#   AC-002-MUT-001  swallowing a signing failure (the exact defect this
#                   card refuses to allow) must turn AC-002's own test RED
# A build failure during the mutation run is infrastructure, never a
# silently-passing mutation. Unknown selectors exit 64; infrastructure
# failures exit 79; behavioral failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-551'
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
  cmd/aurumcode/aur551.go \
  cmd/aurumcode/aur549.go \
  cmd/aurumcode/main.go \
  internal/supplychain/signer.go \
  internal/config/qualitygates.go \
  internal/config/central.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in cmd/aurumcode/aur551_test.go internal/supplychain/signer_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a551.XXXXXX")" || infra mktemp
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

# internal/supplychain and internal/config carry their own, package-level
# proof of this card's rules (digest-pinning, engine vocabulary,
# per-section central-policy precedence) that cmd/aurumcode's own
# TestAUR551* names never exercise directly. Every selector below runs
# this once, unconditionally, so a regression there can never pass this
# script just because the cmd-level table above it still does.
run_package_proof() {
  local log="$run_dir/package_proof.log"
  set +e
  (cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 120s ./internal/supplychain/... ./internal/config/...) >"$log" 2>&1
  local status=$?
  set -e
  cat "$log" >&2
  (( status == 0 )) || fail "package-proof-exit:$status"
}
run_package_proof

# AC-002-MUT-001: cmd/aurumcode/aur551.go has exactly one place EACH of
# the SBOM and the image signing failure's own return statements appear
# (unique in the file on purpose -- see aur551.go's own comments at those
# lines). Dropping either return (while still printing the error) treats
# that branch's own "cosign failed" as non-fatal -- the exact defect this
# card exists to refuse: AC-002 says a signing failure must
# unconditionally fail the command, for the SBOM branch AND the image
# branch alike. mutate_one_anchor applies the same swallow to one named
# anchor; apply_mutation_swallow_failure applies it to BOTH, in the same
# pass, so a single mutation run exercises both of AC-002's own tests.
mutate_one_anchor() {
  local target="$1" anchor="$2" replacement="$3"
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  local count
  count="$(grep -Fc "$anchor" "$target")"
  [[ "$count" == "1" ]] || infra mutation-anchor-not-unique
  local line
  line="$(grep -Fn "$anchor" "$target" | head -1 | cut -d: -f1)"
  [[ -n "$line" ]] || infra mutation-anchor-missing
  # Whole-line replacement by line number, not a pattern substitution:
  # the anchor's own slashes/quotes are plain text here, never a regex
  # this tool has to escape correctly.
  sed -i "${line}s/.*/${replacement}/" "$target"
  sed -n "${line}p" "$target" | grep -Fq "$anchor" && infra mutation-not-applied
  sed -n "${line}p" "$target" | grep -Fq 'MUT-001: failure swallowed' || infra mutation-not-applied
}

apply_mutation_swallow_failure() {
  local target="$run_dir/root/cmd/aurumcode/aur551.go"
  mutate_one_anchor "$target" \
    'return exitQualityNotReviewed // AUR-551 AC-002: sbom signing failure must never be swallowed' \
    '\t\t\t\t_ = signErr \/\/ AUR-551 MUT-001: failure swallowed'
  mutate_one_anchor "$target" \
    'return exitQualityNotReviewed // AUR-551 AC-002: image signing failure must never be swallowed' \
    '\t\t\t\t_ = signErr \/\/ AUR-551 MUT-001: failure swallowed'
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
  local log="$1"
  grep -Eq -- '^--- FAIL: TestAUR551SignFailureNamesArtifactAndFailsClosed' "$log" || fail 'mutation-survived:sbom'
  grep -Eq -- '^--- FAIL: TestAUR551ImageSignFailureNamesArtifactAndFailsClosed' "$log" || fail 'mutation-survived:image'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
}

ac001_pattern='^TestAUR551SignsSBOMAndImage$'
ac002_pattern='^(TestAUR551SignFailureNamesArtifactAndFailsClosed|TestAUR551ImageSignFailureNamesArtifactAndFailsClosed)$'
ac003_pattern='^TestAUR551NoSectionIsNoOp$'

case "$selector" in
  AC-001)
    log="$run_dir/test.log"
    run_go_test "$ac001_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR551SignsSBOMAndImage " "$log" || fail 'missing-pass:SignsSBOMAndImage'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002)
    log="$run_dir/test.log"
    run_go_test "$ac002_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR551SignFailureNamesArtifactAndFailsClosed " "$log" || fail 'missing-pass:SignFailureNamesArtifactAndFailsClosed'
    grep -q "^--- PASS: TestAUR551ImageSignFailureNamesArtifactAndFailsClosed " "$log" || fail 'missing-pass:ImageSignFailureNamesArtifactAndFailsClosed'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003)
    log="$run_dir/test.log"
    run_go_test "$ac003_pattern" "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    grep -q "^--- PASS: TestAUR551NoSectionIsNoOp " "$log" || fail 'missing-pass:NoSectionIsNoOp'
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-002-MUT-001)
    log="$run_dir/mutation.log"
    apply_mutation_swallow_failure
    run_go_test "$ac002_pattern" "$log" || true
    check_mutation_red "$log"
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    log="$run_dir/test.log"
    run_go_test '^TestAUR551' "$log"
    status=$?
    (( status == 0 )) || fail "go-test-exit:$status"
    for name in \
      SignsSBOMAndImage SignFailureNamesArtifactAndFailsClosed ImageSignFailureNamesArtifactAndFailsClosed NoSectionIsNoOp \
      EngineValidation UnpinnedArtifactRejectedAtConfigLoad SBOMFallsBackToSBOMGeneratorOutputFile \
      RejectsFlagLikeSBOMPath RejectsFlagLikeImageRef; do
      grep -q "^--- PASS: TestAUR551$name " "$log" || fail "missing-pass:$name"
    done

    mutlog="$run_dir/mutation.log"
    apply_mutation_swallow_failure
    run_go_test "$ac002_pattern" "$mutlog" || true
    check_mutation_red "$mutlog"

    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
