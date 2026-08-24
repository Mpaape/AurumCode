#!/usr/bin/env bash
# AUR-474 E2E: run the real Bash extractor, through its real Extract()
# entrypoint, against a synthetic script whose functions all open and close
# on the SAME physical line -- the exact "Achado medido" shape the card
# measured -- and check the page actually written to disk.
#
# WHY A SMALL DRIVER PROGRAM INSTEAD OF cmd/regenerate-docs
#   internal/documentation/extractors/bash is this card's own package and
#   needs no other extractor, no LLM provider, and no pipeline wiring to run
#   end-to-end: Extract() takes a source dir and an output dir and writes
#   real files. A tiny `go run` driver exercises the exact same public
#   entrypoint cmd/regenerate-docs would call, without pulling in
#   cmd/regenerate-docs's much larger dependency closure (which sits outside
#   this card's read_paths).
#
# WHAT IT PROVES
#   1. A one-line function declaration (`name() { ... }`) produces a real
#      "### function <name>" heading, with its preceding comment attached
#      as ITS doc -- never falling mute into "## Script Notes".
#   2. The `function name { ... }` and `function name() { ... }` one-line
#      variants are recognized the same way.
#   3. The four AC-002 false-positive shapes (my-array=(), x=$(cmd), a
#      one-line if/fi, and a quoted string containing "foo() { }") produce
#      zero symbols.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR474}" == E2EAUR474 ]] || { printf 'AUR-474/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-474/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-474/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e474.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home" "$run_dir/src" "$run_dir/out"

host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra 'gomodcache_absent'

cat >"$run_dir/src/oneliners.sh" <<'SH'
#!/bin/bash
set -euo pipefail

# Greets a name.
greet() { echo "hello $1"; }

# Says goodbye.
function farewell { echo "bye $1"; }

# Restarts a service.
function restart_svc() { systemctl restart "$1"; }

my-array=()
x=$(cmd)
if [ -f "$1" ]; then echo "found"; fi
echo "foo() { }"
SH

# The driver must physically live inside this module's own tree (not under
# /tmp) because it imports internal/... packages: Go's internal-import rule
# is a property of the FILE's own path relative to the module root, not of
# the working directory a `go run` is launched from. tests/e2e is this
# card's own owned path, so a driver file written there (and removed again
# before this script exits, success or failure) never leaves the tree
# dirty and never needs any path outside this card's paths/read_paths.
driver_file="$repo_root/tests/e2e/aur474_e2e_driver_main.go"
cleanup_driver() { rm -f -- "$driver_file"; }
trap 'cleanup_driver; rm -rf -- "$run_dir"' EXIT INT TERM HUP
cat >"$driver_file" <<'GOEOF'
package main

import (
	"context"
	"fmt"
	"os"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func main() {
	src := os.Args[1]
	out := os.Args[2]
	ext := bashextractor.NewBashExtractor(site.NewMockRunner())
	res, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
		Language: extractors.LanguageBash, SourceDir: src, OutputDir: out,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "extract error:", err)
		os.Exit(1)
	}
	if len(res.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "extract errors:", res.Errors)
		os.Exit(1)
	}
}
GOEOF

run_driver() {
  local src="$1" out="$2"
  (
    cd "$repo_root" && ulimit -v 8388608 && \
    HOME="$run_dir/home" GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    GOMODCACHE="$host_modcache" GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local \
    GOFLAGS=-mod=mod GOMEMLIMIT=2GiB \
    go run "$driver_file" "$src" "$out"
  )
}

run_driver "$run_dir/src" "$run_dir/out" || fail 'behavior-missing: bash driver failed'

page="$run_dir/out/oneliners.sh.md"
[[ -s "$page" ]] || fail 'behavior-missing: page not generated'

# AC-001: all three one-line forms became real symbols with their own doc.
grep -Fq '### function greet' "$page" || fail 'behavior-missing: greet heading missing'
grep -Fq '### function farewell' "$page" || fail 'behavior-missing: farewell heading missing'
grep -Fq '### function restart_svc' "$page" || fail 'behavior-missing: restart_svc heading missing'

check_own_doc() {
  local heading="$1" doc="$2"
  awk -v h="$heading" -v d="$doc" '
    $0 == h { f=1; next }
    /^```/ { if (f) { exit } }
    f && index($0, d) { found=1; exit }
    END { exit !found }
  ' "$page" || fail "behavior-missing: ${heading#\#\#\# } lost its own doc (likely swept into Script Notes)"
}
check_own_doc '### function greet' 'Greets a name.'
check_own_doc '### function farewell' 'Says goodbye.'
check_own_doc '### function restart_svc' 'Restarts a service.'

# AC-001 second half: none of these docs leaked into Script Notes instead.
if grep -Fq '## Script Notes' "$page"; then
  notes_block="$(awk '/^## Script Notes/{f=1;next} /^### /{f=0} f' "$page")"
  for doc in 'Greets a name.' 'Says goodbye.' 'Restarts a service.'; do
    grep -Fq "$doc" <<<"$notes_block" && fail "false-claim: '$doc' leaked into Script Notes"
  done
fi

# AC-002: the four false-positive shapes produced zero symbols.
grep -Fq '### function my-array' "$page" && fail 'false-claim: my-array=() became a symbol'
grep -Fq '### function cmd' "$page" && fail 'false-claim: x=$(cmd) became a symbol'
grep -Fq '### function found' "$page" && fail 'false-claim: one-line if/fi became a symbol'
grep -Fq '### function foo' "$page" && fail 'false-claim: quoted foo() { } became a symbol'

symbol_count="$(grep -Fc '### function ' "$page")"
[[ "$symbol_count" -eq 3 ]] || fail "false-claim: expected exactly 3 symbols, got $symbol_count"

printf 'AUR-474/AC-001/pass\n'
