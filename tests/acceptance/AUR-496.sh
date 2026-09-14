#!/usr/bin/env bash
# AUR-496 acceptance: the deterministic scanner only flags additions, file
# permission findings require an actual write-for-others mode (not any
# nearby digit), comments are never mistaken for credential assignments,
# and the reusable review workflow checks out the reviewed PR into its own
# read-only mount instead of reusing AurumCode's own checkout.
#
# Exit: 0 green; 1 behavioural failure; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077
readonly card='AUR-496'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac
if [[ "$selector" == all ]]; then
  for scenario in AC-001 AC-002 AC-003 AC-004 MUT-001 MUT-002; do
    bash "${BASH_SOURCE[0]}" "$scenario"
  done
  exit 0
fi
scenario="$selector"
fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum internal/analysis pkg/types .github/workflows/review.yml docs/review-quality.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a496.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="${GOCACHE:-$run_dir/gocache}" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir" GOMAXPROCS=1
export XDG_CACHE_HOME="$run_dir/xdg" HOME="$run_dir/home"

# Whole top-level trees, never a per-package list: in a sealed run only the
# declared inputs exist, so this copies exactly what the card materializes.
root="$run_dir/root"; mkdir -p "$root"
for top in go.mod go.sum internal pkg; do
  [[ -e "$repo_root/$top" ]] || continue
  cp -R "$repo_root/$top" "$root/$top"
done
chmod -R u+w -- "$root"

# gotest PKG SELECTOR -> 0 when the named test PASSES, 1 when it FAILS,
# 79 when the package does not build.
gotest() {
  local pkg="$1" sel="$2"
  local log="$run_dir/$sel.log" rc=0
  (cd "$root" && go test "./$pkg/" -run "^${sel}\$" -count=1 -timeout=60s -v) >"$log" 2>&1 || rc=$?
  grep -q 'build failed\|cannot find package\|setup failed' "$log" && { cat "$log" >&2; return 79; }
  if ((rc == 0)) && grep -q -- "--- PASS: $sel" "$log"; then return 0; fi
  cat "$log" >&2
  return 1
}
expect_pass() { local rc=0; gotest "$1" "$2" || rc=$?; ((rc == 79)) && infra "build_failed:$2"; ((rc == 0)) || fail "selector-failed:$2"; }
expect_fail() { local rc=0; gotest "$1" "$2" || rc=$?; ((rc == 79)) && infra "build_failed:$2"; ((rc == 1)) || fail "mutation-not-detected:$2"; }

workflow="$repo_root/.github/workflows/review.yml"

case "$selector" in
  AC-001)
    expect_pass internal/analysis TestAnalyzeTable
    ;;
  AC-002)
    expect_pass internal/analysis TestAUR496FilePermissions
    expect_pass internal/analysis TestAUR496CommentsNotCredentials
    expect_pass internal/analysis TestAUR489SecretNaming
    ;;
  AC-003)
    grep -Fq 'path: .aurumcode-target' "$workflow" || fail missing-separate-checkout
    grep -Fq 'persist-credentials: false' "$workflow" || fail credentials-persisted
    grep -Fq '/.aurumcode-target:/github/workspace:ro' "$workflow" || fail workspace-not-readonly
    grep -Fq -- '-w /github/workspace' "$workflow" || fail workdir-not-set
    if grep -Eq 'npm (ci|install)|go (build|test)|make ' "$workflow"; then fail runs-pr-code; fi
    ;;
  AC-004)
    doc="$repo_root/docs/review-quality.md"
    grep -qi 'heur' "$doc" || fail 'docs-missing:heuristic-language'
    grep -qi 'nao afirma superioridade\|não afirma superioridade' "$doc" || fail 'docs-missing:no-superiority-claim'
    ;;
  MUT-001)
    # Restoring the LEFT match must make the removed-secret AC fail again.
    sed -i 's|case "-":|case "-":\n\t\t\t\t\tfindings = append(findings, r.match(file.Path, oldLine, SideLeft, body)...)|' "$root/internal/analysis/analysis.go"
    grep -q 'SideLeft, body' "$root/internal/analysis/analysis.go" || infra 'mutation-anchor-missing:MUT-001'
    expect_fail internal/analysis TestAnalyzeTable
    ;;
  MUT-002)
    staged="$run_dir/workflow-mutation"
    mkdir -p "$staged/.github/workflows"
    sed 's#/.aurumcode-target:/github/workspace:ro#/.aurumcode-target:/github/workspace#' "$workflow" > "$staged/.github/workflows/review.yml"
    grep -Fq '/.aurumcode-target:/github/workspace:ro' "$staged/.github/workflows/review.yml" && infra 'mutation-anchor-missing:MUT-002'
    if grep -Fq '/.aurumcode-target:/github/workspace"' "$staged/.github/workflows/review.yml"; then
      : # mutation applied: workspace is now mounted read-write
    else
      infra 'mutation-not-applied:MUT-002'
    fi
    ;;
esac
printf '%s/%s/pass\n' "$card" "$scenario"
