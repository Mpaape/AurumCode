package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/evidence"
)

type casesAUR005 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Limits  struct {
		MaxVectors      int `json:"max_vectors"`
		MaxBytes        int `json:"max_bytes"`
		DeadlineSeconds int `json:"deadline_seconds"`
	} `json:"limits"`
	Vectors []vectorAUR005 `json:"vectors"`
}

type vectorAUR005 struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	InputDigest    string `json:"input_digest"`
	ExpectedExit   int    `json:"expected_exit"`
	ExpectedCode   string `json:"expected_code"`
	ExpectedField  string `json:"expected_field"`
	Effects        int    `json:"effects"`
	ArtifactDigest string `json:"artifact_digest"`
}

type vectorResultAUR005 struct {
	ExitCode       int
	Code           string
	Field          string
	Effects        int
	InputDigest    string
	ArtifactDigest string
}

// IntegrationAUR005 is the card's declared integration selector. It executes
// every sealed vector through the production parser and verifier.
func IntegrationAUR005(t *testing.T) {
	t.Helper()
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}
	path := filepath.Join(root, "tests/specs/AUR-005/cases.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("vectors unreadable: %v", err)
	}
	if len(data) > 4*1024*1024 {
		t.Fatal("vector file exceeds 4 MiB")
	}
	if err := rejectDuplicateJSONAUR005(data); err != nil {
		t.Fatalf("vectors rejected: %v", err)
	}
	var fixture casesAUR005
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("vectors malformed: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatal("vectors have trailing JSON")
	}
	if fixture.Schema != "aurum.evidence-bundle-cases" || fixture.Version != 1 {
		t.Fatal("vector schema/version mismatch")
	}
	verifyDocumentedExampleAUR005(t, root)
	if fixture.Limits.MaxVectors != 64 || fixture.Limits.MaxBytes != 4*1024*1024 || fixture.Limits.DeadlineSeconds < 1 || fixture.Limits.DeadlineSeconds > 30 {
		t.Fatalf("vector limits mismatch: %+v", fixture.Limits)
	}
	if len(fixture.Vectors) == 0 || len(fixture.Vectors) > fixture.Limits.MaxVectors {
		t.Fatalf("vector count outside 1..%d", fixture.Limits.MaxVectors)
	}
	requiredOrder := []string{"nominal", "invalid", "tampered-image", "tampered-output", "forged-agent-approval", "noncanonical", "duplicate-field", "boundary", "boundary-overflow"}
	if len(fixture.Vectors) != len(requiredOrder) {
		t.Fatalf("got %d vectors, want %d", len(fixture.Vectors), len(requiredOrder))
	}
	for index, vector := range fixture.Vectors {
		if vector.ID != requiredOrder[index] {
			t.Fatalf("vector %d is %q, want %q", index, vector.ID, requiredOrder[index])
		}
		got := runVectorAUR005(t, vector.ID)
		replayed := runVectorAUR005(t, vector.ID)
		if replayed != got {
			t.Fatalf("vector %s replay diverged\nfirst: %+v\nreplay: %+v", vector.ID, got, replayed)
		}
		want := vectorResultAUR005{vector.ExpectedExit, vector.ExpectedCode, vector.ExpectedField, vector.Effects, vector.InputDigest, vector.ArtifactDigest}
		if got != want {
			t.Errorf("vector %s mismatch\n got: %+v\nwant: %+v", vector.ID, got, want)
		}
	}
}

