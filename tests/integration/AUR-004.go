package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/governance/dag"
)

const candidateDigestIntegrationAUR004 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

type fixtureAUR004 struct {
	Schema  string `json:"schema"`
	Version int    `json:"version"`
	Limits  struct {
		MaxVectors      int `json:"max_vectors"`
		MaxBytes        int `json:"max_bytes"`
		DeadlineSeconds int `json:"deadline_seconds"`
	} `json:"limits"`
	Vectors []vectorAUR004 `json:"vectors"`
}

type vectorAUR004 struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	InputDigest string `json:"input_digest"`
	Expected    struct {
		ExitCode         int      `json:"exit_code"`
		Code             string   `json:"code"`
		Field            string   `json:"field"`
		CardID           string   `json:"card_id"`
		Effects          int      `json:"effects"`
		TopologicalOrder []string `json:"topological_order"`
		Releasable       []string `json:"releasable"`
		ArtifactDigest   string   `json:"artifact_digest"`
	} `json:"expected"`
}

type observationAUR004 struct {
	ExitCode         int      `json:"exit_code"`
	Code             string   `json:"code"`
	Field            string   `json:"field"`
	CardID           string   `json:"card_id"`
	Effects          int      `json:"effects"`
	TopologicalOrder []string `json:"topological_order"`
	Releasable       []string `json:"releasable"`
	ArtifactDigest   string   `json:"artifact_digest"`
}

func digestAUR004(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func strictDecodeAUR004(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}

func fixtureQueryAUR004(id string) (dag.Query, error) {
	base := dag.Query{
		Schema:            dag.SchemaName,
		Version:           dag.SchemaVersion,
		CandidateIdentity: dag.CandidateIdentityV1{Digest: candidateDigestIntegrationAUR004},
	}
	switch id {
	case "nominal":
		base.Cards = []dag.Card{
			{ID: "AUR-003", PreviousStatus: dag.StatusReady, Status: dag.StatusReady, DependsOn: []string{"AUR-002"}},
			{ID: "AUR-001", PreviousStatus: dag.StatusReview, Status: dag.StatusDone},
			{ID: "AUR-004", PreviousStatus: dag.StatusBacklog, Status: dag.StatusReady},
			{ID: "AUR-002", PreviousStatus: dag.StatusBacklog, Status: dag.StatusReady, DependsOn: []string{"AUR-001"}},
		}
	case "invalid-missing-dependency":
		base.Cards = []dag.Card{{ID: "AUR-001", PreviousStatus: dag.StatusReady, Status: dag.StatusReady, DependsOn: []string{"AUR-999"}}}
	case "invalid-cycle":
		base.Cards = []dag.Card{
			{ID: "AUR-001", PreviousStatus: dag.StatusReady, Status: dag.StatusReady, DependsOn: []string{"AUR-002"}},
			{ID: "AUR-002", PreviousStatus: dag.StatusReady, Status: dag.StatusReady, DependsOn: []string{"AUR-001"}},
		}
	case "invalid-transition":
		base.Cards = []dag.Card{{ID: "AUR-001", PreviousStatus: dag.StatusBacklog, Status: dag.StatusDone}}
	case "invalid-agent-approval":
		base.Cards = []dag.Card{{ID: "AUR-001", PreviousStatus: dag.StatusReady, Status: dag.StatusReady}}
		base.AuthorizationEvents = []dag.AuthorizationEvent{{CardID: "AUR-001", ActorType: dag.ActorAgent, Decision: "approve"}}
	case "boundary", "boundary-overflow":
		count := dag.MaxCards
		if id == "boundary-overflow" {
			count++
		}
		base.Cards = make([]dag.Card, count)
		for index := range base.Cards {
			base.Cards[index] = dag.Card{ID: fmt.Sprintf("AUR-%03d", 100+index), PreviousStatus: dag.StatusReady, Status: dag.StatusReady}
		}
	default:
		return dag.Query{}, fmt.Errorf("unknown vector")
	}
	return base, nil
}

func queryDigestAUR004(query dag.Query) (string, error) {
	encoded, err := json.Marshal(query)
	if err != nil {
		return "", err
	}
	return digestAUR004(encoded), nil
}

func observeAUR004(query dag.Query) observationAUR004 {
	result, err := dag.Release(query)
	if err == nil {
		return observationAUR004{
			ExitCode: 0, Code: "valid", Effects: result.Effects,
			TopologicalOrder: result.TopologicalOrder, Releasable: result.Releasable,
			ArtifactDigest: result.ArtifactDigest,
		}
	}
	typed, ok := err.(*dag.ValidationError)
	if !ok {
		return observationAUR004{ExitCode: 3, Code: "untyped_error"}
	}
	artifact := digestAUR004([]byte(fmt.Sprintf("1\n%s\n%s\n%s\n0\n", typed.Code, typed.Field, typed.CardID)))
	return observationAUR004{
		ExitCode: 1, Code: typed.Code, Field: typed.Field, CardID: typed.CardID,
		Effects: 0, TopologicalOrder: []string{}, Releasable: []string{}, ArtifactDigest: artifact,
	}
}

func readExampleAUR004(path string) (dag.Query, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return dag.Query{}, err
	}
	const start = "<!-- aurum-dag-example:start -->\n"
	const end = "\n<!-- aurum-dag-example:end -->"
	begin := bytes.Index(data, []byte(start))
	finish := bytes.Index(data, []byte(end))
	if begin < 0 || finish < 0 || finish <= begin {
		return dag.Query{}, fmt.Errorf("example markers absent")
	}
	payload := data[begin+len(start) : finish]
	var query dag.Query
	if err := strictDecodeAUR004(payload, &query); err != nil {
		return dag.Query{}, err
	}
	return query, nil
}

