#!/usr/bin/env bash
# AUR-440 E2E: execute the user journey the card promises, end to end, inside
# the offline sandbox.
#
# WHAT IT DOES
#   1. Copies .github/workflows/examples/code-review.yml VERBATIM into a
#      scratch "user repository" as .github/workflows/code-review.yml --
#      exactly the copy the card's Outcome describes -- and proves the copy
#      is byte-identical to the example.
#   2. Compiles a small validator (real yaml.v3 parse, structural walk, no
#      grep) and runs it over THE COPY: pull_request must be the trigger, the
#      top level must stay contents: read with no pull-requests grant, the
#      single job must declare contents: read plus pull-requests: write, the
#      release checkout must reference <owner/repo from go.mod> at a semver
#      tag (never a local SHA, never a branch), no step may delegate to the
#      root composite action, no run body may interpolate ${{ }}, and the
#      review + `gh pr comment` steps must exist with their env contracts.
#   3. Runs the validator a second time over the same copy and requires
#      byte-identical output (AC-001's repeat clause).
#
# WHAT IT HONESTLY CANNOT DO, PER THE CARD'S PRECONDITIONS
#   The sandbox denies network, so no real pull request is opened and no live
#   comment is posted. This is STATIC verification of the copied manifest;
#   GitHub Actions itself triggers natively on pull_request with the Action's
#   own token (no webhook server, no HMAC), and publishing the real `v1` tag
#   is a documented human action (docs/specs/AUR-440.md).
#
# MUT-001 FALSIFIER
#   Remove the publishing job's `pull-requests: write` permission and this
#   script fails non-zero printing AUR-440/AC-001/MUT-001, because the
#   validator refuses the mutated manifest. Restoring the workflow reproduces
#   the exact GREEN.
set -euo pipefail
export LC_ALL=C
[[ "${1:-E2EAUR440}" == E2EAUR440 ]] || { printf 'AUR-440/AC-001/unknown-selector\n' >&2; exit 64; }

fail() { printf 'AUR-440/AC-001/%s\n' "$1" >&2; exit 1; }
infra() { printf 'AUR-440/AC-001/infrastructure/%s\n' "$1" >&2; exit 69; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go
command -v cmp >/dev/null 2>&1 || infra missing_cmp

example="$repo_root/.github/workflows/examples/code-review.yml"
[[ -f "$example" ]] || fail 'behavior-missing: example workflow absent'
[[ -f "$repo_root/go.mod" && -f "$repo_root/go.sum" ]] || infra go_module_inputs_absent

# The only owner/repo the copied workflow may build the review binary from:
# where go.mod says this module lives.
expected_repo="$(sed -n 's|^module github\.com/\(.*\)$|\1|p' "$repo_root/go.mod" | head -n1)"
[[ -n "$expected_repo" ]] || infra module_path_unreadable

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-e440.XXXXXX")" || infra mktemp
trap 'rm -rf -- "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp" "$run_dir/home"

# GOCACHE is shareable with the acceptance driver so the sandbox's bounded
# 120s window is not spent recompiling yaml.v3 three times.
gocache="${AURUM_A440_GOCACHE:-$run_dir/cache}"
mkdir -p "$gocache"

# --- 1. the user's copy, byte for byte --------------------------------------
user_repo="$run_dir/user-repo"
mkdir -p "$user_repo/.github/workflows"
copy="$user_repo/.github/workflows/code-review.yml"
cp "$example" "$copy"
cmp -s "$example" "$copy" || fail 'behavior-missing: the copied workflow differs from the example'

# --- 2. a real parse of the copy --------------------------------------------
validator="$run_dir/validator"
mkdir -p "$validator"
cp "$repo_root/go.mod" "$repo_root/go.sum" "$validator/"
cat >"$validator/main.go" <<'GOEOF'
// AUR-440 E2E validator: parses the COPIED workflow with yaml.v3 and walks
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
	fmt.Fprintf(os.Stderr, "AUR-440/AC-001/behavior-missing: "+format+"\n", args...)
	os.Exit(2)
}

