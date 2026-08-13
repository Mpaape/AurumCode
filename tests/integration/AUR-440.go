// Package integration holds AUR-440's Integration-layer proof: the example
// workflow, the repository's root action.yml and go.mod agree with each
// other. The Unit layer proves the workflow document alone is sound; this
// layer proves the cross-artifact contract the user's copy depends on:
//
//  1. the release checkout points at THIS repository (the owner/repo derived
//     from go.mod's module path), by a publishable semver tag, so the review
//     binary the job builds is the one this repository ships;
//  2. the workflow never delegates to the root composite action: action.yml,
//     parsed for real, is the documentation generator -- it declares no
//     input that could name a pull request or ask for a review, so a step
//     using it would produce a green check for a review that never ran;
//  3. the Go toolchain the workflow installs can build this module: the
//     setup-go version satisfies go.mod's `go` directive, and the build step
//     compiles `./cmd/aurumcode` out of the release checkout's path;
//  4. the secrets reach the binary only through the secrets context, and the
//     publication authenticates with the Action's own token.
//
// Scope per the card's Preconditions: the sandbox has no network, so this is
// STATIC verification via real YAML parses, never a live GitHub call.
// Publishing the `v1` tag itself is a human action documented in
// docs/specs/AUR-440.md.
//
// Not named "_test.go" on purpose (same technique as every sibling card):
// tests/acceptance/AUR-440.sh stages a private copy and bridges this
// function into `go test`.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func aur440iRepoRoot(t *testing.T) string {
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

func aur440iLoadYAML(t *testing.T, path, what string) *yaml.Node {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: %s unreadable: %v", what, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: %s is not parseable YAML: %v", what, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("AUR-440/AC-001/behavior-missing: %s does not parse to a YAML mapping", what)
	}
	return doc.Content[0]
}

func aur440iGet(mapping *yaml.Node, key string) *yaml.Node {
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
	aur440iModuleLine   = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)
	aur440iGoDirective  = regexp.MustCompile(`(?m)^go\s+([0-9]+)\.([0-9]+)`)
	aur440iCommitSHA    = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	aur440iSemverTag    = regexp.MustCompile(`^v[0-9]+(\.[0-9]+){0,2}$`)
	aur440iGoVersion    = regexp.MustCompile(`^([0-9]+)\.([0-9]+)`)
	aur440iSecretsValue = regexp.MustCompile(`^\$\{\{\s*secrets\.[A-Za-z0-9_]+\s*\}\}$`)
)

// aur440iExpectedRepo derives the only owner/repo the release checkout may
// reference: the GitHub repository that go.mod says this module lives in.
// Deriving it instead of hardcoding it keeps the assertion honest if the
// repository is ever renamed -- the workflow must follow, or this fails.
func aur440iExpectedRepo(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("AUR-440/AC-001/infrastructure: go.mod unreadable: %v", err)
	}
	match := aur440iModuleLine.FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatalf("AUR-440/AC-001/infrastructure: go.mod declares no module path")
	}
	modulePath := match[1]
	const host = "github.com/"
	if !strings.HasPrefix(modulePath, host) {
		t.Fatalf("AUR-440/AC-001/infrastructure: module path %q is not hosted on github.com", modulePath)
	}
	ownerRepo := strings.TrimPrefix(modulePath, host)
	if strings.Count(ownerRepo, "/") != 1 {
		t.Fatalf("AUR-440/AC-001/infrastructure: module path %q does not resolve to owner/repo", modulePath)
	}
	return ownerRepo
}

// aur440iGoModMinor reads go.mod's `go` directive as (major, minor).
func aur440iGoModMinor(t *testing.T, root string) (int, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("AUR-440/AC-001/infrastructure: go.mod unreadable: %v", err)
	}
	match := aur440iGoDirective.FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatalf("AUR-440/AC-001/infrastructure: go.mod declares no go directive")
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	return major, minor
}

