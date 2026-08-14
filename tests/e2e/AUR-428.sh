#!/usr/bin/env bash
# AUR-428 E2E: execute the user journey the card promises, end to end, inside
# the offline sandbox.
#
# WHAT IT DOES
#   1. Copies .github/workflows/examples/documentation.yml VERBATIM into a
#      scratch "user repository" as .github/workflows/documentation.yml --
#      exactly the copy the card's Outcome describes ("sem editar nada") --
#      and proves the copy is byte-identical to the example.
#   2. Compiles a small validator (real yaml.v3 parse, structural walk, no
#      grep) and runs it over THE COPY: the reference must be
#      <owner/repo from go.mod>@<semver tag>, the deploy job must declare
#      pages: write and id-token: write, the top level must stay
#      contents: read, publish must be 'pages', and workflow_dispatch must
#      exist so `gh workflow run documentation.yml` has something to invoke.
#   3. Runs the validator a second time over the same copy and requires
#      byte-identical output (AC-001's repeat clause).
#   4. Requires LICENSE at the repository root: the semver tag is only
#      publishable over licensed content.
#
# WHAT IT HONESTLY CANNOT DO, PER THE CARD'S "Restricao medida"
#   The sandbox denies network, so no real `gh workflow run`, no real tag
#   push, no live Pages check (that is AUR-429's simulated driver). This is
#   STATIC verification of the copied manifest; publishing the real `v1` tag
#   is a documented human action (docs/specs/AUR-428.md).
#
# MUT-001 FALSIFIER
#   Point the example workflow at a nonexistent reference (wrong repository,
#   branch ref, garbage ref) and this script fails non-zero printing
#   AUR-428/AC-001/MUT-001, because the validator refuses the mutated
#   manifest. Restoring the workflow reproduces the exact GREEN.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR428}" == E2EAUR428 ]] || { printf 'AUR-428/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-428/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-428/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
command -v cmp >/dev/null 2>&1 || infra missing_cmp

example="$repo_root/.github/workflows/examples/documentation.yml"
[[ -f "$example" ]] || fail 'behavior-missing: example workflow absent'
[[ -f "$repo_root/go.mod" && -f "$repo_root/go.sum" ]] || infra go_module_inputs_absent
[[ -s "$repo_root/LICENSE" ]] || fail 'behavior-missing: LICENSE absent at the repository root'

# The only owner/repo the copied workflow may reference: where go.mod says
# this module (and therefore the root action.yml) lives.
expected_repo="$(sed -n 's|^module github\.com/\(.*\)$|\1|p' "$repo_root/go.mod" | head -n1)"
[[ -n "$expected_repo" ]] || infra module_path_unreadable

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e428.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp" "$run_dir/home"

# GOCACHE is shareable with the acceptance driver so the sandbox's bounded
# 120s window is not spent recompiling yaml.v3 three times.
gocache="${AURUM_A428_GOCACHE:-$run_dir/cache}"
mkdir -p "$gocache"

# --- 1. the user's copy, byte for byte --------------------------------------
user_repo="$run_dir/user-repo"
mkdir -p "$user_repo/.github/workflows"
copy="$user_repo/.github/workflows/documentation.yml"
cp "$example" "$copy"
cmp -s "$example" "$copy" || fail 'behavior-missing: the copied workflow differs from the example'

# --- 2. a real parse of the copy --------------------------------------------
validator="$run_dir/validator"
mkdir -p "$validator"
cp "$repo_root/go.mod" "$repo_root/go.sum" "$validator/"
cat >"$validator/main.go" <<'GOEOF'
// AUR-428 E2E validator: parses the COPIED workflow with yaml.v3 and walks
// the node tree. Exit 0 only when the manifest delivers the card's outcome;
// exit 2 (behavior-missing) or 3 (MUT-001) otherwise, printing the label the
// card declares. Output is deterministic so the repeat clause can compare it.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func get(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func missing(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-428/AC-001/behavior-missing: "+format+"\n", args...)
	os.Exit(2)
}

