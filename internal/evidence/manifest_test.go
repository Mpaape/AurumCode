package evidence_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/evidence"
	integration "github.com/Mpaape/AurumCode/tests/integration"
	unit "github.com/Mpaape/AurumCode/tests/unit"
)

// TestAUR005 is the executable bridge for the exact selector declared at
// tests/unit/AUR-005.go. It cannot pass unless that function returns after its
// production schema, sealing, canonical replay and verification assertions.
func TestAUR005(t *testing.T) {
	unit.TestAUR005(t)
	t.Log("assertion executed: tests/unit/AUR-005.go::TestAUR005")
}

// TestIntegrationAUR005 bridges Go's Test prefix to the exact card selector
// IntegrationAUR005 and executes every sealed vector through production code.
func TestIntegrationAUR005(t *testing.T) {
	integration.IntegrationAUR005(t)
	t.Log("assertion executed: tests/integration/AUR-005.go::IntegrationAUR005")
}

func TestManifestCanonicalReplayAndTamperRejection(t *testing.T) {
	candidate := packageCandidate()
	outputs := []evidence.ArtifactInput{
		{Path: "z/result.txt", Kind: "integration-result", MediaType: "text/plain", Data: []byte("z")},
		{Path: "a/result.json", Kind: "unit-result", MediaType: "application/json", Data: []byte(`{"ok":true}`)},
	}
	manifest, err := evidence.Seal(candidate, outputs)
	if err != nil {
		if typed, ok := err.(*evidence.Error); ok && typed.Code == "behavior_missing" {
			t.Fatal("AUR-005/AC-001/behavior-missing")
		}
		t.Fatal(err)
	}
	encoded, err := evidence.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := evidence.Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := evidence.Marshal(parsed)
	if err != nil || !bytes.Equal(encoded, replay) {
		t.Fatalf("canonical replay changed bytes: err=%v", err)
	}
	if got := parsed.Artifacts[0].Path; got != "a/result.json" {
		t.Fatalf("artifacts not sorted: first=%s", got)
	}
	if err := evidence.Verify(parsed, candidate, outputs); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}

	t.Run("output", func(t *testing.T) {
		mutated := append([]evidence.ArtifactInput(nil), outputs...)
		mutated[0].Data = []byte("changed")
		assertCode(t, evidence.Verify(parsed, candidate, mutated), evidence.CodeDigestMismatch)
	})
	t.Run("candidate", func(t *testing.T) {
		mutated := candidate
		mutated.TaskSpecDigest = digestOfByte('f')
		assertCode(t, evidence.Verify(parsed, mutated, outputs), evidence.CodeCandidateMismatch)
	})
	t.Run("authority", func(t *testing.T) {
		mutated := bytes.Replace(encoded, []byte(`"authority":"none"`), []byte(`"authority":"agent"`), 1)
		assertCode(t, parseError(mutated), evidence.CodeAuthorityDenied)
	})
	t.Run("noncanonical", func(t *testing.T) {
		assertCode(t, parseError(append(encoded, '\n')), evidence.CodeNonCanonical)
	})
	t.Run("duplicate", func(t *testing.T) {
		mutated := bytes.Replace(encoded, []byte(`{"schema":`), []byte(`{"schema":"aurum.evidence-bundle","schema":`), 1)
		assertCode(t, parseError(mutated), evidence.CodeDuplicateField)
	})
	t.Run("unknown", func(t *testing.T) {
		mutated := bytes.Replace(encoded, []byte(`,"manifest_digest":`), []byte(`,"forged":true,"manifest_digest":`), 1)
		assertCode(t, parseError(mutated), evidence.CodeUnknownField)
	})
	t.Run("missing-output", func(t *testing.T) {
		assertCode(t, evidence.Verify(parsed, candidate, outputs[:1]), evidence.CodeArtifactMissing)
	})
	t.Run("unexpected-output", func(t *testing.T) {
		extra := append([]evidence.ArtifactInput(nil), outputs...)
		extra = append(extra, evidence.ArtifactInput{Path: "extra/result.json", Kind: "unit-result", MediaType: "application/json", Data: []byte(`{}`)})
		assertCode(t, evidence.Verify(parsed, candidate, extra), evidence.CodeArtifactUnexpected)
	})
	t.Run("unsafe-path", func(t *testing.T) {
		unsafe := []evidence.ArtifactInput{{Path: "../result.json", Kind: "unit-result", MediaType: "application/json", Data: []byte(`{}`)}}
		_, err := evidence.Seal(candidate, unsafe)
		assertCode(t, err, evidence.CodeUnsafePath)
	})
	t.Run("duplicate-path", func(t *testing.T) {
		duplicate := append([]evidence.ArtifactInput(nil), outputs...)
		duplicate[1].Path = duplicate[0].Path
		_, err := evidence.Seal(candidate, duplicate)
		assertCode(t, err, evidence.CodeDuplicateField)
	})
	t.Run("artifact-count-overflow", func(t *testing.T) {
		overflow := make([]evidence.ArtifactInput, evidence.MaxArtifacts+1)
		for index := range overflow {
			overflow[index] = evidence.ArtifactInput{Path: fmt.Sprintf("results/%02d.json", index), Kind: "unit-result", MediaType: "application/json", Data: []byte(`{}`)}
		}
		_, err := evidence.Seal(candidate, overflow)
		assertCode(t, err, evidence.CodeLimitExceeded)
	})
	t.Run("artifact-byte-overflow", func(t *testing.T) {
		overflow := []evidence.ArtifactInput{{Path: "result.bin", Kind: "unit-result", MediaType: "application/octet-stream", Data: make([]byte, evidence.MaxArtifactBytes+1)}}
		_, err := evidence.Seal(candidate, overflow)
		assertCode(t, err, evidence.CodeLimitExceeded)
	})
}

func parseError(data []byte) error {
	_, err := evidence.Parse(data)
	return err
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	typed, ok := err.(*evidence.Error)
	if !ok || typed.Code != code {
		t.Fatalf("expected %s, got %T %v", code, err, err)
	}
}

func packageCandidate() evidence.CandidateIdentityV1 {
	return evidence.CandidateIdentityV1{
		RepositoryIdentity: digestOfByte('0'), BaseTreeDigest: digestOfByte('1'), HeadTreeDigest: digestOfByte('2'),
		ChangeDigest: digestOfByte('3'), TaskSpecDigest: digestOfByte('4'), ConfigurationDigest: digestOfByte('5'),
		PolicyDigest: digestOfByte('6'), PromptAndRubricDigest: digestOfByte('7'), SkillSetDigest: digestOfByte('8'),
		ProviderModelBackendIdentityDigest: digestOfByte('9'), ToolchainAndToolSetDigest: digestOfByte('a'),
		DependencyLockDigest: digestOfByte('b'), ContainerImageSetDigest: digestOfByte('c'),
		TestManifestDigest: digestOfByte('d'), RoleContextManifestDigest: digestOfByte('e'),
	}
}

func digestOfByte(ch byte) string {
	return "sha256:" + strings.Repeat(string(ch), 64)
}
