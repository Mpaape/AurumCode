#!/usr/bin/env bash
#
# Acceptance program for card AUR-463, scenario AC-001.
#
# WHAT THIS PROVES
#
#   internal/documentation/extractors/javascript's new NativeExtractor
#   (native.go) reads real JSDoc comments out of real .js/.jsx/.mjs/.cjs
#   source text and renders real Markdown pages from them, without any
#   external tool (no `typedoc`, no `npm`, no network), never synthesizing
#   prose for a symbol that carries no JSDoc, and without touching the
#   existing typedoc-backed JSExtractor (extractor.go) at all.
#
#   SCOPE NOTE (see docs/specs/AUR-463.md "Limitacao de escopo medida"): this
#   card's `paths` grant only internal/documentation/extractors/javascript
#   and this card's own test/spec files. cmd/regenerate-docs is read_paths
#   (read-only) and cmd/aurumcode is outside read_paths entirely, so this
#   card cannot register NativeExtractor at either composition root the way
#   AUR-427 registered rust/csharp's NativeExtractor in both binaries.
#   Consequently `aurumcode docs` and `regenerate-docs` still route
#   JavaScript exclusively through JSExtractor, and internal/pipeline still
#   skips JavaScript on ToolUnavailableError before Extract is ever called --
#   a defect the outside-of-paths tests
#   internal/documentation/extractors/tool_unavailable_test.go and
#   tool_failure_test.go additionally PIN by requiring
#   javascript.NewJSExtractor(r).Validate() to keep returning a classifiable
#   ToolUnavailableError for missing typedoc. This acceptance program
#   therefore proves NativeExtractor's own correctness directly against the
#   package's public API, not through the `aurumcode docs` CLI end to end.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   69 = inconclusive / infrastructure: an input this card does not own was
#        never materialized, a required tool is missing. Never valid red
#        evidence, never a pass.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-463'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|AC-002|AC-003|TestAUR463|IntegrationAUR463|E2EAUR463|AC-001-MUT-001|AC-001-MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a463.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 (single build/test process) for every go
# invocation in this file -- the profile's memory ceiling kills a parallel
# compile.
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

# stage_source materializes exactly what building and testing the javascript
# package needs: the extractor types/errors it imports, the site package
# extractor.go (untouched) depends on, this package itself, and this card's
# own test/spec files. No registry, detector, pipeline, or cmd package: this
# card never registers anything at a composition root (see the scope note
# above), so those are not needed to prove NativeExtractor's own contract.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" \
    internal/documentation/extractors/types.go \
    internal/documentation/extractors/errors.go \
    internal/documentation/extractors/javascript \
    internal/documentation/site \
    docs/specs/AUR-463.md
  chmod -R u+w -- "$root"
}

# stage_test_files copies this card's own unit/integration proof files into
# an already-staged root.
stage_test_files() {
  local root="$1"
  copy "$root" tests/unit/AUR-463.go tests/integration/AUR-463.go
}

unit_case() {
  local root="$run_dir/root-unit-$$-$RANDOM"
  stage_source "$root"
  stage_test_files "$root"
  cat >"$root/tests/unit/aur463_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR463UnitBridge(t *testing.T) { TestAUR463(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/unit -run '^TestAUR463UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | tail -n 40
  cleanup_root "$root"
  return "$rc"
}

integration_case() {
  local root="$run_dir/root-integration-$$-$RANDOM"
  stage_source "$root"
  stage_test_files "$root"
  cat >"$root/tests/integration/aur463_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR463IntegrationBridge(t *testing.T) { IntegrationAUR463(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/integration -run '^TestAUR463IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | tail -n 40
  cleanup_root "$root"
  return "$rc"
}

e2e_case() {
  local root="$run_dir/root-e2e-$$-$RANDOM"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-463.sh
  local rc
  set +e
  (cd "$root" && bash tests/e2e/AUR-463.sh E2EAUR463)
  rc=$?
  set -e
  cleanup_root "$root"
  return "$rc"
}

# ac003_case proves the Non-goal that cannot fall: extractor.go (the
# typedoc-backed path) is byte-identical to the committed, pre-AUR-463 file
# this program itself ships as a reference copy is unnecessary -- instead it
# asserts the textual contract directly, since this card never edits
# extractor.go: the file still shells out to "typedoc" with the same flags,
# and NativeExtractor is an additive, separate type in native.go.
ac003_case() {
  local ext="$repo_root/internal/documentation/extractors/javascript/extractor.go"
  [[ -f "$ext" ]] || fail 'AC-003/extractor.go-missing'
  grep -Fq 'j.runner.Run(ctx, "typedoc"' "$ext" || fail 'AC-003/typedoc-path-removed'
  grep -Fq 'typedoc-plugin-markdown' "$ext" || fail 'AC-003/typedoc-flags-changed'
  local native="$repo_root/internal/documentation/extractors/javascript/native.go"
  [[ -f "$native" ]] || fail 'AC-003/native-missing'
  ! grep -Fq 'exec.Command' "$ext" || fail 'AC-003/unexpected-direct-exec-in-typedoc-path'
}

