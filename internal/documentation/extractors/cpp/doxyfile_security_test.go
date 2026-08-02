package cpp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
)

// injectedDirective is the payload an attacker smuggles into a Doxyfile through
// a path value. Doxygen runs INPUT_FILTER as a shell command for every input
// file, so a single unescaped line break in a path is arbitrary code execution.
const injectedDirective = `INPUT_FILTER = "sh -c 'id > /dev/null'"`

// capturingRunner stands in for doxygen. It records every invocation plus the
// exact Doxyfile it was handed, so a test can inspect what the extractor asked
// doxygen to execute instead of guessing.
type capturingRunner struct {
	mu       sync.Mutex
	calls    int
	paths    []string
	contents []string
}

func (r *capturingRunner) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if len(args) > 0 {
		r.paths = append(r.paths, args[0])
		if data, err := os.ReadFile(args[0]); err == nil {
			r.contents = append(r.contents, string(data))
		}
	}
	return "", nil
}

func (r *capturingRunner) snapshot() (int, []string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := append([]string(nil), r.paths...)
	return r.calls, paths, strings.Join(r.contents, "\n")
}

// mustSourceTree builds a directory holding one compilable-looking C++ file so
// findCPPFiles reports work to do and Extract proceeds past its early exit.
func mustSourceTree(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.cpp"), []byte("int main() { return 0; }\n"), 0o644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	return dir
}

// TestExtract_RefusesLineBreakInSourceDir reproduces the directive-injection
// attack end to end: a directory whose name really does contain a newline
// followed by a doxygen directive. POSIX permits such a name, so os.Stat
// accepts it and the value used to reach the Doxyfile template verbatim.
func TestExtract_RefusesLineBreakInSourceDir(t *testing.T) {
	base := t.TempDir()
	evilSrc := mustSourceTree(t, filepath.Join(base, "src\n"+injectedDirective))
	outputDir := filepath.Join(base, "out")

	runner := &capturingRunner{}
	_, err := NewCPPExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCPP,
		SourceDir: evilSrc,
		OutputDir: outputDir,
	})

	if err == nil {
		t.Fatal("Extract accepted a source directory containing a line break; expected refusal")
	}

	calls, _, contents := runner.snapshot()
	if calls != 0 {
		t.Errorf("doxygen was invoked %d time(s); refusal must happen before any command runs", calls)
	}
	if strings.Contains(contents, "INPUT_FILTER") {
		t.Error("generated Doxyfile carries an injected INPUT_FILTER directive")
	}
	if _, statErr := os.Stat(outputDir); !os.IsNotExist(statErr) {
		t.Errorf("output directory exists; refusal must happen before any write (stat error: %v)", statErr)
	}
}

// TestExtract_RefusesLineBreakInOutputDir covers the second interpolated field.
func TestExtract_RefusesLineBreakInOutputDir(t *testing.T) {
	base := t.TempDir()
	src := mustSourceTree(t, filepath.Join(base, "src"))
	evilOut := filepath.Join(base, "out\n"+injectedDirective)

	runner := &capturingRunner{}
	_, err := NewCPPExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCPP,
		SourceDir: src,
		OutputDir: evilOut,
	})

	if err == nil {
		t.Fatal("Extract accepted an output directory containing a line break; expected refusal")
	}

	calls, _, contents := runner.snapshot()
	if calls != 0 {
		t.Errorf("doxygen was invoked %d time(s); refusal must happen before any command runs", calls)
	}
	if strings.Contains(contents, "INPUT_FILTER") {
		t.Error("generated Doxyfile carries an injected INPUT_FILTER directive")
	}
	if _, statErr := os.Stat(evilOut); !os.IsNotExist(statErr) {
		t.Errorf("attacker-named output directory was created; refusal must happen before any write (stat error: %v)", statErr)
	}
}

