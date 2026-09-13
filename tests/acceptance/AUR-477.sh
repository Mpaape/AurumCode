#!/usr/bin/env bash
# AUR-477 acceptance: the coverage-declaration reservation tracks what is
# actually declared, not the total file count, so a large diff still receives
# a review instead of refusing with review zero. Each AC runs a real _test.go
# selector through go test on a staged copy of the module; MUT selectors apply
# the card's mutation to the staged copy and pass only when the selector FAILS
# -- a mutation nobody notices is a test that asserts nothing.
#
# Exit: 0 green; 1 behavioural failure; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077
ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB
readonly card='AUR-477'
selector="${1:-all}"
if [[ "$selector" == all ]]; then
  for scenario in AC-001 AC-002 AC-003 IntegrationAUR477 E2EAUR477; do
    bash "${BASH_SOURCE[0]}" "$scenario"
  done
  exit 0
fi
scenario="$selector"
case "$selector" in
  AC-001|AC-002|AC-003|MUT-001|MUT-002|TestAUR477|IntegrationAUR477|E2EAUR477) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac
fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum internal pkg tests; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a477.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir" GOMAXPROCS=1
export XDG_CACHE_HOME="$run_dir/xdg" HOME="$run_dir/home"
unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

root="$run_dir/root"; mkdir -p "$root"
for top in go.mod go.sum cmd internal pkg tests; do
  [[ -e "$repo_root/$top" ]] || continue
  cp -R "$repo_root/$top" "$root/$top"
done
chmod -R u+w -- "$root"

cat >"$root/tests/unit/aur477_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR477UnitBridge(t *testing.T) { TestAUR477(t) }
EOF
cat >"$root/tests/integration/aur477_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR477IntegrationBridge(t *testing.T) { IntegrationAUR477(t) }
EOF

gotest_unit() {
  (cd "$root" && go test -mod=mod -p 1 -timeout 300s ./tests/unit -run '^TestAUR477UnitBridge$' -count=1)
}
gotest_integration() {
  (cd "$root" && go test -mod=mod -p 1 -timeout 300s ./tests/integration -run '^TestAUR477IntegrationBridge$' -count=1)
}

case "$selector" in
  AC-001|AC-002|AC-003|TestAUR477)
    gotest_unit || fail unit-failed
    ;;
  IntegrationAUR477)
    gotest_integration || fail integration-failed
    ;;
  E2EAUR477)
    (cd "$repo_root" && bash tests/e2e/AUR-477.sh E2EAUR477) || fail e2e-failed
    ;;
  MUT-001)
    # Revert to dimensioning the reservation by the TOTAL file count: unbounded
    # the omitted list so the declaration (and its reserve) names every file.
    sed -i 's/maxOmittedBullets = 20/maxOmittedBullets = 1000000000/' "$root/internal/prompt/coverage.go"
    grep -q 'maxOmittedBullets = 1000000000' "$root/internal/prompt/coverage.go" || infra 'mutation-anchor-missing:MUT-001'
    if gotest_unit 2>/dev/null; then fail 'mutation-not-detected:MUT-001'; fi
    ;;
  MUT-002)
    # Exclude the coverage declaration from the ceiling measurement: stop the
    # reservation from growing to cover it, so the assembled prompt overshoots.
    sed -i 's/^\t\treserve += total - opts.MaxTokens$/\t\tbreak/' "$root/internal/prompt/builder.go"
    grep -q 'reserve += total - opts.MaxTokens' "$root/internal/prompt/builder.go" && infra 'mutation-anchor-missing:MUT-002'
    if gotest_unit 2>/dev/null; then fail 'mutation-not-detected:MUT-002'; fi
    ;;
esac
printf '{"card":"%s","scenario":"%s","result":"pass"}\n' "$card" "$scenario"
