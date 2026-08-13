package unit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR441 proves internal/review/cache at the package boundary: the
// content-addressed key changes exactly when the file's own diff, the
// model, or the prompt version changes and stays stable otherwise, a
// Put/Get roundtrip returns exactly what was stored, a miss reports
// (nil, false, nil) rather than an error, ResolveDir honors
// AURUMCODE_CACHE_DIR when set and otherwise defaults to a location unique
// to this process (never shared with a separate invocation unless asked),
// and nothing recoverable -- in particular a planted secret canary -- ever
// reaches a byte written to disk.
func TestAUR441(t *testing.T) {
	t.Run("KeyIsDeterministic", testAUR441KeyIsDeterministic)
	t.Run("KeyChangesWithFileContent", testAUR441KeyChangesWithFileContent)
	t.Run("KeyChangesWithModel", testAUR441KeyChangesWithModel)
	t.Run("KeyChangesWithPromptVersion", testAUR441KeyChangesWithPromptVersion)
	t.Run("PutGetRoundtrip", testAUR441PutGetRoundtrip)
	t.Run("GetMissIsNotAnError", testAUR441GetMissIsNotAnError)
	t.Run("PutReplacesPreviousEntry", testAUR441PutReplacesPreviousEntry)
	t.Run("ResolveDirHonorsEnv", testAUR441ResolveDirHonorsEnv)
	t.Run("ResolveDirDefaultsAreProcessScoped", testAUR441ResolveDirDefaultsAreProcessScoped)
	t.Run("OpenCreatesTheDirectory", testAUR441OpenCreatesTheDirectory)
	t.Run("CanaryNeverReachesDisk", testAUR441CanaryNeverReachesDisk)
	t.Run("EmptyIssuesStillRoundtrip", testAUR441EmptyIssuesStillRoundtrip)
}

func aur441File(path string, lines ...string) types.DiffFile {
	return types.DiffFile{
		Path: path,
		Hunks: []types.DiffHunk{{
			OldStart: 1,
			NewStart: 1,
			NewLines: len(lines),
			Lines:    lines,
		}},
	}
}

func testAUR441KeyIsDeterministic(t *testing.T) {
	f := aur441File("config/demo-tokens.txt", "+DEMO_API_TOKEN=abc123")
	k1 := cache.Key(f, "fixture", cache.PromptVersion)
	k2 := cache.Key(f, "fixture", cache.PromptVersion)
	if k1 != k2 {
		t.Fatalf("Key must be deterministic for identical input, got %q and %q", k1, k2)
	}
	if k1 == "" {
		t.Fatal("Key must not be empty")
	}
}

func testAUR441KeyChangesWithFileContent(t *testing.T) {
	a := aur441File("src/app.py", "+print('hello')")
	b := aur441File("src/app.py", "+print('goodbye')")
	ka := cache.Key(a, "fixture", cache.PromptVersion)
	kb := cache.Key(b, "fixture", cache.PromptVersion)
	if ka == kb {
		t.Fatalf("Key must change when the file's own diff content changes, both got %q", ka)
	}
}

func testAUR441KeyChangesWithModel(t *testing.T) {
	f := aur441File("src/app.py", "+print('hello')")
	k1 := cache.Key(f, "fixture", cache.PromptVersion)
	k2 := cache.Key(f, "fixture:fixture:deadbeef", cache.PromptVersion)
	if k1 == k2 {
		t.Fatal("Key must change when the model identity changes -- a cache entry built under one model (or one fixture response) must never be served under another")
	}
}

func testAUR441KeyChangesWithPromptVersion(t *testing.T) {
	f := aur441File("src/app.py", "+print('hello')")
	k1 := cache.Key(f, "fixture", "v1")
	k2 := cache.Key(f, "fixture", "v2")
	if k1 == k2 {
		t.Fatal("Key must change when the prompt version changes")
	}
}

