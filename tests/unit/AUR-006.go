package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/sandbox/profile"
	"gopkg.in/yaml.v3"
)

type unitFixtureAUR006 struct {
	SchemaPath string `yaml:"schema_path"`
	Lock       string `yaml:"lock"`
	Cases      []struct {
		Name    string `yaml:"name"`
		Profile string `yaml:"profile"`
	} `yaml:"cases"`
}

// TestAUR006 is the unit-level contract probe named by AUR-006.
func TestAUR006(t *testing.T) {
	root := repoRoot(t)
	fixtureBytes, err := os.ReadFile(filepath.Join(root, "tests/specs/AUR-006/cases.yaml"))
	if err != nil {
		t.Fatalf("fixture unreadable: %v", err)
	}
	var fixture unitFixtureAUR006
	if err := yaml.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("fixture is not valid YAML: %v", err)
	}
	schema, err := os.ReadFile(filepath.Join(root, fixture.SchemaPath))
	if err != nil {
		t.Fatalf("schema unreadable: %v", err)
	}
	if len(fixture.Cases) == 0 || fixture.Cases[0].Name != "nominal" {
		t.Fatal("fixture does not start with the nominal case")
	}
	result := profile.ValidateBootstrapProfile([]byte(fixture.Cases[0].Profile), schema, []byte(fixture.Lock))
	if result.Code != "valid" || result.EngineInvocations != 0 {
		t.Fatalf("nominal result=%+v", result)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	t.Fatal("AURUMCODE_ROOT is required when running the standalone unit contract")
	return ""
}