// TestCreateDoxyfile_RefusesHostileValuesAndWritesNothing pins the refusal at
// the unit that builds the file, for every character that can end a directive
// or break out of the quoting.
func TestCreateDoxyfile_RefusesHostileValuesAndWritesNothing(t *testing.T) {
	cases := []struct {
		name string
		frag string
	}{
		{"line feed", "\n" + injectedDirective},
		{"carriage return", "\r" + injectedDirective},
		{"crlf", "\r\n" + injectedDirective},
		{"double quote", `" ` + injectedDirective},
		{"nul byte", "\x00"},
		{"tab", "\tX"},
		{"trailing backslash", `\`},
		{"env expansion", "$(HOME)"},
	}

	for _, tc := range cases {
		t.Run(tc.name+" in source dir", func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "Doxyfile")
			err := NewCPPExtractor(&capturingRunner{}).createDoxyfile(path, "/src"+tc.frag, filepath.Join(dir, "out"))
			if err == nil {
				t.Fatal("createDoxyfile accepted a hostile source directory; expected refusal")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("Doxyfile was written despite refusal (stat error: %v)", statErr)
			}
		})
		t.Run(tc.name+" in output dir", func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "Doxyfile")
			err := NewCPPExtractor(&capturingRunner{}).createDoxyfile(path, dir, "/out"+tc.frag)
			if err == nil {
				t.Fatal("createDoxyfile accepted a hostile output directory; expected refusal")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("Doxyfile was written despite refusal (stat error: %v)", statErr)
			}
		})
	}
}

// TestCreateDoxyfile_QuotesPathValues proves the accepted values are quoted, so
// a path containing spaces stays one value instead of spilling into the next
// token position.
func TestCreateDoxyfile_QuotesPathValues(t *testing.T) {
	dir := t.TempDir()
	src := mustSourceTree(t, filepath.Join(dir, "my source"))
	out := filepath.Join(dir, "my output")
	path := filepath.Join(dir, "Doxyfile")

	if err := NewCPPExtractor(&capturingRunner{}).createDoxyfile(path, src, out); err != nil {
		t.Fatalf("createDoxyfile rejected a legitimate path: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read generated Doxyfile: %v", err)
	}
	content := string(data)
	if want := "INPUT = \"" + src + "\"\n"; !strings.Contains(content, want) {
		t.Error("INPUT value is not quoted in the generated Doxyfile")
	}
	if want := "OUTPUT_DIRECTORY = \"" + out + "\"\n"; !strings.Contains(content, want) {
		t.Error("OUTPUT_DIRECTORY value is not quoted in the generated Doxyfile")
	}
}

// TestCreateDoxyfile_RefusesToFollowSymlink reproduces the arbitrary-overwrite
// attack: a symlink planted at the Doxyfile path pointing at a victim file.
// os.WriteFile follows it, clobbers the victim, and the caller's deferred
// remove then deletes the victim outright. Both the link and the victim live
// inside the test's own temporary directory.
func TestCreateDoxyfile_RefusesToFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.conf")
	const victimContent = "important-untouched-content\n"
	if err := os.WriteFile(victim, []byte(victimContent), 0o644); err != nil {
		t.Fatalf("failed to create victim file: %v", err)
	}

	path := filepath.Join(dir, "Doxyfile")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatalf("failed to plant symlink: %v", err)
	}

	err := NewCPPExtractor(&capturingRunner{}).createDoxyfile(path, dir, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("createDoxyfile wrote through a pre-existing symlink; expected refusal")
	}

	got, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatalf("victim file is no longer readable: %v", readErr)
	}
	if string(got) != victimContent {
		t.Error("symlink was followed: victim file was overwritten")
	}
}

// TestCreateDoxyfile_RefusesDanglingSymlink covers the create-through-link
// variant, where the victim does not exist yet and O_CREAT would otherwise
// create it at the link target.
func TestCreateDoxyfile_RefusesDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "not-yet-there.conf")
	path := filepath.Join(dir, "Doxyfile")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatalf("failed to plant dangling symlink: %v", err)
	}

	if err := NewCPPExtractor(&capturingRunner{}).createDoxyfile(path, dir, filepath.Join(dir, "out")); err == nil {
		t.Fatal("createDoxyfile created a file through a dangling symlink; expected refusal")
	}
	if _, statErr := os.Lstat(victim); !os.IsNotExist(statErr) {
		t.Errorf("dangling symlink target was created (stat error: %v)", statErr)
	}
}

// TestCreateDoxyfile_RefusesExistingRegularFile proves O_EXCL is in force, so a
// second run can never silently reuse or truncate somebody else's file.
func TestCreateDoxyfile_RefusesExistingRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Doxyfile")
	const existing = "someone-elses-file\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to create pre-existing file: %v", err)
	}

	if err := NewCPPExtractor(&capturingRunner{}).createDoxyfile(path, dir, filepath.Join(dir, "out")); err == nil {
		t.Fatal("createDoxyfile overwrote a pre-existing file; expected refusal")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("pre-existing file is no longer readable: %v", readErr)
	}
	if string(got) != existing {
		t.Error("pre-existing file was overwritten")
	}
}

// TestExtract_ConcurrentRunsUseDistinctDoxyfiles proves the shared fixed-name
// temporary file is gone: N simultaneous extractions must each get their own
// private path, otherwise one run's deferred remove deletes another run's
// configuration mid-flight.
func TestExtract_ConcurrentRunsUseDistinctDoxyfiles(t *testing.T) {
	const n = 12

	base := t.TempDir()
	reqs := make([]*extractors.ExtractRequest, n)
	for i := 0; i < n; i++ {
		reqs[i] = &extractors.ExtractRequest{
			Language:  extractors.LanguageCPP,
			SourceDir: mustSourceTree(t, filepath.Join(base, "src", string(rune('a'+i)))),
			OutputDir: filepath.Join(base, "out", string(rune('a'+i))),
		}
	}

	runner := &capturingRunner{}
	extractor := NewCPPExtractor(runner)

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = extractor.Extract(context.Background(), reqs[i])
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent extraction %d failed: %v", i, err)
		}
	}

	calls, paths, _ := runner.snapshot()
	if calls != n {
		t.Fatalf("expected %d doxygen invocations, got %d", n, calls)
	}

	seen := make(map[string]int, n)
	for _, p := range paths {
		seen[p]++
	}
	if len(seen) != n {
		t.Errorf("%d concurrent runs shared %d distinct Doxyfile path(s); each run must own its own file", n, len(seen))
	}
	fixed := filepath.Join(os.TempDir(), "Doxyfile")
	if _, collided := seen[fixed]; collided {
		t.Error("extractor still uses the predictable fixed-name path in the shared temporary directory")
	}
}

// TestExtract_CleansUpItsTemporaryDirectory makes sure the private directory
// does not leak once extraction returns.
func TestExtract_CleansUpItsTemporaryDirectory(t *testing.T) {
	base := t.TempDir()
	runner := &capturingRunner{}
	_, err := NewCPPExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCPP,
		SourceDir: mustSourceTree(t, filepath.Join(base, "src")),
		OutputDir: filepath.Join(base, "out"),
	})
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	_, paths, _ := runner.snapshot()
	if len(paths) != 1 {
		t.Fatalf("expected 1 doxygen invocation, got %d", len(paths))
	}
	if _, statErr := os.Lstat(filepath.Dir(paths[0])); !os.IsNotExist(statErr) {
		t.Errorf("temporary Doxyfile directory leaked (stat error: %v)", statErr)
	}
}