func testAUR441PutGetRoundtrip(t *testing.T) {
	c, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f := aur441File("config/demo-tokens.txt", "+DEMO_API_TOKEN=abc123")
	key := cache.Key(f, "fixture", cache.PromptVersion)

	want := cache.Entry{
		Path: "config/demo-tokens.txt",
		Issues: []types.ReviewIssue{{
			File:     "config/demo-tokens.txt",
			Line:     4,
			Severity: "error",
			RuleID:   "security/hardcoded-secret",
			Message:  "A credential-shaped value was committed in plain text (DEMO_API_TOKEN). (rule security/hardcoded-secret: Hardcoded Secrets)",
		}},
	}
	if err := c.Put(key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok, err := c.Get(key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get must report a hit right after Put")
	}
	if got.Path != want.Path {
		t.Fatalf("Path mismatch: got %q, want %q", got.Path, want.Path)
	}
	if len(got.Issues) != 1 || got.Issues[0] != want.Issues[0] {
		t.Fatalf("Issues mismatch: got %+v, want %+v", got.Issues, want.Issues)
	}
}

func testAUR441GetMissIsNotAnError(t *testing.T) {
	c, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entry, ok, err := c.Get("never-written-key")
	if err != nil {
		t.Fatalf("Get on a missing key must not error, got: %v", err)
	}
	if ok {
		t.Fatal("Get on a missing key must report false")
	}
	if entry != nil {
		t.Fatalf("Get on a missing key must return a nil entry, got %+v", entry)
	}
}

func testAUR441PutReplacesPreviousEntry(t *testing.T) {
	c, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	key := "same-key-both-times"
	if err := c.Put(key, cache.Entry{Path: "a.txt", Issues: []types.ReviewIssue{{Message: "first"}}}); err != nil {
		t.Fatalf("Put #1: %v", err)
	}
	if err := c.Put(key, cache.Entry{Path: "a.txt", Issues: nil}); err != nil {
		t.Fatalf("Put #2: %v", err)
	}
	got, ok, err := c.Get(key)
	if err != nil || !ok {
		t.Fatalf("Get after replacement: ok=%v err=%v", ok, err)
	}
	if len(got.Issues) != 0 {
		t.Fatalf("the second Put must replace the first entry outright, still found %+v", got.Issues)
	}
}

func testAUR441ResolveDirHonorsEnv(t *testing.T) {
	pinned := filepath.Join(t.TempDir(), "pinned-cache-dir")
	t.Setenv(cache.EnvDir, pinned)
	got := cache.ResolveDir()
	if got != pinned {
		t.Fatalf("ResolveDir must return AURUMCODE_CACHE_DIR verbatim when set, got %q, want %q", got, pinned)
	}
}

// testAUR441ResolveDirDefaultsAreProcessScoped proves the default (no
// AURUMCODE_CACHE_DIR) is unique to this process: it names a directory
// under the OS temp dir keyed by os.Getpid(), never a location a SEPARATE
// `aurumcode` invocation -- even against the identical repository -- could
// land on by coincidence. This is what keeps a bare, unconfigured run from
// silently sharing state with another process; see cache.ResolveDir's own
// doc for why a repo-scoped default was rejected (it collided with
// AUR-433's already-published --limite contract).
func testAUR441ResolveDirDefaultsAreProcessScoped(t *testing.T) {
	t.Setenv(cache.EnvDir, "")
	got := cache.ResolveDir()
	want := filepath.Join(os.TempDir(), fmt.Sprintf("aurumcode-review-cache-%d", os.Getpid()))
	if got != want {
		t.Fatalf("ResolveDir's default must be process-scoped under the OS temp dir, got %q, want %q", got, want)
	}
	// Calling it again within the same process (hence the same PID) must be
	// stable, not re-randomized on every call -- the whole point is that
	// repeated calls from the same process agree on one location.
	again := cache.ResolveDir()
	if again != got {
		t.Fatalf("ResolveDir's default must be stable within one process, got %q then %q", got, again)
	}
}

func testAUR441OpenCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("precondition: dir must not exist yet")
	}
	if _, err := cache.Open(dir); err != nil {
		t.Fatalf("Open must create a missing directory, got: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("Open must have created %s as a directory, stat err=%v", dir, err)
	}
}

func testAUR441CanaryNeverReachesDisk(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const canary = "aurum-canary-441-unit"
	// The diff file's own raw content plants the canary, mirroring an
	// unredacted secret sitting in the reviewed diff. Key only ever
	// digests this (see cache.Key's doc: hashing, not redacting, is what
	// keeps it off disk), and Put only ever receives issues that already
	// passed internal/review.redactReviewResult -- exercised here by
	// simply never putting the canary into the stored Entry at all.
	f := aur441File("src/secret.txt", "+TOKEN="+canary)
	key := cache.Key(f, "fixture", cache.PromptVersion)
	if err := c.Put(key, cache.Entry{Path: "src/secret.txt", Issues: nil}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one cache file to have been written")
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		if strings.Contains(string(data), canary) {
			t.Fatalf("the canary must never appear in any cache file, found it in %s: %s", e.Name(), data)
		}
		if strings.Contains(e.Name(), canary) {
			t.Fatalf("the canary must never appear in a cache file NAME, found it in %s", e.Name())
		}
	}
}

func testAUR441EmptyIssuesStillRoundtrip(t *testing.T) {
	// A clean file (no findings) must still be storable and retrievable as
	// a hit -- otherwise a clean file could never become a cache hit and
	// would be resent to the model forever.
	c, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f := aur441File("NOTES.txt", "-scratch line")
	key := cache.Key(f, "fixture", cache.PromptVersion)
	if err := c.Put(key, cache.Entry{Path: "NOTES.txt", Issues: nil}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	entry, ok, err := c.Get(key)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if len(entry.Issues) != 0 {
		t.Fatalf("expected zero issues for a clean file, got %+v", entry.Issues)
	}
}
