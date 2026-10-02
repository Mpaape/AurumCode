#!/usr/bin/env bash
# AUR-533 acceptance: the analysis-data artifact is built, tested and
# published by automation; the runtime verifies digest and date and refuses
# data older than the policy's maximum age. GitHub and the public OSV source
# are local fake servers (internal/artifacts/artifacts_test.go); no real
# network.
#
# Selectors:
#   all              every AC below plus the mutation
#   AC-001           scheduled workflow; artifact built, tested, published only
#                    after the tests pass; a failing test publishes nothing
#   AC-002           fresh artifact is used; digest and date exposed for the
#                    audit record
#   AC-003           artifact above max age is unusable with its reason; the
#                    policy section (default, repo, central precedence)
#   AC-004           tampered payload/manifest digest is refused
#   AC-005           an ecosystem new in the source enters the next artifact
#                    with no code change
#   AC-003-MUT-001   ignoring the artifact age (applied to a copy) turns the
#                    AC-003 test RED
# Unknown selectors exit 64; infrastructure failures 79; behavior failures 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-533'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-003-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

inputs=(go.mod go.sum internal/artifacts scripts/artifacts .github/workflows/analysis-data.yml)
for input in "${inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a533.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp"
seed_root() {
  rm -rf "$run_dir/root"
  mkdir -p "$run_dir/root/internal" "$run_dir/root/scripts" "$run_dir/root/.github/workflows"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$run_dir/root/"
  cp -R "$repo_root/internal/artifacts" "$run_dir/root/internal/artifacts"
  cp -R "$repo_root/scripts/artifacts" "$run_dir/root/scripts/artifacts"
  cp "$repo_root/.github/workflows/analysis-data.yml" "$run_dir/root/.github/workflows/"
  chmod -R u+w -- "$run_dir/root"
}
seed_root

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# run_tests <regex> <log>: go test of the (possibly mutated) package copy.
run_tests() {
  set +e
  (cd "$run_dir/root" && go vet ./internal/artifacts/... && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./internal/artifacts/... -run "$1") >"$2" 2>&1
  local status=$?
  set -e
  cat "$2" >&2
  return $status
}

# require_pass <log> <TestName>...: every named test PASSed (not skipped).
require_pass() {
  local log="$1"; shift
  local name
  for name in "$@"; do
    grep -q "^--- PASS: $name " "$log" || fail "missing-pass:$name"
  done
}

ac_tests() {
  case "$1" in
    AC-001) echo TestAUR533BuildProducesVerifiableManifest TestAUR533BuildFailsClosedOnBadSource TestAUR533PassingTestsPublishAndFailingTestsDoNot TestAUR533PublishRefusesUntestedOrChangedArtifact TestAUR533WorkflowOrderingAndPins TestAUR533GeneratedArtifactStepRunsAgainstFreshBuild ;;
    AC-002) echo TestAUR533ResolveUsesFreshArtifactAndExposesAudit ;;
    AC-003) echo TestAUR533ResolveRefusesStaleArtifact TestAUR533ResolveOfflineIsInconclusive TestAUR533ResolveRejectsBadInputs TestAUR533PolicyPrecedenceAndDefaults ;;
    AC-004) echo TestAUR533ResolveRefusesDigestMismatch ;;
    AC-005) echo TestAUR533NewEcosystemEntersNextArtifact ;;
  esac
}

pattern_for() { local IFS='|'; local names=($(ac_tests "$1")); echo "^(${names[*]})\$"; }

check_ac() {
  local ac="$1" log="$run_dir/$1.log" names
  read -r -a names <<<"$(ac_tests "$ac")"
  run_tests "$(pattern_for "$ac")" "$log" || fail "go-test-exit:$ac"
  require_pass "$log" "${names[@]}"
}

# AC-003-MUT-001: the artifact's age is compared in exactly one line of
# client.go (anchored by its comment). Neutralize that comparison so a stale
# artifact is used, then the AC-003 stale test must go RED.
apply_mutation_ignore_age() {
  local target="$run_dir/root/internal/artifacts/client.go"
  local anchor='if age > maxAge { // AUR-533 AC-003: the age limit'
  [[ "$(grep -Fc "$anchor" "$target")" == "1" ]] || infra mutation-anchor-not-unique
  local line
  line="$(grep -Fn "$anchor" "$target" | head -1 | cut -d: -f1)"
  sed -i "${line}s/.*/\t_ = maxAge \/\/ MUT-001: age ignored\n\tif false {/" "$target"
  grep -Fq 'MUT-001: age ignored' "$target" || infra mutation-not-applied
}

check_mutation_red() {
  local log="$run_dir/mutation.log"
  apply_mutation_ignore_age
  run_tests '^TestAUR533ResolveRefusesStaleArtifact$' "$log" || true
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  grep -Eq -- '^--- FAIL: TestAUR533ResolveRefusesStaleArtifact' "$log" || fail 'mutation-survived'
}

case "$selector" in
  AC-001|AC-002|AC-003|AC-004|AC-005)
    check_ac "$selector"
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
  AC-003-MUT-001)
    check_mutation_red
    printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
    ;;
  all)
    all_log="$run_dir/all.log"
    run_tests '^TestAUR533' "$all_log" || fail 'go-test-exit:all'
    for ac in AC-001 AC-002 AC-003 AC-004 AC-005; do
      read -r -a names <<<"$(ac_tests "$ac")"
      require_pass "$all_log" "${names[@]}"
    done
    check_mutation_red
    printf '%s/%s/pass\n' "$card" "$selector"
    ;;
esac
