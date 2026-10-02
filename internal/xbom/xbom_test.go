package xbom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

type fakeLLM struct {
	resp string
	err  error
}

func (f fakeLLM) Complete(string, llm.Options) (llm.Response, error) {
	return llm.Response{Text: f.resp}, f.err
}
func (f fakeLLM) Tokens(s string) (int, error) { return len(s) / 4, nil }
func (f fakeLLM) Name() string                 { return "fake" }

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sampleRepo(t *testing.T) string {
	root := t.TempDir()
	write(t, root, ".github/workflows/ci.yml", "name: ci\njobs:\n  b:\n    steps:\n      - uses: actions/checkout@v4\n      - uses: ./local\n")
	write(t, root, "Dockerfile", "FROM golang:1.22 AS build\nFROM gcr.io/distroless/static@sha256:"+strings.Repeat("a", 64)+"\n")
	write(t, root, "main.go", "package main\n\nimport \"crypto/sha256\"\n\nfunc f() { rsa.GenerateKey(rand.Reader, 2048) }\n")
	write(t, root, "app.py", "import hashlib\nhashlib.md5(b'x')\ncipher = 'AES-256-GCM'\n")
	return root
}

func load(t *testing.T, typ, root, policy string) *Catalog {
	t.Helper()
	r, err := LoadCatalog(typ, root, policy)
	if err != nil {
		t.Fatal(err)
	}
	return r.Catalog
}

