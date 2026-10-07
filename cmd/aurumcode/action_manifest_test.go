package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The Action manifest and every workflow must be valid YAML: GitHub refuses
// to load an action whose action.yml does not parse, so a plain scalar with
// ": " inside (a description such as "gate: error, warning") breaks every
// repository that uses the Action, with no CI signal in this repository.
func TestActionManifestAndWorkflowsAreValidYAML(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{filepath.Join(root, "action.yml")}
	workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, workflows...)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Errorf("%s is not valid YAML: %v", path, err)
			continue
		}
		if len(doc) == 0 {
			t.Errorf("%s parsed to an empty document", path)
		}
	}
}
