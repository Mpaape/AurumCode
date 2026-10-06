package main

// AUR-536 behavior proof: closes two gaps AUR-515's own review left open.
//
//   - AC-001: a checkout verified as the reviewed repository at the
//     reviewed head commit (AUR-515) can still carry an untracked file.
//     verifiedCleanCheckoutReason (aur536.go) must still catch it: the
//     untracked file's own path -- the fixture's observable marker here --
//     must never reach the provider prompt, and the published review must
//     declare why.
//   - AC-002: codebaseContextMismatch (aur515.go) already returns
//     codebaseContextReasonUnverifiable for three cases -- no local git
//     metadata at all, a checkout with no configured "origin", and a
//     failure fetching the pull request's head SHA from the GitHub API --
//     but AUR-515 never actually drove any of them through the real --pr
//     path. These tests are that missing proof: codebase context is
//     omitted and the omission is published, for each case, with the
//     marker absent and the reason-specific wording present (so a mutation
//     that papers over one case with another's wording still gets caught).
//
// All four tests reuse aur515_test.go's fixture/server/env helpers
// (aur515Fixture, aur515Server, aur515Env) and aur476_test.go's loose-object
// helpers (gitObject, treeEntry) already in this package.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/gittest"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur536UntrackedMarkerPath is the relative path of the untracked file
// TestAUR536UntrackedFileOmitsContextFromPrompt adds on top of an otherwise
// clean, verified checkout. Its path is the fixture's marker: the resolver
// can only ever surface it through a Reference edge's File field (it
// references the committed aur515LocalMarker symbol below), never through
// Pack.Symbols (which comes only from the diff's own changed paths), so
// this path reaching the prompt is specific evidence of the dirty tree
// leaking, not a coincidence of some other field.
const aur536UntrackedMarkerPath = "zz_aur536_untracked_marker.go"

// TestAUR536UntrackedFileOmitsContextFromPrompt covers AC-001.
func TestAUR536UntrackedFileOmitsContextFromPrompt(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")

	// An uncommitted, untracked file on top of the otherwise clean,
	// verified checkout -- it references the committed aur515LocalMarker
	// symbol so the pre-AUR-536 resolver would have recorded a Reference
	// edge naming this file's own path.
	untracked := filepath.Join(dir, aur536UntrackedMarkerPath)
	content := "package demo\n\n// references " + aur515LocalMarker + "\nvar _ = \"" + aur515LocalMarker + "\"\n"
	if err := os.WriteFile(untracked, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	var posted struct{ Body string }
	server := aur515Server(t, localHead, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), aur536UntrackedMarkerPath) {
		t.Fatalf("an untracked file's path must never reach the provider prompt as context:\n%s", string(captured))
	}
	if !strings.Contains(posted.Body, "Repository context omitted") {
		t.Fatalf("published review does not declare the dirty-tree omission:\n%s", posted.Body)
	}
	if !strings.Contains(posted.Body, "uncommitted changes, untracked files, or content that does not match the reviewed commit") {
		t.Fatalf("published review does not use the dirty-tree-specific wording:\n%s", posted.Body)
	}
}

// aur536UnverifiableBareDir builds a plain directory with a committed-style
// app.go carrying aur515LocalMarker, but NO ".git" at all, and chdir's the
// test into it. This is AC-002's first unverifiable case: there is no local
// git metadata to even attempt an identity check against.
func aur536UnverifiableBareDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	app := filepath.Join(dir, "app.go")
	body := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n")
	if err := os.WriteFile(app, body, 0600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir
}

// assertContextOmittedUnverifiable is the shared assertion for AC-002's
// three cases: exit 0, the marker absent from the captured prompt, and the
// published review declaring the codebaseContextReasonUnverifiable-specific
// wording -- not just the generic "Repository context omitted" prefix the
// dirty-tree case (AC-001) also uses, so a mutation that collapses every
// reason into one cannot slip past this by reusing another case's text.
func assertContextOmittedUnverifiable(t *testing.T, code int, stdout, stderr string, capturePath string, posted string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("an unverifiable checkout's content must never reach the provider prompt as context:\n%s", string(captured))
	}
	if !strings.Contains(posted, "Repository context omitted") {
		t.Fatalf("published review does not declare the omitted codebase context:\n%s", posted)
	}
	if !strings.Contains(posted, "the local checkout's identity could not be confirmed") {
		t.Fatalf("published review does not use the unverifiable-specific wording:\n%s", posted)
	}
}

