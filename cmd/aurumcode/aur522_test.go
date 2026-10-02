package main

// AUR-522: the review and the gate work on a repository in any language. Each
// test drives the real `aurumcode review --base` command offline
// (AURUMCODE_LLM_FIXTURE) over a git repository whose changed files are in
// languages the old heuristics never knew. Nothing here names a language to the
// implementation: the corpus lives in internal/grammar/testdata/corpus and the
// grammar runtime decides what each file is.

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// corpusFiles reads every file under the shared grammar corpus.
func aur522Corpus(t *testing.T) map[string][]byte {
	t.Helper()
	root := filepath.Join("..", "..", "internal", "grammar", "testdata", "corpus")
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		data, rerr := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = data
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// aur522Repo writes a two-commit git repository (HEAD~1 holds base, HEAD holds
// head) with raw loose objects, nested directories included, and chdirs in.
func aur522Repo(t *testing.T, base, head map[string][]byte, configYAML string) string {
	t.Helper()
	dir := t.TempDir()
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
	object := func(kind string, body []byte) string {
		payload := append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...)
		sum := sha1.Sum(payload)
		id := hex.EncodeToString(sum[:])
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		_, _ = w.Write(payload)
		_ = w.Close()
		write(".git/objects/"+id[:2]+"/"+id[2:], z.Bytes())
		return id
	}
	var tree func(files map[string][]byte) string
	tree = func(files map[string][]byte) string {
		type entry struct{ sortKey, mode, name, id string }
		dirs := map[string]map[string][]byte{}
		var entries []entry
		for name, data := range files {
			if i := strings.IndexByte(name, '/'); i >= 0 {
				if dirs[name[:i]] == nil {
					dirs[name[:i]] = map[string][]byte{}
				}
				dirs[name[:i]][name[i+1:]] = data
				continue
			}
			entries = append(entries, entry{name, "100644", name, object("blob", data)})
		}
		for d, sub := range dirs {
			entries = append(entries, entry{d + "/", "40000", d, tree(sub)})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].sortKey < entries[j].sortKey })
		var body []byte
		for _, e := range entries {
			raw, _ := hex.DecodeString(e.id)
			body = append(body, []byte(e.mode+" "+e.name+"\x00")...)
			body = append(body, raw...)
		}
		return object("tree", body)
	}
	commit := func(files map[string][]byte, parent string) string {
		body := "tree " + tree(files) + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return object("commit", []byte(body))
	}
	baseID := commit(base, "")
	headID := commit(head, baseID)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(headID+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	for name, data := range head {
		write(name, data)
	}
	if configYAML != "" {
		write(".aurumcode/config.yml", []byte(configYAML))
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	t.Cleanup(chdir(t, dir))
	return dir
}

func aur522Fixture(t *testing.T, response string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(p, []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", p)
}

const aur522Clean = `{"summary":"ok","verdict":"approve","issues":[]}`

func aur522Review(t *testing.T, extra ...string) (int, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := runReview(append([]string{"--base", "HEAD~1"}, extra...), &out, &errOut, redaction.NewFilter())
	return code, out.String() + errOut.String()
}

// AC-001: every changed file in the corpus is either reviewed or declared not
// reviewed with a reason; no error for an unknown language.
func TestAUR522CorpusEveryFileReviewedOrDeclared(t *testing.T) {
	corpus := aur522Corpus(t)
	base := map[string][]byte{"README.txt": []byte("base\n")}
	aur522Repo(t, base, mergeFiles(base, corpus), "")
	aur522Fixture(t, aur522Clean)

	code, out := aur522Review(t)
	if code != 0 {
		t.Fatalf("exit=%d, want 0 for a clean review of an unfamiliar-language corpus:\n%s", code, out)
	}
	for _, bad := range []string{"unsupported language", "unknown language", "no heuristic", "panic"} {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Fatalf("a language the code never heard of must not be an error (%q):\n%s", bad, out)
		}
	}
	// Reviewed files are not listed as skipped; the generated one is declared
	// with its reason, and nothing is silently dropped.
	if !strings.Contains(out, "schema.pb.txt (generated)") {
		t.Fatalf("the generated corpus file must be declared not reviewed with its reason:\n%s", out)
	}
	for name := range corpus {
		if name == "schema.pb.txt" {
			continue
		}
		if strings.Contains(out, name+" (") {
			t.Fatalf("%s is reviewable and must not be declared skipped:\n%s", name, out)
		}
	}
	if strings.Contains(out, "**Verdict:** Approve") {
		t.Fatalf("a review that left a generated file unreviewed must not approve:\n%s", out)
	}
	// The one file without a grammar is declared as such, not hidden.
	if !strings.Contains(out, "have no grammar in the runtime") || !strings.Contains(out, "  - notes.zzqx") {
		t.Fatalf("a file with no grammar must be named in the declaration:\n%s", out)
	}
}

func mergeFiles(a, b map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// AC-002: a policy finding in a language with no context heuristic blocks the
// check exactly like one in Go (exitFindings).
func TestAUR522PolicyFindingInUnheuristicLanguageBlocks(t *testing.T) {
	corpus := aur522Corpus(t)
	base := map[string][]byte{"README.txt": []byte("base\n")}
	dir := aur522Repo(t, base, mergeFiles(base, map[string][]byte{"main.tf": corpus["main.tf"]}), "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n")
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte("## No Public Buckets\n\nNever make a bucket public.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	aur522Fixture(t, `{"summary":"ok","verdict":"approve","issues":[{"file":"main.tf","line":3,"severity":"error","rule_id":"security#no-public-buckets","message":"Bucket is public","evidence":"acl = \"public-read\"","impact":"Data exposure","verification":"Use a private ACL"}]}`)

	code, out := aur522Review(t)
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d), the same as for a Go file:\n%s", code, exitFindings, out)
	}
	if !strings.Contains(out, "security#no-public-buckets") {
		t.Fatalf("the finding's rule must be named:\n%s", out)
	}
}

// AC-003: the notice declares that the extra context is absent; and the gate's
// inconclusive setting applies only when the policy asks -- a language gap
// alone never turns a complete review inconclusive.
func TestAUR522MissingContextDeclaredAndNotInconclusiveByItself(t *testing.T) {
	base := map[string][]byte{"README.txt": []byte("base\n")}
	aur522Repo(t, base, mergeFiles(base, map[string][]byte{"notes.zzqx": []byte("plain text, no grammar\n")}), "gate:\n  fail_on: [high]\n  inconclusive: block\n")
	aur522Fixture(t, aur522Clean)

	code, out := aur522Review(t)
	if code != 0 {
		t.Fatalf("exit=%d, want 0: absent structural context is declared, not blocking:\n%s", code, out)
	}
	if !strings.Contains(out, "have no grammar in the runtime") || !strings.Contains(out, "  - notes.zzqx") {
		t.Fatalf("the absence of structural context must be declared by name:\n%s", out)
	}
	if strings.Contains(out, "review inconclusive") {
		t.Fatalf("a language gap alone must not make the review inconclusive:\n%s", out)
	}
}

// AC-004: a binary and a generated file are declared out of reach with their
// reason and never count as approved; the policy's inconclusive setting then
// applies because those files were not reviewed.
func TestAUR522BinaryAndGeneratedAreNeverApproved(t *testing.T) {
	corpus := aur522Corpus(t)
	base := map[string][]byte{"README.txt": []byte("base\n")}
	head := mergeFiles(base, map[string][]byte{
		"tool.bin":      {0x7f, 'E', 'L', 'F', 0, 1, 2, 3, 0, 0, 9},
		"schema.pb.txt": corpus["schema.pb.txt"],
	})
	aur522Repo(t, base, head, "gate:\n  fail_on: [high]\n  inconclusive: block\n")
	aur522Fixture(t, aur522Clean)

	code, out := aur522Review(t)
	if code == 0 {
		t.Fatalf("exit=0: files that were not reviewed must not pass a blocking gate:\n%s", out)
	}
	if !strings.Contains(out, "tool.bin") || !strings.Contains(out, "schema.pb.txt") {
		t.Fatalf("both skipped files must be named:\n%s", out)
	}
	if !strings.Contains(out, "(binary)") || !strings.Contains(out, "(generated)") {
		t.Fatalf("each skipped file must carry its reason:\n%s", out)
	}
	if strings.Contains(out, "**Verdict:** Approve") {
		t.Fatalf("a review that skipped files must not read as approved:\n%s", out)
	}
}
