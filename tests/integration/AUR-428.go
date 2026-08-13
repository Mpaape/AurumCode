// Package integration holds AUR-428's Integration-layer proof: the example
// workflow, the repository's root action.yml, go.mod and the LICENSE agree
// with each other. The Unit layer proves the workflow document alone is
// sound; this layer proves the cross-artifact contract the user's copy
// depends on:
//
//  1. the `uses:` reference points at THIS repository (the owner/repo derived
//     from go.mod's module path), by a publishable semver tag;
//  2. every `with:` key the workflow passes is an input action.yml actually
//     declares, so the copied file drives capabilities the repository really
//     offers;
//  3. the action is the composite the workflow expects, and it declares the
//     `publish` input the example sets to `pages`;
//  4. LICENSE exists with a real grant -- the artifact that makes the semver
//     tag publishable and the action legally consumable from a copied
//     workflow.
//
// Scope per the card's "Restricao medida": the sandbox has no network, so
// this is STATIC verification via real YAML parses, never a live GitHub
// call. Publishing the `v1` tag itself is a human action documented in
// docs/specs/AUR-428.md.
//
// Not named "_test.go" on purpose (same technique as every sibling card):
// tests/acceptance/AUR-428.sh stages a private copy and bridges this
// function into `go test`.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func aur428iRepoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-428/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-428/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

func aur428iLoadYAML(t *testing.T, path, what string) *yaml.Node {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: %s unreadable: %v", what, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: %s is not parseable YAML: %v", what, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("AUR-428/AC-001/behavior-missing: %s does not parse to a YAML mapping", what)
	}
	return doc.Content[0]
}

func aur428iMapGet(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

var (
	aur428iModuleLine = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)
	aur428iCommitSHA  = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	aur428iSemverTag  = regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)
)

// aur428iExpectedRepo derives the only owner/repo the example workflow may
// reference: the GitHub repository that go.mod says this module lives in.
// Deriving it instead of hardcoding it keeps the assertion honest if the
// repository is ever renamed -- the workflow must follow, or this fails.
func aur428iExpectedRepo(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("AUR-428/AC-001/infrastructure: go.mod unreadable: %v", err)
	}
	match := aur428iModuleLine.FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatalf("AUR-428/AC-001/infrastructure: go.mod declares no module path")
	}
	modulePath := match[1]
	const host = "github.com/"
	if !strings.HasPrefix(modulePath, host) {
		t.Fatalf("AUR-428/AC-001/infrastructure: module path %q is not hosted on github.com", modulePath)
	}
	ownerRepo := strings.TrimPrefix(modulePath, host)
	if strings.Count(ownerRepo, "/") != 1 {
		t.Fatalf("AUR-428/AC-001/infrastructure: module path %q does not resolve to owner/repo", modulePath)
	}
	return ownerRepo
}

// aur428iFindActionStep mirrors the Unit-layer walk: the single step whose
// `uses` lies outside the official actions/ namespace.
func aur428iFindActionStep(t *testing.T, workflow *yaml.Node) (step *yaml.Node, uses string) {
	t.Helper()
	jobs := aur428iMapGet(workflow, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow declares no jobs")
	}
	var found int
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		steps := aur428iMapGet(jobs.Content[i+1], "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		for _, candidate := range steps.Content {
			usesNode := aur428iMapGet(candidate, "uses")
			if usesNode == nil || strings.HasPrefix(usesNode.Value, "actions/") {
				continue
			}
			found++
			step, uses = candidate, usesNode.Value
		}
	}
	if found != 1 {
		t.Fatalf("AUR-428/AC-001/behavior-missing: expected exactly one AurumCode step, found %d", found)
	}
	return step, uses
}

