#!/usr/bin/env bash
# AUR-464 E2E: run the real Bash and PowerShell extractors, through their
# real Extract() entrypoint, against a real script -- including this
# repository's own committed run-docs-pipeline.sh, the exact fixture the
# card's "Achado medido" measured the defect against -- and check the pages
# actually written to disk.
#
# WHY A SMALL DRIVER PROGRAM INSTEAD OF cmd/regenerate-docs
#   internal/documentation/extractors/{bash,powershell} are this card's own
#   packages and need no other extractor, no LLM provider, and no pipeline
#   wiring to run end-to-end: Extract() takes a source dir and an output dir
#   and writes real files. A tiny `go run` driver exercises the exact same
#   public entrypoint cmd/regenerate-docs would call, without pulling in
#   cmd/regenerate-docs's much larger dependency closure (which sits outside
#   this card's read_paths).
#
# WHAT IT PROVES
#   1. Feeding the extractor this repository's OWN run-docs-pipeline.sh (the
#      script the card's finding quotes) produces a page with no "##
#      Documentation" placeholder anywhere.
#   2. A synthetic script with a documented function, an undocumented
#      function, and a stray mid-script comment produces one heading per
#      function (named for the function), no repeated heading, and the
#      undocumented function's section carries no prose.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR464}" == E2EAUR464 ]] || { printf 'AUR-464/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-464/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-464/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

fixture_script="$repo_root/run-docs-pipeline.sh"
[[ -f "$fixture_script" ]] || fail 'behavior-missing: run-docs-pipeline.sh absent'

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e464.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home" "$run_dir/srcbash" "$run_dir/outbash" "$run_dir/srcps" "$run_dir/outps"

host_modcache="$(go env GOMODCACHE 2>/dev/null || true)"
[[ -n "$host_modcache" && -d "$host_modcache" ]] || infra 'gomodcache_absent'

cp -- "$fixture_script" "$run_dir/srcbash/run-docs-pipeline.sh"
cat >"$run_dir/srcbash/synthetic.sh" <<'SH'
#!/bin/bash
# Overview note, not attached to any function.

# Greets the caller.
greet() {
  echo "hi"
}

echo "loading"

undocumented_task() {
  echo "noop"
}
SH

cat >"$run_dir/srcps/synthetic.ps1" <<'PS1'
# Greets the caller.
function Get-Greeting {
    Write-Output "hi"
}

function Undocumented-Task {
    Write-Output "noop"
}
PS1

# Second review round's fixtures: code before a documented function must not
# sweep its doc into Notes (Blocker 1), and a same-name-different-case trio
# must end up on three distinct anchors, not just three distinct heading
# texts (Blocker 2).
cat >"$run_dir/srcbash/round3.sh" <<'SH2'
#!/bin/bash
set -euo pipefail

# Restarts the whole stack.
restart_all() {
  systemctl restart svc
}

foo() {
  echo "1"
}

Foo() {
  echo "2"
}

foo-2() {
  echo "3"
}
SH2


