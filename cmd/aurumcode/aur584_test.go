package main

import (
	"os"
	"path/filepath"

	"crypto/sha256"
	"encoding/hex"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

// builtinProfilePromptDigest is the sha256 of every built-in profile's
// signature and prompt prefix, in Names() order, measured on the tree before
// the built-ins moved from Go literals to embedded YAML. The same bytes must
// reach the model whichever form holds the profiles.
const builtinProfilePromptDigest = "f7c22d8d6452d8b8de3c36ccf8e9844aa19741156373570d7e02f3ff1841f2bd"

func TestAUR584BuiltinProfilesKeepThePromptDigest(t *testing.T) {
	h := sha256.New()
	names := reviewprofile.Names()
	if len(names) != 4 {
		t.Fatalf("built-in profiles = %v, want the four shipped ones", names)
	}
	for _, name := range names {
		p, ok := reviewprofile.Builtin(name)
		if !ok {
			t.Fatalf("built-in %q not found", name)
		}
		h.Write([]byte(p.Signature() + "\x00" + profileProvider{profile: p}.prefix() + "\x00"))
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != builtinProfilePromptDigest {
		t.Fatalf("built-in profile prompt digest = %s, want %s (names %s)", got, builtinProfilePromptDigest, strings.Join(names, ","))
	}
}

// TestAUR584MalformedRepositorySkillDropsOnlyItself: a SKILL.md whose front
// matter yaml.v3 refuses (an unquoted glob is an alias) is declared by name,
// and the repository's other skills still reach the prompt.
func TestAUR584MalformedRepositorySkillDropsOnlyItself(t *testing.T) {
	root := aur565Repo(t, "app.ts")
	aur565WriteSkill(t, root, "boa", "---\nname: boa\nlanguages: [ts]\n---\nMARCADOR-SKILL-BOA\n")
	aur565WriteSkill(t, root, "ruim", "---\nname: ruim\npaths: *.ts\n---\nMARCADOR-SKILL-RUIM\n")
	prompt, stdout, _ := aur565Review(t)
	if !strings.Contains(stdout, "repository skill .aurumcode/skills/ruim/SKILL.md unavailable") {
		t.Fatalf("the malformed skill is not named in the review:\n%s", stdout)
	}
	if strings.Contains(stdout, "boa/SKILL.md") || strings.Contains(stdout, "repository skills unavailable") {
		t.Fatalf("a warning reaches beyond the malformed skill:\n%s", stdout)
	}
	if !strings.Contains(prompt, "MARCADOR-SKILL-BOA") || strings.Contains(prompt, "MARCADOR-SKILL-RUIM") {
		t.Fatalf("the good skill must reach the prompt and the malformed one must not:\n%s", prompt)
	}
}

// TestAUR584MalformedPolicySkillFailsClosedNamingTheFile: the same defect in
// the central policy stops the review before any model call, naming the file.
func TestAUR584MalformedPolicySkillFailsClosedNamingTheFile(t *testing.T) {
	aur565Repo(t, "app.ts")
	policy := policyFixture(t, "")
	aur565WriteSkill(t, policy, "ruim", "---\nname: ruim\npaths: *.ts\n---\nMARCADOR\n")
	fixture := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(fixture, []byte(aur565Response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1", "--politica", policy}, &out, &errOut, redaction.NewFilter()); code != 1 {
		t.Fatalf("exit=%d, want 1; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "central policy") || !strings.Contains(errOut.String(), "skills/ruim/SKILL.md") {
		t.Fatalf("stderr does not name the malformed policy skill:\n%s", errOut.String())
	}
}
