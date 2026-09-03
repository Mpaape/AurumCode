#!/usr/bin/env bash
# AUR-484 E2E: build and run a real, standalone Go binary -- not `go test`,
# not a direct in-module call -- that imports internal/documentation/site
# exactly the way cmd/regenerate-docs does, runs Scaffold.Generate() against
# fixture pages, and asserts the published contract this card exists for:
# a copy-pasteable, no-credential command and declared limitations above the
# generated-page listing (AC-001/AC-003), and that listing itself moved off
# the landing page onto its own page (AC-002).
#
# WHY IT STAGES A SCRATCH MODULE (same technique as AUR-422/424/428/440/465)
#   The sandbox rootfs is read-only; only this card's paths/read_paths are
#   materialized and /tmp is the only writable space. This script stages
#   go.mod, go.sum and internal/documentation/site/*.go (stdlib-only, no
#   internal/llm dependency) into a private module under /tmp, then builds
#   and RUNS a throwaway main package against them.
#
# MUT-001 FALSIFIER (AC-001)
#   Removing the copyable command from renderQuickstartBlock in a SEPARATE
#   staged copy must make the binary's own no-credential-command check fail.
#   This script's MUT-001 selector proves it. The tracked scaffold.go is
#   never touched.
#
# MUT-002 FALSIFIER (AC-002)
#   Putting the full per-page enumeration back into renderPageSummaryBlock
#   (the index.md path) in a SEPARATE staged copy must make the binary's
#   own "listing is not the index body" check fail. This script's MUT-002
#   selector proves it, independent of any other change.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-484 scenario=AC-001
selector="${1:-E2EAUR484}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  E2EAUR484|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for p in go.mod go.sum internal/documentation/site/scaffold.go; do
  [[ -f "$repo_root/$p" ]] || fail "entrypoint_missing:$p"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e484.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/cache" "$run_dir/gotmp" "$run_dir/home"

stage() {
  local dest="$1"
  mkdir -p "$dest/internal/documentation/site" "$dest/cmd/aur484e2e"
  cp "$repo_root/go.mod" "$repo_root/go.sum" "$dest/"
  cp "$repo_root/internal/documentation/site/scaffold.go" "$repo_root/internal/documentation/site/types.go" \
    "$dest/internal/documentation/site/"
}

cat >"$run_dir/main.go.tmpl" <<'GOEOF'
// AUR-484 E2E driver: a real, standalone binary exercising
// internal/documentation/site.Scaffold.Generate() exactly the way
// cmd/regenerate-docs calls it, against fixture pages. Exit 0 only when
// every guardrail behaves; exit 2 (behavior-missing) or 3 (mutation)
// otherwise, printing the card's own label so the driving shell script can
// tell a real RED from a deliberate mutation.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func missing(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-484/AC-001/behavior-missing: "+format+"\n", args...)
	os.Exit(2)
}

func mutated(label, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-484/AC-001/%s: "+format+"\n", append([]any{label}, args...)...)
	os.Exit(3)
}

func main() {
	dir, err := os.MkdirTemp("", "aur484-e2e")
	if err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: mkdtemp: %v\n", err)
		os.Exit(69)
	}
	defer os.RemoveAll(dir)

	if err := os.MkdirAll(filepath.Join(dir, "go"), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: mkdir: %v\n", err)
		os.Exit(69)
	}
	// Two pages, deliberately: reference.md exists to index MULTIPLE pages, so
	// a single-page fixture would never reach the behaviour this asserts. With
	// one page the scaffold writes no reference.md at all and index.md links
	// straight to the page -- that half is pinned by the Unit selector.
	vault := "---\ntitle: Vault\npermalink: /go/vault/\n---\n\n# vault\n\n## func Seal\n"
	if err := os.WriteFile(filepath.Join(dir, "go", "vault.md"), []byte(vault), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: write vault page: %v\n", err)
		os.Exit(2)
	}
	page := "---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n\n## func AddMoney\n"
	if err := os.WriteFile(filepath.Join(dir, "go", "ledger.md"), []byte(page), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: write fixture: %v\n", err)
		os.Exit(69)
	}

	result, err := site.NewScaffold(site.ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "tinyrepo"}).Generate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: Generate: %v\n", err)
		os.Exit(69)
	}

	indexBytes, err := os.ReadFile(result.IndexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: read index: %v\n", err)
		os.Exit(69)
	}
	index := string(indexBytes)

	mode := os.Getenv("AUR484_MODE")

	const noCredCmd = "./aurumcode review --base HEAD~1 --seguranca"
	if !strings.Contains(index, noCredCmd) {
		if mode == "mut-001" {
			mutated("MUT-001", "no-credential command %q is absent from index.md", noCredCmd)
		}
		missing("index.md has no copy-pasteable no-credential command %q", noCredCmd)
	}
	if strings.Index(index, noCredCmd) > strings.Index(index, "Generated API documentation") {
		missing("the no-credential command is not above the generated listing")
	}

	if !strings.Contains(index, "4 das 8") {
		if mode == "mut-001" {
			mutated("MUT-001", "the declared-limitation text is absent from index.md")
		}
		missing("index.md does not declare the 4-of-8 security rule limitation")
	}

	hasEnumeration := strings.Contains(index, "### Go") || strings.Contains(index, "func AddMoney")
	if hasEnumeration {
		mutated("MUT-002", "index.md still embeds the per-page enumeration it should have moved to reference.md")
	}
	if !strings.Contains(index, "reference.html") {
		missing("index.md does not reference reference.md")
	}

	refBytes, err := os.ReadFile(result.ReferencePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AUR-484/AC-001/infrastructure: read reference: %v\n", err)
		os.Exit(69)
	}
	reference := string(refBytes)
	if !strings.Contains(reference, "func AddMoney") || !strings.Contains(reference, "### Go") {
		missing("reference.md does not carry the generated-page enumeration")
	}

	fmt.Printf("AUR-484/E2E ok pages=%d cmd_present=%v enumeration_on_reference=%v\n",
		len(result.Pages), strings.Contains(index, noCredCmd), strings.Contains(reference, "func AddMoney"))
}
GOEOF

