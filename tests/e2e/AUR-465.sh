#!/usr/bin/env bash
# AUR-465 E2E: build and run a real, standalone Go binary -- not `go test`,
# not a direct function call from within this repo's own module -- that
# imports internal/documentation/welcome exactly the way a consumer's own
# compiled tooling would, and drives its three deterministic guardrails
# against fixture data representing the documented defect: a generated
# welcome page whose Quick Start still says "uses: Mpaape/AurumCode@main",
# an invented internal link nothing on the site actually produced, and a
# _config.yml that declares a logo the generated tree never received.
#
# WHY IT STAGES A SCRATCH MODULE (same technique as AUR-422/424/428/440/445)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. This script stages
#   go.mod, go.sum and internal/documentation/welcome/sanitize.go -- the
#   guardrail file alone, deliberately not generator.go, so this layer never
#   depends on internal/llm and proves the guardrail functions are usable on
#   their own -- into a private module under /tmp, then builds and RUNS a
#   throwaway main package against them.
#
# MUT-001 FALSIFIER (AC-001)
#   Neutering SanitizeActionRef in a SEPARATE staged copy (regex substitution
#   replaced with a no-op) must make the binary's own @main check fail. This
#   script's MUT-001 selector proves it: it stages that mutated copy, runs
#   the same binary against it, and requires the AUR-465/AC-001/MUT-001
#   label. The tracked sanitize.go is never touched.
#
# MUT-002 FALSIFIER (AC-002)
#   A _config.yml fixture that declares a logo the scratch site tree never
#   received must make the binary's AssetExists check fail. This script's
#   MUT-002 selector proves it, independent of any code mutation: only the
#   fixture data changes.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-465 scenario=AC-001
selector="${1:-E2EAUR465}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  E2EAUR465|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for p in go.mod go.sum internal/documentation/welcome/sanitize.go; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e465.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  mkdir -p "$dest/internal/documentation/welcome" "$dest/cmd/aur465e2e"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$dest/"
  cp "$repo_root/internal/documentation/welcome/sanitize.go" "$dest/internal/documentation/welcome/sanitize.go"
}

cat >"$run_dir/main.go.tmpl" <<'GOEOF'
// AUR-465 E2E driver: a real, standalone binary exercising the guardrails
// exactly as internal/documentation/welcome/generator.go's Generate() calls
// them, against fixture data shaped like the measured defect. Exit 0 only
// when every guardrail behaves; exit 2 (behavior-missing) or 3 (mutation)
// otherwise, printing the card's own label so the driving shell script can
// tell a real RED from a deliberate mutation.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
)

func missing(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-465/AC-001/behavior-missing: "+format+"\n", args...)
	os.Exit(2)
}

func mutated(label, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-465/AC-001/%s: "+format+"\n", append([]any{label}, args...)...)
	os.Exit(3)
}

