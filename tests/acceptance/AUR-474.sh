#!/usr/bin/env bash
#
# Acceptance program for card AUR-474, scenario AC-001.
#
# WHAT THIS PROVES
#   The Bash documentation extractor (internal/documentation/extractors/bash)
#   recognizes a function whose body opens AND closes on the same physical
#   line (`name() { ... }`, `function name { ... }`, `function name() {
#   ... }`) as a real symbol -- with its own real signature and its own
#   preceding comment attached as its own doc, never falling mute into "##
#   Script Notes" -- while the AC-002 false-positive shapes (`my-array=()`,
#   `x=$(cmd)`, a one-line if/fi, and a quoted string containing
#   `foo() { }`) still produce zero symbols, and AUR-464's own four-case
#   leading-comment table is untouched. See docs/specs/AUR-474.md.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   69 = inconclusive / infrastructure
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-474'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR474|IntegrationAUR474|E2EAUR474|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a474.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache"
export HOME="$run_dir/home"
export TMPDIR="$run_dir"

run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB go "$@" )
}

copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly this card's owned packages and the
# read-only extractors/site slice both extractors' compile closure needs.
# AC-003 (AUR-464's four-case leading-comment table must survive) is proven
# out of band, not by pulling tests/unit/AUR-464.go into this sealed input
# set -- it is not in this card's paths/read_paths -- but by running
# tests/acceptance/AUR-464.sh AC-001 directly against the same candidate
# source; that program is unmodified by this card and stays exit 0.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" \
    internal/documentation/extractors/types.go \
    internal/documentation/extractors/errors.go \
    internal/documentation/extractors/registry.go \
    internal/documentation/extractors/detector.go \
    internal/documentation/extractors/bash \
    internal/documentation/extractors/powershell \
    internal/documentation/normalizer \
    internal/documentation/site \
    tests/unit/AUR-474.go \
    tests/integration/AUR-474.go \
    tests/e2e/AUR-474.sh \
    docs/specs/AUR-474.md
  chmod -R u+w -- "$root"
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  cat >"$root/tests/unit/aur474_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR474UnitBridge(t *testing.T) {
	TestAUR474(t)
	TestAUR474NoBodyStillRecognized(t)
	TestAUR474PowerShellAlreadyHandlesOneLiners(t)
}
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/unit -run '^TestAUR474UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | tail -n 30
  ((rc == 0)) || fail "selector:TestAUR474:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR474:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  cat >"$root/tests/integration/aur474_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR474IntegrationBridge(t *testing.T) { IntegrationAUR474(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/integration -run '^TestAUR474IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | tail -n 20
  ((rc == 0)) || fail "selector:IntegrationAUR474:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR474:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  local root="$run_dir/root-e2e"
  stage_source "$root"
  local rc
  set +e
  (cd "$root" && bash tests/e2e/AUR-474.sh E2EAUR474)
  rc=$?
  set -e
  ((rc != 69)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# mutate_line locates an EXACT, whole-line, fixed-string anchor (grep -Fx)
# and replaces it by line number, so the replacement text never has to be
# regex/sed-escaped. It fails (return 1) if the anchor is absent or not
# unique.
mutate_line() {
  local file="$1" anchor="$2" replacement="$3" n escaped delim
  n="$(grep -Fxn "$anchor" "$file" | cut -d: -f1)"
  [[ -n "$n" ]] || return 1
  [[ "$(wc -l <<<"$n")" -eq 1 ]] || return 1
  escaped="${replacement//\\/\\\\}"
  delim=$'\x01'
  sed -i "${n}s${delim}.*${delim}${escaped}${delim}" "$file"
}

# mutation_case_001 is MUT-001: revert bashBodyTail to require the body to
# open on its own line (the pre-AUR-474 behavior), and prove the same Unit
# selector that is GREEN against the real source turns RED against the
# mutant -- then prove a fresh, unmutated stage reproduces the exact same
# GREEN.
mutation_case_001() {
  local root="$run_dir/root-mut1"
  stage_source "$root"
  local target="$root/internal/documentation/extractors/bash/extractor.go"

  local anchor='const bashBodyTail = `(?:\{\s*.*\}\s*|\{\s*)?$`'
  local replacement='const bashBodyTail = `\{?\s*$`'
  mutate_line "$target" "$anchor" "$replacement" || fail 'MUT-001/anchor-not-unique'
  grep -Fxq "$replacement" "$target" || fail 'MUT-001/mutation-not-applied'

  cat >"$root/tests/unit/aur474_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR474UnitBridge(t *testing.T) {
	TestAUR474(t)
	TestAUR474NoBodyStillRecognized(t)
	TestAUR474PowerShellAlreadyHandlesOneLiners(t)
}
EOF
  local rc
  set +e
  run_go "$root" test -timeout 300s ./tests/unit -run '^TestAUR474UnitBridge$' -count=1 >"$root/mut.log" 2>&1
  rc=$?
  set -e
  if ((rc == 0)); then
    cat "$root/mut.log" >&2
    fail 'MUT-001/mutation-had-no-observable-effect'
  fi
  grep -q 'AUR-474/AC-001' "$root/mut.log" || fail 'MUT-001/red-not-labeled'
  cleanup_root "$root"

  local restore_root="$run_dir/root-mut1-restore"
  stage_source "$restore_root"
  cat >"$restore_root/tests/unit/aur474_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR474UnitBridge(t *testing.T) {
	TestAUR474(t)
	TestAUR474NoBodyStillRecognized(t)
	TestAUR474PowerShellAlreadyHandlesOneLiners(t)
}
EOF
  run_go "$restore_root" test -timeout 300s ./tests/unit -run '^TestAUR474UnitBridge$' -count=1 >/dev/null 2>&1 \
    || fail 'MUT-001/restoration-broken'
  cleanup_root "$restore_root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# mutation_case_002 is MUT-002: widen bashParenPattern's name anchor so it
# no longer requires "(" immediately after the name -- allowing
# "my-array=()" through, the exact false positive AUR-464's own reviewer
# checked against -- and prove the same Unit selector turns RED, then that
# a fresh, unmutated stage reproduces the exact same GREEN.
mutation_case_002() {
  local root="$run_dir/root-mut2"
  stage_source "$root"
  local target="$root/internal/documentation/extractors/bash/extractor.go"

  local anchor='var bashParenPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*\(\s*\)\s*` + bashBodyTail)'
  local replacement='var bashParenPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_-]*)\s*.*\(\s*\)\s*` + bashBodyTail)'
  mutate_line "$target" "$anchor" "$replacement" || fail 'MUT-002/anchor-not-unique'
  grep -Fxq "$replacement" "$target" || fail 'MUT-002/mutation-not-applied'

  cat >"$root/tests/unit/aur474_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR474UnitBridge(t *testing.T) {
	TestAUR474(t)
	TestAUR474NoBodyStillRecognized(t)
	TestAUR474PowerShellAlreadyHandlesOneLiners(t)
}
EOF
  local rc
  set +e
  run_go "$root" test -timeout 300s ./tests/unit -run '^TestAUR474UnitBridge$' -count=1 >"$root/mut.log" 2>&1
  rc=$?
  set -e
  if ((rc == 0)); then
    cat "$root/mut.log" >&2
    fail 'MUT-002/mutation-had-no-observable-effect'
  fi
  grep -q 'AUR-474/AC-00[12]' "$root/mut.log" || fail 'MUT-002/red-not-labeled'
  cleanup_root "$root"

  local restore_root="$run_dir/root-mut2-restore"
  stage_source "$restore_root"
  cat >"$restore_root/tests/unit/aur474_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR474UnitBridge(t *testing.T) {
	TestAUR474(t)
	TestAUR474NoBodyStillRecognized(t)
	TestAUR474PowerShellAlreadyHandlesOneLiners(t)
}
EOF
  run_go "$restore_root" test -timeout 300s ./tests/unit -run '^TestAUR474UnitBridge$' -count=1 >/dev/null 2>&1 \
    || fail 'MUT-002/restoration-broken'
  cleanup_root "$restore_root"
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

run_all() {
  unit_case
  integration_case
  e2e_case
  mutation_case_001
  mutation_case_002
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR474) unit_case ;;
  IntegrationAUR474) integration_case ;;
  E2EAUR474) e2e_case ;;
  MUT-001) mutation_case_001 ;;
  MUT-002) mutation_case_002 ;;
esac