build_and_run() {
  local dest="$1" out rc
  cp "$run_dir/main.go.tmpl" "$dest/cmd/aur484e2e/main.go"

  set +e
  out="$( ulimit -v 8388608
    cd "$dest" && HOME="$run_dir/home" GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod \
    GOTOOLCHAIN=local GOMAXPROCS=1 GOMEMLIMIT=2GiB \
    GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" \
    go run ./cmd/aur484e2e 2>&1)"
  rc=$?
  set -e
  aur484_last_output="$out"
  printf '%s\n' "$out"
  return "$rc"
}

case "$selector" in
  E2EAUR484)
    stage "$run_dir/root"
    rc=0; build_and_run "$run_dir/root" || rc=$?
    if (( rc != 0 )); then
      (( rc == 69 )) && exit 69
      fail "e2e-exit:$rc"
    fi
    grep -Fq 'AUR-484/E2E ok' <<<"$aur484_last_output" || fail 'selector-did-not-run'
    printf '{"card":"%s","scenario":"%s","layer":"e2e","result":"pass"}\n' "$card" "$scenario"
    ;;
  MUT-001)
    mut_root="$run_dir/mut-root"
    stage "$mut_root"
    # Neuter the guardrail in the SCRATCH copy only: renderQuickstartBlock
    # loses its command, as if a future edit reintroduced the empty
    # top-of-page.
    sed -i.bak "s#./aurumcode review --base HEAD~1 --seguranca#REMOVED#" \
      "$mut_root/internal/documentation/site/scaffold.go"
    rm -f "$mut_root/internal/documentation/site/scaffold.go.bak"

    rc=0; AUR484_MODE=mut-001 build_and_run "$mut_root" || rc=$?
    (( rc != 0 )) || fail 'MUT-001:e2e binary passed on a neutered quickstart block, expected failure'
    grep -Fq "$card/$scenario/MUT-001" <<<"$aur484_last_output" \
      || fail 'MUT-001:e2e binary failed but not on the expected label'

    printf '%s/%s/MUT-001 confirmed: tracked scaffold.go untouched\n' "$card" "$scenario"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-001","result":"detected"}\n' "$card" "$scenario"
    ;;
  MUT-002)
    mut_root="$run_dir/mut-root2"
    stage "$mut_root"
    # Restore the pre-AUR-484 behavior in the SCRATCH copy only: writeIndex
    # embeds the full listing again instead of the short summary.
    sed -i.bak "s#renderPageSummaryBlock(pages)#renderPageBlock(pages)#" \
      "$mut_root/internal/documentation/site/scaffold.go"
    rm -f "$mut_root/internal/documentation/site/scaffold.go.bak"

    rc=0; build_and_run "$mut_root" || rc=$?
    (( rc != 0 )) || fail 'MUT-002:e2e binary passed with the full listing back in index.md, expected failure'
    grep -Fq "$card/$scenario/MUT-002" <<<"$aur484_last_output" \
      || fail 'MUT-002:e2e binary failed but not on the expected label'

    printf '%s/%s/MUT-002 confirmed: tracked scaffold.go untouched\n' "$card" "$scenario"
    printf '{"card":"%s","scenario":"%s","mutation":"MUT-002","result":"detected"}\n' "$card" "$scenario"
    ;;
esac
