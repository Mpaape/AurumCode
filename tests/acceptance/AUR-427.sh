#!/usr/bin/env bash
#
# Acceptance program for card AUR-427, scenario AC-001.
#
# WHAT THIS PROVES
#
#   Rust and C# projects generate real documentation pages through
#   `cmd/regenerate-docs`'s default registration, without any external
#   toolchain (no `cargo`, no `dotnet`, no `xmldocmd`, no network) and
#   without executing any code from the documented repository. It reuses
#   internal/documentation/extractors -- the exact engine cmd/regenerate-docs
#   already wires -- adding two new, tool-free extractors
#   (rust.NativeExtractor, csharp.NativeExtractor) rather than reimplementing
#   extraction or touching the existing cargo/dotnet-backed extractors.
#   See docs/specs/AUR-427.md, including its "Nota de contrato" section on
#   why this proves the command through `cmd/regenerate-docs`
#   (AURUMCODE_SOURCE_DIR/AURUMCODE_OUTPUT_DIR) rather than through
#   `aurumcode docs --source`: `cmd/aurumcode` is outside this card's
#   paths/read_paths.
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

readonly card='AUR-427'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR427|IntegrationAUR427|E2EAUR427|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Offline module cache holding gopkg.in/yaml.v3 (internal/pipeline's
# _config.yml exclude-list reader): resolved BEFORE HOME is redirected,
# since the default cache location derives from HOME.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a427.XXXXXX")" || infra mktemp
# The staged copies below preserve the read-only modes of the materialized
# input tree, so force write permission back before removing, and never let
# a residual removal error decide the exit code (see AUR-430.sh).
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/home"

# REGRAS INEGOCIAVEIS: bounded memory, GOFLAGS carries -mod=mod (offline,
# read-only module list) and -p=1 (single build/test process) for every go
# invocation in this file -- the profile's 256 MB ceiling kills a parallel
# compile.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" GOMODCACHE="$host_modcache"
export HOME="$run_dir/home"
export TMPDIR="$run_dir"

# run_go <dir> <go-args...> runs `go` inside dir with the memory ceiling and
# GOMEMLIMIT the card requires, isolated to a subshell so the ulimit does not
# leak into the rest of this script (e.g. the `cp -R` staging below).
run_go() {
  local dir="$1"; shift
  ( cd "$dir" && ulimit -v 8388608 && GOMAXPROCS=1 GOMEMLIMIT=2GiB go "$@" )
}

# copy materializes one repo path into a staged root. Every path it copies is
# either owned by this card (paths) or read by it (read_paths); a missing
# source is an input this card does not own that was never materialized -- an
# environment gap, never a verdict.
copy() {
  local root="$1"; shift
  local p
  for p in "$@"; do
    [[ -e "$repo_root/$p" ]] || infra "missing_input:$p"
    mkdir -p "$root/$(dirname "$p")"
    cp -R "$repo_root/$p" "$root/$p"
  done
}

# stage_source materializes exactly this card's paths/read_paths: the two new
# native extractor packages (rust, csharp), cmd/regenerate-docs (the whole
# closure it needs to build: the other seven language extractors, the shared
# extractors.{types,errors,registry,detector}, site/incremental/normalizer/
# welcome, internal/llm and internal/pipeline), the card's own checked-in
# Rust fixture, and this card's own test/spec files.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" \
    internal/documentation/extractors/types.go \
    internal/documentation/extractors/errors.go \
    internal/documentation/extractors/registry.go \
    internal/documentation/extractors/detector.go \
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
    internal/documentation/site internal/documentation/review \
    internal/documentation/welcome internal/documentation/review \
    internal/pipeline \
    internal/llm \
    cmd/regenerate-docs \
    tests/fixtures/docs/rustproject \
    action.yml \
    docs/specs/AUR-427.md
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# build_shared builds the binary once per acceptance run and reuses it for
# every lane; GOCACHE is shared process-wide, so the go test compiles and the
# mutation rebuild start warm instead of cold.
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/regenerate-docs"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! run_go "$shared_root" build -o "$shared_bin" ./cmd/regenerate-docs >"$log" 2>&1; then
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# ephemeral_csharp_fixture writes a small, real C# project into dir. This
# card's `paths` grants a checked-in Rust fixture directory only (see
# docs/specs/AUR-427.md's "Limitação de infraestrutura medida"), so C#'s
# nominal proof uses a runtime-only input, exactly as tests/e2e/AUR-427.sh
# does.
ephemeral_csharp_fixture() {
  local dir="$1"
  mkdir -p "$dir"
  cat >"$dir/Greeter.cs" <<'CSEOF'
namespace Fixture
{
    /// <summary>
    /// A greeter that says hello in a configurable language.
    /// </summary>
    public class Greeter
    {
        /// <summary>
        /// Creates a greeter for the given language tag.
        /// </summary>
        /// <param name="lang">A BCP-47-ish language tag, e.g. "en" or "pt".</param>
        public Greeter(string lang)
        {
            Lang = lang;
        }

        /// <summary>
        /// The greeter's configured language tag.
        /// </summary>
        public string Lang { get; }

        /// <summary>
        /// Renders a greeting for the given name.
        /// </summary>
        /// <returns>A human-readable greeting sentence.</returns>
        public string Greet(string name)
        {
            return Lang == "pt" ? "Ola, " + name + "!" : "Hello, " + name + "!";
        }

        private bool IsPortuguese() => Lang == "pt";
    }
}
CSEOF
}

# nominal_case is AC-001's core behavioral proof, run directly against the
# real binary: the declared command (AURUMCODE_SOURCE_DIR/AURUMCODE_OUTPUT_DIR
# `go run ./cmd/regenerate-docs`, here the already-built shared binary)
# generates real Rust and real C# pages carrying the fixtures' real symbols
# and real doc comments; the run is deterministic; a symbol this parser
# cannot extract (a macro-generated Rust function, a private C# method) is
# never claimed; and a secret canary never reaches stdout or stderr.
nominal_case() {
  build_shared
  local rc

  local rust_fixture="$shared_root/tests/fixtures/docs/rustproject"
  [[ -d "$rust_fixture" ]] || fail missing-rust-fixture

  run_declared() {
    local src="$1" out="$2" logbase="$3"
    set +e
    ( ulimit -v 8388608
      AURUMCODE_SOURCE_DIR="$src" AURUMCODE_OUTPUT_DIR="$out" GOMEMLIMIT=2GiB "$shared_bin"
    ) >"$run_dir/$logbase.out" 2>"$run_dir/$logbase.err"
    rc=$?
    set -e
    return "$rc"
  }

  # Real generation against the card's own Rust fixture.
  local rust_out="$run_dir/rust-site"
  run_declared "$rust_fixture" "$rust_out" rust1 || fail behavior-missing:rust-run
  local rust_page="$rust_out/rust/src__lib.md"
  [[ -s "$rust_page" ]] || fail missing-generated-rust-page
  for want in 'pub struct Entry' 'pub fn new_entry' 'Creates a new ledger entry.' \
              'pub const MAX_ENTRIES_PER_PAGE' 'pub struct Ledger' 'pub enum EntryKind'; do
    grep -Fq "$want" "$rust_page" || fail "generated-rust-page-missing-symbol:$want"
  done
  ! grep -Fq 'pub fn record_internal' "$rust_page" || fail 'false-claim:record_internal'
  ! grep -Fq 'entry_count' "$rust_page" || fail 'false-claim:entry_count'

  # Determinism: repeating the run over the same input produces the same
  # output (AC-001's "And" clause).
  run_declared "$rust_fixture" "$run_dir/rust-site2" rust2 || fail non-deterministic:second-run-failed
  local d1 d2
  d1="$(cd "$rust_out" && find . -type f | LC_ALL=C sort | xargs -r sha256sum | sha256sum | cut -d' ' -f1)"
  d2="$(cd "$run_dir/rust-site2" && find . -type f | LC_ALL=C sort | xargs -r sha256sum | sha256sum | cut -d' ' -f1)"
  [[ "$d1" == "$d2" ]] || fail "non-deterministic:$d1:$d2"

  # Real generation against an ephemeral C# fixture.
  local cs_src="$run_dir/csharpproject"
  ephemeral_csharp_fixture "$cs_src"
  local cs_out="$run_dir/cs-site"
  run_declared "$cs_src" "$cs_out" cs1 || fail behavior-missing:csharp-run
  local cs_page="$cs_out/csharp/Greeter.md"
  [[ -s "$cs_page" ]] || fail missing-generated-csharp-page
  for want in 'public class Greeter' 'public Greeter(string lang)' \
              'public string Greet(string name)' 'A human-readable greeting sentence.'; do
    grep -Fq "$want" "$cs_page" || fail "generated-csharp-page-missing-symbol:$want"
  done
  ! grep -Fq 'IsPortuguese' "$cs_page" || fail 'false-claim:IsPortuguese'

  # A secret canary must never reach stdout or stderr (AUR-009/AUR-432).
  local canary='aur427-nominal-canary-5e91da'
  set +e
  ( ulimit -v 8388608
    AURUM_SECRET_CANARY="$canary" AURUMCODE_SOURCE_DIR="$rust_fixture" \
      AURUMCODE_OUTPUT_DIR="$run_dir/canary-site" GOMEMLIMIT=2GiB "$shared_bin"
  ) >"$run_dir/canary.out" 2>"$run_dir/canary.err"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail canary-run-failed
  ! grep -Fq "$canary" "$run_dir/canary.out" || fail canary-leaked-stdout
  ! grep -Fq "$canary" "$run_dir/canary.err" || fail canary-leaked-stderr
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-427.go
  cat >"$root/tests/unit/aur427_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR427UnitBridge(t *testing.T) { TestAUR427(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/unit -run '^TestAUR427UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR427:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR427:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-427.go
  cat >"$root/tests/integration/aur427_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR427IntegrationBridge(t *testing.T) { IntegrationAUR427(t) }
EOF
  local out rc
  set +e
  out="$(AURUMCODE_ROOT="$root" run_go "$root" test -v -timeout 300s ./tests/integration -run '^TestAUR427IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR427:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR427:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-427.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-compile its own copy via
  # `go run`. The nested script's own exit-code vocabulary is preserved: its
  # 69 is an environment gap and must be re-emitted as infra here, never
  # collapsed into behavioral RED.
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-427.sh E2EAUR427)
  rc=$?
  set -e
  ((rc != 69)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# regression_case proves the non-negotiable "cargo/dotnet-backed RustExtractor
# and CSharpExtractor are untouched, and cmd/regenerate-docs's own
# pre-existing security gate (repo_code_execution.go's opt-in) still holds":
# its full test suite, plus both extractor packages' own suites (which cover
# the untouched RustExtractor/CSharpExtractor alongside the new
# NativeExtractor), entirely inside this card's own `paths`, still pass.
# internal/documentation/extractors' own top-level tests
# (tool_unavailable_test.go, tool_failure_test.go, registry_test.go,
# output_confirmed_test.go) are NOT re-run here: they sit outside this card's
# read_paths on purpose (see docs/specs/AUR-427.md's "Contratos pré-existentes
# preservados"), so they are not materialized inside this sandbox at all --
# verified locally instead, and that is stated rather than silently skipped.
regression_case() {
  local root="$run_dir/root-regression"
  stage_source "$root"
  local out rc
  set +e
  out="$(run_go "$root" test -timeout 300s \
    ./cmd/regenerate-docs \
    ./internal/documentation/extractors/rust \
    ./internal/documentation/extractors/csharp \
    -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | tail -n 30
  ((rc == 0)) || fail "regression:exit:$rc"
  local ok_count
  ok_count="$(grep -c '^ok ' <<<"$out" || true)"
  ((ok_count == 3)) || fail "regression-package-count:$ok_count"
  cleanup_root "$root"
}

# mutate_line locates an EXACT, whole-line, fixed-string anchor (grep -Fx)
# and replaces it by line number, so the replacement text never has to be
# regex/sed-escaped. It fails (return 1) if the anchor is absent or not
# unique.
mutate_line() {
  local file="$1" anchor="$2" replacement="$3" n
  n="$(grep -Fxn "$anchor" "$file" | cut -d: -f1)"
  [[ -n "$n" ]] || return 1
  [[ "$(wc -l <<<"$n")" -eq 1 ]] || return 1
  sed -i "${n}s#.*#${replacement}#" "$file"
}

# mutation_case is MUT-001: turn off the default registration of the native
# Rust extractor and prove the same accept can never pass with the defect in
# place.
#
# The guard's condition is replaced by a constant `false` rather than the
# registration block being deleted: deleting the block would leave the
# `rustExtractor` import in cmd/regenerate-docs/main.go unused, which is a
# COMPILE failure -- and this card's own TDD proof states plainly that a
# compile failure does not count as RED/mutation evidence. `if false {`
# keeps the block's body (and so the import) referenced, keeps the binary
# compiling, and is behaviorally exact: the native Rust extractor is never
# registered by default, which is precisely "desligar o registro de Rust".
# The committed source is never touched (only a scratch copy), so
# restoration is by construction: shared_bin still documents Rust afterward.
mutation_case() {
  build_shared

  local root="$run_dir/root-mut"
  stage_source "$root"
  local target="$root/cmd/regenerate-docs/main.go"

  local anchor=$'\tif !repoCodeOptIn["rust"] {'
  local replacement=$'\tif false {'

  mutate_line "$target" "$anchor" "$replacement" || fail 'MUT-001/anchor-not-unique'
  grep -Fxq "$replacement" "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/regenerate-docs-mut"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/regenerate-docs >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  # Direct behavioral evidence: the mutant produces zero Rust pages from the
  # card's own fixture -- the exact defect MUT-001 names.
  local mut_out="$run_dir/mut-site"
  set +e
  ( ulimit -v 8388608
    AURUMCODE_SOURCE_DIR="$root/tests/fixtures/docs/rustproject" \
      AURUMCODE_OUTPUT_DIR="$mut_out" GOMEMLIMIT=2GiB "$bin"
  ) >"$run_dir/mut.out" 2>"$run_dir/mut.err"
  local mut_rc=$?
  set -e
  if [[ -s "$mut_out/rust/src__lib.md" ]]; then
    fail 'MUT-001/mutation-had-no-observable-effect'
  fi
  # The fixture holds only Rust source, so with Rust's registration off the
  # run documents nothing at all: AUR-425's degradation floor makes that
  # exit non-zero rather than a silent empty success. Both signals -- the
  # missing page and the non-zero exit -- are the mutation's effect.
  [[ "$mut_rc" -ne 0 ]] || fail 'MUT-001/mutant-exited-zero-with-no-documentation'

  # This mutation is deliberately NOT caught by IntegrationAUR427's textual
  # call-site check: that check greps main.go for the constructor call text
  # "rustExtractor.NewNativeExtractor()", which is still present (just
  # unreachable) after this mutation -- the guard's condition changed, not
  # the call itself. A grep-for-call-site check can only ever prove the call
  # was not DELETED, never that it is still REACHED; catching a
  # reachability defect needs the direct behavioral evidence above (the
  # built mutant binary's own generated tree), which this mutation_case
  # already produced. That asymmetry is why MUT-001's own falsifier here is
  # the binary run, not a rebuilt Integration selector.

  # Restoration: the unmutated shared binary still documents Rust -- the
  # GREEN reproduces exactly.
  local restore_out="$run_dir/restore-site"
  ( ulimit -v 8388608
    AURUMCODE_SOURCE_DIR="$root/tests/fixtures/docs/rustproject" \
      AURUMCODE_OUTPUT_DIR="$restore_out" GOMEMLIMIT=2GiB "$shared_bin"
  ) >/dev/null 2>&1
  [[ -s "$restore_out/rust/src__lib.md" ]] || fail 'MUT-001/restoration-broken'

  cleanup_root "$root"
  printf '%s/%s/MUT-001/rejected\n' "$card" "$scenario"
}

run_all() {
  nominal_case
  unit_case
  integration_case
  e2e_case
  regression_case
  mutation_case
  cleanup_root "$shared_root"
  printf '%s/%s/ok\n' "$card" "$scenario"
}

case "$selector" in
  AC-001) run_all ;;
  TestAUR427) unit_case ;;
  IntegrationAUR427) integration_case ;;
  E2EAUR427) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