// TestAUR536NoGitMetadataOmitsContext covers AC-002's first case: a
// checkout with no ".git" at all.
func TestAUR536NoGitMetadataOmitsContext(t *testing.T) {
	aur536UnverifiableBareDir(t)

	var posted struct{ Body string }
	server := aur515Server(t, "head-sha", &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	assertContextOmittedUnverifiable(t, code, stdout.String(), stderr.String(), capturePath, posted.Body)
}

// TestAUR536NoOriginRemoteOmitsContext covers AC-002's second case: a real
// checkout, at the reviewed head, with no configured "origin" remote at
// all, so the local repository identity can never be read.
func TestAUR536NoOriginRemoteOmitsContext(t *testing.T) {
	aur515Fixture(t, "")

	var posted struct{ Body string }
	server := aur515Server(t, "head-sha", &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	assertContextOmittedUnverifiable(t, code, stdout.String(), stderr.String(), capturePath, posted.Body)
}

// aur536MetadataFailureServer serves aur515DiffBody for the diff Accept
// header (so the remote diff review still runs) but fails the pull
// request metadata fetch (GetPullRequestMetadata) with a 500 -- AC-002's
// third unverifiable case: the API call resolvePullRequestHeadSHA needs
// simply errors.
func aur536MetadataFailureServer(t *testing.T, posted *struct{ Body string }) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(aur515DiffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			posted.Body = buf.String()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestAUR536PullRequestMetadataFailureOmitsContext covers AC-002's third
// case: the local checkout IS owner/repo, but the GitHub API call for the
// pull request's reviewed head SHA fails, so there is nothing to verify
// the local HEAD against.
func TestAUR536PullRequestMetadataFailureOmitsContext(t *testing.T) {
	aur515Fixture(t, "https://github.com/owner/repo.git")

	var posted struct{ Body string }
	server := aur536MetadataFailureServer(t, &posted)
	capturePath := aur515Env(t, server.URL)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	assertContextOmittedUnverifiable(t, code, stdout.String(), stderr.String(), capturePath, posted.Body)
}

// assertDirty is the shared assertion for every scenario below that must
// come back codebaseContextReasonDirty: exit 0, the given leakMarker never
// reaching the captured prompt, and the published review declaring the
// dirty-specific wording (never the generic "unverifiable" one -- so a
// mutation collapsing the two reasons together still gets caught here).
func assertDirty(t *testing.T, code int, stdout, stderr, capturePath, posted, leakMarker string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), leakMarker) {
		t.Fatalf("marker %q must never reach the provider prompt on a dirty checkout:\n%s", leakMarker, string(captured))
	}
	if !strings.Contains(posted, "Repository context omitted") {
		t.Fatalf("published review does not declare the omission:\n%s", posted)
	}
	if !strings.Contains(posted, "uncommitted changes, untracked files, or content that does not match the reviewed commit") {
		t.Fatalf("published review does not use the dirty-specific wording:\n%s", posted)
	}
}

// runAUR536Review drives the standard fixture through runPRReview, matching
// at a verified (owner/repo, localHead) pair, and returns what was
// captured/published.
func runAUR536Review(t *testing.T, localHead string) (code int, stdout, stderr, capturePath string, posted string) {
	t.Helper()
	return runAUR536ReviewWith(t, localHead, reviewDeps{})
}

// runAUR536ReviewWith is runAUR536Review with injected session dependencies.
func runAUR536ReviewWith(t *testing.T, localHead string, deps reviewDeps) (code int, stdout, stderr, capturePath string, posted string) {
	t.Helper()
	var postedBody struct{ Body string }
	server := aur515Server(t, localHead, &postedBody)
	capturePath = aur515Env(t, server.URL)

	var out, errOut strings.Builder
	code = runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: deps}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true, check: false,
		publicationSet: true,
		publication:    "review",
	})
	return code, out.String(), errOut.String(), capturePath, postedBody.Body
}