func mutated(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-428/AC-001/MUT-001: "+format+"\n", args...)
	os.Exit(3)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: validate <workflow.yml> <expected owner/repo>")
		os.Exit(64)
	}
	path, expectedRepo := os.Args[1], os.Args[2]
	raw, err := os.ReadFile(path)
	if err != nil {
		missing("copied workflow unreadable: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		missing("copied workflow is not parseable YAML: %v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		missing("copied workflow does not parse to a YAML mapping")
	}
	wf := doc.Content[0]

	if on := get(wf, "on"); on == nil || get(on, "workflow_dispatch") == nil {
		missing("no workflow_dispatch trigger: `gh workflow run documentation.yml` has nothing to invoke")
	}
	if top := get(wf, "permissions"); top == nil || get(top, "contents") == nil || get(top, "contents").Value != "read" {
		missing("top-level permissions must pin contents: read")
	}

	jobs := get(wf, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		missing("no jobs declared")
	}
	var step, job *yaml.Node
	var uses string
	found := 0
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		steps := get(jobs.Content[i+1], "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		for _, s := range steps.Content {
			u := get(s, "uses")
			if u == nil || strings.HasPrefix(u.Value, "actions/") {
				continue
			}
			found++
			step, job, uses = s, jobs.Content[i+1], u.Value
		}
	}
	if found != 1 {
		missing("expected exactly one AurumCode step, found %d", found)
	}

	perms := get(job, "permissions")
	if perms == nil {
		missing("deploy job declares no permissions block")
	}
	for key, want := range map[string]string{"contents": "read", "pages": "write", "id-token": "write"} {
		if node := get(perms, key); node == nil || node.Value != want {
			missing("deploy job permissions must declare %s: %s", key, want)
		}
	}

	at := strings.LastIndex(uses, "@")
	if at <= 0 || at == len(uses)-1 {
		mutated("uses %q carries no @ref", uses)
	}
	ownerRepo, ref := uses[:at], uses[at+1:]
	if ownerRepo != expectedRepo {
		mutated("uses %q references %q, not this repository %q", uses, ownerRepo, expectedRepo)
	}
	if regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(ref) {
		missing("uses %q pins a local commit SHA instead of the publishable semver tag", uses)
	}
	if !regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`).MatchString(ref) {
		mutated("uses %q ref %q is not a publishable semver tag", uses, ref)
	}

	if with := get(step, "with"); with == nil || get(with, "publish") == nil || get(with, "publish").Value != "pages" {
		missing("AurumCode step must set publish: 'pages'")
	}

	fmt.Printf("AUR-428/E2E ok uses=%s sha256=%x\n", uses, sha256.Sum256(raw))
}
GOEOF

go_env=(
  HOME="$run_dir/home"
  GOPROXY=off GOSUMDB=off GOFLAGS=-mod=mod GOTOOLCHAIN=local
  GOMAXPROCS=1 GOMEMLIMIT=2GiB
  GOCACHE="$gocache" GOTMPDIR="$run_dir/gotmp"
)
# A validator that does not compile is missing infrastructure, never a RED:
# the card counts only behavioral failures as failures.
build_rc=0
( ulimit -v 8388608
  cd "$validator" && env "${go_env[@]}" \
    timeout 300s go build -o "$run_dir/validate" . ) || build_rc=$?
(( build_rc == 0 )) || infra "validator_build:$build_rc"

run_validator() {
  local out rc=0
  set +e
  out="$("$run_dir/validate" "$copy" "$expected_repo" 2>"$run_dir/validate.stderr")"
  rc=$?
  set -e
  cat "$run_dir/validate.stderr" >&2
  if (( rc != 0 )); then
    # The labeled line is already on stderr; keep the raw exit distinct so a
    # MUT-001 refusal and a behavior gap never read as infrastructure.
    fail "validator-refused-copy:$rc"
  fi
  printf '%s\n' "$out"
}

# --- 3. run twice; the repeat must be byte-identical -------------------------
first="$(run_validator)"
second="$(run_validator)"
[[ "$first" == "$second" ]] || fail 'repeat-divergence: two runs over the same copy produced different output'

printf '%s\n' "$first"
printf '{"card":"AUR-428","scenario":"AC-001","layer":"e2e","result":"pass","copy":"byte-identical","repeat":"deterministic"}\n'
