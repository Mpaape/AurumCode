package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// #177: the reusable workflow's whole-branch scan is opt-in, runs only on
// schedule or a manual run of the default branch, reads the whole history,
// never prints the review's output, and uploads only a completed scan, in
// its own category.
func TestFullScanJobOfTheReusableWorkflow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "review.yml"))
	if os.IsNotExist(err) {
		t.Skip("the module was staged without the workflow")
	}
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		On struct {
			WorkflowCall struct {
				Inputs map[string]struct {
					Default any `yaml:"default"`
				} `yaml:"inputs"`
			} `yaml:"workflow_call"`
		} `yaml:"on"`
		Jobs map[string]struct {
			If          string         `yaml:"if"`
			Permissions map[string]any `yaml:"permissions"`
			Steps       []struct {
				Name string            `yaml:"name"`
				If   string            `yaml:"if"`
				Run  string            `yaml:"run"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	if in, ok := wf.On.WorkflowCall.Inputs["scan"]; !ok || in.Default != false {
		t.Fatalf("input scan must exist and default to false: %+v", in)
	}
	scan, ok := wf.Jobs["scan"]
	if !ok {
		t.Fatal("no scan job")
	}
	for _, want := range []string{"inputs.scan", "'schedule'", "'workflow_dispatch'", "default_branch"} {
		if !strings.Contains(scan.If, want) {
			t.Fatalf("scan.if lacks %s: %q", want, scan.If)
		}
	}
	if scan.Permissions != nil {
		t.Fatal("the scan job declares no permissions: the caller grants them")
	}
	var history, ran, uploaded bool
	for _, s := range scan.Steps {
		if s.Name == "Checkout default branch" && s.With["fetch-depth"] == "0" {
			history = true
		}
		if strings.Contains(s.Run, "review --completo") {
			ran = true
			if !strings.Contains(s.Run, `>"${RUNNER_TEMP}/aurumcode-scan.log" 2>&1`) {
				t.Fatal("the review's output must go to a file, never to the log")
			}
		}
		if s.Name == "Upload to code scanning" {
			uploaded = true
			if s.If != "success()" || s.With["category"] != "aurumcode/initial-scan" {
				t.Fatalf("upload: if=%q category=%q", s.If, s.With["category"])
			}
		}
	}
	if !history || !ran || !uploaded {
		t.Fatalf("history=%v ran=%v uploaded=%v", history, ran, uploaded)
	}
	if !strings.Contains(wf.Jobs["dependencies"].If, "'workflow_dispatch'") {
		t.Fatalf("dependencies must also run on a manual run: %q", wf.Jobs["dependencies"].If)
	}
}
