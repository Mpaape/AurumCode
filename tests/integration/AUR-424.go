// Package integration holds AUR-424's Integration-layer proof: the Go
// extractor is registered into a real internal/documentation/extractors
// registry -- exactly the mechanism cmd/regenerate-docs/main.go's
// registerLanguageExtractors uses -- and, resolved back out of that registry,
// documents the card's own checked-in fixture at
// tests/fixtures/docs/goproject, twice, byte-for-byte identically.
//
// Not named "_test.go" for the same reason as tests/unit/AUR-424.go: see that
// file's package comment. tests/acceptance/AUR-424.sh bridges IntegrationAUR424
// into a real `go test` run inside the sandbox.
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	goextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
)

// blockingRunnerAUR424 is passed through the real registry wiring, so this
// test proves the whole chain -- register, resolve by language, extract --
// never reaches it. Structural interface satisfaction (Go's, not this
// package's) is what lets a plain struct like this stand in for whatever
// minimal runner type the extractor constructor declares.
type blockingRunnerAUR424 struct {
	calls int
}

var errBlockedAUR424 = errors.New("AUR-424/AC-001/MUT-001: external tool must never be invoked")

func (r *blockingRunnerAUR424) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	r.calls++
	return "", errBlockedAUR424
}

func readFileIntAUR424(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// digestTreeAUR424 hashes every regular file under dir by relative path and
// content, so two runs can be compared for byte-for-byte identical output
// without depending on file iteration order.
func digestTreeAUR424(t *testing.T, dir string) string {
	t.Helper()
	var names []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(readFileIntAUR424(t, filepath.Join(dir, name))))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// IntegrationAUR424 is AUR-424's Integration-layer selector.
func IntegrationAUR424(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}

	runner := &blockingRunnerAUR424{}
	ext := goextractor.NewGoExtractor(runner)

	registry := extractors.NewRegistry()
	if err := registry.Register(ext); err != nil {
		t.Fatalf("AUR-424/AC-001/behavior-missing: registry rejected the Go extractor: %v", err)
	}
	resolved, err := registry.Get(extractors.LanguageGo)
	if err != nil {
		t.Fatalf("AUR-424/AC-001/behavior-missing: registry lost the Go extractor: %v", err)
	}
	if resolved.Language() != extractors.LanguageGo {
		t.Fatalf("registry returned the wrong extractor for %s: %s", extractors.LanguageGo, resolved.Language())
	}

	fixture := filepath.Join(root, "tests", "fixtures", "docs", "goproject")
	if info, statErr := os.Stat(fixture); statErr != nil || !info.IsDir() {
		t.Fatalf("card fixture missing: %s (%v)", fixture, statErr)
	}

	// First run: the AC-001 Given/When/Then proof itself. A real, checked-in
	// Go project goes in; real pages with real symbols come out.
	outDir1 := filepath.Join(t.TempDir(), "site")
	result, err := resolved.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: fixture,
		OutputDir: outDir1,
	})
	if err != nil {
		t.Fatalf("AUR-424/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
		t.Fatalf("AUR-424/AC-001/behavior-missing: zero pages for the checked-in fixture: %+v", result.Stats)
	}

	var combined strings.Builder
	for _, f := range result.Files {
		combined.WriteString(readFileIntAUR424(t, f))
		combined.WriteString("\n")
	}
	page := combined.String()
	for _, want := range []string{
		"goproject",
		"Greeting", "Greeting represents a salutation",
		"func NewGreeting", "NewGreeting builds a Greeting",
		"func Add", "Add returns the sum of a and b",
		"func Max", "Max returns the larger of a and b",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("AUR-424/AC-001/behavior-missing: generated pages missing %q", want)
		}
	}

	if runner.calls != 0 {
		t.Fatalf("AUR-424/AC-001/MUT-001: %d external tool call(s) went through the registry wiring", runner.calls)
	}

	// Second run over the same input: AC-001's "And" clause -- repeating the
	// run on the same input must reproduce the same output, byte for byte.
	outDir2 := filepath.Join(t.TempDir(), "site")
	if _, err := resolved.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageGo,
		SourceDir: fixture,
		OutputDir: outDir2,
	}); err != nil {
		t.Fatalf("AUR-424/AC-001/behavior-missing: second Extract failed: %v", err)
	}

	digest1 := digestTreeAUR424(t, outDir1)
	digest2 := digestTreeAUR424(t, outDir2)
	if digest1 != digest2 {
		t.Fatalf("AUR-424/AC-001/behavior-missing: repeated extraction is not deterministic: %s != %s", digest1, digest2)
	}

	// Read-only: this never executes cmd/regenerate-docs/main.go (its full
	// transitive dependency graph is outside this card's paths/read_paths).
	// It only proves the source text still wires the Go extractor the way
	// this package's public constructor signature expects, so a refactor here
	// cannot silently drift from that call site.
	mainSrc := readFileIntAUR424(t, filepath.Join(root, "cmd", "regenerate-docs", "main.go"))
	if !strings.Contains(mainSrc, "goExtractor.NewGoExtractor(runner)") {
		t.Fatal("cmd/regenerate-docs/main.go no longer registers the Go extractor the way this package expects")
	}

	t.Logf("AUR-424/AC-001/pass registry=ok docs_generated=%d external_calls=%d digest=%s",
		result.Stats.DocsGenerated, runner.calls, digest1)
}
