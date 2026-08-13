// Package unit holds AUR-440's Unit-layer proof: the example workflow the
// user copies verbatim (.github/workflows/examples/code-review.yml) is a
// real, parseable GitHub Actions manifest whose own declarations deliver the
// card's outcome -- every pull request opened in the user's repository is
// reviewed automatically and receives the AurumCode comment, with nobody
// running a command.
//
// The architecture this card fixed in its Escopo: GitHub Actions triggers
// natively on the `pull_request` event and authenticates with the Action's
// own token. There is NO self-hosted webhook server and NO HMAC validation
// here -- that second trigger model (internal/git/webhook, cmd/server) stays
// out of scope, recoverable from commit c12d7ab if a future card wants it.
//
// Scope per the card's Preconditions: the acceptance sandbox has no network,
// so this layer is STATIC verification of the manifest -- a real YAML parse
// into a node tree (gopkg.in/yaml.v3), walked structurally. It is not a grep
// over bytes: a workflow that does not parse, or whose structure moved,
// fails here even if the right substrings still appear somewhere in the
// file.
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office (AUR-402..AUR-411, AUR-422, AUR-424, AUR-428):
// tests/acceptance/AUR-440.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR440, so these
// assertions run inside the sandboxed acceptance instead of being swept into
// an unrelated top-level `go test ./...`.
package unit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// aur440RepoRoot resolves the repository root: the acceptance script exports
// AURUMCODE_ROOT pointing at its staged copy; a developer running the bridge
// manually gets a walk-up from the working directory to the nearest go.mod.
func aur440RepoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-440/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-440/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

// aur440LoadWorkflow parses the file at path as YAML and returns the document
// root mapping. A file that fails to parse is a broken manifest, which is the
// behavior this card promises, so the failure label is behavior-missing.
func aur440LoadWorkflow(t *testing.T, path string) *yaml.Node {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow unreadable: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow is not parseable YAML: %v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow does not parse to a YAML mapping")
	}
	return doc.Content[0]
}

// aur440Get returns the value node for the mapping entry whose key scalar is
// written exactly as key. Comparing the key node's literal Value sidesteps
// YAML 1.1 boolean resolution of bare `on`, which is a key in every workflow.
func aur440Get(mapping *yaml.Node, key string) *yaml.Node {
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
	aur440CommitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	aur440SemverTag = regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)
	aur440OwnerRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