// IntegrationAUR428 is AUR-428's Integration-layer selector.
//
// RED before the fix: the example pinned a 40-hex local commit SHA and no
// LICENSE existed at the repository root -- both fail below with the
// AUR-428/AC-001/behavior-missing label. A MUT-001 mutation that points the
// workflow at a repository other than this one, at a non-semver reference,
// or at inputs action.yml does not declare, fails with AUR-428/AC-001/
// MUT-001: the manifest validation refuses a reference that cannot exist as
// the published contract.
func IntegrationAUR428(t *testing.T) {
	root := aur428iRepoRoot(t)
	workflow := aur428iLoadYAML(t,
		filepath.Join(root, ".github", "workflows", "examples", "documentation.yml"),
		"example workflow")
	action := aur428iLoadYAML(t, filepath.Join(root, "action.yml"), "action.yml")

	// -- workflow -> repository identity ---------------------------------
	expectedRepo := aur428iExpectedRepo(t, root)
	step, uses := aur428iFindActionStep(t, workflow)
	at := strings.LastIndex(uses, "@")
	if at <= 0 || at == len(uses)-1 {
		t.Fatalf("AUR-428/AC-001/MUT-001: uses %q carries no @ref", uses)
	}
	ownerRepo, ref := uses[:at], uses[at+1:]
	if ownerRepo != expectedRepo {
		t.Fatalf("AUR-428/AC-001/MUT-001: uses %q references %q, but this repository's action lives at %q (per go.mod); the copied workflow would execute something else or nothing",
			uses, ownerRepo, expectedRepo)
	}
	if aur428iCommitSHA.MatchString(ref) {
		t.Fatalf("AUR-428/AC-001/behavior-missing: uses %q pins a local commit SHA instead of the publishable semver tag", uses)
	}
	if !aur428iSemverTag.MatchString(ref) {
		t.Fatalf("AUR-428/AC-001/MUT-001: uses %q ref %q is not a publishable semver tag", uses, ref)
	}

	// -- workflow -> action.yml contract ---------------------------------
	runs := aur428iMapGet(action, "runs")
	if runs == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: action.yml declares no runs block")
	}
	if using := aur428iMapGet(runs, "using"); using == nil || using.Value != "composite" {
		t.Fatalf("AUR-428/AC-001/behavior-missing: action.yml is not a composite action")
	}

	inputs := aur428iMapGet(action, "inputs")
	if inputs == nil || inputs.Kind != yaml.MappingNode {
		t.Fatalf("AUR-428/AC-001/behavior-missing: action.yml declares no inputs")
	}
	declared := make(map[string]bool, len(inputs.Content)/2)
	for i := 0; i+1 < len(inputs.Content); i += 2 {
		declared[inputs.Content[i].Value] = true
	}

	with := aur428iMapGet(step, "with")
	if with == nil || with.Kind != yaml.MappingNode {
		t.Fatalf("AUR-428/AC-001/behavior-missing: AurumCode step declares no `with` block")
	}
	for i := 0; i+1 < len(with.Content); i += 2 {
		key := with.Content[i].Value
		if !declared[key] {
			t.Fatalf("AUR-428/AC-001/MUT-001: workflow passes input %q, which action.yml does not declare; the copied manifest references a capability this repository does not offer", key)
		}
	}

	if !declared["publish"] {
		t.Fatalf("AUR-428/AC-001/behavior-missing: action.yml declares no `publish` input, so the workflow cannot ask for a Pages deploy")
	}
	if publish := aur428iMapGet(with, "publish"); publish == nil || publish.Value != "pages" {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow does not set publish: 'pages'")
	}

	// -- LICENSE ----------------------------------------------------------
	// The tag the workflow references is publishable only when the content
	// it ships is licensed: a user copying the workflow executes this
	// repository's code inside their own CI.
	licenseRaw, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: LICENSE absent at the repository root: %v", err)
	}
	license := string(licenseRaw)
	for _, want := range []string{
		"MIT License",
		"Permission is hereby granted, free of charge",
		"THE SOFTWARE IS PROVIDED \"AS IS\"",
	} {
		if !strings.Contains(license, want) {
			t.Fatalf("AUR-428/AC-001/behavior-missing: LICENSE does not carry the expected grant text %q", want)
		}
	}

	t.Logf("AUR-428/AC-001/integration pass uses=%s inputs_checked=%d", uses, len(with.Content)/2)
}
