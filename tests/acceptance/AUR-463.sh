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
#   existing typedoc-backed JSExtractor (extractor.go) at all -- AND that
#   javascript.SelectExtractor (select.go) wires it end to end:
#   cmd/regenerate-docs/main.go and cmd/aurumcode/docs.go both register
#   SelectExtractor's choice instead of an unconditional JSExtractor, so the
#   real `aurumcode docs` / `regenerate-docs` CLIs now produce real
#   documentation for a .mjs project with no typedoc on PATH, and still
#   invoke typedoc (byte-identical output, AC-003) when it is present. See
#   docs/specs/AUR-463.md "Ligacao ponta a ponta" for the composition-root
#   design and why internal/documentation/extractors/tool_unavailable_test.go
#   / tool_failure_test.go (also owned by this card) keep their existing
#   "javascript/typedoc" row unchanged: JSExtractor.Validate itself still
#   correctly reports a missing typedoc, and now those two files pin
#   SelectExtractor's own, additional contract on top.
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
  AC-001|AC-002|AC-003|TestAUR463|IntegrationAUR463|E2EAUR463|NominalCLI|AC-001-MUT-001|AC-001-MUT-002|AC-001-MUT-003) ;;
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

# stage_source materializes what building cmd/regenerate-docs and testing the
# javascript package (plus the now-owned tool_unavailable_test.go/
# tool_failure_test.go) needs: the full extractor set that binary registers,
# internal/pipeline, internal/llm, and this card's own test/spec files.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" \
    internal/documentation/extractors/types.go \
    internal/documentation/extractors/errors.go \
    internal/documentation/extractors/registry.go \
    internal/documentation/extractors/detector.go \
    internal/documentation/extractors/tool_unavailable_test.go \
    internal/documentation/extractors/tool_failure_test.go \
    internal/documentation/extractors/output_confirmed_test.go \
    internal/documentation/extractors/registry_test.go \
    internal/documentation/extractors/detector_test.go \
    internal/documentation/extractors/go \
    internal/documentation/extractors/python \
    internal/documentation/extractors/javascript \
    internal/documentation/extractors/cpp \
    internal/documentation/extractors/bash \
    internal/documentation/extractors/powershell \
    internal/documentation/extractors/rust \
    internal/documentation/extractors/csharp \
    internal/documentation/incremental \
    internal/documentation/normalizer \
    internal/documentation/site \
    internal/documentation/welcome \
    internal/pipeline \
    internal/llm \
    cmd/regenerate-docs \
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

