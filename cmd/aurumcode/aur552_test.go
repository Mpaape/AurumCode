package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/xbom"
)

func xbomWrite(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func xbomRepo(t *testing.T) string {
	root := t.TempDir()
	xbomWrite(t, root, ".github/workflows/ci.yml", "name: ci\njobs:\n  b:\n    steps:\n      - uses: actions/checkout@v4\n      - uses: docker/build-push-action@"+strings.Repeat("c", 40)+"\n")
	xbomWrite(t, root, "Dockerfile", "FROM golang:1.22 AS build\nFROM gcr.io/distroless/static:nonroot\n")
	xbomWrite(t, root, "svc/main.go", "package svc\n\nimport \"crypto/sha256\"\n\nvar _ = sha256.New\n")
	xbomWrite(t, root, "svc/keys.py", "import hashlib\nK = rsa.generate_private_key(key_size=3072)\nc = 'AES-256-GCM'\nkem = 'ML-KEM-768'\n")
	xbomWrite(t, root, "config/tls.yml", "min_version: TLS1.2\ncert: certs/server.pem\n")
	return root
}

func xbomNoLLMEnv(t *testing.T) {
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("AURUMCODE_POLICY", "")
}

type xbomDoc struct {
	BOMFormat   string `json:"bomFormat"`
	SpecVersion string `json:"specVersion"`
	Metadata    struct {
		Properties []struct{ Name, Value string } `json:"properties"`
	} `json:"metadata"`
	Components []struct {
		Name       string         `json:"name"`
		Type       string         `json:"type"`
		Crypto     map[string]any `json:"cryptoProperties"`
		Properties []struct{ Name, Value string }
		Evidence   struct {
			Occurrences []struct {
				Location string `json:"location"`
				Line     int    `json:"line"`
			} `json:"occurrences"`
		} `json:"evidence"`
	} `json:"components"`
}

func (b xbomDoc) meta(name string) string {
	for _, p := range b.Metadata.Properties {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

func runXBOMTo(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runXBOM(args, &out, &errb)
	return code, out.String(), errb.String()
}

func xbomRead(t *testing.T, path string) xbomDoc {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d xbomDoc
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// AC-001: Build BOM and CBOM for a sample repository validate as CycloneDX
// and every component cites file and line that really contain its token.
func TestAUR552BuildAndCBOMValidateAndCiteFileAndLine(t *testing.T) {
	xbomNoLLMEnv(t)
	repo := xbomRepo(t)
	for _, typ := range []string{"build", "cbom"} {
		out := filepath.Join(t.TempDir(), typ+".json")
		code, _, stderr := runXBOMTo(t, "--type", typ, "--repo", repo, "--out", out)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", typ, code, stderr)
		}
		if err := xbom.ValidateBOM(out, "1.6"); err != nil {
			t.Fatalf("%s does not validate: %v", typ, err)
		}
		d := xbomRead(t, out)
		if d.BOMFormat != "CycloneDX" || d.SpecVersion != "1.6" || len(d.Components) == 0 {
			t.Fatalf("%s: bad document: %+v", typ, d)
		}
		if d.meta("aurumcode:xbom:llm") != "absent" {
			t.Fatalf("%s: llm=absent not recorded", typ)
		}
		for _, c := range d.Components {
			if len(c.Evidence.Occurrences) == 0 {
				t.Fatalf("%s: %s has no evidence", typ, c.Name)
			}
			for _, o := range c.Evidence.Occurrences {
				lines := strings.Split(xbomMustRead(t, filepath.Join(repo, o.Location)), "\n")
				if o.Line < 1 || o.Line > len(lines) {
					t.Fatalf("%s: %s cites %s:%d out of range", typ, c.Name, o.Location, o.Line)
				}
			}
		}
	}
	// CBOM carries cryptoProperties and the post-quantum asset.
	out := filepath.Join(t.TempDir(), "c.json")
	runXBOMTo(t, "--type", "cbom", "--repo", repo, "--out", out)
	d := xbomRead(t, out)
	have := map[string]bool{}
	for _, c := range d.Components {
		if c.Type != "cryptographic-asset" || c.Crypto["assetType"] == nil {
			t.Fatalf("%s lacks cryptoProperties", c.Name)
		}
		have[c.Name] = true
	}
	for _, want := range []string{"SHA256", "AES-256-GCM", "RSA-3072", "MLKEM768", "TLS-1.2"} {
		if !have[want] {
			t.Errorf("CBOM misses %s (have %v)", want, have)
		}
	}
}

func xbomMustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// AC-002: a component the model proposes without verifiable evidence in the
// repository does not enter the BOM; a verifiable one does.
func TestAUR552ComponentWithoutEvidenceDoesNotEnterBOM(t *testing.T) {
	xbomNoLLMEnv(t)
	repo := xbomRepo(t)
	fixture := filepath.Join(t.TempDir(), "llm.json")
	xbomWrite(t, filepath.Dir(fixture), filepath.Base(fixture), `{
	  "candidates":[{"id":"c1","description":"enriched by the model"}],
	  "additional":[
	    {"type":"library","name":"phantom-action","occurrences":[{"location":"Dockerfile","line":1,"token":"phantom-action"}]},
	    {"type":"library","name":"phantom-file","occurrences":[{"location":"nope/missing.yml","line":1,"token":"x"}]},
	    {"type":"library","name":"phantom-escape","occurrences":[{"location":"../../etc/passwd","line":1,"token":"root"}]},
	    {"type":"container","name":"distroless/static","occurrences":[{"location":"Dockerfile","line":2,"token":"FROM"}]}
	  ]}`)
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	out := filepath.Join(t.TempDir(), "b.json")
	code, _, stderr := runXBOMTo(t, "--type", "build", "--repo", repo, "--out", out)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	d := xbomRead(t, out)
	names := map[string]bool{}
	for _, c := range d.Components {
		names[c.Name] = true
	}
	for _, bad := range []string{"phantom-action", "phantom-file", "phantom-escape"} {
		if names[bad] {
			t.Fatalf("component without evidence entered the BOM: %s", bad)
		}
	}
	if !names["distroless/static"] {
		t.Fatal("a component with a verifiable occurrence was dropped")
	}
	if d.meta("aurumcode:xbom:dropped_without_evidence") != "3" {
		t.Fatalf("dropped count = %q, want 3", d.meta("aurumcode:xbom:dropped_without_evidence"))
	}
	if d.meta("aurumcode:xbom:llm") != "present" {
		t.Fatal("llm=present not recorded")
	}
	if !strings.Contains(xbomMustRead(t, out), "enriched by the model") {
		t.Fatal("enrichment lost")
	}
}

// AC-003 (command side) and the exit-code contract: documented-only types
// exit 2 naming the doc section, unknown types exit 64, and a validation
// failure leaves no file behind.
func TestAUR552DocumentedAndUnknownTypesAndNoPartialFile(t *testing.T) {
	xbomNoLLMEnv(t)
	repo := xbomRepo(t)
	for _, typ := range []string{"aibom", "saasbom", "netbom"} {
		code, _, stderr := runXBOMTo(t, "--type", typ, "--repo", repo)
		if code != 2 || !strings.Contains(stderr, "docs/configuration.md#xbom-"+typ) {
			t.Fatalf("%s: exit %d stderr %q", typ, code, stderr)
		}
	}
	for _, typ := range []string{"bogus", ""} {
		if code, _, _ := runXBOMTo(t, "--type", typ, "--repo", repo); code != 64 {
			t.Fatalf("type %q: exit %d, want 64", typ, code)
		}
	}
	// Configured minimum above what is written (1.7 > 1.6): validation fails
	// and nothing is left at the destination or next to it.
	xbomWrite(t, repo, ".aurumcode/config.yml", "quality_gates:\n  ssor_dtrack:\n    sbom_generator:\n      tool: trivy\n      format: cyclonedx\n      spec_version: \"1.7\"\n      output_file: sbom.json\n")
	dir := t.TempDir()
	out := filepath.Join(dir, "x.json")
	code, _, stderr := runXBOMTo(t, "--type", "build", "--repo", repo, "--out", out)
	if code == 0 {
		t.Fatalf("validation failure exited 0: %s", stderr)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("partial file left behind: %v", ents)
	}
	// Unwritable destination directory: non-zero, no file.
	if code, _, _ := runXBOMTo(t, "--type", "build", "--repo", xbomRepo(t), "--out", filepath.Join(dir, "missing", "x.json")); code == 0 {
		t.Fatal("write failure exited 0")
	}
}

// A central policy catalog governs the repository's own (per section).
func TestAUR552PolicyCatalogOverridesRepository(t *testing.T) {
	xbomNoLLMEnv(t)
	repo := xbomRepo(t)
	pol := t.TempDir()
	xbomWrite(t, pol, ".aurumcode/config.yml", "")
	cat := func(name string) string {
		return "version: 1\ntype: build\nentries:\n  - id: e\n    files: [\"Dockerfile\"]\n    pattern: '^FROM (?P<name>\\S+)'\n    token: name\n    component: {type: container, name: \"" + name + "-{name}\"}\n"
	}
	xbomWrite(t, repo, ".aurumcode/xbom/build.yml", cat("repo"))
	xbomWrite(t, pol, ".aurumcode/xbom/build.yml", cat("policy"))
	out := filepath.Join(t.TempDir(), "b.json")
	code, _, stderr := runXBOMTo(t, "--type", "build", "--repo", repo, "--politica", pol, "--out", out)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	d := xbomRead(t, out)
	if d.meta("aurumcode:xbom:catalog") != "policy" || !strings.HasPrefix(d.Components[0].Name, "policy-") {
		t.Fatalf("policy catalog not used: %+v", d.Components)
	}
}

// Unwritable destination directory (when not running as root): non-zero and
// no file left behind.
func TestAUR552UnwritableOutDirLeavesNothing(t *testing.T) {
	xbomNoLLMEnv(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	code, _, _ := runXBOMTo(t, "--type", "build", "--repo", xbomRepo(t), "--out", filepath.Join(dir, "x.json"))
	if code == 0 {
		t.Fatal("unwritable directory exited 0")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("partial file left: %v", ents)
	}
}
