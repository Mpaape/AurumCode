#!/usr/bin/env bash
# AUR-427 E2E: run the declared command for real and check the pages it
# actually wrote, for both Rust and C#.
#
# WHY THIS RUNS THE REAL BINARY (AND CAN, UNLIKE AUR-424's E2E)
#   AUR-424's E2E could not compile `cmd/regenerate-docs` inside the offline
#   acceptance sandbox: its `read_paths` named `cmd/regenerate-docs` plus four
#   specific sibling files, not the whole transitive closure `main.go` needs
#   (internal/pipeline, internal/llm and its providers). AUR-427's
#   `read_paths` was widened past that gap -- it names `internal/pipeline` and
#   `internal/llm` directly -- so this script runs the declared command for
#   real and expects a genuine exit 0, not the infrastructure exit 69 AUR-424
#   had to settle for. See docs/specs/AUR-427.md for the measured detail.
#
# WHAT IT PROVES
#   1. The declared command produces real pages for Rust (the card's own
#      checked-in fixture, tests/fixtures/docs/rustproject) AND for C# (an
#      ephemeral fixture this script writes itself: this card's `paths` grants
#      a checked-in fixture directory for Rust only, not C#; see
#      docs/specs/AUR-427.md's "Public contract gap" section for why).
#   2. Repeating the Rust run over the same input produces byte-identical
#      output (AC-001's repeat clause).
#   3. A secret canary never reaches stdout or stderr.
#   4. Coverage is honestly partial: a macro-generated Rust symbol and a
#      non-public C# member, both real and both present in their fixtures,
#      never appear on the generated pages.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR427}" == E2EAUR427 ]] || { printf 'AUR-427/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-427/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-427/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
command -v sha256sum >/dev/null 2>&1 || infra missing_sha256sum

fixture_dir="$repo_root/tests/fixtures/docs/rustproject"
[[ -d "$fixture_dir" ]] || fail 'behavior-missing: card fixture absent'
[[ -f "$repo_root/cmd/regenerate-docs/main.go" ]] || infra 'cmd_regenerate_docs_absent'

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e427.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

# cmd/regenerate-docs depends on gopkg.in/yaml.v3, so the run needs a module
# cache that already holds it: GOPROXY stays off, because this card's
# acceptance is offline and a download would be a network call. The ambient
# cache is resolved BEFORE HOME is overridden, since HOME is what the default
# location is derived from.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra 'gomodcache_absent'

# Every Go invocation below is bounded: the profile's 256 MB memory ceiling
# kills a parallel compile, so GOFLAGS carries -mod=mod (offline, read-only
# module list) and -p=1 (single build/test process); GOMEMLIMIT gives the
# runtime a soft target under the hard ceiling.
go_env=(
  HOME="$run_dir/home"
  GOPROXY=off
  GOSUMDB=off
  GOFLAGS='-mod=mod -p=1'
  GOTOOLCHAIN=local
  GOMAXPROCS=1
  GOMEMLIMIT=2GiB
  GOCACHE="$run_dir/cache"
  GOTMPDIR="$run_dir/gotmp"
  GOMODCACHE="$host_modcache"
)

# declared_run executes the card's declared command verbatim, from the
# repository root, writing into the caller-named output directory. If
# AURUMCODE_BIN names an already-built binary (set by
# tests/acceptance/AUR-427.sh's e2e_case to reuse its warm shared build), that
# binary is run directly instead of `go run` re-compiling from source.
declared_run() {
  local src="$1" out="$2" label="$3" rc
  set +e
  if [[ -n "${AURUMCODE_BIN:-}" ]]; then
    ( ulimit -v 8388608
      AURUMCODE_SOURCE_DIR="$src" AURUMCODE_OUTPUT_DIR="$out" \
        GOMEMLIMIT=2GiB "$AURUMCODE_BIN"
    ) >"$run_dir/$label.out" 2>"$run_dir/$label.err"
  else
    ( ulimit -v 8388608
      cd "$repo_root" &&
        env "${go_env[@]}" \
          AURUMCODE_SOURCE_DIR="$src" AURUMCODE_OUTPUT_DIR="$out" \
          go run ./cmd/regenerate-docs
    ) >"$run_dir/$label.out" 2>"$run_dir/$label.err"
  fi
  rc=$?
  set -e
  return "$rc"
}

# The first run doubles as the infrastructure probe: if this card's
# read_paths ever regressed to not materializing cmd/regenerate-docs's
# transitive local dependencies, the command cannot be compiled at all inside
# the sandbox. That is a missing input, not a failed behaviour.
if ! declared_run "$fixture_dir" "$run_dir/site1" run1; then
  out="$(cat "$run_dir/run1.out" "$run_dir/run1.err" 2>/dev/null || true)"
  if grep -Eq 'no required module provides|cannot find package|no Go files|is not in std|package .* is not in|module lookup disabled' <<<"$out"; then
    printf 'AUR-427/AC-001/infrastructure/regenerate_docs_deps_not_materialized\n' >&2
    sed -n '1,20p' <<<"$out" >&2
    exit 69
  fi
  sed -n '1,40p' <<<"$out" >&2
  fail 'behavior-missing: the declared command exited non-zero for Rust'
