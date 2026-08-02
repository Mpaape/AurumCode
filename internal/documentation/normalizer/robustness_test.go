package normalizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// countDelimiters counts front matter delimiter lines. A correctly normalized
// file has exactly two: the site generator treats any later "---" line at the
// start of a block as page content, not as configuration.
func countDelimiters(content string) int {
	n := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimRight(line, " \t\r") == "---" {
			n++
		}
	}
	return n
}

func writeAndNormalize(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := NewNormalizer(dir).NormalizeFile(path); err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(got)
}

// TestNormalizeFile_RecognizesExistingFrontMatter covers the encodings real
// authoring tools emit. A block the parser fails to recognize is not left
// alone: a second block is prepended and the first one becomes page text.
func TestNormalizeFile_RecognizesExistingFrontMatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"lf", "---\ntitle: Author Title\n---\n\n# Doc\n"},
		{"crlf", "---\r\ntitle: Author Title\r\n---\r\n\r\n# Doc\r\n"},
		{"no trailing newline", "---\ntitle: Author Title\n---"},
		{"no blank line before body", "---\ntitle: Author Title\n---\n# Doc\n"},
		{"utf8 bom", "\ufeff---\ntitle: Author Title\n---\n\n# Doc\n"},
		{"trailing spaces on delimiter", "---  \ntitle: Author Title\n---  \n\n# Doc\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeAndNormalize(t, "test.md", tt.content)

			if n := countDelimiters(got); n != 2 {
				t.Errorf("got %d front matter delimiters, want 2\n---\n%s---", n, got)
			}
			if !strings.Contains(got, "title: Author Title") {
				t.Errorf("author title was discarded\n---\n%s---", got)
			}
		})
	}
}

// TestNormalizeFile_PreservesUnknownKeys pins that normalization is additive.
// Keys this package does not model still drive the rendered site, so dropping
// them silently changes navigation and metadata.
func TestNormalizeFile_PreservesUnknownKeys(t *testing.T) {
	content := "---\ntitle: Author Title\nnav_exclude: true\ndescription: important\ntags:\n    - alpha\n---\n\n# Doc\n"

	got := writeAndNormalize(t, "test.md", content)

	for _, want := range []string{"nav_exclude: true", "description: important", "tags:", "- alpha"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost front matter entry %q\n---\n%s---", want, got)
		}
	}
}

// TestNormalizeFile_IsIdempotent runs the normalizer twice. The second run
// must be a no-op at the byte level, otherwise repeated pipeline runs keep
// rewriting (and eventually corrupting) committed documentation.
func TestNormalizeFile_IsIdempotent(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"plain body", "# Doc\n"},
		{"lf front matter", "---\ntitle: Author Title\n---\n\n# Doc\n"},
		{"crlf front matter", "---\r\ntitle: Author Title\r\n---\r\n\r\n# Doc\r\n"},
		{"no trailing newline", "---\ntitle: Author Title\n---"},
		{"unknown keys", "---\ntitle: T\nnav_exclude: true\n---\n\n# Doc\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.md")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
			n := NewNormalizer(dir)

			if err := n.NormalizeFile(path); err != nil {
				t.Fatalf("first pass: %v", err)
			}
			first, _ := os.ReadFile(path)

			if err := n.NormalizeFile(path); err != nil {
				t.Fatalf("second pass: %v", err)
			}
			second, _ := os.ReadFile(path)

			if string(first) != string(second) {
				t.Errorf("normalization is not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
			}
			if n := countDelimiters(string(first)); n != 2 {
				t.Errorf("got %d delimiters after first pass, want 2\n%s", n, first)
			}
		})
	}
}

func TestGeneratePermalink(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		section  string
		want     string
	}{
		{"root index is site root", "index.md", "", "/"},
		{"top level page", "guide.md", "", "/guide/"},
		{"nested page", "sub/page.md", "", "/sub/page/"},
		{"collection index", "_api/index.md", "_api", "/api/"},
		{"collection page", "_api/thing.md", "_api", "/api/thing/"},
		{"nested collection page", "_api/go/pkg.md", "_api", "/api/go/pkg/"},
		{"section not repeated", "api/thing.md", "_api", "/api/thing/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generatePermalink(tt.filePath, tt.section); got != tt.want {
				t.Errorf("generatePermalink(%q, %q) = %q, want %q", tt.filePath, tt.section, got, tt.want)
			}
		})
	}
}

// TestGeneratePermalink_HostileFilename pins that a permalink stays a single
// line URL path. A newline ends the YAML value and characters like # or ?
// truncate the route the site serves.
func TestGeneratePermalink_HostileFilename(t *testing.T) {
	hostile := []string{
		"x\ntitle: pwned\n.md",
		"a: b.md",
		"say \"hi\".md",
		"what?.md",
		"anchor#frag.md",
		"spaced out.md",
	}

	for _, name := range hostile {
		t.Run(name, func(t *testing.T) {
			got := generatePermalink(name, "")

			for _, bad := range []string{"\n", "\r", "#", "?", "\"", " ", ":"} {
				if strings.Contains(got, bad) {
					t.Errorf("permalink %q contains %q", got, bad)
				}
			}
			if !strings.HasPrefix(got, "/") || !strings.HasSuffix(got, "/") {
				t.Errorf("permalink %q is not a rooted directory path", got)
			}
		})
	}
}

