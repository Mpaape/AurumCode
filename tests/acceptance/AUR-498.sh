#!/usr/bin/env bash
# AUR-498 acceptance: the changelog engine classifies Conventional Commit
# subjects, computes the semver bump (including the 0.x breaking exception),
# renders a deterministic escaped Markdown entry, and treats commit text as
# untrusted bounded data.
#
# Selectors: all | AC-001 | AC-002 | AC-003 | AC-004 | MUT-001 | MUT-002 | MUT-003
# Exit: 0 green; 1 behavioural failure; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077
readonly card='AUR-498'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac
if [[ "$selector" == all ]]; then
  for scenario in AC-001 AC-002 AC-003 AC-004 MUT-001 MUT-002 MUT-003; do
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
for input in go.mod go.sum internal/changelog; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a498.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="${GOCACHE:-$run_dir/gocache}" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir" GOMAXPROCS=1
export XDG_CACHE_HOME="$run_dir/xdg" HOME="$run_dir/home"

# Copy the module root plus the package under test into a writable staging
# tree, so the mutation scenarios can edit source without ever touching the
# repository. Only the declared inputs are copied.
root="$run_dir/root"; mkdir -p "$root/internal"
for f in go.mod go.sum; do
  cp "$repo_root/$f" "$root/$f"
done
cp -R "$repo_root/internal/changelog" "$root/internal/changelog"
chmod -R u+w -- "$root"

# gotest SELECTOR -> 0 when the named test PASSES, 1 when it FAILS, 79 when the
# package does not build.
gotest() {
  local sel="$1"
  local log="$run_dir/$sel.log" rc=0
  (cd "$root" && go test ./internal/changelog/ -run "^${sel}\$" -count=1 -timeout=60s -v) >"$log" 2>&1 || rc=$?
  grep -q 'build failed\|cannot find package\|setup failed' "$log" && { cat "$log" >&2; return 79; }
  if ((rc == 0)) && grep -q -- "--- PASS: $sel" "$log"; then return 0; fi
  cat "$log" >&2
  return 1
}
expect_pass() { local rc=0; gotest "$1" || rc=$?; ((rc == 79)) && infra "build_failed:$1"; ((rc == 0)) || fail "selector-failed:$1"; }
expect_fail() { local rc=0; gotest "$1" || rc=$?; ((rc == 79)) && infra "build_failed:$1"; ((rc == 1)) || fail "mutation-not-detected:$1"; }

case "$selector" in
  AC-001) expect_pass TestAUR498ClassifyCommits ;;
  AC-002) expect_pass TestAUR498SemverBump ;;
  AC-003) expect_pass TestAUR498Render ;;
  AC-004) expect_pass TestAUR498UntrustedText ;;
  MUT-001)
    # Neutralize the fix/perf->patch rule; AC-002 must go red.
    sed -i 's@return Version{Major: current.Major, Minor: current.Minor, Patch: current.Patch + 1}, BumpPatch@return current, BumpNone@' "$root/internal/changelog/semver.go"
    grep -q 'Patch: current.Patch + 1' "$root/internal/changelog/semver.go" && infra 'mutation-anchor-missing:MUT-001'
    expect_fail TestAUR498SemverBump
    ;;
  MUT-002)
    # Stop honoring both the bang and the BREAKING CHANGE: footer while keeping
    # the operands referenced so the package still builds; AC-002 must go red
    # on the breaking cases.
    sed -i 's@cl.Breaking = bang || hasBreakingFooter(c.Body)@cl.Breaking = false \&\& (bang || hasBreakingFooter(c.Body))@' "$root/internal/changelog/changelog.go"
    grep -q 'cl.Breaking = false && (bang || hasBreakingFooter(c.Body))' "$root/internal/changelog/changelog.go" || infra 'mutation-anchor-missing:MUT-002'
    expect_fail TestAUR498SemverBump
    ;;
  MUT-003)
    # Remove escaping in the renderer; AC-004 must go red.
    sed -i 's@func escapeText(s string) string {@func escapeText(s string) string { return s;@' "$root/internal/changelog/render.go"
    grep -q 'func escapeText(s string) string { return s;' "$root/internal/changelog/render.go" || infra 'mutation-anchor-missing:MUT-003'
    expect_fail TestAUR498UntrustedText
    ;;
esac
printf '%s/%s/pass\n' "$card" "$scenario"
