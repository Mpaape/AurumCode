package plugin

import (
	"strings"
	"testing"
)

// AC-004: on the issues destination the plugins job runs the plugin, the
// ingest and `plugin issues` (only after a conclusive ingest), with no
// upload-sarif; the caller's comment documents issues: write; the step
// prints only the command's counts line and never the SARIF.
func TestAUR623WorkflowIssuesDestination(t *testing.T) {
	jobs := aur622Workflow(t)
	run := jobs["plugins"]
	const onIssues = "success() && steps.destination.outputs.destino == 'issues'"
	const onSecurity = "success() && steps.destination.outputs.destino == 'security'"
	var chose, ranPlugin, ingested, synced bool
	for _, s := range run.Steps {
		switch {
		case s.Name == "Choose destination":
			chose = strings.Contains(s.Run, `echo "destino=${destination}" >> "$GITHUB_OUTPUT"`) && !strings.Contains(s.Run, "exit 1")
		case s.Name == "Run plugin" && s.If == "":
			ranPlugin = true
		case s.Name == "Ingest plugin output" && s.If == "":
			ingested = strings.Contains(s.Run, "plugin ingest")
		case s.Name == "Sync issues":
			synced = s.If == onIssues &&
				strings.Contains(s.Run, "plugin issues --name \"${PLUGIN_NAME}\" --repo \"${REPOSITORY}\"") &&
				strings.Contains(s.Run, "-e GITHUB_TOKEN") &&
				strings.Contains(s.Run, ":/aurumcode-out:ro") &&
				!strings.Contains(s.Run, "echo") && !strings.Contains(s.Run, "jq") && !strings.Contains(s.Run, "cat ")
		case strings.HasPrefix(s.Uses, "github/codeql-action/upload-sarif"), s.Name == "Count sent alerts":
			if s.If != onSecurity {
				t.Fatalf("step %q runs on the issues destination: if=%q", s.Name+s.Uses, s.If)
			}
		}
	}
	if !chose || !ranPlugin || !ingested || !synced {
		t.Fatalf("destination output=%v plugin=%v ingest=%v issues step=%v", chose, ranPlugin, ingested, synced)
	}
	raw := string(mustRead(t))
	if !strings.Contains(raw, "plus `issues: write` in a private repository") {
		t.Fatal("the caller's comment must document issues: write")
	}
	if strings.Contains(raw, "ainda não entregue") {
		t.Fatal("the job still refuses the issues destination")
	}
}