// TestGenerateFrontMatter_HostileTitleStaysOneValue guards the YAML escaping
// that keeps a document title from injecting extra front matter keys.
func TestGenerateFrontMatter_HostileTitleStaysOneValue(t *testing.T) {
	hostile := []string{
		"x\ntitle: pwned\nlayout: evil\n.md",
		"a: b.md",
		"say \"hi\".md",
		"*ref.md",
		"- item.md",
	}

	for _, name := range hostile {
		t.Run(name, func(t *testing.T) {
			out, err := GenerateFrontMatter(FrontMatterOptions{FilePath: name}).ToYAML()
			if err != nil {
				t.Fatalf("ToYAML: %v", err)
			}
			if n := countDelimiters(out); n != 2 {
				t.Fatalf("got %d delimiters, want 2\n%s", n, out)
			}

			parsed, body, err := ParseFrontMatter(out + "# body\n")
			if err != nil {
				t.Fatalf("ParseFrontMatter: %v", err)
			}
			if parsed == nil {
				t.Fatal("generated front matter did not parse back")
			}
			if parsed.Layout != "default" {
				t.Errorf("Layout = %q, want %q (title injected a key)", parsed.Layout, "default")
			}
			if body != "# body\n" {
				t.Errorf("body = %q, want %q", body, "# body\n")
			}
		})
	}
}

// TestParseFrontMatter_Rejects pins that unreadable front matter is reported
// rather than silently replaced. NormalizeFile propagates the error and leaves
// the file untouched, which is the safe outcome for authored content.
func TestParseFrontMatter_Rejects(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"malformed yaml", "---\ntitle: [unclosed\n---\n\n# Doc\n"},
		{"not a mapping", "---\n- alpha\n- beta\n---\n\n# Doc\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseFrontMatter(tt.content); err == nil {
				t.Fatal("expected an error, got nil")
			}

			dir := t.TempDir()
			path := filepath.Join(dir, "test.md")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := NewNormalizer(dir).NormalizeFile(path); err == nil {
				t.Error("NormalizeFile accepted unreadable front matter")
			}
			got, _ := os.ReadFile(path)
			if string(got) != tt.content {
				t.Errorf("file was rewritten despite the error:\n%s", got)
			}
		})
	}
}

// TestParseFrontMatter_UnterminatedBlockIsBody pins that an opening delimiter
// with no closing one is page content, not a truncated header.
func TestParseFrontMatter_UnterminatedBlockIsBody(t *testing.T) {
	content := "---\ntitle: never closed\n\n# Doc\n"

	fm, body, err := ParseFrontMatter(content)
	if err != nil {
		t.Fatalf("ParseFrontMatter: %v", err)
	}
	if fm != nil {
		t.Errorf("got front matter %+v, want nil", fm)
	}
	if body != content {
		t.Errorf("body = %q, want the original content", body)
	}
}

func TestMergeFrontMatter_ExtraPrefersExisting(t *testing.T) {
	existing := &FrontMatter{Extra: map[string]interface{}{"nav_exclude": true, "kept": "old"}}
	generated := &FrontMatter{Extra: map[string]interface{}{"kept": "new", "added": "yes"}}

	merged := MergeFrontMatter(existing, generated)

	if merged.Extra["nav_exclude"] != true {
		t.Errorf("nav_exclude = %v, want true", merged.Extra["nav_exclude"])
	}
	if merged.Extra["kept"] != "old" {
		t.Errorf("kept = %v, want %q", merged.Extra["kept"], "old")
	}
	if merged.Extra["added"] != "yes" {
		t.Errorf("added = %v, want %q", merged.Extra["added"], "yes")
	}

	if got := MergeFrontMatter(&FrontMatter{}, &FrontMatter{}); got.Extra != nil {
		t.Errorf("Extra = %v, want nil when neither side has entries", got.Extra)
	}
}

// TestFrontMatter_ToYAML_ExtraIsOrdered pins deterministic output. Map
// iteration order is random, so unordered emission would rewrite unchanged
// files on every pipeline run.
func TestFrontMatter_ToYAML_ExtraIsOrdered(t *testing.T) {
	fm := &FrontMatter{
		Title:  "Page",
		Layout: "default",
		Extra: map[string]interface{}{
			"zeta":        1,
			"alpha":       "a",
			"nav_exclude": true,
		},
	}

	first, err := fm.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := fm.ToYAML()
		if err != nil {
			t.Fatalf("ToYAML: %v", err)
		}
		if again != first {
			t.Fatalf("output is not deterministic:\n%s\nvs\n%s", first, again)
		}
	}

	wantOrder := []string{"title:", "layout:", "alpha:", "nav_exclude:", "zeta:"}
	at := -1
	for _, key := range wantOrder {
		i := strings.Index(first, key)
		if i < 0 {
			t.Fatalf("missing %q in:\n%s", key, first)
		}
		if i < at {
			t.Errorf("%q is out of order in:\n%s", key, first)
		}
		at = i
	}

	parsed, _, err := ParseFrontMatter(first + "# body\n")
	if err != nil {
		t.Fatalf("ParseFrontMatter: %v", err)
	}
	if len(parsed.Extra) != 3 {
		t.Errorf("round trip kept %d extra entries, want 3: %v", len(parsed.Extra), parsed.Extra)
	}
}

// TestShouldSkip_MatchesPathSegments pins that the skip list matches whole
// directory names. Substring matching silently dropped documents whose name
// merely contained one of the words.
func TestShouldSkip_MatchesPathSegments(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"docs/_site/page.md", true},
		{"docs/node_modules/pkg/readme.md", true},
		{"docs/.git/x.md", true},
		{"vendor/lib/doc.md", true},
		{"docs/guides/vendor-selection.md", false},
		{"docs/_site_design.md", false},
		{"docs/node_modules_explained.md", false},
		{"docs/real.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := shouldSkip(tt.path); got != tt.want {
				t.Errorf("shouldSkip(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