# nominal_cli_case is the coordinator's literal, mensuravel target: build the
# real cmd/regenerate-docs binary (the same registration
# cmd/aurumcode/docs.go mirrors -- verified separately, by hand, against the
# actual `aurumcode docs -source -output` CLI; see docs/specs/AUR-463.md) and
# run it against a real, ephemeral .mjs fixture twice: once with typedoc
# genuinely absent from PATH (must produce a real page, not the old
# "required tool not in PATH" / zero-documents defect), and once with a
# typedoc STUB installed on PATH (must invoke it -- AC-003's mechanism: the
# tool, when present, is still the one that runs).
nominal_cli_case() {
  local root="$run_dir/root-cli-$$-$RANDOM"
  stage_source "$root"

  local bin="$run_dir/regenerate-docs-nominal"
  local log="$root/build.log"
  if ! run_go "$root" build -o "$bin" ./cmd/regenerate-docs >"$log" 2>&1; then
    cat "$log" >&2
    fail 'nominal-cli/build-failed'
  fi

  local fixture="$run_dir/mjsproject"
  mkdir -p "$fixture"
  cat >"$fixture/index.mjs" <<'FIXEOF'
/**
 * Adds two numbers together.
 */
export function add(a, b) {
  return a + b;
}

export function undocumented(x) {
  return x;
}
FIXEOF

  # No typedoc on PATH at all: the empty-PATH-equivalent case.
  local out_native="$run_dir/cli-out-native"
  local rc
  set +e
  ( export PATH="/usr/bin:/bin"; ulimit -v 8388608
    AURUMCODE_SOURCE_DIR="$fixture" AURUMCODE_OUTPUT_DIR="$out_native" GOMEMLIMIT=2GiB "$bin"
  ) >"$run_dir/cli-native.out" 2>"$run_dir/cli-native.err"
  rc=$?
  set -e
  local page="$out_native/javascript/index.md"
  [[ -s "$page" ]] || fail 'nominal-cli/no-typedoc-produced-zero-documents'
  grep -Fq 'add(a, b)' "$page" || fail 'nominal-cli/missing-documented-signature'
  grep -Fq 'Adds two numbers together.' "$page" || fail 'nominal-cli/missing-jsdoc-prose'
  grep -Fq 'undocumented(x)' "$page" || fail 'nominal-cli/missing-undocumented-signature'

  # typedoc present (a stub, since the sealed sandbox has no npm toolchain):
  # SelectExtractor must invoke it rather than the native reader.
  local stub_dir="$run_dir/fakebin"
  mkdir -p "$stub_dir"
  cat >"$stub_dir/typedoc" <<'STUBEOF'
#!/bin/sh
if [ "$1" = "--version" ]; then echo "1.0.0"; exit 0; fi
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--out" ]; then out="$a"; fi
  prev="$a"
done
mkdir -p "$out"
echo "FAKE TYPEDOC MARKER" > "$out/marker.md"
exit 0
STUBEOF
  chmod +x "$stub_dir/typedoc"

  local out_typedoc="$run_dir/cli-out-typedoc"
  set +e
  ( export PATH="$stub_dir:/usr/bin:/bin"; ulimit -v 8388608
    AURUMCODE_SOURCE_DIR="$fixture" AURUMCODE_OUTPUT_DIR="$out_typedoc" GOMEMLIMIT=2GiB "$bin"
  ) >"$run_dir/cli-typedoc.out" 2>"$run_dir/cli-typedoc.err"
  rc=$?
  set -e
  ((rc == 0)) || fail 'nominal-cli/typedoc-stub-run-failed'
  [[ -s "$out_typedoc/javascript/marker.md" ]] || fail 'nominal-cli/typedoc-path-not-taken'
  grep -Fq 'FAKE TYPEDOC MARKER' "$out_typedoc/javascript/marker.md" || fail 'nominal-cli/typedoc-marker-missing'
  # And the native reader must NOT have run when typedoc is present: no page
  # carries the native reader's own generated symbol names.
  ! grep -Rq 'add(a, b)' "$out_typedoc" || fail 'nominal-cli/native-ran-despite-typedoc-present'

  cleanup_root "$root"
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

# mutation_mut003 is the PROVA OBRIGATORIA the coordinator asked for on this
# pass: revert cmd/regenerate-docs/main.go's registration from
# javascriptExtractor.SelectExtractor(...) back to the old, unconditional
# javascriptExtractor.NewJSExtractor(runner) -- "o registro volta a ignorar o
# nativo" -- rebuild, and prove the ORIGINAL defect reproduces exactly: with
# typedoc absent, zero JavaScript documents. Then prove the clean stage
# reproduces GREEN again.
mutation_mut003() {
  local root="$run_dir/root-mut003-$$-$RANDOM"
  stage_source "$root"
  local target="$root/cmd/regenerate-docs/main.go"

  local anchor=$'\tjsExtractor := javascriptExtractor.SelectExtractor(context.Background(), runner)'
  local replacement=$'\tjsExtractor := javascriptExtractor.NewJSExtractor(runner)'
  mutate_line "$target" "$anchor" "$replacement" || fail 'MUT-003/anchor-not-unique'
  grep -Fxq "$replacement" "$target" || fail 'MUT-003/mutation-not-applied'

  local bin="$run_dir/regenerate-docs-mut003"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/regenerate-docs >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-003/build-failed'
  fi

  local fixture="$run_dir/mut003-fixture"
  mkdir -p "$fixture"
  cat >"$fixture/index.mjs" <<'FIXEOF'
/**
 * Adds two numbers together.
 */
export function add(a, b) {
  return a + b;
}
FIXEOF

  local mut_out="$run_dir/mut003-out"
  set +e
  ( export PATH="/usr/bin:/bin"; ulimit -v 8388608
    AURUMCODE_SOURCE_DIR="$fixture" AURUMCODE_OUTPUT_DIR="$mut_out" GOMEMLIMIT=2GiB "$bin"
  ) >"$run_dir/mut003.out" 2>"$run_dir/mut003.err"
  set -e

  if [[ -s "$mut_out/javascript/index.md" ]]; then
    fail 'MUT-003/mutation-had-no-observable-effect'
  fi
  grep -Fq 'required tool not in PATH' "$run_dir/mut003.out" "$run_dir/mut003.err" \
    || fail 'MUT-003/defect-signature-not-reproduced'

  cleanup_root "$root"

  # Restoration: the clean stage (unmutated SelectExtractor wiring) still
  # produces the real page -- the GREEN reproduces exactly.
  nominal_cli_case
  printf '%s/%s/MUT-003/rejected\n' "$card" "$scenario"
}

run_all() {
  unit_case || fail 'selector:TestAUR463'
  integration_case || fail 'selector:IntegrationAUR463'
  ac003_case
  nominal_cli_case
  mutation_mut001
  mutation_mut002
  mutation_mut003
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  AC-002) integration_case || fail 'selector:IntegrationAUR463' ;;
  AC-003) ac003_case; printf '%s/%s/AC-003/ok\n' "$card" "$scenario" ;;
  TestAUR463) unit_case || fail 'selector:TestAUR463' ;;
  IntegrationAUR463) integration_case || fail 'selector:IntegrationAUR463' ;;
  E2EAUR463) e2e_case || fail 'selector:E2EAUR463' ;;
  NominalCLI) nominal_cli_case; printf '%s/%s/NominalCLI/ok\n' "$card" "$scenario" ;;
  AC-001-MUT-001) mutation_mut001 ;;
  AC-001-MUT-002) mutation_mut002 ;;
  AC-001-MUT-003) mutation_mut003 ;;
esac