func main() {
	mode := ""
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	// -- AC-001: the fixture Quick Start, straight out of the measured defect.
	// AUR465_MUTATION is set only by the driving shell script's MUT-001 case
	// (mirroring AUR-445's proven AUR445_MUTATION technique): it tells this
	// otherwise-identical check whether a failure here is this card's own
	// pre-fix RED or the deliberate MUT-001 proof that a neutered guard is
	// caught.
	quickStart := "```yaml\n- uses: Mpaape/AurumCode@main\n```"
	fixed, changed := welcome.SanitizeActionRef(quickStart)
	if !changed || strings.Contains(fixed, "@main") {
		if os.Getenv("AUR465_MUTATION") == "MUT-001" {
			mutated("MUT-001", "SanitizeActionRef left @main in the fixture Quick Start despite the neutered guard being the point of this run: %q", fixed)
		}
		missing("SanitizeActionRef left @main in the fixture Quick Start: %q", fixed)
	}
	if !strings.Contains(fixed, "Mpaape/AurumCode@v1") {
		missing("SanitizeActionRef did not produce the published v1 tag: %q", fixed)
	}

	// -- AC-003: the fixture "Documentation" section, one real link and one
	// invented one, matching the prompt template's old placeholder shape.
	docs := "- [Guide](docs/getting-started.md)\n- [Section 1](guides/advanced/)\n"
	fixedDocs, linksChanged := welcome.SanitizeInternalLinks(docs)
	if !linksChanged || strings.Contains(fixedDocs, "](guides/advanced/)") {
		missing("SanitizeInternalLinks left the invented relative link in place: %q", fixedDocs)
	}
	if !strings.Contains(fixedDocs, "[Guide](docs/getting-started.md)") {
		missing("SanitizeInternalLinks dropped the legitimate getting-started link: %q", fixedDocs)
	}

	// -- AC-002: a fixture _config.yml and a fixture published tree.
	siteRoot, err := os.MkdirTemp("", "aur465-site")
	if err != nil {
		missing("mktemp site root: %v", err)
	}
	defer os.RemoveAll(siteRoot)

	config := "title: Fixture\nlogo: \"/assets/images/logo.png\"\n"
	path, declared := welcome.DeclaredAssetPath(config)
	if !declared {
		missing("DeclaredAssetPath found no logo in a config that declares one")
	}

	if mode != "MUT-002" {
		if err := os.MkdirAll(filepath.Join(siteRoot, "assets", "images"), 0o755); err != nil {
			missing("mkdir fixture asset dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(siteRoot, "assets", "images", "logo.png"), []byte("png"), 0o644); err != nil {
			missing("write fixture asset: %v", err)
		}
	}

	exists := welcome.AssetExists(path, siteRoot)
	if mode == "MUT-002" {
		// This mode deliberately never writes the asset file above: it is
		// the MUT-002 fixture, a config that declares a logo the generated
		// tree never received. AC-002 requires that to fail.
		if !exists {
			mutated("MUT-002", "declared logo %q is missing from the published tree at %s; AC-002 requires every declared asset path to exist", path, siteRoot)
		}
		fmt.Println("AUR-465/E2E MUT-002 unexpectedly found the fixture asset present; the fixture itself is broken")
		return
	}
	if !exists {
		missing("AssetExists reports %q missing under %s even though the fixture wrote it", path, siteRoot)
	}

	fmt.Printf("AUR-465/E2E ok quickstart=%q docs=%q asset=%s\n", strings.TrimSpace(fixed), strings.TrimSpace(fixedDocs), path)
}
GOEOF

build_and_run() {
  local dest="$1" mode="${2:-}" mutation_marker="${3:-}" out rc=0
  cp "$run_dir/main.go.tmpl" "$dest/cmd/aur465e2e/main.go"
  set +e
  out="$( ulimit -v 8388608
    cd "$dest" && HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    AUR465_MUTATION="$mutation_marker" \
    timeout 120s go run ./cmd/aur465e2e $mode 2>&1 )"
  rc=$?
  set -e
  aur465_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

root="$run_dir/root"
mkdir -p "$root"
stage "$root"

case "$selector" in
  E2EAUR465)
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
    # Neuter the guardrail in the SCRATCH copy only: the substitution becomes
    # an identity function, exactly as if a future edit reintroduced @main
    # capability by breaking the guard the template comment (AUR-465,
    # docs/specs/AUR-465.md) describes.
    sed -i.bak 's/^func SanitizeActionRef(content string) (string, bool) {$/func SanitizeActionRef(content string) (string, bool) { return content, false; _ = actionRefPattern; _ = pinnedTagPattern; _ = fullSHAPattern/' \
      "$mut_root/internal/documentation/welcome/sanitize.go"
    rm -f "$mut_root/internal/documentation/welcome/sanitize.go.bak"

    rc=0
    build_and_run "$mut_root" "" MUT-001 || rc=$?
    (( rc != 0 )) || fail 'MUT-001:e2e binary passed against a neutered guard, expected failure'
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur465_last_output" \
      || fail 'MUT-001:e2e binary failed but never reported the MUT-001 label'
    # A neutered guard has no reason to fail the AC-003 link check first;
    # confirm the failure is specifically the @main assertion, not a build
    # error dressed up as one.
    grep -Fq '@main' <<<"$aur465_last_output" \
      || fail 'MUT-001:failure did not mention @main, so it is not the guarded assertion'
    printf '%s/%s/MUT-001 confirmed: e2e_rc=%d, tracked sanitize.go untouched\n' "$card" "$scenario" "$rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
  MUT-002)
    rc=0
    build_and_run "$root" MUT-002 || rc=$?
    (( rc != 0 )) || fail 'MUT-002:e2e binary passed with a declared-but-missing logo, expected failure'
    grep -Fq "$card/$scenario/MUT-002" <<<"$aur465_last_output" \
      || fail 'MUT-002:binary failed but never reported the MUT-002 label'
    printf '%s/%s/MUT-002 confirmed: e2e_rc=%d\n' "$card" "$scenario" "$rc"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-002","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
