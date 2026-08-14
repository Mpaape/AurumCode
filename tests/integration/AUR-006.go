package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/sandbox/profile"
	"gopkg.in/yaml.v3"
)

type integrationFixtureAUR006 struct {
	SchemaPath string `yaml:"schema_path"`
	Lock       string `yaml:"lock"`
	Cases      []struct {
		Name    string `yaml:"name"`
		Profile string `yaml:"profile"`
	} `yaml:"cases"`
}

// IntegrationAUR006 checks the profile against the checked-in image lock and
// keeps the validation boundary independent from an OCI engine.
func IntegrationAUR006(t *testing.T) {
	t.Helper()
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required when running the standalone integration contract")
	}
	fixtureBytes, err := os.ReadFile(filepath.Join(root, "tests/specs/AUR-006/cases.yaml"))
	if err != nil {
		t.Fatalf("AUR-006 fixture unreadable: %v", err)
	}
	var fixture integrationFixtureAUR006
	if err := yaml.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("AUR-006 fixture is not valid YAML: %v", err)
	}
	if len(fixture.Cases) == 0 || fixture.Cases[0].Name != "nominal" {
		t.Fatal("AUR-006 fixture does not start with nominal")
	}
	schema, err := os.ReadFile(filepath.Join(root, fixture.SchemaPath))
	if err != nil {
		t.Fatalf("AUR-006 schema unreadable: %v", err)
	}
	result := profile.ValidateBootstrapProfile([]byte(fixture.Cases[0].Profile), schema, []byte(fixture.Lock))
	if result.Status != "valid" || result.Code != "valid" || result.EngineInvocations != 0 {
		t.Fatalf("integration result=%+v", result)
	}
}
