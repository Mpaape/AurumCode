#!/usr/bin/env bash
# AUR-472 E2E: build and run a real, standalone Go binary -- not `go test`,
# not a direct function call from within this repo's own module -- that
# imports internal/documentation/welcome/sanitize.go exactly the way a
# consumer's own compiled tooling would, and drives SanitizeActionRefTags
# against the measured defect: a ref shaped like a semver tag (v2) that was
# never actually published.
#
# WHY IT STAGES A SCRATCH MODULE (same technique as AUR-422/424/428/440/445/465)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. This script stages
#   go.mod, go.sum and internal/documentation/welcome/sanitize.go alone --
#   deliberately not generator.go, so this layer never depends on
#   internal/llm and proves the guardrail is usable standalone -- into a
#   private module under /tmp, then builds and RUNS a throwaway main
#   package against it.
#
# WHY THE TAG LIST NEEDS NO NEW INPUT
#   sanitize.go declares `publishedTags` as a Go source-level literal --
#   "configuracao declarada" per the card's Non-goals -- so this script
#   never touches `git`, `.git`, or the network to learn what tags exist.
#   Only sanitize.go itself is staged; the list travels with it.
#
# MUT-001 FALSIFIER (AC-001: form accepted as value)
#   Reintroducing the old pinnedTagPattern-based accept check in a SEPARATE
#   staged copy must make the binary's own "v2 rejected" assertion fail.
#   This script's MUT-001 selector proves it.
#
# MUT-002 FALSIFIER (AC-003: no-list lets ref pass)
#   Neutering the noList branch so it returns the ref untouched instead of
#   rewriting must make the binary's own no-list assertion fail. This
#   script's MUT-002 selector proves it.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-472 scenario=AC-001
selector="${1:-E2EAUR472}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  E2EAUR472|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for p in go.mod go.sum internal/documentation/welcome/sanitize.go; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e472.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  mkdir -p "$dest/internal/documentation/welcome" "$dest/cmd/aur472e2e"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$dest/"
  cp "$repo_root/internal/documentation/welcome/sanitize.go" "$dest/internal/documentation/welcome/sanitize.go"
}

cat >"$run_dir/main.go.tmpl" <<'GOEOF'
// AUR-472 E2E driver: a real, standalone binary exercising
// SanitizeActionRefTags exactly as internal/documentation/welcome/
// generator.go's Generate() calls it, against fixture refs shaped like the
// measured defect. Exit 0 only when every check behaves; exit 2
// (behavior-missing) or 3 (mutation) otherwise, printing the card's own
// label so the driving shell script can tell a real RED from a deliberate
// mutation.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
)

func missing(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-472/AC-001/behavior-missing: "+format+"\n", args...)
	os.Exit(2)
}

func mutated(label, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-472/AC-001/%s: "+format+"\n", append([]any{label}, args...)...)
	os.Exit(3)
}

func main() {
	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	declared := welcome.PublishedTags()

	// -- AC-001: v2 has the correct semver SHAPE but was never published.
	unpublished := "uses: Mpaape/AurumCode@v2"
	res := welcome.SanitizeActionRefTags(unpublished, declared)
	if !res.Changed || strings.Contains(res.Content, "@v2") {
		if os.Getenv("AUR472_MUTATION") == "MUT-001" {
			mutated("MUT-001", "v2 (semver-shaped, never published) survived unrewritten despite the reverted shape-only guard being the point of this run: %q", res.Content)
		}
		missing("v2 (semver-shaped, never published) was not rewritten: %q", res.Content)
	}
	if !strings.Contains(res.Content, "Mpaape/AurumCode@v1") {
		missing("v2 was not rewritten to the published major tag: %q", res.Content)
	}
	if len(res.Notices) == 0 || !strings.Contains(res.Notices[0], "v2") {
		missing("rewriting v2 produced no notice naming it: %v", res.Notices)
	}

	// -- AC-002: v1 is a real published tag and must survive untouched.
	real := "uses: Mpaape/AurumCode@v1"
	resReal := welcome.SanitizeActionRefTags(real, declared)
	if resReal.Changed {
		missing("v1, a real published tag, was rewritten: %q", resReal.Content)
	}

	// -- AC-002 (Non-goal): a full commit SHA is the strictest pin and is
	// always accepted, list or no list.
	sha := "uses: Mpaape/AurumCode@11bd71901bbe5b1630ceea73d27597364c9af683"
	if welcome.SanitizeActionRefTags(sha, declared).Changed {
		missing("a full commit SHA was rewritten with a tag list available: %q", sha)
	}
	if welcome.SanitizeActionRefTags(sha, nil).Changed {
		missing("a full commit SHA was rewritten with no tag list available: %q", sha)
	}

	// -- AC-003: with no tag list available, even v1 (which would be real)
	// is rewritten, and the notice says validation happened without a list.
	resNoList := welcome.SanitizeActionRefTags(real, nil)
	if !resNoList.Changed {
		if os.Getenv("AUR472_MUTATION") == "MUT-002" {
			mutated("MUT-002", "with no tag list available, v1 passed through unrewritten despite the neutered no-list guard being the point of this run: %q", resNoList.Content)
		}
		missing("with no tag list available, v1 was not rewritten (unsafe default): %q", resNoList.Content)
	}
	if len(resNoList.Notices) == 0 || !strings.Contains(resNoList.Notices[0], "no published-tag list") {
		if os.Getenv("AUR472_MUTATION") == "MUT-002" {
			mutated("MUT-002", "no-list rewrite happened but the no-list notice is missing: %v", resNoList.Notices)
		}
		missing("no-list rewrite notice never says validation happened without a list: %v", resNoList.Notices)
	}

	if mode == "MUT-002" {
		fmt.Println("AUR-472/E2E MUT-002 mode ran to completion without detecting the neutered no-list guard; the fixture itself is broken")
		return
	}

	fmt.Printf("AUR-472/E2E ok v2=%q v1=%q nolist=%q\n", strings.TrimSpace(res.Content), strings.TrimSpace(resReal.Content), strings.TrimSpace(resNoList.Content))
}
GOEOF

