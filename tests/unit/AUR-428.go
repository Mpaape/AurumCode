// Package unit holds AUR-428's Unit-layer proof: the example workflow the
// user copies verbatim (.github/workflows/examples/documentation.yml) is a
// real, parseable GitHub Actions manifest whose own declarations deliver the
// card's outcome -- it can be triggered by `gh workflow run`, it references
// the AurumCode action by a publishable semver tag (never a local commit SHA
// and never a mutable branch), and it grants exactly the permissions GitHub
// Pages deployment requires (`pages: write`, `id-token: write`).
//
// The card's "Restricao medida" fixes the scope: the acceptance sandbox has
// no network, so this layer is STATIC verification of the manifest -- a real
// YAML parse into a node tree, walked structurally. It is not a grep over
// bytes: a workflow that does not parse, or whose structure moved, fails
// here even if the right substrings still appear somewhere in the file.
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office (AUR-402..AUR-411, AUR-422, AUR-424):
// tests/acceptance/AUR-428.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR428, so these
// assertions run inside the sandboxed acceptance instead of being swept into
// an unrelated top-level `go test ./...`.
package unit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// aur428RepoRoot resolves the repository root: the acceptance script exports
// AURUMCODE_ROOT pointing at its staged copy; a developer running the bridge
// manually gets a walk-up from the working directory to the nearest go.mod.
func aur428RepoRoot(t *testing.T) string {
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

// aur428LoadWorkflow parses the file at path as YAML and returns the document
// root mapping. A file that fails to parse is a broken manifest, which is the
// behavior this card promises, so the failure label is behavior-missing.
func aur428LoadWorkflow(t *testing.T, path string) *yaml.Node {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow unreadable: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow is not parseable YAML: %v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow does not parse to a YAML mapping")
	}
	return doc.Content[0]
}

// aur428MapGet returns the value node for the mapping entry whose key scalar
// is written exactly as key. Comparing the key node's literal Value sidesteps
// YAML 1.1 boolean resolution of bare `on`, which is a key in every workflow.
func aur428MapGet(mapping *yaml.Node, key string) *yaml.Node {
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

// aur428ScalarEq fails the test with the given label when the mapping's entry
// is absent or its scalar value differs from want.
func aur428ScalarEq(t *testing.T, mapping *yaml.Node, key, want, context string) {
	t.Helper()
	node := aur428MapGet(mapping, key)
	if node == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: %s declares no %q entry", context, key)
	}
	if node.Value != want {
		t.Fatalf("AUR-428/AC-001/behavior-missing: %s %q is %q, want %q", context, key, node.Value, want)
	}
}

// aur428FindActionStep walks jobs.*.steps and returns the job node and the
// single step whose `uses` references a repository action outside the
// official `actions/` namespace -- the AurumCode action step the whole
// example exists to run. Zero such steps means the copied workflow cannot
// deliver the outcome; more than one means the example stopped being the
// single-purpose document the card describes.
func aur428FindActionStep(t *testing.T, workflow *yaml.Node) (job *yaml.Node, step *yaml.Node, uses string) {
	t.Helper()
	jobs := aur428MapGet(workflow, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode || len(jobs.Content) == 0 {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow declares no jobs")
	}
	var found int
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		jobNode := jobs.Content[i+1]
		steps := aur428MapGet(jobNode, "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		for _, candidate := range steps.Content {
			usesNode := aur428MapGet(candidate, "uses")
			if usesNode == nil || strings.HasPrefix(usesNode.Value, "actions/") {
				continue
			}
			found++
			job, step, uses = jobNode, candidate, usesNode.Value
		}
	}
	switch found {
	case 0:
		t.Fatalf("AUR-428/AC-001/behavior-missing: no step in the example workflow uses the AurumCode action")
	case 1:
		return job, step, uses
	}
	t.Fatalf("AUR-428/AC-001/behavior-missing: %d non-actions/* steps found, expected exactly one AurumCode step", found)
	return nil, nil, ""
}

var (
	aur428CommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	aur428SemverTag = regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)
	aur428OwnerRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