// TestAUR536GitDirectoryFilesNeverReachPrompt covers B1: a verified, clean
// checkout can still carry files sitting under a ".git" directory -- its
// own top-level one, or one nested under an ordinary subdirectory (a
// copied/embedded nested clone) -- that are not themselves part of any
// committed tree. Neither verifiedCleanCheckoutReason's own clean-tree walk
// nor the resolver it hands the verified file set to may ever read them.
// Before AUR-536's B1 fix, internal/context's own resolver.enumerate had no
// ".git" awareness at all and would have read these files directly; this
// pins that it no longer does, and that a path OUTSIDE any ".git" sitting
// right next to the nested one (sub/ok.go) is unaffected.
func TestAUR536GitDirectoryFilesNeverReachPrompt(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")

	const topMarker = "aur536_top_dotgit_marker"
	const nestedMarker = "aur536_nested_dotgit_marker"
	reference := "package demo\n\n// references " + aur515LocalMarker + "\nvar _ = \"" + aur515LocalMarker + "\"\n"
	for _, rel := range []string{
		".git/" + topMarker + ".go",
		"sub/.git/" + nestedMarker + ".go",
	} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(reference), 0600); err != nil {
			t.Fatal(err)
		}
	}

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	for _, marker := range []string{topMarker, nestedMarker} {
		if strings.Contains(string(captured), marker) {
			t.Fatalf("a file under .git (marker %q) reached the provider prompt:\n%s", marker, string(captured))
		}
	}
	if strings.Contains(posted, "Repository context omitted") {
		t.Fatalf("files under .git must not themselves make an otherwise clean checkout dirty:\n%s", posted)
	}
}

// aur536ModifyApp overwrites the fixture's committed app.go with new,
// uncommitted content containing marker, and returns marker for the
// caller's own leak assertion.
func aur536ModifyApp(t *testing.T, dir string) (marker string) {
	t.Helper()
	marker = "AUR536LocallyModifiedMarker"
	modified := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n\nfunc " + marker + "() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), modified, 0600); err != nil {
		t.Fatal(err)
	}
	return marker
}

// TestAUR536ModifiedTrackedFileOmitsContext covers B4's "modified tracked
// file": app.go, tracked and verified at HEAD, is overwritten on disk with
// an uncommitted edit. The edit's own new symbol must never reach the
// prompt, which only a correct comparison against TrackedFiles(HEAD)'s own
// recorded blob id can catch (app.go still exists, is still readable, and
// is still named identically -- only its content changed).
func TestAUR536ModifiedTrackedFileOmitsContext(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")
	marker := aur536ModifyApp(t, dir)

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	assertDirty(t, code, stdout, stderr, capturePath, posted, marker)
}

