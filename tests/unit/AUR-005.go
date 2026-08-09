package unit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/evidence"
)

// TestAUR005 is the card's declared unit selector. It executes schema
// validation, sealing, canonical replay and byte-level output verification.
func TestAUR005(t *testing.T) {
	t.Helper()
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	schema, err := os.ReadFile(filepath.Join(root, ".board/schemas/evidence-bundle.schema.json"))
	if err != nil {
		t.Fatalf("schema unreadable: %v", err)
	}
	if err := evidence.ValidateSchema(schema); err != nil {
		behaviorError(t, err)
		t.Fatalf("schema/parser contract rejected: %v", err)
	}
	for _, mutation := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "artifact-count", old: `"maxItems": 64`, new: `"maxItems": 65`},
		{name: "artifact-authority", old: `"authority": { "const": "none" }`, new: `"authority": { "const": "agent" }`},
		{name: "path-byte-limit", old: `"max_path_bytes": 512`, new: `"max_path_bytes": 513`},
	} {
		mutated := bytes.Replace(schema, []byte(mutation.old), []byte(mutation.new), 1)
		if bytes.Equal(mutated, schema) {
			t.Fatalf("schema mutation %s did not alter the fixture", mutation.name)
		}
		err := evidence.ValidateSchema(mutated)
		typed, ok := err.(*evidence.Error)
		if !ok || typed.Code != evidence.CodeSchemaContract {
			t.Fatalf("schema mutation %s: got %T %v, want schema_contract", mutation.name, err, err)
		}
	}

	candidate := candidateAUR005()
	outputs := []evidence.ArtifactInput{
		{Path: "acceptance/AC-001.json", Kind: "acceptance-result", MediaType: "application/json", Data: []byte(`{"exit_code":0,"result":"pass"}`)},
		{Path: "spec/task.json", Kind: "task-spec", MediaType: "application/json", Data: []byte(`{"id":"AUR-005"}`)},
	}
	manifest, err := evidence.Seal(candidate, outputs)
	if err != nil {
		behaviorError(t, err)
		t.Fatalf("seal failed: %v", err)
	}
	encoded, err := evidence.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	parsed, err := evidence.Parse(encoded)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	replayed, err := evidence.Marshal(parsed)
	if err != nil {
		t.Fatalf("replay marshal failed: %v", err)
	}
	if !bytes.Equal(encoded, replayed) {
		t.Fatal("canonical replay changed manifest bytes")
	}
	if err := evidence.Verify(parsed, candidate, outputs); err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if len(parsed.Artifacts) != 2 || parsed.Artifacts[0].Path != "acceptance/AC-001.json" || parsed.Artifacts[1].Path != "spec/task.json" {
		t.Fatalf("artifact order is not canonical: %+v", parsed.Artifacts)
	}
}

func behaviorError(t *testing.T, err error) {
	t.Helper()
	if typed, ok := err.(*evidence.Error); ok && typed.Code == "behavior_missing" {
		t.Fatal("AUR-005/AC-001/behavior-missing")
	}
}

func candidateAUR005() evidence.CandidateIdentityV1 {
	d := func(ch byte) string { return "sha256:" + strings.Repeat(string(ch), 64) }
	return evidence.CandidateIdentityV1{
		RepositoryIdentity: d('0'), BaseTreeDigest: d('1'), HeadTreeDigest: d('2'),
		ChangeDigest: d('3'), TaskSpecDigest: d('4'), ConfigurationDigest: d('5'),
		PolicyDigest: d('6'), PromptAndRubricDigest: d('7'), SkillSetDigest: d('8'),
		ProviderModelBackendIdentityDigest: d('9'), ToolchainAndToolSetDigest: d('a'),
		DependencyLockDigest: d('b'), ContainerImageSetDigest: d('c'),
		TestManifestDigest: d('d'), RoleContextManifestDigest: d('e'),
	}
}
