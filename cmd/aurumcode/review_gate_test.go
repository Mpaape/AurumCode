package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// declaredGateContributors is the documented order of the shared pipeline
// (docs/specs/AUR-557.md).
var declaredGateContributors = []string{
	"exceptions", "verdict-reuse", "policy-skills", "scanners",
	"embedded-analysis", "security-pass", "analysis-data", "dependency-track",
	"dependencies",
}

// observeGatePipelines returns session dependencies whose observer records
// the contributor names every path executes.
func observeGatePipelines() (reviewDeps, map[string][]string) {
	seen := map[string][]string{}
	return reviewDeps{gateObserver: func(label string, names []string) { seen[label] = names }}, seen
}

// TestAUR557PathsShareOnePipeline is AC-002: a real --base run and a real
// --pr run each execute the pipeline assembleGatePipeline declares, and the
// contributor names they ran are identical, in the declared order.
func TestAUR557PathsShareOnePipeline(t *testing.T) {
	deps, seen := observeGatePipelines()

	// --pr (reuses AUR-499's fake GitHub server and fixture provider).
	if code, stderr, _ := runPR499With(t, true, deps); code != 0 {
		t.Fatalf("--pr exit=%d stderr=%s", code, stderr)
	}

	// --base over a throwaway repository.
	cleanFixture(t, "gate:\n  fail_on: [high]\n")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"ok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	var out, errOut strings.Builder
	if code := runReviewWith(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: deps}, []string{"--base", "HEAD~1"}); code != 0 {
		t.Fatalf("--base exit=%d stderr=%s", code, errOut.String())
	}

	base, pr := seen["--base"], seen["--pr"]
	if len(base) == 0 || len(pr) == 0 {
		t.Fatalf("a path never ran the pipeline: base=%v pr=%v", base, pr)
	}
	if !reflect.DeepEqual(base, pr) {
		t.Fatalf("--base and --pr declare different pipelines:\n base=%v\n pr  =%v", base, pr)
	}
	if !reflect.DeepEqual(base, declaredGateContributors) {
		t.Fatalf("pipeline = %v, want the documented order %v", base, declaredGateContributors)
	}
	if got := assembleGatePipeline(gatePipelineInputs{}).Names(); !reflect.DeepEqual(got, declaredGateContributors) {
		t.Fatalf("assembleGatePipeline = %v, want %v", got, declaredGateContributors)
	}
}