// TestAUR536StagedButUncommittedChangeOmitsContext covers B4's "staged
// change" / B2's "dangling blob after add+reset": app.go is modified on
// disk exactly as above, AND the modified content is also written into
// .git/objects as its own loose blob -- exactly what `git add` (or a
// stash, or a reset after an add) leaves behind. A check that asks "does
// some loose object with this content's hash exist anywhere" (AUR-536's
// first, now-replaced no-git fallback) would wrongly call this clean; only
// comparing against the path's OWN blob id in TrackedFiles(HEAD) -- which
// a stray object elsewhere in the ODB never changes -- catches it.
func TestAUR536StagedButUncommittedChangeOmitsContext(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")
	marker := aur536ModifyApp(t, dir)
	modified, err := os.ReadFile(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	gitObject(t, dir, "blob", modified) // the dangling, staged-then-abandoned blob.

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	assertDirty(t, code, stdout, stderr, capturePath, posted, marker)
}

// aur536SymlinkFixture extends aur515Fixture with a tracked symlink: HEAD's
// tree records "link.go" (mode 120000) with committedTarget as its blob
// content, the same way git itself stores a symlink's target path as its
// blob. onDiskTarget is what the actual filesystem symlink points at --
// equal to committedTarget for a clean fixture, or different to simulate a
// retargeted symlink. When onDiskTarget is "", no on-disk symlink is
// created at all (an untracked-elsewhere test adds its own separately).
func aur536SymlinkFixture(t *testing.T, remoteURL, committedTarget, onDiskTarget string) (dir, headSHA string) {
	t.Helper()
	dir = t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	appLocal := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n")
	appBlob := gitObject(t, dir, "blob", appLocal)
	linkBlob := gitObject(t, dir, "blob", []byte(committedTarget))
	treeBody := append(treeEntry(t, "100644", "app.go", appBlob), treeEntry(t, "120000", "link.go", linkBlob)...)
	rootTree := gitObject(t, dir, "tree", treeBody)
	commitBody := "tree " + rootTree + "\n" +
		"author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nhead\n"
	head := gitObject(t, dir, "commit", []byte(commitBody))

	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	cfg := "[core]\n\trepositoryformatversion = 0\n\tbare = false\n"
	if remoteURL != "" {
		cfg += "[remote \"origin\"]\n\turl = " + remoteURL + "\n"
	}
	write(".git/config", []byte(cfg))
	write("app.go", appLocal)
	if onDiskTarget != "" {
		if err := os.Symlink(onDiskTarget, filepath.Join(dir, "link.go")); err != nil {
			t.Fatal(err)
		}
	}

	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir, head
}

// TestAUR536RetargetedSymlinkOmitsContext covers B5: HEAD tracks "link.go"
// as a symlink to "app.go", but the on-disk symlink actually points
// somewhere else. Before AUR-536's B5 fix, a symlink was invisible to the
// clean-tree check entirely (skipped on both the tracked-map and the
// filesystem-walk side), so a retargeted symlink passed as clean; keeping
// the tracked entry and comparing blobSHA1(readlink target) catches it.
func TestAUR536RetargetedSymlinkOmitsContext(t *testing.T) {
	_, localHead := aur536SymlinkFixture(t, "https://github.com/owner/repo.git", "app.go", "retargeted-elsewhere.go")

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	assertDirty(t, code, stdout, stderr, capturePath, posted, aur515LocalMarker)
}

// TestAUR536UntrackedSymlinkOmitsContext covers B5: an otherwise clean,
// verified checkout gains an extra symlink that HEAD's tree never tracked
// at all. It must be caught exactly like an untracked ordinary file, not
// silently skipped.
func TestAUR536UntrackedSymlinkOmitsContext(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")
	if err := os.Symlink("app.go", filepath.Join(dir, "untracked_link.go")); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	assertDirty(t, code, stdout, stderr, capturePath, posted, aur515LocalMarker)
}

// TestAUR536SubdirectoryCheckoutIsUnverifiable covers this card's own
// re-review finding: TrackedFiles' paths are relative to the repository
// root, while walkVerified computes its relative paths from whatever dir
// it is given. The two only agree when dir IS the root. Before the fix,
// running --pr with the process cwd set to an otherwise-clean checkout's
// subdirectory published a false "dirty" (uncommitted changes) claim --
// every file under the subdirectory looked unmatched against the
// root-relative tracked map, and every file outside it was never walked
// at all, so the match count could never reach len(tracked) either.
// verifiedCleanCheckoutReason now requires dir to carry its own ".git"
// entry before trusting it as the root at all; from a subdirectory it
// reports codebaseContextReasonUnverifiable instead -- never a false
// "clean" (unproven) and never a false "dirty" (actively wrong).
func TestAUR536SubdirectoryCheckoutIsUnverifiable(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("a subdirectory checkout must never have its root's content reach the prompt either:\n%s", string(captured))
	}
	if strings.Contains(posted, "uncommitted changes, untracked files, or content that does not match the reviewed commit") {
		t.Fatalf("a clean checkout run from a subdirectory must never be published as dirty:\n%s", posted)
	}
	if !strings.Contains(posted, "the local checkout's identity could not be confirmed") {
		t.Fatalf("expected the unverifiable-specific wording when run from a subdirectory:\n%s", posted)
	}
}