func gen(t *testing.T, typ, root string, p llm.Provider) (*Result, map[string]any) {
	t.Helper()
	res, err := Generate(Options{Type: typ, Root: root, Catalog: load(t, typ, root, ""), Provider: p, Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(res.JSON, &doc); err != nil {
		t.Fatal(err)
	}
	return res, doc
}

func names(doc map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, c := range doc["components"].([]any) {
		m := c.(map[string]any)
		out[m["name"].(string)] = m
	}
	return out
}

func metaProp(doc map[string]any, name string) string {
	for _, p := range doc["metadata"].(map[string]any)["properties"].([]any) {
		m := p.(map[string]any)
		if m["name"] == name {
			return m["value"].(string)
		}
	}
	return ""
}

func TestEmbeddedCatalogsAreValid(t *testing.T) {
	for _, typ := range Types() {
		if _, err := LoadCatalog(typ, t.TempDir(), ""); err != nil {
			t.Errorf("%s: %v", typ, err)
		}
	}
}

func TestInvalidCatalogRefused(t *testing.T) {
	base := "version: 1\ntype: build\nadditional: {name_pattern: '.+'}\nentries:\n  - id: a\n    files: [\"*.txt\"]\n    pattern: '(?P<name>x)'\n    token: name\n    component: {type: library, name: \"{name}\"}\n"
	if _, err := ParseCatalog([]byte(base), "build", "t"); err != nil {
		t.Fatalf("base catalog should be valid: %v", err)
	}
	cases := map[string]string{
		"bad regex":        strings.Replace(base, "(?P<name>x)", "(?P<name>x", 1),
		"token not group":  strings.Replace(base, "token: name", "token: nope", 1),
		"bad type":         strings.Replace(base, "type: library", "type: bogus", 1),
		"placeholder":      strings.Replace(base, "{name}", "{ghost}", 1),
		"wrong type":       strings.Replace(base, "type: build", "type: cbom", 1),
		"unknown key":      base + "extra: 1\n",
		"no entries":       "version: 1\ntype: build\nadditional: {name_pattern: '.+'}\nentries: []\n",
		"no name_pattern":  strings.Replace(base, "additional: {name_pattern: '.+'}\n", "", 1),
		"bad name_pattern": strings.Replace(base, "'.+'", "'('", 1),
		"unknown set":      strings.Replace(base, `"*.txt"`, `"@nope"`, 1),
		"glob escape":      strings.Replace(base, `"*.txt"`, `"../x"`, 1),
		"crypto no asset":  strings.Replace(base, "type: library", "type: cryptographic-asset", 1),
	}
	for name, y := range cases {
		if _, err := ParseCatalog([]byte(y), "build", "t"); err == nil {
			t.Errorf("%s: invalid catalog was accepted", name)
		}
	}
}

func TestBuildExtractionWithEvidence(t *testing.T) {
	root := sampleRepo(t)
	res, doc := gen(t, "build", root, nil)
	n := names(doc)
	co, ok := n["actions/checkout"]
	if !ok || co["version"] != "v4" || co["purl"] != "pkg:githubactions/actions/checkout@v4" {
		t.Fatalf("checkout component wrong: %v", co)
	}
	occ := co["evidence"].(map[string]any)["occurrences"].([]any)[0].(map[string]any)
	if occ["location"] != ".github/workflows/ci.yml" || occ["line"].(float64) != 5 {
		t.Fatalf("occurrence wrong: %v", occ)
	}
	if _, ok := n["gcr.io/distroless/static"]; !ok {
		t.Fatalf("digest-pinned FROM missing: %v", n)
	}
	if n["golang"]["version"] != "1.22" {
		t.Fatalf("tag missing: %v", n["golang"])
	}
	if metaProp(doc, "aurumcode:xbom:llm") != "absent" {
		t.Fatal("llm=absent not recorded")
	}
	if res.Dropped.DroppedComponents != 0 {
		t.Fatalf("unexpected drops: %+v", res.Dropped)
	}
}

func TestCBOMExtraction(t *testing.T) {
	root := sampleRepo(t)
	_, doc := gen(t, "cbom", root, nil)
	n := names(doc)
	for _, want := range []string{"SHA256", "MD5", "RSA-2048", "AES-256-GCM"} {
		c, ok := n[want]
		if !ok {
			t.Fatalf("%s missing; got %v", want, keys(n))
		}
		if c["type"] != "cryptographic-asset" || c["cryptoProperties"] == nil {
			t.Fatalf("%s lacks cryptoProperties", want)
		}
	}
	aes := n["AES-256-GCM"]["cryptoProperties"].(map[string]any)["algorithmProperties"].(map[string]any)
	if aes["primitive"] != "block-cipher" || aes["mode"] != "gcm" || aes["parameterSetIdentifier"] != "256" {
		t.Fatalf("aes props: %v", aes)
	}
}

func keys(m map[string]map[string]any) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	return k
}

// AC-002: a component whose cited line does not contain the token is dropped
// and counted; one with a verified occurrence stays.
func TestVerifyDropsComponentWithoutEvidence(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "alpha\nbeta\n")
	good := &Component{Type: "library", Name: "g", Occurrences: []Occurrence{{Location: "a.txt", Line: 2, Token: "beta"}}}
	wrongLine := &Component{Type: "library", Name: "w", Occurrences: []Occurrence{{Location: "a.txt", Line: 1, Token: "beta"}}}
	missingFile := &Component{Type: "library", Name: "m", Occurrences: []Occurrence{{Location: "nope.txt", Line: 1, Token: "x"}}}
	outOfRange := &Component{Type: "library", Name: "o", Occurrences: []Occurrence{{Location: "a.txt", Line: 99, Token: "alpha"}}}
	escape := &Component{Type: "library", Name: "e", Occurrences: []Occurrence{{Location: "../a.txt", Line: 1, Token: "alpha"}}}
	none := &Component{Type: "library", Name: "n"}
	kept, r := verifyEvidence(root, []*Component{good, wrongLine, missingFile, outOfRange, escape, none})
	if len(kept) != 1 || kept[0].Name != "g" {
		t.Fatalf("kept=%v", kept)
	}
	if r.DroppedComponents != 5 {
		t.Fatalf("dropped=%d", r.DroppedComponents)
	}
}