// aur428SplitUses validates the shape `owner/repo@ref` and returns both
// halves. Anything else cannot resolve to this repository's root action.
func aur428SplitUses(t *testing.T, uses string) (ownerRepo, ref string) {
	t.Helper()
	at := strings.LastIndex(uses, "@")
	if at <= 0 || at == len(uses)-1 {
		t.Fatalf("AUR-428/AC-001/MUT-001: uses %q carries no @ref; the manifest cannot pin what it executes", uses)
	}
	ownerRepo, ref = uses[:at], uses[at+1:]
	if !aur428OwnerRepo.MatchString(ownerRepo) {
		t.Fatalf("AUR-428/AC-001/MUT-001: uses %q does not reference a repository-root action as owner/repo", uses)
	}
	return ownerRepo, ref
}

// TestAUR428 is AUR-428's Unit-layer selector: the example workflow document,
// parsed for real, declares everything the card's outcome depends on.
//
// Before the fix this failed with AUR-428/AC-001/behavior-missing: the
// example pinned `Mpaape/AurumCode@<40-hex local SHA>`, a reference the user
// cannot rely on as the published contract -- the card requires a publishable
// semver tag (`v1`) that a human publishes once and every copied workflow
// resolves. A MUT-001 run that points the workflow at a nonexistent or
// unpublishable reference fails here with the AUR-428/AC-001/MUT-001 label.
func TestAUR428(t *testing.T) {
	root := aur428RepoRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "examples", "documentation.yml")
	workflow := aur428LoadWorkflow(t, workflowPath)

	// The card's declared command is `gh workflow run documentation.yml`;
	// without a workflow_dispatch trigger that command has nothing to invoke.
	onNode := aur428MapGet(workflow, "on")
	if onNode == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow declares no `on` triggers")
	}
	if aur428MapGet(onNode, "workflow_dispatch") == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow has no workflow_dispatch trigger, so `gh workflow run documentation.yml` cannot start it")
	}

	// Least privilege at the workflow level: the default token must stay
	// read-only; the deploy permissions live only on the job that needs them.
	topPerms := aur428MapGet(workflow, "permissions")
	if topPerms == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: example workflow declares no top-level permissions block, so it would inherit the repository default")
	}
	aur428ScalarEq(t, topPerms, "contents", "read", "top-level permissions")

	job, step, uses := aur428FindActionStep(t, workflow)

	// The measured restriction: the permissions manifest of the deploying job
	// declares pages: write and id-token: write. Without them the deploy step
	// inside the action is refused by GitHub at run time.
	jobPerms := aur428MapGet(job, "permissions")
	if jobPerms == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: deploy job declares no permissions block")
	}
	aur428ScalarEq(t, jobPerms, "contents", "read", "deploy job permissions")
	aur428ScalarEq(t, jobPerms, "pages", "write", "deploy job permissions")
	aur428ScalarEq(t, jobPerms, "id-token", "write", "deploy job permissions")

	// The github-pages environment is what carries the deployed URL.
	environment := aur428MapGet(job, "environment")
	if environment == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: deploy job declares no github-pages environment")
	}
	aur428ScalarEq(t, environment, "name", "github-pages", "deploy job environment")

	// The reference the copied workflow executes. A 40-hex commit SHA is the
	// pre-fix state: it points at a commit of this working repository, which
	// is not the publishable contract the user copies. Anything that is
	// neither a SHA nor a semver tag (a branch, garbage, an empty ref) is a
	// mutated, unpublishable reference and fails the manifest validation with
	// the card's MUT-001 marker.
	ownerRepo, ref := aur428SplitUses(t, uses)
	if aur428CommitSHA.MatchString(ref) {
		t.Fatalf("AUR-428/AC-001/behavior-missing: uses %q pins a local commit SHA; the copied workflow must reference the publishable semver tag", uses)
	}
	if !aur428SemverTag.MatchString(ref) {
		t.Fatalf("AUR-428/AC-001/MUT-001: uses %q ref %q is not a publishable semver tag (vN, vN.N or vN.N.N)", uses, ref)
	}

	// publish: pages is what turns "generate" into "published on GitHub
	// Pages" within the same job the permissions above authorize.
	with := aur428MapGet(step, "with")
	if with == nil {
		t.Fatalf("AUR-428/AC-001/behavior-missing: AurumCode step declares no `with` block")
	}
	aur428ScalarEq(t, with, "publish", "pages", "AurumCode step with")

	t.Logf("AUR-428/AC-001/unit pass uses=%s", fmt.Sprintf("%s@%s", ownerRepo, ref))
}
