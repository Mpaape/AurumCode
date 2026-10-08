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
	skip, err := manifestCheckSkipped(root, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Skip("AURUMCODE_MODULE_ONLY=1: the acceptance staged only the Go module")
	}
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

// manifestCheckSkipped decides whether the manifest check may skip. Only an
// acceptance program that stages the Go module alone (go.mod, cmd, internal,
// pkg) says so, with AURUMCODE_MODULE_ONLY=1. Anywhere else a missing
// .github/workflows is an error, never a silent skip.
func manifestCheckSkipped(root string, getenv func(string) string) (bool, error) {
	if getenv("AURUMCODE_MODULE_ONLY") == "1" {
		return true, nil
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows")); err != nil {
		return false, err
	}
	return false, nil
}

func TestActionManifestSkipsOnlyWhenModuleOnly(t *testing.T) {
	empty := t.TempDir()
	if skip, err := manifestCheckSkipped(empty, func(string) string { return "" }); skip || err == nil {
		t.Fatalf("missing .github/workflows without AURUMCODE_MODULE_ONLY: skip=%v err=%v", skip, err)
	}
	if skip, err := manifestCheckSkipped(empty, func(k string) string {
		if k == "AURUMCODE_MODULE_ONLY" {
			return "1"
		}
		return ""
	}); !skip || err != nil {
		t.Fatalf("module-only staging must skip: skip=%v err=%v", skip, err)
	}
}
