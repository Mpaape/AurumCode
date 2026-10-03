package reviewcache_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	modelA  = "model-a"
	modelB  = "model-b"
	promptV = "prompt-v1"
)

func diffFile(lines ...string) types.DiffFile {
	return types.DiffFile{
		Path: "svc/handler.go",
		Lang: "go",
		Hunks: []types.DiffHunk{{
			OldStart: 1, OldLines: 1, NewStart: 1, NewLines: len(lines), Lines: lines,
		}},
	}
}

func TestKeyIsStableForIdenticalInput(t *testing.T) {
	file := diffFile("+return db.Query(q)")
	if cache.Key(file, modelA, promptV) != cache.Key(file, modelA, promptV) {
		t.Fatal("the same file, model and prompt must always produce the same key")
	}
}

func TestKeyChangesWithEveryInputTheModelSees(t *testing.T) {
	base := diffFile("+return db.Query(q)")
	baseKey := cache.Key(base, modelA, promptV)

	changedContent := diffFile("+return db.Query(userInput)")
	renamed := base
	renamed.Path = "svc/other.go"

	cases := map[string]string{
		"file content": cache.Key(changedContent, modelA, promptV),
		"file path":    cache.Key(renamed, modelA, promptV),
		"model":        cache.Key(base, modelB, promptV),
		"prompt":       cache.Key(base, modelA, "prompt-v2"),
	}
	for name, key := range cases {
		if key == baseKey {
			t.Errorf("key ignores %s: a stale entry would be served after that input changed", name)
		}
	}
}

func TestPutThenGetReturnsTheStoredFindings(t *testing.T) {
	store, err := cache.Open(filepath.Join(t.TempDir(), "entries"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	key := cache.Key(diffFile("+x"), modelA, promptV)
	want := cache.Entry{Path: "svc/handler.go", Issues: []types.ReviewIssue{{ID: "i1", File: "svc/handler.go", Line: 1, Severity: "error", RuleID: "r1", Message: "m"}}}
	if err := store.Put(key, want); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok, err := store.Get(key)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("round trip changed the entry: got %+v want %+v", *got, want)
	}
}

func TestGetMissIsNotAnError(t *testing.T) {
	store, err := cache.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, ok, err := store.Get("absent")
	if entry != nil || ok || err != nil {
		t.Fatalf("a missing key is a plain miss, got entry=%v ok=%v err=%v", entry, ok, err)
	}
}

func TestGetRefusesCorruptEntry(t *testing.T) {
	dir := t.TempDir()
	store, err := cache.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry, ok, err := store.Get("bad")
	if entry != nil || ok || err == nil {
		t.Fatalf("a corrupt entry must be refused with an error, got entry=%v ok=%v err=%v", entry, ok, err)
	}
}

func TestOpenRefusesUnusableDirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(filepath.Join(blocker, "sub")); err == nil {
		t.Fatal("a directory that cannot be created must be reported when opening")
	}
}

func TestResolveDirHonorsEnvironmentPin(t *testing.T) {
	t.Setenv(cache.EnvDir, "/pinned/dir")
	if got := cache.ResolveDir(); got != "/pinned/dir" {
		t.Fatalf("pinned dir ignored: %q", got)
	}
}