// aur440SingleJob returns the single job the example declares. The example is
// a single-purpose document: one job that reviews and publishes; a second job
// would be a place for permissions to hide.
func aur440SingleJob(t *testing.T, workflow *yaml.Node) *yaml.Node {
	t.Helper()
	jobs := aur440Get(workflow, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode || len(jobs.Content) == 0 {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares no jobs")
	}
	if len(jobs.Content) != 2 {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares %d jobs, want exactly one review job", len(jobs.Content)/2)
	}
	return jobs.Content[1]
}

// aur440Steps returns the job's steps sequence.
func aur440Steps(t *testing.T, job *yaml.Node) []*yaml.Node {
	t.Helper()
	steps := aur440Get(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode || len(steps.Content) == 0 {
		t.Fatalf("AUR-440/AC-001/behavior-missing: review job declares no steps")
	}
	return steps.Content
}

// aur440FindToolCheckout walks the job's steps and returns the single
// actions/checkout step that carries a `with.repository` entry -- the step
// that obtains the AurumCode release the job builds the review binary from.
// Zero such steps means the copied workflow has no AurumCode to run; more
// than one means the example stopped being the single-purpose document the
// card describes.
func aur440FindToolCheckout(t *testing.T, steps []*yaml.Node) (step *yaml.Node, repository, ref string) {
	t.Helper()
	var found int
	for _, candidate := range steps {
		usesNode := aur440Get(candidate, "uses")
		if usesNode == nil || !strings.HasPrefix(usesNode.Value, "actions/checkout@") {
			continue
		}
		with := aur440Get(candidate, "with")
		repoNode := aur440Get(with, "repository")
		if repoNode == nil {
			continue
		}
		found++
		step = candidate
		repository = repoNode.Value
		if refNode := aur440Get(with, "ref"); refNode != nil {
			ref = refNode.Value
		}
	}
	switch found {
	case 0:
		t.Fatalf("AUR-440/AC-001/behavior-missing: no checkout step obtains AurumCode at a published release, so the copied workflow has no review binary to build")
	case 1:
		return step, repository, ref
	}
	t.Fatalf("AUR-440/AC-001/behavior-missing: %d checkout steps declare a repository, expected exactly one AurumCode release checkout", found)
	return nil, "", ""
}

// TestAUR440 is AUR-440's Unit-layer selector: the example workflow document,
// parsed for real, declares everything the card's outcome depends on.
//
// Before the fix this failed with AUR-440/AC-001/behavior-missing: the
// example was still the pre-restoration documentation gate -- it delegated to
// the root composite action (which cannot review a pull request), pinned a
// local 40-hex commit SHA instead of the publishable v1 tag, and never
// declared the pull-requests: write permission a review comment needs. A
// MUT-001 run that removes the publishing job's pull-requests: write fails
// here with the AUR-440/AC-001/MUT-001 label.
func TestAUR440(t *testing.T) {
	root := aur440RepoRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "examples", "code-review.yml")
	workflow := aur440LoadWorkflow(t, workflowPath)

	// -- trigger: the event that makes "nobody runs a command" true --------
	// The card's outcome starts when the user opens a pull request, so the
	// workflow must trigger on pull_request (natively, with the Action's own
	// token -- no webhook server, no HMAC) and must cover the events that
	// carry new code: opened, synchronize, reopened.
	onNode := aur440Get(workflow, "on")
	if onNode == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares no `on` triggers")
	}
	pullRequest := aur440Get(onNode, "pull_request")
	if pullRequest == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow has no pull_request trigger, so opening a pull request starts nothing")
	}
	types := aur440Get(pullRequest, "types")
	if types == nil || types.Kind != yaml.SequenceNode {
		t.Fatalf("AUR-440/AC-001/behavior-missing: pull_request trigger declares no types list")
	}
	declaredTypes := make(map[string]bool, len(types.Content))
	for _, node := range types.Content {
		declaredTypes[node.Value] = true
	}
	for _, want := range []string{"opened", "synchronize", "reopened"} {
		if !declaredTypes[want] {
			t.Fatalf("AUR-440/AC-001/behavior-missing: pull_request types %v miss %q, so that event would not be reviewed", declaredTypes, want)
		}
	}

	// -- least privilege at the workflow level -----------------------------
	// The default token stays read-only; the write grant lives only on the
	// job that publishes the comment. A pull-requests entry at the top level
	// -- even read -- would widen every future job silently.
	topPerms := aur440Get(workflow, "permissions")
	if topPerms == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares no top-level permissions block, so it would inherit the repository default")
	}
	if node := aur440Get(topPerms, "contents"); node == nil || node.Value != "read" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: top-level permissions must pin contents: read")
	}
	if aur440Get(topPerms, "pull-requests") != nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: pull-requests permission belongs only on the publishing job, never at the workflow level")
	}

	// -- concurrency: a superseded push must not double-comment ------------
	concurrency := aur440Get(workflow, "concurrency")
	if concurrency == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares no concurrency block, so stale runs would race the fresh one")
	}
	if group := aur440Get(concurrency, "group"); group == nil || group.Value == "" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: concurrency block declares no group")
	}
	if cancel := aur440Get(concurrency, "cancel-in-progress"); cancel == nil || cancel.Value != "true" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: concurrency must cancel in-progress runs so a superseded push is not reviewed twice")
	}

	job := aur440SingleJob(t, workflow)
	steps := aur440Steps(t, job)

	// -- supply chain of the steps themselves ------------------------------
	// Every `uses:` must live in the official actions/ namespace, pinned to
	// an immutable commit. In particular the workflow must NOT delegate to a
	// repository action (`Mpaape/AurumCode@...`): the root composite is the
	// documentation generator, and a step that runs it would produce a green
	// check for a review that never happened. The review binary is built
	// from the release checkout below instead.
	for _, step := range steps {
		usesNode := aur440Get(step, "uses")
		if usesNode == nil {
			continue
		}
		uses := usesNode.Value
		if !strings.HasPrefix(uses, "actions/") {
			t.Fatalf("AUR-440/AC-001/behavior-missing: step uses %q; the review must run through the binary built from the release checkout, never through a repository action that cannot review", uses)
		}
		at := strings.LastIndex(uses, "@")
		if at <= 0 || at == len(uses)-1 || !aur440CommitSHA.MatchString(uses[at+1:]) {
			t.Fatalf("AUR-440/AC-001/behavior-missing: step uses %q is not pinned to an immutable 40-hex commit", uses)
		}
	}

	// -- the AurumCode release the job builds and runs ---------------------
	toolStep, repository, ref := aur440FindToolCheckout(t, steps)
	if !aur440OwnerRepo.MatchString(repository) {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout repository %q is not owner/repo", repository)
	}
	if ref == "" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout declares no ref; the manifest cannot pin what it builds")
	}
	if aur440CommitSHA.MatchString(ref) {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout ref %q pins a local commit SHA; the copied workflow must reference the publishable semver tag", ref)
	}
	if !aur440SemverTag.MatchString(ref) {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout ref %q is not a publishable semver tag (vN, vN.N or vN.N.N)", ref)
	}
	toolPath := aur440Get(aur440Get(toolStep, "with"), "path")
	if toolPath == nil || toolPath.Value == "" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout declares no path, so the tool tree would overwrite the pull request tree it must review")
	}

	// -- MUT-001: the permission the comment cannot exist without ----------
	// The measured mutation: removing pull-requests: write from the
	// publishing job makes this manifest validation fail. contents: read is
	// what lets both checkouts happen; pull-requests: write is what lets
	// `gh pr comment` post the review.
	jobPerms := aur440Get(job, "permissions")
	if jobPerms == nil {
		t.Fatalf("AUR-440/AC-001/MUT-001: publishing job declares no permissions block, so the review comment cannot be posted")
	}
	if node := aur440Get(jobPerms, "contents"); node == nil || node.Value != "read" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: publishing job permissions must pin contents: read")
	}
	if node := aur440Get(jobPerms, "pull-requests"); node == nil || node.Value != "write" {
		t.Fatalf("AUR-440/AC-001/MUT-001: publishing job does not declare pull-requests: write, so posting the review comment would be refused")
	}

	// -- the scripts: injection guard, review run, publication -------------
	// Runtime data reaches every script through env, never through ${{ }}
	// interpolation inside a run: body -- a substituted value carrying a
	// quote or semicolon would execute as shell.
	var reviewStep, publishStep *yaml.Node
	for _, step := range steps {
		runNode := aur440Get(step, "run")
		if runNode == nil {
			continue
		}
		if strings.Contains(runNode.Value, "${{") {
			t.Fatalf("AUR-440/AC-001/behavior-missing: a run script interpolates ${{ }} into its body; runtime data must travel through env")
		}
		if strings.Contains(runNode.Value, "review") && strings.Contains(runNode.Value, "--base") {
			reviewStep = step
		}
		if strings.Contains(runNode.Value, "gh pr comment") {
			publishStep = step
		}
	}
	if reviewStep == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: no run step invokes the review binary with --base, so the diff is never reviewed")
	}
	reviewEnv := aur440Get(reviewStep, "env")
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "BASE_SHA"} {
		if aur440Get(reviewEnv, key) == nil {
			t.Fatalf("AUR-440/AC-001/behavior-missing: review step env declares no %s, so the binary cannot run", key)
		}
	}
	if publishStep == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: no run step posts the review with `gh pr comment`, so the pull request never receives it")
	}
	if aur440Get(aur440Get(publishStep, "env"), "GH_TOKEN") == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: publish step env declares no GH_TOKEN, so gh cannot authenticate with the Action's own token")
	}

	t.Logf("AUR-440/AC-001/unit pass repository=%s ref=%s", repository, ref)
}