func TestLLMEnrichesButCannotInvent(t *testing.T) {
	root := sampleRepo(t)
	resp := `{"candidates":[
	  {"id":"c1","keep":true,"description":"checkout action","properties":{"pinning":"mutable-tag"}},
	  {"id":"zzz","description":"ghost id"}],
	 "additional":[
	  {"type":"library","name":"fabricated.io/x","occurrences":[{"location":"Dockerfile","line":1,"token":"does-not-appear"}]},
	  {"type":"library","name":"no.occurrence/x"},
	  {"type":"library","name":"gcr.io/distroless/static","occurrences":[{"location":"Dockerfile","line":2,"token":"anything"}]}]}`
	res, doc := gen(t, "build", root, fakeLLM{resp: resp})
	n := names(doc)
	if _, ok := n["fabricated.io/x"]; ok {
		t.Fatal("LLM invented a component without verifiable evidence")
	}
	if _, ok := n["no.occurrence/x"]; ok {
		t.Fatal("component without occurrence entered the BOM")
	}
	found := false
	for _, c := range doc["components"].([]any) {
		m := c.(map[string]any)
		if m["name"] == "gcr.io/distroless/static" && m["type"] == "library" {
			found = true
		}
	}
	if !found {
		t.Fatal("LLM-proposed component whose name is on the cited line was dropped")
	}
	if res.Dropped.DroppedComponents != 2 || metaProp(doc, "aurumcode:xbom:dropped_without_evidence") != "2" {
		t.Fatalf("dropped accounting wrong: %+v", res.Dropped)
	}
	if metaProp(doc, "aurumcode:xbom:llm") != "present" || res.LLM.Rejected != 1 {
		t.Fatalf("llm outcome: %+v", res.LLM)
	}
	found = false
	for _, c := range n {
		if c["description"] == "checkout action" {
			found = true
		}
	}
	if !found {
		t.Fatal("enrichment not applied")
	}
}

func TestLLMFailureKeepsDeterministicBOM(t *testing.T) {
	root := sampleRepo(t)
	res, doc := gen(t, "build", root, fakeLLM{resp: "not json"})
	if res.LLM.Status != "failed" || metaProp(doc, "aurumcode:xbom:llm") != "failed" {
		t.Fatalf("status=%s", res.LLM.Status)
	}
	if _, ok := names(doc)["actions/checkout"]; !ok {
		t.Fatal("deterministic evidence lost on LLM failure")
	}
}

func TestLLMKeepFalseExcludes(t *testing.T) {
	root := sampleRepo(t)
	_, doc := gen(t, "build", root, fakeLLM{resp: `{"candidates":[{"id":"c1","keep":false},{"id":"c2","keep":false},{"id":"c3","keep":false}]}`})
	if len(doc["components"].([]any)) != 0 {
		t.Fatalf("excluded candidates remained: %v", names(doc))
	}
}

func TestCatalogPrecedence(t *testing.T) {
	repo, pol := t.TempDir(), t.TempDir()
	mk := func(tag string) string {
		return "version: 1\ntype: build\nadditional: {name_pattern: '.+', reject_tokens: [" + tag + "x]}\nentries:\n  - id: " + tag + "\n    files: [\"*.txt\"]\n    pattern: '(?P<name>" + tag + ")'\n    token: name\n    component: {type: library, name: \"{name}\"}\n"
	}
	r, _ := LoadCatalog("build", repo, pol)
	if r.Catalog.Source != "embedded" {
		t.Fatalf("default should be embedded, got %s", r.Catalog.Source)
	}
	write(t, repo, ".aurumcode/xbom/build.yml", mk("fromrepo"))
	r, _ = LoadCatalog("build", repo, pol)
	if r.Catalog.Source != "repository" || r.Catalog.Entries[0].ID != "fromrepo" {
		t.Fatalf("repo override not used: %+v", r.Catalog.Source)
	}
	write(t, pol, ".aurumcode/xbom/build.yml", mk("frompolicy"))
	r, _ = LoadCatalog("build", repo, pol)
	if r.Catalog.Source != "policy" || r.Catalog.Entries[0].ID != "frompolicy" || len(r.Warnings) != 1 {
		t.Fatalf("policy must win and warn: %+v %v", r.Catalog.Source, r.Warnings)
	}
	// An invalid override is an error, never a silent fallback.
	write(t, pol, ".aurumcode/xbom/build.yml", "version: 2\n")
	if _, err := LoadCatalog("build", repo, pol); err == nil {
		t.Fatal("invalid policy catalog accepted")
	}
	// A symlinked override is refused.
	os.Remove(filepath.Join(pol, ".aurumcode/xbom/build.yml"))
	if err := os.Symlink(filepath.Join(repo, ".aurumcode/xbom/build.yml"), filepath.Join(pol, ".aurumcode/xbom/build.yml")); err == nil {
		if _, err := LoadCatalog("build", repo, pol); err == nil {
			t.Fatal("symlinked override accepted")
		}
	}
	if _, err := LoadCatalog("nope", repo, ""); err == nil {
		t.Fatal("unknown type accepted")
	}
}

