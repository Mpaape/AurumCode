package plugin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type aur622Step struct {
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
}

type aur622Job struct {
	If          string         `yaml:"if"`
	Needs       any            `yaml:"needs"`
	Permissions map[string]any `yaml:"permissions"`
	Steps       []aur622Step   `yaml:"steps"`
}

func aur622Workflow(t *testing.T) map[string]aur622Job {
	t.Helper()
	path := os.Getenv("AUR622_WORKFLOW")
	if path == "" {
		path = filepath.Join("..", "..", ".github", "workflows", "review.yml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]aur622Job `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("workflow is not valid YAML: %v", err)
	}
	return wf.Jobs
}

func mustRead(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("AUR622_WORKFLOW")
	if path == "" {
		path = filepath.Join("..", "..", ".github", "workflows", "review.yml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

var pinnedUpload = regexp.MustCompile(`^github/codeql-action/upload-sarif@[0-9a-f]{40}$`)

// AC-005: the plugins jobs run only on schedule/workflow_dispatch of the
// default branch, refuse any other event, never publish through a public
// channel, keep the plugin's output in a file and upload with a pinned
// upload-sarif under the plugin's own category, only on success.
func TestAUR622WorkflowPluginsJob(t *testing.T) {
	jobs := aur622Workflow(t)
	list, ok := jobs["plugins-list"]
	run, ok2 := jobs["plugins"]
	if !ok || !ok2 {
		t.Fatalf("jobs plugins-list/plugins missing")
	}
	if !strings.Contains(list.If, "github.event_name == 'schedule'") ||
		!strings.Contains(list.If, "github.event_name == 'workflow_dispatch' && github.ref_name == github.event.repository.default_branch") ||
		strings.Contains(list.If, "pull_request") {
		t.Fatalf("plugins-list.if = %q", list.If)
	}
	if !strings.Contains(jobs["review"].If, "!= 'workflow_dispatch'") {
		t.Fatalf("review job runs on workflow_dispatch: %q", jobs["review"].If)
	}
	for name, job := range map[string]aur622Job{"plugins-list": list, "plugins": run} {
		if len(job.Permissions) != 0 {
			t.Fatalf("%s declares permissions; it must inherit the caller's grant", name)
		}
		if len(job.Steps) == 0 || !strings.Contains(job.Steps[0].Run, "schedule|workflow_dispatch) ;;") || !strings.Contains(job.Steps[0].Run, "exit 1") {
			t.Fatalf("%s: the first step must refuse any other event with exit 1", name)
		}
		for _, s := range job.Steps {
			for _, banned := range []string{"upload-artifact", "GITHUB_STEP_SUMMARY", "docker.sock", "LANGSMITH", "LANGCHAIN"} {
				if strings.Contains(s.Uses+s.Run, banned) {
					t.Fatalf("%s step %q uses %s", name, s.Name, banned)
				}
			}
			if strings.Contains(s.Run, "::error") && name == "plugins" {
				t.Fatalf("plugins step %q writes an annotation", s.Name)
			}
		}
	}
	if !strings.Contains(string(mustRead(t)), "timeout-minutes: ${{ matrix.plugin.job_timeout_minutes }}") {
		t.Fatal("the plugins job timeout must be the plugin's plus the slack")
	}
	var ran, uploaded, counted bool
	for _, s := range run.Steps {
		if s.Name == "Run plugin" {
			ran = strings.Contains(s.Run, `if [ "${rc}" -eq 124 ]; then`) && strings.Contains(s.Run, `>"${RUNNER_TEMP}/plugin-${PLUGIN_NAME}.log" 2>&1`) && strings.Contains(s.Run, `-v "${copy}:/alvo:ro"`)
		}
		if strings.HasPrefix(s.Uses, "github/codeql-action/upload-sarif") {
			uploaded = pinnedUpload.MatchString(strings.Fields(s.Uses)[0]) && s.If == "success() && steps.destination.outputs.destino == 'security'" &&
				s.With["category"] == "aurumcode/plugin/${{ matrix.plugin.name }}"
		}
		if s.Name == "Count sent alerts" {
			counted = strings.Contains(s.Run, "alertas enviados para a aba Security (visível só para quem tem escrita)")
		}
	}
	if !ran || !uploaded || !counted {
		t.Fatalf("run redirected=%v upload pinned/success/category=%v count=%v", ran, uploaded, counted)
	}
}
