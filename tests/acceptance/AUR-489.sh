#!/usr/bin/env bash
# AUR-489 acceptance: the parity work enters the board without its three
# reproduced defects. Every AC runs a real _test.go selector through go test
# on a staged copy of the module, so a sealed run proves the declared inputs
# are the whole build. MUT selectors apply the card's mutation to the staged
# copy and pass only when the selector FAILS -- a mutation nobody notices is
# a test that asserts nothing.
#
# Exit: 0 green; 1 behavioural failure; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077
ulimit -v 8388608 2>/dev/null || true
export GOMEMLIMIT=2GiB
readonly card='AUR-489'
selector="${1:-AC-001}"
scenario="$selector"
case "$selector" in
  AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002|MUT-003|TestAUR489MemoryScope|TestAUR489PRMemoryDirNotEmpty|TestAUR489SecretNaming|TestAUR489FixApplies|TestAUR489FixRejectsStale) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac
fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }
script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd/aurumcode internal/memory internal/analysis internal/apply pkg/types docs/configuration.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a489.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir" GOMAXPROCS=1
export XDG_CACHE_HOME="$run_dir/xdg" HOME="$run_dir/home"
unset LLM_API_KEY LLM_BASE_URL AURUMCODE_LLM_FIXTURE

# Whole top-level trees, never a per-package list: in a sealed run only the
# declared inputs exist, so this copies exactly what the card materialises
# and cannot rot when a package gains a file.
root="$run_dir/root"; mkdir -p "$root"
for top in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$top" ]] || continue
  cp -R "$repo_root/$top" "$root/$top"
done
chmod -R u+w -- "$root"

# gotest PKG SELECTOR -> 0 when the named test PASSES, 1 when it FAILS or is
# not found, 79 when the package does not build.
gotest() {
  local pkg="$1" sel="$2" log="$run_dir/$sel.log"
  (cd "$root" && go test "./$pkg/" -run "^$sel\$" -count=1 -v) >"$log" 2>&1 || true
  grep -q 'build failed\|cannot find package\|setup failed' "$log" && { cat "$log" >&2; return 79; }
  grep -q "^--- PASS: $sel " "$log" && return 0
  grep -q "^--- SKIP: $sel " "$log" && return 2
  return 1
}
expect_pass() { local rc; gotest "$1" "$2"; rc=$?; ((rc == 79)) && infra "build_failed:$2"; ((rc == 0)) || fail "selector-failed:$2"; }
expect_fail() { local rc; gotest "$1" "$2"; rc=$?; ((rc == 79)) && infra "build_failed:$2"; ((rc == 1)) || fail "mutation-not-detected:$2"; }

case "$selector" in
  AC-001|TestAUR489MemoryScope|TestAUR489PRMemoryDirNotEmpty)
    expect_pass internal/memory TestAUR489MemoryScope
    expect_pass cmd/aurumcode TestAUR489PRMemoryDirNotEmpty ;;
  AC-002|TestAUR489SecretNaming)
    expect_pass internal/analysis TestAUR489SecretNaming ;;
  AC-003|TestAUR489FixApplies|TestAUR489FixRejectsStale)
    expect_pass cmd/aurumcode TestAUR489FixRejectsStale
    # git is absent from the sealed image; the apply-for-real half then skips
    # and says so, it never passes silently.
    rc=0; gotest cmd/aurumcode TestAUR489FixApplies || rc=$?
    case "$rc" in 0) ;; 2) printf '%s/AC-003/note: TestAUR489FixApplies skipped (no git); RejectsStale carried the AC\n' "$card" >&2 ;; 79) infra build_failed:TestAUR489FixApplies ;; *) fail selector-failed:TestAUR489FixApplies ;; esac ;;
  AC-004)
    grep -qiE 'por reposit[oó]rio' "$repo_root/docs/configuration.md" || fail 'docs-missing:memory-per-repository'
    grep -qE 'aurumcode/memory|notes\.json|UserCacheDir|cache' "$repo_root/docs/configuration.md" || fail 'docs-missing:memory-location' ;;
  MUT-001)
    sed -i 's|return memory.New(mode, dir)|_ = dir\n\treturn memory.New(mode, "")|' "$root/cmd/aurumcode/memorydir.go"
    grep -q 'memory.New(mode, "")' "$root/cmd/aurumcode/memorydir.go" || infra 'mutation-anchor-missing:MUT-001'
    expect_fail cmd/aurumcode TestAUR489PRMemoryDirNotEmpty ;;
  MUT-002)
    python_free_sed='s|(?:\[a-z\]\[a-z0-9\]\*)?||'
    sed -i "$python_free_sed" "$root/internal/analysis/analysis.go"
    grep -q '(?:\[a-z\]\[a-z0-9\]\*)?' "$root/internal/analysis/analysis.go" && infra 'mutation-anchor-missing:MUT-002'
    expect_fail internal/analysis TestAUR489SecretNaming ;;
  MUT-003)
    sed -i 's|if err := validateFixPatch(dir, patch, plan); err != nil {|if err := error(nil); err != nil { _ = dir; _ = plan;|' "$root/cmd/aurumcode/main.go"
    grep -q 'if err := error(nil); err != nil' "$root/cmd/aurumcode/main.go" || infra 'mutation-anchor-missing:MUT-003'
    expect_fail cmd/aurumcode TestAUR489FixRejectsStale ;;
esac
printf '{"card":"%s","scenario":"%s","result":"pass"}\n' "$card" "$scenario"
