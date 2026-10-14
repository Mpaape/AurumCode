package config

import (
	"strings"
	"testing"
)

const aur622Digest = "example.test/plugin@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func aur622Doc(plugin string) string {
	return "scheduled:\n  plugins:\n" + plugin
}

// AC-001: every malformed plugin entry is refused when the configuration is
// read, naming the offending key.
func TestAUR622ConfigRefusesMalformedPlugins(t *testing.T) {
	ok := "    - name: vvah\n      image: " + aur622Digest + "\n      env: [OPENAI_API_KEY]\n      output: {format: vvah-findings, path: security-scan/findings.json}\n"
	if _, err := Parse([]byte(aur622Doc(ok)), "config.yml"); err != nil {
		t.Fatalf("valid plugin refused: %v", err)
	}
	cases := map[string]struct{ doc, want string }{
		"no image and no dockerfile": {"    - name: a\n      output: {format: sarif, path: out.sarif}\n", "pinned by digest"},
		"image without digest":       {"    - name: a\n      image: example.test/plugin:latest\n      output: {format: sarif, path: out.sarif}\n", "not pinned by digest"},
		"image and dockerfile":       {"    - name: a\n      image: " + aur622Digest + "\n      dockerfile: Dockerfile\n      output: {format: sarif, path: out.sarif}\n", "not both"},
		"dockerfile outside":         {"    - name: a\n      dockerfile: ../Dockerfile\n      output: {format: sarif, path: out.sarif}\n", "dockerfile"},
		"env with value":             {"    - name: a\n      image: " + aur622Digest + "\n      env: [OPENAI_API_KEY=sk-value]\n      output: {format: sarif, path: out.sarif}\n", "not a variable name"},
		"env as a map":               {"    - name: a\n      image: " + aur622Digest + "\n      env: {OPENAI_API_KEY: value}\n      output: {format: sarif, path: out.sarif}\n", "cannot unmarshal !!map"},
		"absolute output":            {"    - name: a\n      image: " + aur622Digest + "\n      output: {format: sarif, path: /tmp/out.sarif}\n", "output.path"},
		"output with dotdot":         {"    - name: a\n      image: " + aur622Digest + "\n      output: {format: sarif, path: ../out.sarif}\n", "output.path"},
		"unknown format":             {"    - name: a\n      image: " + aur622Digest + "\n      output: {format: json, path: out.json}\n", "output.format"},
		"repeated name":              {"    - name: a\n      image: " + aur622Digest + "\n      output: {format: sarif, path: a.sarif}\n    - name: a\n      image: " + aur622Digest + "\n      output: {format: sarif, path: b.sarif}\n", "declared twice"},
		"invalid name":               {"    - name: 'a b'\n      image: " + aur622Digest + "\n      output: {format: sarif, path: out.sarif}\n", "valid plugin name"},
	}
	for name, c := range cases {
		_, err := Parse([]byte(aur622Doc(c.doc)), "config.yml")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, c.want)
		}
	}
}

// AC-001: under a policy entry with required: true the repository neither
// removes nor replaces it; a non-required one yields to the repository's.
func TestAUR622PolicyRequiredPluginStays(t *testing.T) {
	central, err := Parse([]byte(aur622Doc(
		"    - name: vvah\n      required: true\n      image: "+aur622Digest+"\n      output: {format: vvah-findings, path: security-scan/findings.json}\n"+
			"    - name: extra\n      image: "+aur622Digest+"\n      output: {format: sarif, path: extra.sarif}\n")), "policy")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := Parse([]byte(aur622Doc(
		"    - name: vvah\n      image: "+aur622Digest+"\n      output: {format: sarif, path: nothing.sarif}\n"+
			"    - name: extra\n      image: "+aur622Digest+"\n      output: {format: sarif, path: repo.sarif}\n")), "repo")
	if err != nil {
		t.Fatal(err)
	}
	eff, warnings := ApplyCentralPolicy(repo, central)
	got := map[string]PluginConfig{}
	for _, p := range eff.Scheduled.EffectivePlugins() {
		got[p.Name] = p
	}
	if p := got["vvah"]; p.Output.Path != "security-scan/findings.json" || !p.Required {
		t.Fatalf("required policy plugin was replaced by the repository: %+v", p)
	}
	if p := got["extra"]; p.Output.Path != "repo.sarif" {
		t.Fatalf("non-required policy plugin did not yield: %+v", p)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %+v", warnings)
	}
	// A repository without the section cannot remove the required entry.
	eff, _ = ApplyCentralPolicy(&Config{}, central)
	if len(eff.Scheduled.EffectivePlugins()) != 2 {
		t.Fatalf("policy plugins lost without a repository section: %+v", eff.Scheduled)
	}
}
