package tools

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// gitBlob is git's blob id, computed independently of the code under test.
func gitBlob(data []byte) string {
	h := sha1.New()
	h.Write([]byte("blob " + strconv.Itoa(len(data)) + "\x00"))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// revisionFixture writes files under a fresh root and returns the root and
// the tracked map of a commit holding exactly those bytes.
func revisionFixture(t *testing.T, files map[string]string) (string, map[string]analyzer.TrackedEntry) {
	t.Helper()
	root := t.TempDir()
	tracked := map[string]analyzer.TrackedEntry{}
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		tracked[rel] = analyzer.TrackedEntry{SHA: gitBlob([]byte(content)), Mode: "100644"}
	}
	return root, tracked
}

func javaFixture(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range []string{"Contract.java", "Caller.java"} {
		data, err := os.ReadFile(filepath.Join("testdata", "aur526", name))
		if err != nil {
			t.Fatal(err)
		}
		out["src/shop/"+name] = string(data)
	}
	return out
}

func noSecret(string) bool { return false }

func openRevision(t *testing.T, root string, tracked map[string]analyzer.TrackedEntry, opts RevisionOptions) *Revision {
	t.Helper()
	opts.Root, opts.Tracked = root, tracked
	if opts.Secret == nil {
		opts.Secret = noSecret
	}
	rev, err := NewRevision(opts)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func run(t *testing.T, tool deliberation.Tool, args string) (deliberation.Result, error) {
	t.Helper()
	if err := deliberation.ValidateArguments(tool.Spec().Parameters, json.RawMessage(args)); err != nil {
		t.Fatalf("%s refused %s by schema: %v", tool.Spec().Name, args, err)
	}
	return tool.Run(context.Background(), json.RawMessage(args))
}

// AC-001: in a Java fixture the symbol tool finds the definition through
// the grammar and the caller outside the diff, with file and line.
func TestAUR526SymbolFindsTheJavaCallerOutsideTheDiff(t *testing.T) {
	root, tracked := revisionFixture(t, javaFixture(t))
	rev := openRevision(t, root, tracked, RevisionOptions{})
	res, err := run(t, NewSymbolTool(rev, grammar.Default(), nil), `{"name":"priceOf"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "definido em src/shop/Contract.java") {
		t.Fatalf("definition not attributed by the grammar:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "src/shop/Caller.java:6: return Contract.priceOf(sku);") {
		t.Fatalf("caller line missing:\n%s", res.Content)
	}
	if strings.Contains(res.Content, "Caller.java:5") {
		t.Fatalf("a comment line was reported as a use:\n%s", res.Content)
	}
	read, err := run(t, NewReadFileTool(rev, nil), `{"path":"src/shop/Caller.java","start_line":6,"end_line":6}`)
	if err != nil || !strings.Contains(read.Content, "6:         return Contract.priceOf(sku);") || read.Digest == "" {
		t.Fatalf("read_file = %+v, %v", read, err)
	}
	search, err := run(t, NewSearchTool(rev, nil), `{"query":"Contract.priceOf"}`)
	if err != nil || !strings.Contains(search.Content, "src/shop/Caller.java:6:") {
		t.Fatalf("search_text = %+v, %v", search, err)
	}
}

// AC-002: the tools read the reviewed revision only: a working-tree edit
// after the commit and a path the commit does not hold are refused.
func TestAUR526ReadsTheReviewedRevisionNeverTheWorkingTree(t *testing.T) {
	root, tracked := revisionFixture(t, javaFixture(t))
	if err := os.WriteFile(filepath.Join(root, "src/shop/Caller.java"), []byte("edited after the commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("not in the commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rev := openRevision(t, root, tracked, RevisionOptions{})
	read := NewReadFileTool(rev, nil)
	if _, err := run(t, read, `{"path":"src/shop/Caller.java"}`); err == nil || !strings.Contains(err.Error(), "difere da revisão") {
		t.Fatalf("a working-tree edit was read: %v", err)
	}
	if _, err := run(t, read, `{"path":"untracked.txt"}`); err == nil || !strings.Contains(err.Error(), "não existe na revisão") {
		t.Fatalf("an untracked file was read: %v", err)
	}
	search, err := run(t, NewSearchTool(rev, nil), `{"query":"edited after"}`)
	if err != nil || strings.Contains(search.Content, "edited after") {
		t.Fatalf("search reached the working tree: %+v %v", search, err)
	}
}

// AC-003: outside paths, symbolic links (in the tree, on disk, through a
// directory), ignored and secret files are refused; content is redacted.
func TestAUR526RefusesEscapesIgnoredAndSecretFiles(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "host.txt"), []byte("host data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := javaFixture(t)
	files["config/.env"] = "TOKEN=abc\n"
	files["vendor/lib.java"] = "class Lib {}\n"
	files["link"] = "../outside/host.txt"
	root, tracked := revisionFixture(t, files)
	tracked["link"] = analyzer.TrackedEntry{SHA: tracked["link"].SHA, Mode: gitSymlinkMode}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	tracked["escape/host.txt"] = analyzer.TrackedEntry{SHA: gitBlob([]byte("host data\n")), Mode: "100644"}
	secret := func(p string) bool { return strings.HasSuffix(p, ".env") }
	ignored := func(p string) bool { return strings.HasPrefix(p, "vendor/") }
	rev := openRevision(t, root, tracked, RevisionOptions{Secret: secret, Ignored: ignored})
	read := NewReadFileTool(rev, nil)
	for path, want := range map[string]string{
		"../outside/host.txt": "não é um caminho relativo",
		"/etc/passwd":         "não é um caminho relativo",
		"src/../../x":         "não é um caminho relativo",
		"link":                "link simbólico",
		"escape/host.txt":     "escapa do repositório",
		"config/.env":         "arquivo de segredo",
		"vendor/lib.java":     "ignorado pela política",
	} {
		args, _ := json.Marshal(map[string]string{"path": path})
		if _, err := run(t, read, string(args)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", path, err, want)
		}
	}
	if nilSecret := openRevision(t, root, tracked, RevisionOptions{}); nilSecret.opts.Secret == nil {
		t.Fatal("fixture helper must install a secret matcher")
	}
	failClosed, _ := NewRevision(RevisionOptions{Root: root, Tracked: tracked})
	if _, err := failClosed.Read("src/shop/Caller.java"); err == nil {
		t.Fatal("a revision without a secret catalog read a file")
	}
	redact := func(s string) string { return strings.ReplaceAll(s, "Contract", "[REDACTED]") }
	res, err := run(t, NewReadFileTool(rev, redact), `{"path":"src/shop/Caller.java"}`)
	if err != nil || strings.Contains(res.Content, "Contract") || !strings.Contains(res.Content, "[REDACTED]") {
		t.Fatalf("content did not pass the redaction filter: %+v %v", res, err)
	}
}

// MUT-001: a tracked regular file replaced on disk by a symlink to an
// outside file with identical bytes is refused; only the lstat guard
// catches it (the blob id matches, the directory is the root).
func TestAUR526MUT001SymlinkWithMatchingBytesIsRefused(t *testing.T) {
	outside := t.TempDir()
	body := "same bytes outside\n"
	target := filepath.Join(outside, "host.txt")
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	root, tracked := revisionFixture(t, map[string]string{"a.txt": body})
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	rev := openRevision(t, root, tracked, RevisionOptions{})
	if res, err := run(t, NewReadFileTool(rev, nil), `{"path":"a.txt"}`); err == nil || !strings.Contains(err.Error(), "link simbólico") {
		t.Fatalf("a symlink escaping the repository was followed: %+v %v", res, err)
	}
}

// AC-004: the byte ceiling refuses the result that would cross it, and the
// offers' Partial hook then reports max_read_bytes.
func TestAUR526ByteCeilingMakesTheReviewPartial(t *testing.T) {
	root, tracked := revisionFixture(t, javaFixture(t))
	budget := NewByteBudget(120)
	rev := openRevision(t, root, tracked, RevisionOptions{Budget: budget})
	diff := &types.Diff{Files: []types.DiffFile{{Path: "src/shop/Contract.java", Hunks: []types.DiffHunk{{NewStart: 5, NewLines: 1, Lines: []string{"+x"}}}}}}
	offers := RepositoryOffers(rev, diff, grammar.Default(), nil)
	partial := PartialLimit(offers)
	if partial == nil || partial() != nil {
		t.Fatal("a fresh budget must not report a limit")
	}
	if _, err := run(t, offers[0].Tool, `{"path":"src/shop/Caller.java"}`); err == nil || !strings.Contains(err.Error(), LimitMaxReadBytes) {
		t.Fatalf("a result over the ceiling was returned: %v", err)
	}
	limit := partial()
	if limit == nil || limit.Limit != LimitMaxReadBytes || !errors.Is(limit, deliberation.ErrLimit) {
		t.Fatalf("partial = %+v", limit)
	}
	if _, err := run(t, offers[3].Tool, `{"path":"src/shop/Contract.java"}`); err == nil {
		t.Fatal("an exhausted budget still returned a diff")
	}
}

func TestAUR526DiffToolAnswersOnlyChangedFiles(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{Path: "a.go", Hunks: []types.DiffHunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []string{"-old", "+new"}}}}}}
	tool := NewDiffFileTool(diff, NewByteBudget(0), nil)
	res, err := run(t, tool, `{"path":"a.go"}`)
	if err != nil || !strings.Contains(res.Content, "@@ -1,1 +1,1 @@\n-old\n+new") {
		t.Fatalf("diff = %+v %v", res, err)
	}
	if _, err := run(t, tool, `{"path":"b.go"}`); err == nil {
		t.Fatal("the diff tool answered a file outside the diff")
	}
}