// TestAUR536PackedRepositoryWithoutGitIsUnverifiable is the automated
// counterpart of this card's own manual real-clone check: a repository
// whose objects have been packed (git repack -a -d, exactly what a real
// clone or a `git gc` leaves behind) is unreadable by the pure-Go
// loose-object reader TrackedFiles falls back to without a git binary.
// That must surface as codebaseContextReasonUnverifiable -- an inability
// to prove the tree is dirty is not evidence that it is -- never as a
// false "dirty" and never as a false "clean". Building the fixture uses a
// real git binary (shelling out, same as any other test fixture setup);
// PATH is then cleared so the code under test cannot find one.
func TestAUR536PackedRepositoryWithoutGitIsUnverifiable(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available to build a packed fixture")
	}
	dir := t.TempDir()
	gitHome := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(gitBin, args...)
		cmd.Dir = dir
		cmd.Env = gittest.HermeticEnv(gitHome)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	appLocal := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), appLocal, 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "app.go")
	run("commit", "-q", "-m", "initial")
	run("remote", "add", "origin", "https://github.com/owner/repo.git")
	run("repack", "-a", "-d", "-q") // packs every object, including the commit/tree themselves.

	revParse := exec.Command(gitBin, "-C", dir, "rev-parse", "HEAD")
	revParse.Env = gittest.HermeticEnv(gitHome)
	headOut, err := revParse.Output()
	if err != nil {
		t.Fatal(err)
	}
	localHead := strings.TrimSpace(string(headOut))

	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	restore := chdir(t, dir)
	t.Cleanup(restore)
	t.Setenv("PATH", "") // the fixture above already used git; the code under test must not find one.

	code, stdout, stderr, capturePath, posted := runAUR536Review(t, localHead)
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("reading captured prompt: %v", err)
	}
	if strings.Contains(string(captured), aur515LocalMarker) {
		t.Fatalf("a packed, git-less checkout must never reach the prompt as context:\n%s", string(captured))
	}
	if strings.Contains(posted, "uncommitted changes, untracked files, or content that does not match the reviewed commit") {
		t.Fatalf("a packed repository the fallback cannot read must be unverifiable, never dirty:\n%s", posted)
	}
	if !strings.Contains(posted, "the local checkout's identity could not be confirmed") {
		t.Fatalf("expected the unverifiable-specific wording for a packed, git-less checkout:\n%s", posted)
	}
}

// TestAUR536VerifiedCodebaseContextReadsExactlyTheVerifiedSet kills
// mutation Ma: it proves, end to end through a real runPRReview call,
// that resolveVerifiedCodebaseContext hands the resolver exactly the file
// set verifiedCleanCheckoutReason proved clean -- not a silently
// re-substituted unrestricted walk. It injects the session's resolveFiles
// dependency (reviewDeps) to capture the call's own arguments; TestAUR536* above prove
// the resulting PROMPT never carries disallowed content, but none of them
// pin the WIRING itself, which could regress (back to Resolve's own walk)
// without changing any of those tests' outcomes once the tree is already
// proven fully clean.
func TestAUR536VerifiedCodebaseContextReadsExactlyTheVerifiedSet(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")

	var sawDir string
	var sawFiles []string
	called := false
	deps := reviewDeps{resolveFiles: func(resolver *codebasectx.Resolver, hookDir string, changed, files []string) (*codebasectx.Pack, error) {
		called = true
		sawDir = hookDir
		sawFiles = append([]string(nil), files...)
		return resolver.ResolveWithFiles(hookDir, changed, files)
	}}

	code, stdout, stderr, _, posted := runAUR536ReviewWith(t, localHead, deps)
	if code != 0 {
		t.Fatalf("runPRReview exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !called {
		t.Fatal("the injected resolveFiles was never invoked -- --pr did not go through ResolveWithFiles at all")
	}
	if sawDir != dir {
		t.Fatalf("hook saw dir=%q, want %q", sawDir, dir)
	}
	if len(sawFiles) != 1 || sawFiles[0] != "app.go" {
		t.Fatalf("hook saw files=%v, want exactly [app.go] (the verified tracked set)", sawFiles)
	}
	if strings.Contains(posted, "Repository context omitted") {
		t.Fatalf("a verified, clean checkout must not have its context omitted:\n%s", posted)
	}
}