// IntegrationAUR004 executes every sealed vector and the documentation example
// against the public package. Digests and replay results are checked outside
// the implementation under test.
func IntegrationAUR004(root string) (int, error) {
	assertions := 0
	fixturePath := filepath.Join(root, "tests/specs/AUR-004/cases.yaml")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/infrastructure: fixture unreadable: %w", err)
	}
	if len(data) > 4*1024*1024 {
		return assertions, fmt.Errorf("AUR-004/AC-001/limit-exceeded: fixture bytes")
	}
	var fixture fixtureAUR004
	if err := strictDecodeAUR004(data, &fixture); err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: %w", err)
	}
	if fixture.Schema != "aurum.dag-release-cases" || fixture.Version != 1 || fixture.Limits.MaxVectors != 64 || fixture.Limits.MaxBytes != 4194304 || fixture.Limits.DeadlineSeconds < 1 || fixture.Limits.DeadlineSeconds > 30 {
		return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: limits")
	}
	if len(fixture.Vectors) == 0 || len(fixture.Vectors) > fixture.Limits.MaxVectors {
		return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: vector count")
	}
	assertions++

	seen := map[string]bool{}
	kinds := map[string]int{}
	for _, vector := range fixture.Vectors {
		if seen[vector.ID] || (vector.Kind != "nominal" && vector.Kind != "invalid" && vector.Kind != "boundary") {
			return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: duplicate or kind")
		}
		seen[vector.ID] = true
		kinds[vector.Kind]++
		query, err := fixtureQueryAUR004(vector.ID)
		if err != nil {
			return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: %s", vector.ID)
		}
		inputDigest, err := queryDigestAUR004(query)
		if err != nil {
			return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: input digest %s", vector.ID)
		}
		observed := observeAUR004(query)
		replay := observeAUR004(query)
		if !reflect.DeepEqual(observed, replay) {
			return assertions, fmt.Errorf("AUR-004/AC-001/replay-mismatch: %s", vector.ID)
		}
		if inputDigest != vector.InputDigest {
			return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: input digest %s", vector.ID)
		}
		expected := observationAUR004{
			ExitCode: vector.Expected.ExitCode, Code: vector.Expected.Code,
			Field: vector.Expected.Field, CardID: vector.Expected.CardID, Effects: vector.Expected.Effects,
			TopologicalOrder: vector.Expected.TopologicalOrder, Releasable: vector.Expected.Releasable,
			ArtifactDigest: vector.Expected.ArtifactDigest,
		}
		if !reflect.DeepEqual(observed, expected) {
			switch vector.ID {
			case "invalid-cycle":
				if observed.ExitCode == 0 || expected.Code != dag.CodeCycle {
					return assertions, fmt.Errorf("AUR-004/AC-001/MUT-001: cycle accepted")
				}
			case "invalid-agent-approval":
				if observed.ExitCode == 0 || expected.Code != dag.CodeAgentIdentityForbidden {
					return assertions, fmt.Errorf("AUR-004/AC-001/MUT-002: agent approval accepted")
				}
			}
			return assertions, fmt.Errorf("AUR-004/AC-001/vector-mismatch: %s got=%+v", vector.ID, observed)
		}
		assertions++
	}
	if kinds["nominal"] == 0 || kinds["invalid"] == 0 || kinds["boundary"] == 0 || !seen["boundary-overflow"] {
		return assertions, fmt.Errorf("AUR-004/AC-001/fixture-invalid: coverage")
	}
	assertions++

	example, err := readExampleAUR004(filepath.Join(root, "docs/specs/AUR-004.md"))
	if err != nil {
		return assertions, fmt.Errorf("AUR-004/AC-001/example-invalid: %w", err)
	}
	exampleResult, err := dag.Release(example)
	if err != nil || !reflect.DeepEqual(exampleResult.Releasable, []string{"AUR-002"}) {
		return assertions, fmt.Errorf("AUR-004/AC-001/example-mismatch")
	}
	if !sort.StringsAreSorted(exampleResult.TopologicalOrder) || strings.Contains(strings.Join(exampleResult.Releasable, ","), "AURUM_SECRET_CANARY") {
		return assertions, fmt.Errorf("AUR-004/AC-001/example-mismatch: unsafe output")
	}
	assertions++
	return assertions, nil
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "IntegrationAUR004" {
		fmt.Fprintln(os.Stderr, "AUR-004/AC-001/unknown-selector")
		os.Exit(64)
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "AUR-004/AC-001/infrastructure: cwd unavailable")
		os.Exit(69)
	}
	assertions, err := IntegrationAUR004(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if assertions == 0 {
		fmt.Fprintln(os.Stderr, "AUR-004/AC-001/zero-assertions")
		os.Exit(1)
	}
	fmt.Printf("{\"card\":\"AUR-004\",\"scenario\":\"AC-001\",\"selector\":\"IntegrationAUR004\",\"assertions\":%d,\"result\":\"pass\"}\n", assertions)
}