mutate_line() {
  local file="$1" anchor="$2" replacement="$3" n
  n="$(grep -Fxn "$anchor" "$file" | cut -d: -f1)"
  [[ -n "$n" ]] || return 1
  [[ "$(wc -l <<<"$n")" -eq 1 ]] || return 1
  sed -i "${n}s#.*#${replacement}#" "$file"
}

# mutation_mut001 removes native.go entirely (the defect MUT-001 names: "the
# native extractor is removed") and proves the same unit selector can never
# pass with it gone, then proves the clean stage still reproduces GREEN.
mutation_mut001() {
  local root="$run_dir/root-mut001-$$-$RANDOM"
  stage_source "$root"
  stage_test_files "$root"
  rm -f "$root/internal/documentation/extractors/javascript/native.go"
  cat >"$root/tests/unit/aur463_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR463UnitBridge(t *testing.T) { TestAUR463(t) }
EOF
  local rc
  set +e
  run_go "$root" test -timeout 300s ./tests/unit -run '^TestAUR463UnitBridge$' -count=1 >/dev/null 2>&1
  rc=$?
  set -e
  cleanup_root "$root"
  if [[ "$rc" -eq 0 ]]; then
    fail 'MUT-001/mutation-had-no-observable-effect'
  fi

  unit_case >/dev/null 2>&1 || fail 'MUT-001/restoration-broken'
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

# mutation_mut002 makes native.go synthesize prose for a symbol with no
# JSDoc -- renderJSMarkdown always prints a fixed sentence instead of only
# printing item.Doc when it is non-empty -- and proves the unit selector
# (which asserts noDoc/greet are followed directly by their code fence) goes
# red, then that the clean stage reproduces GREEN.
mutation_mut002() {
  local root="$run_dir/root-mut002-$$-$RANDOM"
  stage_source "$root"
  stage_test_files "$root"
  local native="$root/internal/documentation/extractors/javascript/native.go"
  local anchor='		if item.Doc != "" {'
  local replacement='		if true {'
  mutate_line "$native" "$anchor" "$replacement" || fail 'MUT-002/anchor-not-unique'
  # Force a non-empty string even when Doc is "" so the synthesized branch
  # has real, visible content to leak.
  local anchor2='			fmt.Fprintf(&b, "%s\n\n", item.Doc)'
  local replacement2='			doc := item.Doc; if doc == "" { doc = "No documentation available." }; fmt.Fprintf(&b, "%s\n\n", doc)'
  mutate_line "$native" "$anchor2" "$replacement2" || fail 'MUT-002/anchor2-not-unique'

  cat >"$root/tests/unit/aur463_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR463UnitBridge(t *testing.T) { TestAUR463(t) }
EOF
  local rc
  set +e
  run_go "$root" test -timeout 300s ./tests/unit -run '^TestAUR463UnitBridge$' -count=1 >/dev/null 2>&1
  rc=$?
  set -e
  cleanup_root "$root"
  if [[ "$rc" -eq 0 ]]; then
    fail 'MUT-002/mutation-had-no-observable-effect'
  fi

  unit_case >/dev/null 2>&1 || fail 'MUT-002/restoration-broken'
  printf '%s/%s/MUT-002/rejected\n' "$card" "$scenario"
}

run_all() {
  unit_case || fail 'selector:TestAUR463'
  integration_case || fail 'selector:IntegrationAUR463'
  ac003_case
  mutation_mut001
  mutation_mut002
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  AC-002) integration_case || fail 'selector:IntegrationAUR463' ;;
  AC-003) ac003_case; printf '%s/%s/AC-003/ok\n' "$card" "$scenario" ;;
  TestAUR463) unit_case || fail 'selector:TestAUR463' ;;
  IntegrationAUR463) integration_case || fail 'selector:IntegrationAUR463' ;;
  E2EAUR463) e2e_case || fail 'selector:E2EAUR463' ;;
  AC-001-MUT-001) mutation_mut001 ;;
  AC-001-MUT-002) mutation_mut002 ;;
esac