# The driver must physically live inside this module's own tree (not under
# /tmp) because it imports internal/... packages: Go's internal-import rule
# is a property of the FILE's own path relative to the module root, not of
# the working directory a `go run` is launched from. tests/e2e is this
# card's own owned path, so a driver file written there (and removed again
# before this script exits, success or failure) never leaves the tree
# dirty and never needs any path outside this card's paths/read_paths.
driver_file="$repo_root/tests/e2e/aur464_e2e_driver_main.go"
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
	powershellextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func main() {
	kind := os.Args[1]
	src := os.Args[2]
	out := os.Args[3]

	switch kind {
	case "bash":
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
	case "powershell":
		ext := powershellextractor.NewPowerShellExtractor(site.NewMockRunner())
		res, err := ext.Extract(context.Background(), &extractors.ExtractRequest{
			Language: extractors.LanguagePowerShell, SourceDir: src, OutputDir: out,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "extract error:", err)
			os.Exit(1)
		}
		if len(res.Errors) > 0 {
			fmt.Fprintln(os.Stderr, "extract errors:", res.Errors)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown kind", kind)
		os.Exit(2)
	}
}
GOEOF

go_env=(
  HOME="$run_dir/home"
  GOCACHE="$run_dir/cache"
  GOTMPDIR="$run_dir/gotmp"
  GOMODCACHE="$host_modcache"
  GOPROXY=off
  GOSUMDB=off
  GOTOOLCHAIN=local
  GOFLAGS=-mod=mod
  GOMEMLIMIT=2GiB
)

run_driver() {
  local kind="$1" src="$2" out="$3"
  ( cd "$repo_root" && ulimit -v 8388608 && env "${go_env[@]}" go run "$driver_file" "$kind" "$src" "$out" )
}

run_driver bash "$run_dir/srcbash" "$run_dir/outbash" || fail 'behavior-missing: bash driver failed'
run_driver powershell "$run_dir/srcps" "$run_dir/outps" || fail 'behavior-missing: powershell driver failed'

real_page="$run_dir/outbash/run-docs-pipeline.sh.md"
[[ -s "$real_page" ]] || fail 'behavior-missing: real fixture page not generated'
grep -Fq '## Documentation' "$real_page" && fail 'behavior-missing: real fixture page still carries the fixed placeholder heading'

synthetic_page="$run_dir/outbash/synthetic.sh.md"
[[ -s "$synthetic_page" ]] || fail 'behavior-missing: synthetic bash page not generated'
grep -Fq '## Documentation' "$synthetic_page" && fail 'behavior-missing: synthetic bash page still carries the fixed placeholder heading'
grep -Fq '### function greet' "$synthetic_page" || fail 'behavior-missing: greet heading missing'
grep -Fq '### function undocumented_task' "$synthetic_page" || fail 'behavior-missing: undocumented_task heading missing'
# AC-003: no prose synthesized for the undocumented function.
awk '/^### function undocumented_task/{f=1;next} /^```/{if(f){print "FENCE"; exit}} f && NF{print "PROSE:" $0; exit}' "$synthetic_page" | grep -q '^FENCE$' \
  || fail 'behavior-missing: undocumented_task carries synthesized prose'

ps_page="$run_dir/outps/synthetic.ps1.md"
[[ -s "$ps_page" ]] || fail 'behavior-missing: synthetic powershell page not generated'
grep -Fq '## Documentation' "$ps_page" && fail 'behavior-missing: synthetic powershell page still carries the fixed placeholder heading'
grep -Fq '### function Get-Greeting' "$ps_page" || fail 'behavior-missing: Get-Greeting heading missing'
grep -Fq '### function Undocumented-Task' "$ps_page" || fail 'behavior-missing: Undocumented-Task heading missing'

# No heading repeats within either synthetic page.
for p in "$synthetic_page" "$ps_page"; do
  dup="$(grep -E '^#' "$p" | sort | uniq -d)"
  [[ -z "$dup" ]] || fail "behavior-missing: repeated heading in $p: $dup"
done

# Second review round's Blocker 1, end to end: real code before the doc
# comment must not sweep it into Notes.
round3_page="$run_dir/outbash/round3.sh.md"
[[ -s "$round3_page" ]] || fail 'behavior-missing: round3 page not generated'
awk '/^### function restart_all/{f=1;next} /^```/{if(f){print "FENCE"; exit}} f{print}' "$round3_page" \
  | grep -Fq 'Restarts the whole stack.' \
  || fail 'behavior-missing: restart_all real doc lost even though real code preceded it'

# Second review round's Blocker 2, end to end: foo/Foo/foo-2 must slug to
# three distinct anchors under the same normalization the site applies
# (lowercase, spaces to hyphens, strip anything else).
round3_anchor_count="$(
  grep -E '^###' "$round3_page" \
    | sed -E 's/^###[[:space:]]*//' \
    | tr '[:upper:]' '[:lower:]' \
    | tr ' ' '-' \
    | sed -E 's/[^a-z0-9_-]+//g' \
    | sort -u | wc -l
)"
# round3.sh has 4 functions: restart_all, plus the foo/Foo/foo-2 trio. All
# four must land on distinct anchors after the same slug normalization the
# site applies.
[[ "$round3_anchor_count" -eq 4 ]] || fail "behavior-missing: expected 4 distinct anchors (restart_all + foo/Foo/foo-2 trio), got $round3_anchor_count"

printf 'AUR-464/AC-001/pass\n'