func TestPromptPrecedence(t *testing.T) {
	pol := t.TempDir()
	emb, err := LoadPrompt("cbom", pol)
	if err != nil || !strings.Contains(emb, "CBOM") {
		t.Fatalf("embedded prompt: %v", err)
	}
	write(t, pol, ".aurumcode/xbom/cbom.md", "POLICY PROMPT")
	if p, _ := LoadPrompt("cbom", pol); p != "POLICY PROMPT" {
		t.Fatal("policy prompt not used")
	}
}

func TestValidateBOMRejectsEvidencelessComponent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "b.json")
	os.WriteFile(p, []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[{"type":"library","name":"x"}]}`), 0o644)
	if err := ValidateBOM(p, "1.6"); err == nil {
		t.Fatal("component without evidence validated")
	}
	os.WriteFile(p, []byte(`{"bomFormat":"CycloneDX","specVersion":"1.5","components":[]}`), 0o644)
	if err := ValidateBOM(p, "1.6"); err == nil {
		t.Fatal("old specVersion validated")
	}
}

// A relative --repo (".") must verify the same way an absolute one does.
func TestVerifyWithRelativeRoot(t *testing.T) {
	root := sampleRepo(t)
	wd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	res, _ := gen(t, "build", ".", nil)
	if res.Components == 0 || res.Dropped.DroppedComponents != 0 {
		t.Fatalf("relative root: components=%d dropped=%+v", res.Components, res.Dropped)
	}
}

func occCount(doc map[string]any, name, typ string) int {
	for _, c := range doc["components"].([]any) {
		m := c.(map[string]any)
		if m["name"] == name && m["type"] == typ {
			return len(m["evidence"].(map[string]any)["occurrences"].([]any))
		}
	}
	return -1
}

// The model's token is ignored and structural words of the covered formats
// are not component names: the name must be a qualified identifier on a
// token boundary of the cited line, outside the catalog's reject_tokens.
func TestLLMAdditionalTokenIsDerivedFromName(t *testing.T) {
	root := sampleRepo(t)
	add := func(name, typ string, line int, loc string) string {
		return `{"type":"` + typ + `","name":"` + name + `","occurrences":[{"location":"` + loc + `","line":` + strconv.Itoa(line) + `,"token":"FROM"}]}`
	}
	resp := `{"additional":[` + strings.Join([]string{
		add("evil-pkg", "library", 1, "Dockerfile"),
		add("FROM", "library", 1, "Dockerfile"),
		add("AS", "library", 1, "Dockerfile"),
		add("build", "library", 1, "Dockerfile"),
		add("RUN", "library", 1, "Dockerfile"),
		add("import", "library", 3, "main.go"),
		add("x", "library", 1, "Dockerfile"),
		add("gcr.io/distroless/static", "container", 2, "Dockerfile"),
	}, ",") + `]}`
	_, doc := gen(t, "build", root, fakeLLM{resp: resp})
	n := names(doc)
	for _, bad := range []string{"evil-pkg", "FROM", "AS", "build", "RUN", "import", "x"} {
		if c, ok := n[bad]; ok && c["type"] == "library" {
			t.Fatalf("forged component %q entered the BOM", bad)
		}
	}
	if occCount(doc, "gcr.io/distroless/static", "container") != 1 {
		t.Fatal("qualified name on the cited line was refused or duplicated")
	}
}

func TestContainsAtBoundary(t *testing.T) {
	cases := []struct {
		line, name string
		want       bool
	}{
		{"FROM x AS build", "AS", true},
		{"ASSERT(x)", "AS", false},
		{"CLASS = 1", "AS", false},
		{"x = AES-256", "AES", true},
		{"AESTHETIC", "AES", false},
		{"a gcr.io/x:1", "gcr.io/x", true},
	}
	for _, c := range cases {
		if got := containsAtBoundary(c.line, c.name); got != c.want {
			t.Errorf("%q in %q = %v", c.name, c.line, got)
		}
	}
}

// The catalog of the policy governs reject_tokens: a name the embedded
// catalog accepts is refused when the policy rejects it.
func TestPolicyRejectTokensAreRespected(t *testing.T) {
	root := sampleRepo(t)
	pol := t.TempDir()
	write(t, pol, ".aurumcode/xbom/build.yml", "version: 1\ntype: build\nadditional: {name_pattern: '.+', reject_tokens: [gcr.io/distroless/static]}\nentries:\n  - id: e\n    files: [\"Dockerfile\"]\n    pattern: '^FROM (?P<name>golang)'\n    token: name\n    component: {type: container, name: \"{name}\"}\n")
	cat := load(t, "build", root, pol)
	resp := `{"additional":[{"type":"container","name":"gcr.io/distroless/static","occurrences":[{"location":"Dockerfile","line":2}]}]}`
	res, err := Generate(Options{Type: "build", Root: root, Catalog: cat, Provider: fakeLLM{resp: resp}, Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if res.LLM.Rejected != 1 {
		t.Fatalf("policy reject_tokens not applied: %+v", res.LLM)
	}
}

// CBOM: a legitimate name on a line the catalog already covers is accepted;
// structural words and names inside other words are not.
func TestCBOMAdditionalNames(t *testing.T) {
	root := sampleRepo(t)
	write(t, root, "n.py", "x = 'AESTHETIC'\n")
	cr := `"crypto_properties":{"assetType":"algorithm"}`
	resp := `{"additional":[
	 {"type":"cryptographic-asset","name":"AES",` + cr + `,"occurrences":[{"location":"app.py","line":3}]},
	 {"type":"cryptographic-asset","name":"import",` + cr + `,"occurrences":[{"location":"app.py","line":1}]},
	 {"type":"cryptographic-asset","name":"hashlib",` + cr + `,"occurrences":[{"location":"app.py","line":1}]},
	 {"type":"cryptographic-asset","name":"AES",` + cr + `,"occurrences":[{"location":"n.py","line":1}]}]}`
	_, doc := gen(t, "cbom", root, fakeLLM{resp: resp})
	n := names(doc)
	if _, ok := n["import"]; ok {
		t.Fatal("structural word entered the CBOM")
	}
	if got := occCount(doc, "AES", "cryptographic-asset"); got != 1 {
		t.Fatalf("AES occurrences = %d, want 1 (app.py only)", got)
	}
}

// A model proposal with the same Key as a real candidate cannot add
// occurrences it chose: the false one is dropped, the real one stays.
func TestLLMAdditionalSameKeyCannotAddFalseOccurrence(t *testing.T) {
	root := sampleRepo(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	resp := `{"additional":[{"type":"container","name":"gcr.io/distroless/static","version":"` + digest + `","purl":"pkg:docker/gcr.io/distroless/static@` + digest + `",
	  "occurrences":[{"location":"Dockerfile","line":1},{"location":"main.go","line":1}]}]}`
	res, doc := gen(t, "build", root, fakeLLM{resp: resp})
	if got := occCount(doc, "gcr.io/distroless/static", "container"); got != 1 {
		t.Fatalf("occurrences = %d, want only the real one", got)
	}
	if res.Dropped.DroppedOccurrences != 2 {
		t.Fatalf("dropped occurrences = %d, want 2", res.Dropped.DroppedOccurrences)
	}
}

// The model can only change description/properties/keep of a candidate,
// never its name, version or purl.
func TestLLMCannotRewriteCandidateIdentity(t *testing.T) {
	root := sampleRepo(t)
	resp := `{"candidates":[{"id":"c1","name":"hacked","version":"9.9","purl":"pkg:evil/x@1","type":"application","description":"d"}]}`
	_, doc := gen(t, "build", root, fakeLLM{resp: resp})
	n := names(doc)
	if _, ok := n["hacked"]; ok {
		t.Fatal("model renamed a candidate")
	}
	co := n["actions/checkout"]
	if co == nil || co["version"] != "v4" || co["purl"] != "pkg:githubactions/actions/checkout@v4" || co["description"] != "d" {
		t.Fatalf("identity changed or description lost: %v", co)
	}
}
