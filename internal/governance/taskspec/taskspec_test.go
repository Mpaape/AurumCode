package taskspec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testTaskSpec = `schema: aurum.task-spec
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

func TestAUR003(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate schema fixture")
	}
	schemaPath := filepath.Join(filepath.Dir(sourceFile), "../../../.board/schemas/task-spec.schema.json")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if err := ValidateSchema(schema); err != nil {
		t.Fatalf("nominal schema: %v", err)
	}
	for _, invalidSchema := range []string{
		strings.Replace(string(schema), "    \"mutation\"\n", "", 1),
		strings.Replace(string(schema), "\"scope_disjointness\":", "\"scope_disjointness_removed\":", 1),
	} {
		if err := ValidateSchema([]byte(invalidSchema)); err == nil {
			t.Fatal("invalid schema was accepted")
		}
	}

	spec, err := Load([]byte(testTaskSpec))
	if err != nil {
		t.Fatalf("nominal load: %v", err)
	}
	if spec.Mutation.Boundary != "repository" || spec.Outcome == "" {
		t.Fatal("nominal load did not preserve the atomic contract")
	}

	for _, test := range []struct {
		name string
		data string
	}{
		{"interior-space", strings.Replace(testTaskSpec, "internal/governance/taskspec", "internal/governance/task spec", 1)},
		{"double-slash", strings.Replace(testTaskSpec, "internal/governance/taskspec", "internal//governance/taskspec", 1)},
		{"mutation-boundary-not-declared", strings.Replace(strings.Replace(testTaskSpec, "trust_boundaries: [repository, authorization-source]", "trust_boundaries: [repository]", 1), "boundary: repository", "boundary: authorization-source", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Load([]byte(test.data)); err == nil {
				t.Fatal("invalid TaskSpec was accepted")
			}
		})
	}

	for _, canary := range []string{
		testTaskSpec + "AURUM_SECRET_CANARY: forged\n",
		testTaskSpec + "AURUM_SECRET_CANARY\n: forged\n",
	} {
		_, err = Load([]byte(canary))
		if err == nil {
			t.Fatal("unknown key was accepted")
		}
		if strings.Contains(err.Error(), "AURUM_SECRET_CANARY") || strings.ContainsAny(err.Error(), "\r\n") {
			t.Fatalf("unknown key escaped into error output: %q", err)
		}
	}
}
