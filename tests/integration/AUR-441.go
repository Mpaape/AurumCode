package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// aur441Root resolves the repository root exactly like the sibling engine
// integration programs do: AURUMCODE_ROOT wins (the acceptance harness sets
// it to the staged materialization root), and a direct run from a full
// checkout climbs two directories back to the root.
func aur441Root(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

// aur441KnownProblemFixture mirrors tests/fixtures/review/known-problem-response.json
// (AUR-430's own fixture): exactly one finding on config/demo-tokens.txt.
// A local copy lets this program plant its own, differently-worded second
// fixture (aur441OtherFixture) without depending on that file's exact text.
const aur441KnownProblemFixture = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text (DEMO_API_TOKEN).",
      "suggestion": "Remove the secret from version control and load it from the environment instead."
    }
  ],
  "summary": "The change adds config/demo-tokens.txt, which commits plaintext credential-shaped values."
}`

// aur441OtherFixture is a deliberately DIFFERENT canned response from
// aur441KnownProblemFixture, used to prove that switching
// AURUMCODE_LLM_FIXTURE against the same repository is treated as a
// different "model" for caching purposes (see cmd/aurumcode/review_cache.go's
// modelCacheKey) -- never served a stale cached answer from the other
// fixture.
const aur441OtherFixture = `{
  "issues": [],
  "summary": "PLANTED-AUR441-OTHER: nothing to report under this fixture."
}`

// IntegrationAUR441 builds the real aurumcode binary and proves AUR-441's
// outcome at the CLI boundary: a second `review --base HEAD~1` invocation
// against an unchanged repository does not resend any file to the model
// (observed through AURUMCODE_PROMPT_CAPTURE -- absent entirely on a full
// cache hit, since GenerateReview, and so FakeProvider.Complete, is never
// called), reports how many files were reused on stderr, and produces
// byte-identical stdout to the first run. It also proves the cache never
// serves a stale answer across a changed model identity (a different
// AURUMCODE_LLM_FIXTURE), and that the pre-existing "nothing changed"
// (--base HEAD) contract is unaffected.
func IntegrationAUR441(t *testing.T) {
	root := aur441Root(t)
	demoRepoSrc := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(demoRepoSrc); err != nil {
		t.Fatalf("required input missing: %s: %v", demoRepoSrc, err)
	}

	// AURUMCODE_BIN, when set, names an already-built binary to reuse
	// instead of building a fresh one: tests/acceptance/AUR-441.sh's
	// integration_case sets it to the shared binary build_shared already
	// produced, the same reuse tests/e2e/AUR-441.sh's own AURUMCODE_BIN
	// already documents for e2e_case. This card's sealed acceptance profile
	// (bootstrap-readonly-v1) builds the whole cmd/aurumcode closure
	// several times per run already (the shared build, the MUT-001 rebuild,
	// the unit and integration test compiles); skipping a fifth cold build
	// here, under a 256MB memory ceiling, is not just an optimization, it
	// keeps the run inside the profile's bounds.
	var binPath string
	if pre := os.Getenv("AURUMCODE_BIN"); pre != "" {
		binPath = pre
	} else {
		binPath = filepath.Join(t.TempDir(), "aurumcode-aur441")
		build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
		build.Dir = root
		build.Env = os.Environ()
		var buildOut bytes.Buffer
		build.Stdout = &buildOut
		build.Stderr = &buildOut
		if err := build.Run(); err != nil {
			t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
		}
	}

	// A private, writable copy of the fixture repo: this program's own
	// cache writes (default AURUMCODE_CACHE_DIR-less runs) must never touch
	// the tracked source tree.
	demoRepo := filepath.Join(t.TempDir(), "repo.git")
	if out, err := exec.Command("cp", "-R", demoRepoSrc, demoRepo).CombinedOutput(); err != nil {
		t.Fatalf("copying fixture repo: %v\n%s", err, out)
	}

	fixture := filepath.Join(t.TempDir(), "known-problem.json")
	if err := os.WriteFile(fixture, []byte(aur441KnownProblemFixture), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	otherFixture := filepath.Join(t.TempDir(), "other.json")
	if err := os.WriteFile(otherFixture, []byte(aur441OtherFixture), 0o600); err != nil {
		t.Fatalf("writing other fixture: %v", err)
	}

	type runResult struct {
		stdout, stderr  string
		code            int
		captureSections int // count of "### File:" in the capture file, or -1 if absent
	}
	run := func(dir, cacheDir, fixturePath, capturePath string, args ...string) runResult {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = dir
		env := append(os.Environ(), "AURUMCODE_LLM_FIXTURE="+fixturePath, "AURUMCODE_PROMPT_CAPTURE="+capturePath)
		if cacheDir != "" {
			env = append(env, "AURUMCODE_CACHE_DIR="+cacheDir)
		}
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("running %v: %v\nstderr=%s", args, err, stderr.String())
			}
			code = exitErr.ExitCode()
		}
		sections := -1
		if data, statErr := os.ReadFile(capturePath); statErr == nil {
			sections = strings.Count(string(data), "### File:")
		}
		return runResult{stdout: stdout.String(), stderr: stderr.String(), code: code, captureSections: sections}
	}

	// --- Core AC-001 proof: pinned AURUMCODE_CACHE_DIR, same repo, same
	// fixture, two invocations. ---
	cacheDir := filepath.Join(t.TempDir(), "cache")
	capture1 := filepath.Join(t.TempDir(), "capture1.txt")
	capture2 := filepath.Join(t.TempDir(), "capture2.txt")

	first := run(demoRepo, cacheDir, fixture, capture1, "review", "--base", "HEAD~1")
	if first.code != 0 {
		t.Fatalf("first run must exit 0, got %d, stderr=%s", first.code, first.stderr)
	}
	if !strings.Contains(first.stdout, "config/demo-tokens.txt") || !strings.Contains(first.stdout, "[error]") {
		t.Fatalf("first run must report the known finding, got:\n%s", first.stdout)
	}
	if first.captureSections != 2 {
		t.Fatalf("first run (cold cache) must send both changed files to the model, got %d captured file sections", first.captureSections)
	}
	if strings.Contains(first.stderr, "reused") {
		t.Fatalf("a cold-cache first run must not claim any reuse, got stderr=%s", first.stderr)
	}

	second := run(demoRepo, cacheDir, fixture, capture2, "review", "--base", "HEAD~1")
	if second.code != 0 {
		t.Fatalf("second run must exit 0, got %d, stderr=%s", second.code, second.stderr)
	}
	if second.stdout != first.stdout {
		t.Fatalf("stdout must be byte-identical across runs:\nfirst=%q\nsecond=%q", first.stdout, second.stdout)
	}
	if second.captureSections != -1 {
		t.Fatalf("second run (full cache hit) must never call the model at all -- expected no prompt capture file, got %d sections", second.captureSections)
	}
	if !strings.Contains(second.stderr, "reused 2 file") {
		t.Fatalf("second run must report reusing both files on stderr, got: %s", second.stderr)
	}

	// --- A different fixture on the same repository must NOT be served
	// the first fixture's cached answer: it is a different "model" for
	// caching purposes. ---
	capture3 := filepath.Join(t.TempDir(), "capture3.txt")
	third := run(demoRepo, cacheDir, otherFixture, capture3, "review", "--base", "HEAD~1")
	if third.code != 0 {
		t.Fatalf("third run (different fixture) must exit 0, got %d, stderr=%s", third.code, third.stderr)
	}
	if third.captureSections != 2 {
		t.Fatalf("a different fixture must be treated as a different model and force a fresh call, got %d captured sections", third.captureSections)
	}
	if strings.Contains(third.stdout, "config/demo-tokens.txt") {
		t.Fatalf("the different fixture's own (empty) findings must be what prints, not the first fixture's cached finding:\n%s", third.stdout)
	}
	if third.stdout == first.stdout {
		t.Fatal("the different fixture must not reproduce the first fixture's output via a stale cache hit")
	}

	// Repeating the first fixture again afterward must still hit the
	// original cache entries (unaffected by the intervening different
	// fixture run under a different key).
	capture4 := filepath.Join(t.TempDir(), "capture4.txt")
	fourth := run(demoRepo, cacheDir, fixture, capture4, "review", "--base", "HEAD~1")
	if fourth.code != 0 || fourth.stdout != first.stdout {
		t.Fatalf("re-running the original fixture must still hit its own cache and reproduce the original output, got code=%d stdout=%q", fourth.code, fourth.stdout)
	}
	if fourth.captureSections != -1 {
		t.Fatalf("re-running the original fixture must still be a full cache hit, got %d captured sections", fourth.captureSections)
	}

	// --- A PARTIAL cache hit must not duplicate a cache-hit file's
	// finding. The offline fixture provider is a fixed canned response
	// that ignores the prompt entirely (internal/review/fakeprovider.go):
	// even though the diff sent on this run carries only the miss file
	// (NOTES.txt), the fixture still answers with its usual
	// config/demo-tokens.txt finding. Without cmd/aurumcode/review_cache.go's
	// mergeCacheHits dropping any fresh issue that names a cache-hit file
	// before appending the cached ones, that finding would print twice:
	// once from the stale fresh answer, once from the cache. ---
	partialCacheDir := filepath.Join(t.TempDir(), "cache-partial")
	captureP1 := filepath.Join(t.TempDir(), "capture-p1.txt")
	p1 := run(demoRepo, partialCacheDir, fixture, captureP1, "review", "--base", "HEAD~1")
	if p1.code != 0 || p1.captureSections != 2 {
		t.Fatalf("partial-hit setup run must send both files cold, got code=%d sections=%d", p1.code, p1.captureSections)
	}
	entries, err := os.ReadDir(partialCacheDir)
	if err != nil {
		t.Fatalf("reading partial cache dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected exactly 2 cache entries after reviewing 2 files, got %d", len(entries))
	}
	var notesEntry string
	for _, e := range entries {
		data, rErr := os.ReadFile(filepath.Join(partialCacheDir, e.Name()))
		if rErr != nil {
			t.Fatalf("reading cache entry %s: %v", e.Name(), rErr)
		}
		if strings.Contains(string(data), `"NOTES.txt"`) {
			notesEntry = filepath.Join(partialCacheDir, e.Name())
		}
	}
	if notesEntry == "" {
		t.Fatal("expected one cache entry naming NOTES.txt")
	}
	if err := os.Remove(notesEntry); err != nil {
		t.Fatalf("removing the NOTES.txt cache entry: %v", err)
	}

	captureP2 := filepath.Join(t.TempDir(), "capture-p2.txt")
	p2 := run(demoRepo, partialCacheDir, fixture, captureP2, "review", "--base", "HEAD~1")
	if p2.code != 0 {
		t.Fatalf("partial-hit run must exit 0, got %d, stderr=%s", p2.code, p2.stderr)
	}
	if p2.captureSections != 1 {
		t.Fatalf("partial-hit run must send exactly the one missed file (NOTES.txt), got %d sections", p2.captureSections)
	}
	if !strings.Contains(p2.stderr, "reused 1 file") {
		t.Fatalf("partial-hit run must report reusing exactly one file, got stderr=%s", p2.stderr)
	}
	occurrences := strings.Count(p2.stdout, "config/demo-tokens.txt:4:")
	if occurrences != 1 {
		t.Fatalf("the cache-hit file's finding must appear exactly once, not duplicated by a stale fresh answer for that same file, got %d occurrences:\n%s", occurrences, p2.stdout)
	}

	// --- The default cache directory (AURUMCODE_CACHE_DIR unset) is
	// process-scoped, not repository-scoped: two separate invocations
	// against the identical repository must NOT share cache state by
	// default -- each independently sends the full diff to the model. A
	// repo-scoped default was this card's first design; it was reverted
	// because it collided with AUR-433's already-published --limite
	// contract, whose own acceptance program runs this binary several
	// times in a row against one repository and asserts every invocation
	// independently reaches the model (see
	// internal/review/cache.ResolveDir's doc for the full account). This
	// is the regression test for that reversal: without it, a future
	// change that made the default shared again would silently break
	// AUR-433 exactly as it did once already. ---
	defaultRepo := filepath.Join(t.TempDir(), "default-repo.git")
	if out, err := exec.Command("cp", "-R", demoRepoSrc, defaultRepo).CombinedOutput(); err != nil {
		t.Fatalf("copying fixture repo: %v\n%s", err, out)
	}
	capture5 := filepath.Join(t.TempDir(), "capture5.txt")
	capture6 := filepath.Join(t.TempDir(), "capture6.txt")
	d1 := run(defaultRepo, "", fixture, capture5, "review", "--base", "HEAD~1")
	if d1.code != 0 || d1.captureSections != 2 {
		t.Fatalf("default-dir first run: code=%d sections=%d stderr=%s", d1.code, d1.captureSections, d1.stderr)
	}
	d2 := run(defaultRepo, "", fixture, capture6, "review", "--base", "HEAD~1")
	if d2.code != 0 {
		t.Fatalf("default-dir second run must exit 0, got %d, stderr=%s", d2.code, d2.stderr)
	}
	if d2.captureSections != 2 {
		t.Fatalf("default-dir second run must NOT be served from a cache shared with the first (separate processes never share the default): expected both files sent again, got %d sections", d2.captureSections)
	}
	if d2.stdout != d1.stdout {
		t.Fatalf("both independent runs must still agree on the finding (same content, same fixture), got:\nfirst=%q\nsecond=%q", d1.stdout, d2.stdout)
	}
	if strings.Contains(d2.stderr, "reused") {
		t.Fatalf("an isolated default run must never claim reuse, got stderr=%s", d2.stderr)
	}

	// --- A metered (--limite) and an unmetered run against the SAME
	// repository, under a cache dir the caller explicitly shares, must
	// NOT collide: fixedModelProvider (cmd/aurumcode/cost.go) wraps the
	// provider without changing Name(), so modelCacheKey folds in the
	// resolved model key (llm.ModelResolver) precisely so these two
	// requests -- one budget-guarded, one not -- key differently and each
	// independently reaches the model. ---
	limiteRepo := filepath.Join(t.TempDir(), "limite-repo.git")
	if out, err := exec.Command("cp", "-R", demoRepoSrc, limiteRepo).CombinedOutput(); err != nil {
		t.Fatalf("copying fixture repo: %v\n%s", err, out)
	}
	limiteCacheDir := filepath.Join(t.TempDir(), "cache-limite")
	captureUnmetered := filepath.Join(t.TempDir(), "capture-unmetered.txt")
	captureMetered := filepath.Join(t.TempDir(), "capture-metered.txt")
	unmetered := run(limiteRepo, limiteCacheDir, fixture, captureUnmetered, "review", "--base", "HEAD~1")
	if unmetered.code != 0 || unmetered.captureSections != 2 {
		t.Fatalf("unmetered run: code=%d sections=%d stderr=%s", unmetered.code, unmetered.captureSections, unmetered.stderr)
	}
	metered := run(limiteRepo, limiteCacheDir, fixture, captureMetered, "review", "--base", "HEAD~1", "--limite", "0.50")
	if metered.code != 0 {
		t.Fatalf("metered run must exit 0, got %d, stderr=%s", metered.code, metered.stderr)
	}
	if metered.captureSections != 2 {
		t.Fatalf("a metered run sharing the unmetered run's cache dir must still independently reach the model (different resolved model key), got %d sections", metered.captureSections)
	}
	if !strings.Contains(metered.stderr, "actual cost $") {
		t.Fatalf("the metered run must report a real, freshly-computed cost -- it must not have been served from the unmetered run's cache entry, got stderr=%s", metered.stderr)
	}

	// --- The pre-existing "nothing changed" contract is unaffected: the
	// model is still called even though there are zero files to send. ---
	noopCacheDir := filepath.Join(t.TempDir(), "noop-cache")
	captureNoop := filepath.Join(t.TempDir(), "capture-noop.txt")
	noop := run(demoRepo, noopCacheDir, fixture, captureNoop, "review", "--base", "HEAD")
	if noop.code != 0 {
		t.Fatalf("--base HEAD (no changes) must still exit 0, got %d, stderr=%s", noop.code, noop.stderr)
	}
	if noop.captureSections < 0 {
		t.Fatal("--base HEAD must still call the model exactly like before AUR-441, even with zero files to send")
	}

	// --- --fail-on composes correctly with a cache-hit result: the gate
	// must still see the reused finding. ---
	gateCacheDir := filepath.Join(t.TempDir(), "gate-cache")
	captureGate1 := filepath.Join(t.TempDir(), "capture-gate1.txt")
	captureGate2 := filepath.Join(t.TempDir(), "capture-gate2.txt")
	g1 := run(demoRepo, gateCacheDir, fixture, captureGate1, "review", "--base", "HEAD~1", "--fail-on", "high")
	if g1.code != 3 {
		t.Fatalf("cold-cache run with --fail-on high must exit 3 (the finding is severity error), got %d", g1.code)
	}
	g2 := run(demoRepo, gateCacheDir, fixture, captureGate2, "review", "--base", "HEAD~1", "--fail-on", "high")
	if g2.code != 3 {
		t.Fatalf("a full-cache-hit run with --fail-on high must still exit 3 -- the gate must see the reused finding, got %d", g2.code)
	}
	if g2.captureSections != -1 {
		t.Fatalf("the gated second run must still be a full cache hit, got %d sections", g2.captureSections)
	}

	// --- Security: the planted canary must never reach the cache
	// directory on disk. ---
	canary := "aurum-canary-441-integration-" + strconv.Itoa(os.Getpid())
	canaryCacheDir := filepath.Join(t.TempDir(), "canary-cache")
	captureCanary := filepath.Join(t.TempDir(), "capture-canary.txt")
	cmd := exec.Command(binPath, "review", "--base", "HEAD~1")
	cmd.Dir = demoRepo
	cmd.Env = append(os.Environ(),
		"AURUMCODE_LLM_FIXTURE="+fixture,
		"AURUMCODE_PROMPT_CAPTURE="+captureCanary,
		"AURUMCODE_CACHE_DIR="+canaryCacheDir,
		"AURUM_SECRET_CANARY="+canary,
	)
	var canOut, canErr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &canOut, &canErr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("canary run: %v\nstderr=%s", err, canErr.String())
		}
	}
	if strings.Contains(canOut.String(), canary) || strings.Contains(canErr.String(), canary) {
		t.Fatal("the secret canary must never reach stdout or stderr")
	}
	walkErr := filepath.WalkDir(canaryCacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), canary) {
			t.Fatalf("the secret canary must never appear in a cache file, found in %s", path)
		}
		if strings.Contains(path, canary) {
			t.Fatalf("the secret canary must never appear in a cache file PATH, found in %s", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking cache dir: %v", walkErr)
	}
}
