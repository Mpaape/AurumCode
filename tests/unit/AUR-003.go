package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/governance/taskspec"
)

const validTaskSpecYAML = `schema: aurum.task-spec
version: 1
id: AUR-003
title: Definir schema de card atômico
status: doing
validation: tested
office: O00-governance
depends_on: [AUR-001]
requirements: [PR-ARCH-001, PR-EVD-001]
controls: [CR-EVD-001]
paths: [internal/governance/taskspec, tests/acceptance/AUR-003.sh]
read_paths: []
forbidden_paths: [.git, .env, secrets, .board/cards]
base_sha: lock-at-execution
spec_digest: lock-at-execution
risk: medium
data_class: internal
trust_boundaries: [repository, authorization-source]
outcome: incomplete cards are refused
mutation:
  id: MUT-001
  boundary: repository
  change: remove the mutation requirement from the schema
  expected: acceptance returns a typed failure before any effect
`

type observation struct {
	Card           string `json:"card"`
	Scenario       string `json:"scenario"`
	Vector         string `json:"vector"`
	ExitCode       int    `json:"exit_code"`
	Code           string `json:"code"`
	Field          string `json:"field"`
	Effects        int    `json:"effects"`
	InputDigest    string `json:"input_digest"`
	ArtifactDigest string `json:"artifact_digest"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func caseInput(vector string) (string, error) {
	switch vector {
	case "nominal":
		return validTaskSpecYAML, nil
	case "invalid":
		return validTaskSpecYAML + "AURUM_SECRET_CANARY: forged\n", nil
	case "invalid-path-whitespace":
		return strings.Replace(validTaskSpecYAML, "internal/governance/taskspec", "internal/governance/task spec", 1), nil
	case "invalid-double-slash":
		return strings.Replace(validTaskSpecYAML, "internal/governance/taskspec", "internal//governance/taskspec", 1), nil
	case "boundary":
		return strings.Replace(validTaskSpecYAML, "title: Definir schema de card atômico", "title: "+strings.Repeat("x", 256), 1), nil
	case "boundary-overflow":
		return strings.Replace(validTaskSpecYAML, "title: Definir schema de card atômico", "title: "+strings.Repeat("x", 257), 1), nil
	default:
		return "", fmt.Errorf("unknown vector")
	}
}

func runVector(vector string) (observation, int, error) {
	input, err := caseInput(vector)
	if err != nil {
		return observation{}, 64, err
	}
	inputDigest := digest([]byte(input))
	spec, err := taskspec.Load([]byte(input))
	if err == nil {
		artifactDigest, digestErr := taskspec.Digest(spec)
		if digestErr != nil {
			return observation{}, 3, fmt.Errorf("artifact digest unavailable")
		}
		return observation{
			Card: "AUR-003", Scenario: "AC-001", Vector: vector, ExitCode: 0,
			Code: "valid", Effects: 0, InputDigest: inputDigest, ArtifactDigest: artifactDigest,
		}, 0, nil
	}
	fieldErr, ok := err.(*taskspec.FieldError)
	if !ok {
		return observation{}, 3, fmt.Errorf("loader returned an untyped error")
	}
	artifactDigest := digest([]byte(vector + "\n" + inputDigest + "\n" + fieldErr.Code + "\n" + fieldErr.Field))
	return observation{
		Card: "AUR-003", Scenario: "AC-001", Vector: vector, ExitCode: 1,
		Code: fieldErr.Code, Field: fieldErr.Field, Effects: 0,
		InputDigest: inputDigest, ArtifactDigest: artifactDigest,
	}, 1, nil
}

func main() {
	var vector, schemaPath string
	for index := 1; index < len(os.Args); index++ {
		switch os.Args[index] {
		case "--case":
			if index+1 >= len(os.Args) {
				fmt.Fprintln(os.Stderr, "AUR-003/AC-001/missing-case")
				os.Exit(64)
			}
			index++
			vector = os.Args[index]
		case "--schema":
			if index+1 >= len(os.Args) {
				fmt.Fprintln(os.Stderr, "AUR-003/AC-001/missing-schema")
				os.Exit(64)
			}
			index++
			schemaPath = os.Args[index]
		default:
			fmt.Fprintln(os.Stderr, "AUR-003/AC-001/unknown-selector")
			os.Exit(64)
		}
	}
	if vector == "" {
		fmt.Fprintln(os.Stderr, "AUR-003/AC-001/missing-case")
		os.Exit(64)
	}
	if schemaPath != "" {
		schema, err := os.ReadFile(schemaPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "AUR-003/AC-001/infrastructure: schema unreadable")
			os.Exit(3)
		}
		if err := taskspec.ValidateSchema(schema); err != nil {
			fmt.Fprintln(os.Stderr, "AUR-003/AC-001/schema-invalid")
			os.Exit(1)
		}
	}
	result, exitCode, err := runVector(vector)
	if err != nil {
		fmt.Fprintf(os.Stderr, "AUR-003/AC-001/%s\n", err.Error())
		os.Exit(exitCode)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "AUR-003/AC-001/infrastructure: result encoding failed")
		os.Exit(3)
	}
	fmt.Println(string(encoded))
	os.Exit(exitCode)
}