// aur440iSteps flattens jobs.*.steps into one list; the Unit layer already
// pins the job count to exactly one.
func aur440iSteps(t *testing.T, workflow *yaml.Node) []*yaml.Node {
	t.Helper()
	jobs := aur440iGet(workflow, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares no jobs")
	}
	var all []*yaml.Node
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		steps := aur440iGet(jobs.Content[i+1], "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		all = append(all, steps.Content...)
	}
	if len(all) == 0 {
		t.Fatalf("AUR-440/AC-001/behavior-missing: example workflow declares no steps")
	}
	return all
}

// IntegrationAUR440 is AUR-440's Integration-layer selector.
//
// RED before the fix: the example was still the documentation gate -- it
// delegated to the root composite action by a local 40-hex SHA, which fails
// below both as a composite delegation and as an unpublishable reference. A
// MUT-001 mutation that removes the publishing job's pull-requests: write
// permission fails in the Unit layer and the E2E with the AUR-440/AC-001/
// MUT-001 label; this layer keeps the cross-artifact contract pinned.
func IntegrationAUR440(t *testing.T) {
	root := aur440iRepoRoot(t)
	workflow := aur440iLoadYAML(t,
		filepath.Join(root, ".github", "workflows", "examples", "code-review.yml"),
		"example workflow")
	action := aur440iLoadYAML(t, filepath.Join(root, "action.yml"), "action.yml")

	expectedRepo := aur440iExpectedRepo(t, root)
	steps := aur440iSteps(t, workflow)

	// -- the root composite action cannot review, so no step may use it ----
	// action.yml is the documentation generator: a composite whose declared
	// inputs carry no way to name a pull request or ask for a review. That
	// is proven from its parsed structure, and it is why the workflow must
	// build and run the review binary instead of delegating to the action.
	runs := aur440iGet(action, "runs")
	if runs == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: action.yml declares no runs block")
	}
	if using := aur440iGet(runs, "using"); using == nil || using.Value != "composite" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: action.yml is not a composite action")
	}
	actionInputs := aur440iGet(action, "inputs")
	if actionInputs == nil || actionInputs.Kind != yaml.MappingNode {
		t.Fatalf("AUR-440/AC-001/behavior-missing: action.yml declares no inputs")
	}
	for i := 0; i+1 < len(actionInputs.Content); i += 2 {
		key := actionInputs.Content[i].Value
		switch key {
		case "pr", "pr-number", "pull-request", "review", "publish-comment":
			t.Fatalf("AUR-440/AC-001/behavior-missing: action.yml now declares review input %q; this card's premise (the composite cannot review) no longer holds -- revisit the workflow", key)
		}
	}
	for _, step := range steps {
		usesNode := aur440iGet(step, "uses")
		if usesNode == nil {
			continue
		}
		uses := usesNode.Value
		ownerRepo := uses
		if at := strings.LastIndex(uses, "@"); at > 0 {
			ownerRepo = uses[:at]
		}
		if ownerRepo == expectedRepo || strings.HasPrefix(ownerRepo, expectedRepo+"/") {
			t.Fatalf("AUR-440/AC-001/behavior-missing: step uses %q delegates to the root composite action, which action.yml proves is the documentation generator and cannot review a pull request", uses)
		}
	}

	// -- release checkout -> repository identity ---------------------------
	var toolWith *yaml.Node
	var repository, ref string
	var found int
	for _, step := range steps {
		usesNode := aur440iGet(step, "uses")
		if usesNode == nil || !strings.HasPrefix(usesNode.Value, "actions/checkout@") {
			continue
		}
		with := aur440iGet(step, "with")
		repoNode := aur440iGet(with, "repository")
		if repoNode == nil {
			continue
		}
		found++
		toolWith = with
		repository = repoNode.Value
		if refNode := aur440iGet(with, "ref"); refNode != nil {
			ref = refNode.Value
		}
	}
	if found != 1 {
		t.Fatalf("AUR-440/AC-001/behavior-missing: expected exactly one AurumCode release checkout, found %d", found)
	}
	if repository != expectedRepo {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout references %q, but this repository's review binary lives at %q (per go.mod); the copied workflow would build something else or nothing",
			repository, expectedRepo)
	}
	if aur440iCommitSHA.MatchString(ref) {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout ref %q pins a local commit SHA instead of the publishable semver tag", ref)
	}
	if !aur440iSemverTag.MatchString(ref) {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout ref %q is not a publishable semver tag", ref)
	}

	// -- the toolchain can build this module -------------------------------
	modMajor, modMinor := aur440iGoModMinor(t, root)
	var goVersion string
	for _, step := range steps {
		usesNode := aur440iGet(step, "uses")
		if usesNode == nil || !strings.HasPrefix(usesNode.Value, "actions/setup-go@") {
			continue
		}
		if v := aur440iGet(aur440iGet(step, "with"), "go-version"); v != nil {
			goVersion = v.Value
		}
	}
	if goVersion == "" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: no setup-go step declares a go-version, so the runner may lack the toolchain go.mod requires")
	}
	vMatch := aur440iGoVersion.FindStringSubmatch(goVersion)
	if vMatch == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: setup-go go-version %q is not a numeric version", goVersion)
	}
	wfMajor, _ := strconv.Atoi(vMatch[1])
	wfMinor, _ := strconv.Atoi(vMatch[2])
	if wfMajor < modMajor || (wfMajor == modMajor && wfMinor < modMinor) {
		t.Fatalf("AUR-440/AC-001/behavior-missing: setup-go go-version %q cannot build a module requiring go %d.%d (per go.mod)", goVersion, modMajor, modMinor)
	}

	// -- the build compiles the review command out of the release tree -----
	toolPath := aur440iGet(toolWith, "path")
	if toolPath == nil || toolPath.Value == "" {
		t.Fatalf("AUR-440/AC-001/behavior-missing: release checkout declares no path")
	}
	var buildFound bool
	for _, step := range steps {
		runNode := aur440iGet(step, "run")
		if runNode == nil || !strings.Contains(runNode.Value, "go build") {
			continue
		}
		if !strings.Contains(runNode.Value, "./cmd/aurumcode") {
			t.Fatalf("AUR-440/AC-001/behavior-missing: the go build step does not compile ./cmd/aurumcode, the review command this module ships")
		}
		if !strings.Contains(runNode.Value, toolPath.Value) {
			t.Fatalf("AUR-440/AC-001/behavior-missing: the go build step does not build from the release checkout path %q, so it would compile the pull request's own code", toolPath.Value)
		}
		buildFound = true
	}
	if !buildFound {
		t.Fatalf("AUR-440/AC-001/behavior-missing: no run step builds the review binary with go build")
	}

	// -- secrets through the secrets context, token from the Action --------
	var reviewEnv, publishEnv *yaml.Node
	for _, step := range steps {
		runNode := aur440iGet(step, "run")
		if runNode == nil {
			continue
		}
		if strings.Contains(runNode.Value, "review") && strings.Contains(runNode.Value, "--base") {
			reviewEnv = aur440iGet(step, "env")
		}
		if strings.Contains(runNode.Value, "gh pr comment") {
			publishEnv = aur440iGet(step, "env")
		}
	}
	if reviewEnv == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: no review step with an env block invokes the binary with --base")
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL"} {
		node := aur440iGet(reviewEnv, key)
		if node == nil {
			t.Fatalf("AUR-440/AC-001/behavior-missing: review step env declares no %s", key)
		}
		if !aur440iSecretsValue.MatchString(node.Value) {
			t.Fatalf("AUR-440/AC-001/behavior-missing: review step env %s is %q; credentials must come from the secrets context, which GitHub masks in every log line", key, node.Value)
		}
	}
	if publishEnv == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: no publish step with an env block runs gh pr comment")
	}
	ghToken := aur440iGet(publishEnv, "GH_TOKEN")
	if ghToken == nil {
		t.Fatalf("AUR-440/AC-001/behavior-missing: publish step env declares no GH_TOKEN")
	}
	if !strings.Contains(ghToken.Value, "github.token") && !strings.Contains(ghToken.Value, "secrets.GITHUB_TOKEN") {
		t.Fatalf("AUR-440/AC-001/behavior-missing: GH_TOKEN is %q; the publication must authenticate with the Action's own token, never a personal one", ghToken.Value)
	}

	t.Logf("AUR-440/AC-001/integration pass uses=%s@%s go-version=%s", repository, ref, goVersion)
}
