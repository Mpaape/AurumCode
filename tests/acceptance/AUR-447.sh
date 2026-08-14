#!/usr/bin/env bash
#
# Acceptance program for card AUR-447, scenario AC-001.
#
# WHAT THIS PROVES
#
#   `aurumcode docs --source <projeto Rust ou C#>` generates real
#   documentation pages the same way `cmd/regenerate-docs` already does.
#   Before this card, `cmd/aurumcode/docs.go` carried its own registration
#   list that predated AUR-427 and never grew the native, tool-free Rust/C#
#   extractors cmd/regenerate-docs registers by default: running the CLI
#   against a Rust-only or C#-only source tree returned exit 1, "no
#   extractor registered", zero pages, while cmd/regenerate-docs documented
#   the same tree correctly. This card makes cmd/aurumcode/docs.go register
#   the same native extractors (internal/documentation/extractors/rust,
#   .../csharp -- AUR-427), reusing them rather than reimplementing
#   extraction, and proves Go still works (no regression) alongside Rust and
#   C#. See docs/specs/AUR-447.md.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED
#   64 = unknown scenario selector
#   79 = inconclusive / infrastructure: an input this card does not own was
#        never materialized, a required tool is missing. Never valid red
#        evidence, never a pass.
#
# This program emits observations only. It never writes evidence, issues a
# verdict, or asserts approval.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-447'
readonly scenario='AC-001'
selector="${1:-AC-001}"

