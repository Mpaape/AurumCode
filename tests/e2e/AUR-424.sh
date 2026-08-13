#!/usr/bin/env bash
# AUR-424 E2E: run the card's declared command for real and check the pages it
# actually wrote.
#
# WHAT CHANGED AND WHY
#   The first version of this file was a grep over the bytes of extractor.go:
#   it checked that "os/exec" was absent and that go/parser was imported. That
#   is a lint, not an end-to-end test -- it passes on source that does not
#   compile, and it cannot tell a working extractor from a broken one. The
#   board's own rule refuses a selector that passes without executing a real
#   assertion, so this now runs `go run ./cmd/regenerate-docs` exactly as
#   AC-001 declares it and asserts against the generated Markdown.
#
# WHAT IT PROVES
#   1. The declared command produces a real page carrying the fixture's real
#      exported symbols and their real doc comments.
#   2. Repeating the run over the same input produces byte-identical output
#      (AC-001's repeat clause).
#   3. The same binary produces the same pages with an EMPTY PATH. This is the
#      behavioural falsifier for MUT-001: an extractor that goes back to
#      shelling out to gomarkdoc finds nothing to shell out to here, reports
#      the language as skipped, and writes no page.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR424}" == E2EAUR424 ]] || { printf 'AUR-424/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-424/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-424/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
command -v sha256sum >/dev/null 2>&1 || infra missing_sha256sum

fixture_dir="$repo_root/tests/fixtures/docs/goproject"
[[ -d "$fixture_dir" ]] || fail 'behavior-missing: card fixture absent'
[[ -f "$repo_root/cmd/regenerate-docs/main.go" ]] || infra 'cmd_regenerate_docs_absent'

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e424.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home" "$run_dir/emptypath"

# cmd/regenerate-docs depends on gopkg.in/yaml.v3, so the run needs a module
# cache that already holds it: GOPROXY stays off, because this card's
# acceptance is offline and a download would be a network call. The ambient
# cache is resolved BEFORE HOME is overridden, since HOME is what the default
# location is derived from.
host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra 'gomodcache_absent'

# Every Go invocation below is bounded: 8 GiB of address space and a 2 GiB heap
# ceiling, so a reviewer running this file directly gets the same protection
# the office session does.
go_env=(
  HOME="$run_dir/home"
  GOPROXY=off
  GOSUMDB=off
  GOFLAGS=-mod=mod
  GOTOOLCHAIN=local
  GOMAXPROCS=1
  GOMEMLIMIT=2GiB
  GOCACHE="$run_dir/cache"
  GOTMPDIR="$run_dir/gotmp"
  GOMODCACHE="$host_modcache"
)

# declared_run executes the card's Public contract command verbatim, from the
# repository root, writing into the caller-named output directory.
declared_run() {
  local out="$1" label="$2" rc
  set +e
  ( ulimit -v 8388608
    cd "$repo_root" &&
      env "${go_env[@]}" \
        AURUMCODE_SOURCE_DIR=./tests/fixtures/docs/goproject \
        AURUMCODE_OUTPUT_DIR="$out" \
        go run ./cmd/regenerate-docs
  ) >"$run_dir/$label.out" 2>&1
  rc=$?
  set -e
  return "$rc"
}

# The first run doubles as the infrastructure probe: this card's read_paths
# does not materialize cmd/regenerate-docs's transitive local dependencies, so
# inside the acceptance sandbox the command cannot be compiled at all. That is
# a missing input, not a failed behaviour, and must be reported as such rather
# than as a red assertion.
if ! declared_run "$run_dir/site1" run1; then
  if grep -Eq 'no required module provides|cannot find package|no Go files|is not in std|package .* is not in|module lookup disabled' "$run_dir/run1.out"; then
    printf 'AUR-424/AC-001/infrastructure/regenerate_docs_deps_not_materialized\n' >&2
    sed -n '1,20p' "$run_dir/run1.out" >&2
    exit 69
  fi
  sed -n '1,40p' "$run_dir/run1.out" >&2
  fail 'behavior-missing: the declared command exited non-zero'
fi

page="$run_dir/site1/go/root.md"
[[ -s "$page" ]] || {
  printf 'generated tree:\n' >&2
  find "$run_dir/site1" -type f >&2 || true
  fail 'behavior-missing: no page was generated for the Go fixture'
}

# The page has to carry the fixture's real API and the real prose attached to
# it: an empty or placeholder page would satisfy a mere existence check.
for want in \
  'Package goproject' \
  'Greeting' 'Greeting represents a salutation' \
  'NewGreeting' 'NewGreeting builds a Greeting for name in the given language' \
  'func Add' 'Add returns the sum of a and b' \
  'func Max' 'Max returns the larger of a and b' \
  'String renders the greeting as a human-readable sentence'
do
  grep -Fq "$want" "$page" || {
    printf -- '--- generated page ---\n' >&2
    cat "$page" >&2
    fail "behavior-missing: generated page does not carry ${want}"
  }
done

# AC-001: repeating the run over the same input produces the same output.
declared_run "$run_dir/site2" run2 || {
  sed -n '1,40p' "$run_dir/run2.out" >&2
  fail 'behavior-missing: the second declared run exited non-zero'
}

digest_of() { (cd "$1" && find . -type f | LC_ALL=C sort | xargs -r sha256sum | sha256sum | cut -d' ' -f1); }

digest1="$(digest_of "$run_dir/site1")"
digest2="$(digest_of "$run_dir/site2")"
[[ "$digest1" == "$digest2" ]] || {
  diff -ru "$run_dir/site1" "$run_dir/site2" >&2 || true
  fail "nondeterministic-output: ${digest1} != ${digest2}"
}

# MUT-001, behaviourally: the compiled binary must document this project with
# no executable of any kind reachable on PATH. `go build` needs the toolchain,
# so the build keeps the real PATH and only the run is stripped.
binary="$run_dir/regenerate-docs"
if ! ( ulimit -v 8388608
       cd "$repo_root" && env "${go_env[@]}" go build -o "$binary" ./cmd/regenerate-docs
     ) >"$run_dir/build.out" 2>&1; then
  sed -n '1,20p' "$run_dir/build.out" >&2
  infra 'regenerate_docs_build_failed'
fi

if ! ( ulimit -v 8388608
       cd "$repo_root" &&
         env -i PATH="$run_dir/emptypath" HOME="$run_dir/home" GOMEMLIMIT=2GiB \
           AURUMCODE_SOURCE_DIR=./tests/fixtures/docs/goproject \
           AURUMCODE_OUTPUT_DIR="$run_dir/site3" \
           "$binary"
     ) >"$run_dir/run3.out" 2>&1; then
  sed -n '1,40p' "$run_dir/run3.out" >&2
  fail 'MUT-001/declared-command-failed-without-path'
fi

! grep -Fq 'skipped: go' "$run_dir/run3.out" ||
  fail 'MUT-001/go-skipped-without-an-external-tool'

[[ -s "$run_dir/site3/go/root.md" ]] ||
  fail 'MUT-001/no-page-generated-without-an-external-tool'

digest3="$(digest_of "$run_dir/site3")"
[[ "$digest1" == "$digest3" ]] || {
  diff -ru "$run_dir/site1" "$run_dir/site3" >&2 || true
  fail "MUT-001/output-depends-on-path: ${digest1} != ${digest3}"
}

printf 'e2e=ok runs=3 empty_path_run=1 digest=%s\n' "$digest1"