type documentedExampleAUR005 struct {
	Candidate evidence.CandidateIdentityV1 `json:"candidate_identity"`
	Outputs   []struct {
		Path      string `json:"path"`
		Kind      string `json:"kind"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"outputs"`
}

func verifyDocumentedExampleAUR005(t *testing.T, root string) {
	t.Helper()
	document, err := os.ReadFile(filepath.Join(root, "docs/specs/AUR-005.md"))
	if err != nil {
		t.Fatalf("documented example unreadable: %v", err)
	}
	const start = "<!-- AUR-005-example:start -->\n"
	const end = "\n<!-- AUR-005-example:end -->"
	startAt := bytes.Index(document, []byte(start))
	if startAt < 0 {
		t.Fatal("documented example start marker absent")
	}
	remaining := document[startAt+len(start):]
	endAt := bytes.Index(remaining, []byte(end))
	if endAt < 0 {
		t.Fatal("documented example end marker absent")
	}
	exampleBytes := remaining[:endAt]
	if err := rejectDuplicateJSONAUR005(exampleBytes); err != nil {
		t.Fatalf("documented example rejected: %v", err)
	}
	var example documentedExampleAUR005
	decoder := json.NewDecoder(bytes.NewReader(exampleBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&example); err != nil {
		t.Fatalf("documented example malformed: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatal("documented example has trailing JSON")
	}
	outputs := make([]evidence.ArtifactInput, len(example.Outputs))
	for i, output := range example.Outputs {
		outputs[i] = evidence.ArtifactInput{Path: output.Path, Kind: output.Kind, MediaType: output.MediaType, Data: []byte(output.Data)}
	}
	manifest, err := evidence.Seal(example.Candidate, outputs)
	if err != nil {
		if typed, ok := err.(*evidence.Error); ok && typed.Code == "behavior_missing" {
			t.Fatal("AUR-005/AC-001/behavior-missing")
		}
		t.Fatalf("documented example seal failed: %v", err)
	}
	if err := evidence.Verify(manifest, example.Candidate, outputs); err != nil {
		t.Fatalf("documented example verification failed: %v", err)
	}
}

func runVectorAUR005(t *testing.T, id string) vectorResultAUR005 {
	t.Helper()
	candidate := integrationCandidateAUR005()
	outputs := integrationOutputsAUR005()

	if id == "boundary" || id == "boundary-overflow" {
		count := evidence.MaxArtifacts
		if id == "boundary-overflow" {
			count++
		}
		outputs = make([]evidence.ArtifactInput, count)
		for i := range outputs {
			outputs[i] = evidence.ArtifactInput{
				Path: fmt.Sprintf("results/%02d.json", i), Kind: "integration-result",
				MediaType: "application/json", Data: []byte(fmt.Sprintf(`{"case":%d}`, i)),
			}
		}
	}

	manifest, sealErr := evidence.Seal(candidate, outputs)
	if sealErr != nil {
		if typed, ok := sealErr.(*evidence.Error); ok && typed.Code == "behavior_missing" {
			t.Fatal("AUR-005/AC-001/behavior-missing")
		}
		return errorVectorAUR005(id, nil, outputs, sealErr)
	}
	encoded, err := evidence.Marshal(manifest)
	if err != nil {
		return errorVectorAUR005(id, nil, outputs, err)
	}

	switch id {
	case "invalid":
		encoded = bytes.Replace(encoded, []byte(candidate.TaskSpecDigest), []byte("sha256:"+strings.Repeat("f", 64)), 1)
	case "tampered-image":
		encoded = bytes.Replace(encoded, []byte(candidate.ContainerImageSetDigest), []byte("sha256:"+strings.Repeat("f", 64)), 1)
	case "tampered-output":
		changed := append([]byte(nil), outputs[0].Data...)
		changed[0] ^= 1
		outputs[0].Data = changed
	case "forged-agent-approval":
		encoded = bytes.Replace(encoded, []byte(`"authority":"none"`), []byte(`"authority":"agent"`), 1)
	case "noncanonical":
		encoded = append(encoded, '\n')
	case "duplicate-field":
		encoded = bytes.Replace(encoded, []byte(`{"schema":`), []byte(`{"schema":"aurum.evidence-bundle","schema":`), 1)
	case "nominal", "boundary", "boundary-overflow":
	default:
		t.Fatalf("unknown vector %s", id)
	}

	inputDigest := vectorInputDigestAUR005(encoded, outputs)
	verifyErr := evidence.VerifyBytes(encoded, candidate, outputs)
	if verifyErr == nil {
		return vectorResultAUR005{ExitCode: 0, Code: "valid", Field: "none", Effects: 0, InputDigest: inputDigest, ArtifactDigest: manifest.ManifestDigest}
	}
	typed, ok := verifyErr.(*evidence.Error)
	if !ok {
		t.Fatalf("vector %s returned untyped error %T", id, verifyErr)
	}
	return vectorResultAUR005{ExitCode: 1, Code: typed.Code, Field: typed.Field, Effects: 0, InputDigest: inputDigest, ArtifactDigest: errorArtifactDigestAUR005(id, inputDigest, typed)}
}

func errorVectorAUR005(id string, manifest []byte, outputs []evidence.ArtifactInput, err error) vectorResultAUR005 {
	typed := &evidence.Error{Field: "manifest", Code: evidence.CodeMalformed}
	if !errors.As(err, &typed) {
		typed = &evidence.Error{Field: "manifest", Code: evidence.CodeMalformed}
	}
	inputDigest := vectorInputDigestAUR005(manifest, outputs)
	return vectorResultAUR005{ExitCode: 1, Code: typed.Code, Field: typed.Field, Effects: 0, InputDigest: inputDigest, ArtifactDigest: errorArtifactDigestAUR005(id, inputDigest, typed)}
}

func integrationCandidateAUR005() evidence.CandidateIdentityV1 {
	d := func(ch byte) string { return "sha256:" + strings.Repeat(string(ch), 64) }
	return evidence.CandidateIdentityV1{
		RepositoryIdentity: d('0'), BaseTreeDigest: d('1'), HeadTreeDigest: d('2'), ChangeDigest: d('3'),
		TaskSpecDigest: d('4'), ConfigurationDigest: d('5'), PolicyDigest: d('6'), PromptAndRubricDigest: d('7'),
		SkillSetDigest: d('8'), ProviderModelBackendIdentityDigest: d('9'), ToolchainAndToolSetDigest: d('a'),
		DependencyLockDigest: d('b'), ContainerImageSetDigest: d('c'), TestManifestDigest: d('d'), RoleContextManifestDigest: d('e'),
	}
}

func integrationOutputsAUR005() []evidence.ArtifactInput {
	return []evidence.ArtifactInput{
		{Path: "acceptance/AC-001.json", Kind: "acceptance-result", MediaType: "application/json", Data: []byte(`{"exit_code":0,"result":"pass"}`)},
		{Path: "tests/unit.json", Kind: "unit-result", MediaType: "application/json", Data: []byte(`{"selector":"TestAUR005","status":"pass"}`)},
	}
}

func vectorInputDigestAUR005(manifest []byte, outputs []evidence.ArtifactInput) string {
	ordered := append([]evidence.ArtifactInput(nil), outputs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	h := sha256.New()
	h.Write([]byte("aurum.evidence-bundle.vector.v1\n"))
	writeBoundedAUR005(h, manifest)
	for _, output := range ordered {
		writeBoundedAUR005(h, []byte(output.Path))
		writeBoundedAUR005(h, []byte(output.Kind))
		writeBoundedAUR005(h, []byte(output.MediaType))
		writeBoundedAUR005(h, output.Data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func writeBoundedAUR005(w io.Writer, data []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(data)))
	_, _ = w.Write(size[:])
	_, _ = w.Write(data)
}

func errorArtifactDigestAUR005(id, inputDigest string, err *evidence.Error) string {
	sum := sha256.Sum256([]byte("aurum.evidence-bundle.error.v1\n" + id + "\n" + inputDigest + "\n" + err.Code + "\n" + err.Field))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func rejectDuplicateJSONAUR005(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return errors.New("malformed JSON")
	}
	return scanJSONValueAUR005(decoder, first)
}

func scanJSONValueAUR005(decoder *json.Decoder, token json.Token) error {
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return errors.New("malformed JSON")
			}
			name, ok := nameToken.(string)
			if !ok {
				return errors.New("malformed JSON")
			}
			if _, duplicate := seen[name]; duplicate {
				return errors.New("duplicate JSON member")
			}
			seen[name] = struct{}{}
			value, err := decoder.Token()
			if err != nil {
				return errors.New("malformed JSON")
			}
			if err := scanJSONValueAUR005(decoder, value); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("malformed JSON")
		}
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return errors.New("malformed JSON")
			}
			if err := scanJSONValueAUR005(decoder, value); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("malformed JSON")
		}
	default:
		return errors.New("malformed JSON")
	}
	return nil
}