fi

rust_page="$run_dir/site1/rust/src__lib.md"
[[ -s "$rust_page" ]] || {
  printf 'generated tree:\n' >&2
  find "$run_dir/site1" -type f >&2 || true
  fail 'behavior-missing: no page was generated for the Rust fixture'
}

for want in \
  'pub struct Entry' 'amount in cents' \
  'pub fn new_entry' 'Creates a new ledger entry.' \
  'pub const MAX_ENTRIES_PER_PAGE' \
  'pub struct Ledger' 'pub fn add' 'pub fn balance_cents' \
  'pub enum EntryKind'
do
  grep -Fq "$want" "$rust_page" || {
    printf -- '--- generated Rust page ---\n' >&2
    cat "$rust_page" >&2
    fail "behavior-missing: generated Rust page does not carry ${want}"
  }
done

# Honesty: a macro-generated symbol and a private method, both real and both
# present in the fixture source, must never be claimed as extracted.
if grep -Fq 'pub fn record_internal' "$rust_page"; then
  fail 'false-claim: generated Rust page reports record_internal as an extracted item'
fi
if grep -Fq 'entry_count' "$rust_page"; then
  fail 'false-claim: generated Rust page mentions non-public symbol entry_count'
fi

# AC-001: repeating the run over the same input produces the same output.
declared_run "$fixture_dir" "$run_dir/site2" run2 || {
  cat "$run_dir/run2.out" "$run_dir/run2.err" >&2 2>/dev/null || true
  fail 'behavior-missing: the second declared Rust run exited non-zero'
}

digest_of() { (cd "$1" && find . -type f | LC_ALL=C sort | xargs -r sha256sum | sha256sum | cut -d' ' -f1); }

digest1="$(digest_of "$run_dir/site1")"
digest2="$(digest_of "$run_dir/site2")"
[[ "$digest1" == "$digest2" ]] || {
  diff -ru "$run_dir/site1" "$run_dir/site2" >&2 || true
  fail "nondeterministic-output: ${digest1} != ${digest2}"
}

# --- C#: an ephemeral fixture this script writes itself. This card's `paths`
# grants a checked-in fixture directory for Rust only (see
# docs/specs/AUR-427.md); a runtime-only tmpfs input still exercises the real
# binary against real C# source without adding a file outside `paths`.
csharp_src="$run_dir/csharpproject"
mkdir -p "$csharp_src"
cat >"$csharp_src/Greeter.cs" <<'CSEOF'
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

declared_run "$csharp_src" "$run_dir/csite1" cs1 || {
  cat "$run_dir/cs1.out" "$run_dir/cs1.err" >&2 2>/dev/null || true
  fail 'behavior-missing: the declared command exited non-zero for C#'
}

csharp_page="$run_dir/csite1/csharp/Greeter.md"
[[ -s "$csharp_page" ]] || {
  printf 'generated tree:\n' >&2
  find "$run_dir/csite1" -type f >&2 || true
  fail 'behavior-missing: no page was generated for the C# fixture'
}

for want in \
  'public class Greeter' 'A greeter that says hello' \
  'public Greeter(string lang)' 'Creates a greeter for the given language tag.' \
  'public string Greet(string name)' 'Renders a greeting for the given name.' \
  'A human-readable greeting sentence.'
do
  grep -Fq "$want" "$csharp_page" || {
    printf -- '--- generated C# page ---\n' >&2
    cat "$csharp_page" >&2
    fail "behavior-missing: generated C# page does not carry ${want}"
  }
done

if grep -Fq 'IsPortuguese' "$csharp_page"; then
  fail 'false-claim: generated C# page mentions non-public member IsPortuguese'
fi

# Secrets: AURUM_SECRET_CANARY must never reach stdout or stderr.
canary='aur427-e2e-canary-9c2f61'
set +e
if [[ -n "${AURUMCODE_BIN:-}" ]]; then
  ( ulimit -v 8388608
    AURUM_SECRET_CANARY="$canary" AURUMCODE_SOURCE_DIR="$fixture_dir" \
      AURUMCODE_OUTPUT_DIR="$run_dir/site_canary" GOMEMLIMIT=2GiB "$AURUMCODE_BIN"
  ) >"$run_dir/canary.out" 2>"$run_dir/canary.err"
else
  ( ulimit -v 8388608
    cd "$repo_root" &&
      env "${go_env[@]}" AURUM_SECRET_CANARY="$canary" \
        AURUMCODE_SOURCE_DIR="$fixture_dir" AURUMCODE_OUTPUT_DIR="$run_dir/site_canary" \
        go run ./cmd/regenerate-docs
  ) >"$run_dir/canary.out" 2>"$run_dir/canary.err"
fi
canary_rc=$?
set -e
(( canary_rc == 0 )) || fail 'behavior-missing: the canary run exited non-zero'
! grep -Fq "$canary" "$run_dir/canary.out" || fail 'secret-leak: canary reached stdout'
! grep -Fq "$canary" "$run_dir/canary.err" || fail 'secret-leak: canary reached stderr'

printf 'e2e=ok rust_digest=%s csharp_page=1 canary=clean\n' "$digest1"