func mutated(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "AUR-440/AC-001/MUT-001: "+format+"\n", args...)
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

	on := get(wf, "on")
	if on == nil || get(on, "pull_request") == nil {
		missing("no pull_request trigger: opening a pull request starts nothing")
	}
	top := get(wf, "permissions")
	if top == nil || get(top, "contents") == nil || get(top, "contents").Value != "read" {
		missing("top-level permissions must pin contents: read")
	}
	if get(top, "pull-requests") != nil {
		missing("pull-requests permission belongs only on the publishing job, never at the workflow level")
	}

	jobs := get(wf, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode || len(jobs.Content) != 2 {
		missing("expected exactly one review job")
	}
	job := jobs.Content[1]
	steps := get(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		missing("review job declares no steps")
	}

	shaRe := regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	semverRe := regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)

	var repository, ref string
	found := 0
	var reviewSeen, publishSeen bool
	for _, s := range steps.Content {
		if u := get(s, "uses"); u != nil {
			ownerRepo := u.Value
			if at := strings.LastIndex(u.Value, "@"); at > 0 {
				ownerRepo = u.Value[:at]
			}
			if ownerRepo == expectedRepo || strings.HasPrefix(ownerRepo, expectedRepo+"/") {
				missing("step uses %q delegates to the root composite action, which cannot review a pull request", u.Value)
			}
			if !strings.HasPrefix(u.Value, "actions/") {
				missing("step uses %q outside the pinned actions/ namespace", u.Value)
			}
			if strings.HasPrefix(u.Value, "actions/checkout@") {
				with := get(s, "with")
				if repoNode := get(with, "repository"); repoNode != nil {
					found++
					repository = repoNode.Value
					if refNode := get(with, "ref"); refNode != nil {
						ref = refNode.Value
					}
				}
			}
		}
		if r := get(s, "run"); r != nil {
			if strings.Contains(r.Value, "${{") {
				missing("a run script interpolates ${{ }} into its body; runtime data must travel through env")
			}
			if strings.Contains(r.Value, "review") && strings.Contains(r.Value, "--base") {
				env := get(s, "env")
				if get(env, "LLM_API_KEY") == nil || get(env, "LLM_BASE_URL") == nil || get(env, "BASE_SHA") == nil {
					missing("review step env misses LLM_API_KEY, LLM_BASE_URL or BASE_SHA")
				}
				reviewSeen = true
			}
			if strings.Contains(r.Value, "gh pr comment") {
				if env := get(s, "env"); get(env, "GH_TOKEN") == nil {
					missing("publish step env declares no GH_TOKEN")
				}
				publishSeen = true
			}
		}
	}
	if found != 1 {
		missing("expected exactly one AurumCode release checkout, found %d", found)
	}
	if repository != expectedRepo {
		missing("release checkout references %q, not this repository %q", repository, expectedRepo)
	}
	if shaRe.MatchString(ref) {
		missing("release checkout ref %q pins a local commit SHA instead of the publishable semver tag", ref)
	}
	if !semverRe.MatchString(ref) {
		missing("release checkout ref %q is not a publishable semver tag", ref)
	}
	if !reviewSeen {
		missing("no run step invokes the review binary with --base")
	}
	if !publishSeen {
		missing("no run step posts the review with gh pr comment")
	}

	perms := get(job, "permissions")
	if perms == nil {
		mutated("publishing job declares no permissions block, so the review comment cannot be posted")
	}
	if node := get(perms, "contents"); node == nil || node.Value != "read" {
		missing("publishing job permissions must pin contents: read")
	}
	if node := get(perms, "pull-requests"); node == nil || node.Value != "write" {
		mutated("publishing job does not declare pull-requests: write, so posting the review comment would be refused")
	}

	fmt.Printf("AUR-440/E2E ok uses=%s@%s sha256=%x\n", repository, ref, sha256.Sum256(raw))
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
printf '{"card":"AUR-440","scenario":"AC-001","layer":"e2e","result":"pass","copy":"byte-identical","repeat":"deterministic"}\n'