case "$selector" in
  AC-001|TestAUR447|IntegrationAUR447|E2EAUR447|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Offline module cache holding gopkg.in/yaml.v3 (internal/pipeline's
# _config.yml exclude-list reader): resolved BEFORE HOME is redirected,
# since the default cache location derives from HOME.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra gomodcache_absent

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a447.XXXXXX")" || infra mktemp
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
# invocation in this file -- the profile's memory ceiling kills a parallel
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

# stage_source materializes exactly this card's read-set: the full closure
# cmd/aurumcode needs to build (mirroring AUR-426.sh/AUR-443.sh's own
# stage_source -- cmd/aurumcode is one Go package shared with concurrently
# dispatched cards, so pr.go's internal/git/githubclient import has to
# resolve too), plus this card's own Rust fixture and doc/spec.
stage_source() {
  local root="$1"
  mkdir -p "$root"
  copy "$root" go.mod go.sum
  copy "$root" cmd/aurumcode
  copy "$root" internal/git
  copy "$root" cmd/regenerate-docs
  copy "$root" internal/documentation/extractors internal/documentation/incremental internal/documentation/normalizer internal/documentation/site internal/documentation/welcome
  copy "$root" internal/pipeline
  copy "$root" internal/analyzer internal/prompt internal/review internal/llm internal/security/redaction pkg/types
  copy "$root" tests/fixtures/docs/goproject tests/fixtures/docs/rustproject tests/fixtures/repos/git-demo tests/fixtures/review
  copy "$root" action.yml
  copy "$root" docs/specs/AUR-447.md
  # cp -R preserves the read-only mode bits of the materialized input; the
  # staged copy is scratch from here on, so force it writable for the
  # mutation case's sed and for cleanup_root.
  chmod -R u+w -- "$root"
}

# build_shared builds the binary once per acceptance run and reuses it for
# every lane; GOCACHE is shared process-wide, so the go test compiles and the
# mutation rebuild start warm instead of cold.
shared_root="$run_dir/root-shared"
shared_bin="$run_dir/aurumcode"
shared_built=0
build_shared() {
  ((shared_built == 0)) || return 0
  stage_source "$shared_root"
  local log="$shared_root/build.log"
  if ! run_go "$shared_root" build -o "$shared_bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail build_failed
  fi
  shared_built=1
}

# ephemeral_csharp_fixture writes a small, real C# project into dir, byte-
# identical to tests/acceptance/AUR-427.sh's own fixture: this card's `paths`
# grants a checked-in Rust fixture directory only (tests/fixtures/docs/
# rustproject), not a checked-in C# one, so C#'s nominal proof uses a
# runtime-only input, exactly as AUR-427's acceptance and e2e programs do.
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
# real binary: `aurumcode docs --source <rust-or-csharp-project>` generates
# real pages carrying real symbols and real doc comments for both languages,
# Go keeps working (no regression), the run is deterministic, a secret
# canary never reaches stdout or stderr, and `aurumcode review`'s published
# contract survives untouched.
nominal_case() {
  build_shared
  local rc

  # --help still works: this card changes registration, not the flag set or
  # its usage text.
  set +e
  "$shared_bin" docs --help >"$run_dir/help.stdout" 2>"$run_dir/help.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail help-broken
  grep -Fq 'usage: aurumcode docs' "$run_dir/help.stdout" || fail help-broken

  # Real generation against the card's own checked-in Rust fixture.
  local rust_fixture="$shared_root/tests/fixtures/docs/rustproject"
  [[ -d "$rust_fixture" ]] || fail missing-rust-fixture
  local rust_out="$run_dir/rust-site"
  set +e
  "$shared_bin" docs --source "$rust_fixture" --output "$rust_out" \
    >"$run_dir/rust1.stdout" 2>"$run_dir/rust1.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail behavior-missing:rust-run
  local rust_page="$rust_out/rust/src__lib.md"
  [[ -s "$rust_page" ]] || fail missing-generated-rust-page
  for want in 'pub struct Entry' 'pub fn new_entry' 'Creates a new ledger entry.' \
              'pub const MAX_ENTRIES_PER_PAGE' 'pub struct Ledger' 'pub enum EntryKind'; do
    grep -Fq "$want" "$rust_page" || fail "generated-rust-page-missing-symbol:$want"
  done
  ! grep -Fq 'pub fn record_internal' "$rust_page" || fail 'false-claim:record_internal'
  ! grep -Fq 'entry_count' "$rust_page" || fail 'false-claim:entry_count'

  # Determinism: repeating the run over the same input produces the same
  # output (AC-001's "And" clause). First, over a SEPARATE output directory,
  # comparing generated file content byte-for-byte (proves the pages
  # themselves never embed anything path- or time-dependent); then, over the
  # SAME output directory (mirrors tests/acceptance/AUR-426.sh's own
  # determinism check), comparing the command's own stdout summary verbatim.
  set +e
  "$shared_bin" docs --source "$rust_fixture" --output "$run_dir/rust-site2" \
    >"$run_dir/rust2.stdout" 2>/dev/null
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail non-deterministic:second-run-failed
  local d1 d2
  d1="$(cd "$rust_out" && find . -type f | LC_ALL=C sort | xargs -r sha256sum | sha256sum | cut -d' ' -f1)"
  d2="$(cd "$run_dir/rust-site2" && find . -type f | LC_ALL=C sort | xargs -r sha256sum | sha256sum | cut -d' ' -f1)"
  [[ "$d1" == "$d2" ]] || fail "non-deterministic:$d1:$d2"

  set +e
  "$shared_bin" docs --source "$rust_fixture" --output "$rust_out" \
    >"$run_dir/rust3.stdout" 2>/dev/null
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail non-deterministic:third-run-failed
  [[ "$(cat "$run_dir/rust1.stdout")" == "$(cat "$run_dir/rust3.stdout")" ]] || fail non-deterministic:stdout

  # Real generation against an ephemeral C# fixture.
  local cs_src="$run_dir/csharpproject"
  ephemeral_csharp_fixture "$cs_src"
  local cs_out="$run_dir/cs-site"
  set +e
  "$shared_bin" docs --source "$cs_src" --output "$cs_out" \
    >"$run_dir/cs1.stdout" 2>"$run_dir/cs1.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail behavior-missing:csharp-run
  local cs_page="$cs_out/csharp/Greeter.md"
  [[ -s "$cs_page" ]] || fail missing-generated-csharp-page
  for want in 'public class Greeter' 'public Greeter(string lang)' \
              'public string Greet(string name)' 'A human-readable greeting sentence.'; do
    grep -Fq "$want" "$cs_page" || fail "generated-csharp-page-missing-symbol:$want"
  done
  ! grep -Fq 'IsPortuguese' "$cs_page" || fail 'false-claim:IsPortuguese'

  # Go keeps working: the fix must not regress the language this subcommand
  # already documented before this card (AUR-426).
  local go_fixture="$shared_root/tests/fixtures/docs/goproject"
  [[ -d "$go_fixture" ]] || fail missing-go-fixture
  local go_out="$run_dir/go-site"
  set +e
  "$shared_bin" docs --source "$go_fixture" --output "$go_out" \
    >"$run_dir/go1.stdout" 2>"$run_dir/go1.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail behavior-missing:go-run
  local go_page="$go_out/go/root.md"
  [[ -s "$go_page" ]] || fail missing-generated-go-page
  for want in 'Greeting' 'func Add' 'func Max'; do
    grep -Fq "$want" "$go_page" || fail "generated-go-page-missing-symbol:$want"
  done

  # A secret canary must never reach stdout or stderr (AUR-009/AUR-432), even
  # when embedded in a path this command itself prints (--output).
  local canary='aur447-nominal-canary-9c2e7b'
  set +e
  ( ulimit -v 8388608
    AURUM_SECRET_CANARY="$canary" GOMEMLIMIT=2GiB \
      "$shared_bin" docs --source "$rust_fixture" --output "$run_dir/${canary}-site"
  ) >"$run_dir/canary.stdout" 2>"$run_dir/canary.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail canary-run-failed
  ! grep -Fq "$canary" "$run_dir/canary.stdout" || fail canary-leaked-stdout
  ! grep -Fq "$canary" "$run_dir/canary.stderr" || fail canary-leaked-stderr

  # aurumcode review's published contract (AUR-430/431/435/436/438) is
  # byte-for-byte unchanged by this card's fix.
  local review_fixture="$run_dir/response-clean.json"
  printf '{"issues":[],"summary":"Nothing to report."}' >"$review_fixture"
  local repo_dir="$shared_root/tests/fixtures/repos/git-demo/repo.git"
  [[ -d "$repo_dir" ]] || fail missing-git-demo-fixture
  set +e
  (cd "$repo_dir" && AURUMCODE_LLM_FIXTURE="$review_fixture" "$shared_bin" review --base HEAD~1) \
    >"$run_dir/review.stdout" 2>"$run_dir/review.stderr"
  rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail review-contract-broken
  [[ "$(cat "$run_dir/review.stdout")" == "No issues found." ]] || fail review-contract-broken
}

unit_case() {
  local root="$run_dir/root-unit"
  stage_source "$root"
  copy "$root" tests/unit/AUR-447.go
  cat >"$root/tests/unit/aur447_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR447UnitBridge(t *testing.T) { TestAUR447(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -v -timeout 300s ./tests/unit -run '^TestAUR447UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:TestAUR447:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:TestAUR447:zero-tests
  cleanup_root "$root"
}

integration_case() {
  local root="$run_dir/root-integration"
  stage_source "$root"
  copy "$root" tests/integration/AUR-447.go
  cat >"$root/tests/integration/aur447_bridge_test.go" <<'EOF'
package integration

import "testing"

func TestAUR447IntegrationBridge(t *testing.T) { IntegrationAUR447(t) }
EOF
  local out rc
  set +e
  out="$(AURUMCODE_ROOT="$root" run_go "$root" test -v -timeout 300s ./tests/integration -run '^TestAUR447IntegrationBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  printf '%s\n' "$out" | sed -E 's#\([0-9]+\.[0-9]+s\)#(TIMEs)#g; s#[0-9]+\.[0-9]+s$#TIMEs#g'
  ((rc == 0)) || fail "selector:IntegrationAUR447:exit:$rc"
  grep -Eq '(^|[[:space:]])ok[[:space:]]' <<<"$out" || fail selector:IntegrationAUR447:zero-tests
  cleanup_root "$root"
}

e2e_case() {
  build_shared
  local root="$run_dir/root-e2e"
  stage_source "$root"
  copy "$root" tests/e2e/AUR-447.sh
  # Reuse the already-built binary and the warm GOCACHE (exported above)
  # instead of letting the nested script cold-compile its own copy. The
  # nested script's own exit-code vocabulary is preserved: its 79 is an
  # environment gap and must be re-emitted as infra here, never collapsed
  # into behavioral RED.
  local rc
  set +e
  (cd "$root" && AURUMCODE_BIN="$shared_bin" bash tests/e2e/AUR-447.sh E2EAUR447)
  rc=$?
  set -e
  ((rc != 79)) || infra "e2e-inconclusive:$rc"
  ((rc == 0)) || fail "e2e-failed:exit:$rc"
  cleanup_root "$root"
}

# regression_case proves the non-negotiable "cmd/aurumcode's own pre-existing
# tests, and the untouched native extractor packages' own tests, still
# pass": cmd/aurumcode carries real _test.go files of its own (pr_test.go,
# failon_test.go, redaction_wiring_test.go), and this card touches only
# cmd/aurumcode/docs.go inside that package.
regression_case() {
  local root="$run_dir/root-regression"
  stage_source "$root"
  local out rc
  set +e
  out="$(run_go "$root" test -timeout 300s \
    ./cmd/aurumcode \
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

# mutation_case is MUT-001: turn off the CLI's registration of the native
# Rust/C# extractors and prove the same accept can never pass with the
# defect in place.
#
# The guard's value is replaced by a constant `false` rather than the
# registration block being deleted: deleting the block would leave the
# `rustExtractor`/`csharpExtractor` imports in cmd/aurumcode/docs.go unused,
# which is a COMPILE failure -- and this card's own TDD proof states plainly
# that a compile failure does not count as RED/mutation evidence (AUR-427's
# own MUT-001 hit the identical constraint and solved it the same way). The
# constant's body still calls both constructors in source, so the imports
# stay referenced and the binary still compiles; only the registration is
# skipped at runtime. The committed source is never touched (only a scratch
# copy), so restoration is by construction: shared_bin still documents Rust
# and C# afterward.
mutation_case() {
  build_shared

  local root="$run_dir/root-mut"
  stage_source "$root"
  local target="$root/cmd/aurumcode/docs.go"

  local anchor=$'\tconst registerNativeRustAndCSharp = true'
  local replacement=$'\tconst registerNativeRustAndCSharp = false'

  mutate_line "$target" "$anchor" "$replacement" || fail 'MUT-001/anchor-not-unique'
  grep -Fxq "$replacement" "$target" || fail 'MUT-001/mutation-not-applied'

  local bin="$run_dir/aurumcode-mut"
  local log="$root/build-mut.log"
  if ! run_go "$root" build -o "$bin" ./cmd/aurumcode >"$log" 2>&1; then
    cat "$log" >&2
    fail 'MUT-001/build-failed'
  fi

  # Direct behavioral evidence: the mutant produces zero Rust pages from the
  # card's own fixture -- the exact defect MUT-001 names ("nenhuma pagina
  # Rust e gerada").
  local mut_out="$run_dir/mut-site"
  set +e
  ( ulimit -v 8388608
    GOMEMLIMIT=2GiB "$bin" docs --source "$root/tests/fixtures/docs/rustproject" --output "$mut_out"
  ) >"$run_dir/mut.stdout" 2>"$run_dir/mut.stderr"
  local mut_rc=$?
  set -e
  if [[ -s "$mut_out/rust/src__lib.md" ]]; then
    fail 'MUT-001/mutation-had-no-observable-effect'
  fi
  grep -Fq 'no extractor registered' "$run_dir/mut.stderr" || fail 'MUT-001/mutant-missing-expected-message'
  # The fixture holds only Rust source, so with Rust's registration off the
  # run documents nothing at all: AUR-425's degradation floor makes that
  # exit non-zero rather than a silent empty success. Both signals -- the
  # missing page and the non-zero exit -- are the mutation's effect.
  [[ "$mut_rc" -ne 0 ]] || fail 'MUT-001/mutant-exited-zero-with-no-documentation'

  # The card's own declared Unit selector independently catches it too:
  # rebuilt against the mutated tree, TestAUR447's Rust-generation subtest
  # must fail -- the same accept the nominal lanes above must never pass
  # under, now proven to actually reject the defect rather than merely
  # asserting it should.
  copy "$root" tests/unit/AUR-447.go
  cat >"$root/tests/unit/aur447_bridge_test.go" <<'EOF'
package unit

import "testing"

func TestAUR447UnitBridge(t *testing.T) { TestAUR447(t) }
EOF
  local out rc
  set +e
  out="$(run_go "$root" test -timeout 300s ./tests/unit -run '^TestAUR447UnitBridge$' -count=1 2>&1)"
  rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail 'MUT-001/not-rejected'
  grep -Fq -- '--- FAIL: TestAUR447UnitBridge' <<<"$out" || fail 'MUT-001/wrong-failure-reason'

  # Restoration: the unmutated shared binary still documents Rust and C# --
  # the GREEN reproduces exactly.
  local restore_out="$run_dir/restore-rust-site"
  ( ulimit -v 8388608
    GOMEMLIMIT=2GiB "$shared_bin" docs --source "$shared_root/tests/fixtures/docs/rustproject" --output "$restore_out"
  ) >/dev/null 2>&1
  [[ -s "$restore_out/rust/src__lib.md" ]] || fail 'MUT-001/restoration-broken'

  local restore_cs_src="$run_dir/restore-csharpproject"
  ephemeral_csharp_fixture "$restore_cs_src"
  local restore_cs_out="$run_dir/restore-cs-site"
  ( ulimit -v 8388608
    GOMEMLIMIT=2GiB "$shared_bin" docs --source "$restore_cs_src" --output "$restore_cs_out"
  ) >/dev/null 2>&1
  [[ -s "$restore_cs_out/csharp/Greeter.md" ]] || fail 'MUT-001/csharp-restoration-broken'

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
  TestAUR447) unit_case ;;
  IntegrationAUR447) integration_case ;;
  E2EAUR447) e2e_case ;;
  AC-001-MUT-001) mutation_case ;;
esac