build_and_run() {
  local dest="$1" mode="${2:-}" mutation_marker="${3:-}" out rc=0
  cp "$run_dir/main.go.tmpl" "$dest/cmd/aur472e2e/main.go"
  set +e
  out="$( ulimit -v 8388608
    cd "$dest" && HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    AUR472_MUTATION="$mutation_marker" \
    timeout 120s go run ./cmd/aur472e2e $mode 2>&1 )"
  rc=$?
  set -e
  aur472_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

root="$run_dir/root"
mkdir -p "$root"
stage "$root"

case "$selector" in
  E2EAUR472)
    build_and_run "$root" || {
      rc=$?
      (( rc == 3 )) && fail "unexpected-mutation-exit:$rc"
      fail "selector-exit:$rc"
    }
    printf '{"card":"%s","scenario":"%s","layer":"e2e","result":"pass"}\n' "$card" "$scenario"
    ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    mkdir -p "$mut_root"
    stage "$mut_root"
    # Reintroduce the old shape-only accept check in the SCRATCH copy only:
    # any ref matching pinnedTagPattern is treated as compliant again,
    # exactly the 2026-08-14 defect, as if AUR-472's fix were reverted.
    sed -i.bak \
      's/if !noList \&\& known\[ref\] {/if !noList \&\& (known[ref] || pinnedTagPattern.MatchString(ref)) {/' \
      "$mut_root/internal/documentation/welcome/sanitize.go"
    rm -f "$mut_root/internal/documentation/welcome/sanitize.go.bak"

    rc=0
    build_and_run "$mut_root" "" MUT-001 || rc=$?
    (( rc != 0 )) || fail 'MUT-001:e2e binary passed against the reverted shape-only guard, expected failure'
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur472_last_output" \
      || fail 'MUT-001:e2e binary failed but never reported the MUT-001 label'
    grep -Fq 'v2' <<<"$aur472_last_output" \
      || fail 'MUT-001:failure did not mention v2, so it is not the guarded assertion'
    printf '%s/%s/MUT-001 confirmed: e2e_rc=%d, tracked sanitize.go untouched\n' "$card" "$scenario" "$rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
  MUT-002)
    mut_root="$run_dir/mut-root2"
    mkdir -p "$mut_root"
    stage "$mut_root"
    # Neuter the no-list branch in the SCRATCH copy only: with no list
    # available, let the ref pass through untouched instead of rewriting it
    # -- the exact regression the card's Non-goals forbid.
    sed -i.bak \
      's/if !noList \&\& known\[ref\] {/if known[ref] || noList {/' \
      "$mut_root/internal/documentation/welcome/sanitize.go"
    rm -f "$mut_root/internal/documentation/welcome/sanitize.go.bak"

    rc=0
    build_and_run "$mut_root" "" MUT-002 || rc=$?
    (( rc != 0 )) || fail 'MUT-002:e2e binary passed with the neutered no-list guard, expected failure'
    grep -Fq "$card/$scenario/MUT-002" <<<"$aur472_last_output" \
      || fail 'MUT-002:e2e binary failed but never reported the MUT-002 label'
    printf '%s/%s/MUT-002 confirmed: e2e_rc=%d, tracked sanitize.go untouched\n' "$card" "$scenario" "$rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-002","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
