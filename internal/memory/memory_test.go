package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func sampleNotes() []Note {
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	return []Note{
		{ID: "n2", RuleID: "r1", PathPattern: "*.go", Action: "require", Author: "alice", At: at.Add(time.Minute), Body: "must have tests"},
		{ID: "n1", RuleID: "r1", PathPattern: "*.md", Action: "suppress", Author: "bob", At: at, Body: "ignore docs"},
		{ID: "n3", RuleID: "r2", PathPattern: "cmd/**", Action: "note", Author: "carol", At: at, Body: "watched area"},
	}
}

func TestNewModes(t *testing.T) {
	cases := []struct {
		mode string
	}{
		{mode: ""},
		{mode: ModeOff},
		{mode: ModeEphemeral},
		{mode: ModeLocal},
	}
	for _, tc := range cases {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			store, err := New(tc.mode, "")
			if err != nil {
				t.Fatalf("New(%q, \"\"): %v", tc.mode, err)
			}
			if store == nil {
				t.Fatalf("New(%q, \"\") returned nil store", tc.mode)
			}
		})
	}
}

func TestUnsupportedMode(t *testing.T) {
	if _, err := New("bogus", ""); err == nil {
		t.Fatal("New with unsupported mode returned nil error")
	}
}

func TestOffIsNoOp(t *testing.T) {
	for _, mode := range []string{"", ModeOff} {
		t.Run("mode="+mode, func(t *testing.T) {
			store, err := New(mode, "")
			if err != nil {
				t.Fatalf("New(%q): %v", mode, err)
			}
			got, err := store.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("Load = %d notes, want 0", len(got))
			}
			if err := store.Save(sampleNotes()); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err = store.Load()
			if err != nil {
				t.Fatalf("Load after Save: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("off mode persisted %d notes, want 0", len(got))
			}
		})
	}
}

func TestEphemeralRoundTrip(t *testing.T) {
	store, err := New(ModeEphemeral, "")
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, store, sampleNotes())
}

func TestLocalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := New(ModeLocal, dir)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, store, sampleNotes())
}

func TestLocalPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	in := sampleNotes()

	first, err := New(ModeLocal, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Save(in); err != nil {
		t.Fatal(err)
	}

	second, err := New(ModeLocal, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := second.Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := sortedCopy(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("reopen mismatch:\n got %#v\nwant %#v", got, want)
	}
}

func TestLocalLoadMissingFile(t *testing.T) {
	store, err := New(ModeLocal, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Load on missing file = %d notes, want 0", len(got))
	}
}

func TestLocalCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "aurumcode")
	store, err := New(ModeLocal, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(sampleNotes()); err != nil {
		t.Fatalf("Save into missing dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, localFileName)); err != nil {
		t.Fatalf("expected notes file under %s: %v", dir, err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	for _, mode := range []string{ModeEphemeral, ModeLocal} {
		t.Run(mode, func(t *testing.T) {
			dir := ""
			if mode == ModeLocal {
				dir = t.TempDir()
			}
			store, err := New(mode, dir)
			if err != nil {
				t.Fatal(err)
			}

			const workers = 16
			const iters = 50
			var wg sync.WaitGroup
			errs := make(chan error, workers*2)
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					for j := 0; j < iters; j++ {
						if _, err := store.Load(); err != nil {
							errs <- err
							return
						}
						n := Note{ID: fmt.Sprintf("w%d-j%d", i, j), At: time.Now(), Body: "x"}
						if err := store.Save([]Note{n}); err != nil {
							errs <- err
							return
						}
					}
				}(i)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Fatal(err)
			}

			got, err := store.Load()
			if err != nil {
				t.Fatalf("final Load: %v", err)
			}
			if len(got) == 0 {
				t.Fatal("expected at least one note after concurrent Save")
			}
		})
	}
}

func roundTrip(t *testing.T, store Store, in []Note) {
	t.Helper()
	got, err := store.Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("initial Load = %d notes, want 0", len(got))
	}
	if err := store.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if want := sortedCopy(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got %#v\nwant %#v", got, want)
	}
}
