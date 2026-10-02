// AUR-555: the reusable workflow wires the SBOM (AUR-549) to the
// Dependency-Track gate (AUR-550) with optional secrets. These tests read
// .github/workflows/review.yml as YAML; the workflow path can be
// overridden through AUR555_WORKFLOW so the acceptance script can mutate a
// copy (MUT-001) without touching the real file.
package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"gopkg.in/yaml.v3"
)

const (
	aur555SBOMStep   = "Generate SBOM (Trivy, AUR-549)"
	aur555ReviewStep = "Run and publish code review"
	aur555SignStep   = "Sign SBOM and image (Cosign, AUR-551)"
)

type aur555Step struct {
	ID   string            `yaml:"id"`
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

type aur555Workflow struct {
	On struct {
		WorkflowCall struct {
			Secrets map[string]struct {
				Required *bool `yaml:"required"`
			} `yaml:"secrets"`
		} `yaml:"workflow_call"`
	} `yaml:"on"`
	Jobs map[string]struct {
		Steps []aur555Step `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadAUR555Workflow(t *testing.T) (aur555Workflow, []aur555Step) {
	t.Helper()
	path := os.Getenv("AUR555_WORKFLOW")
	if path == "" {
		path = filepath.Join("..", "..", ".github", "workflows", "review.yml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf aur555Workflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("workflow is not valid YAML: %v", err)
	}
	job, ok := wf.Jobs["review"]
	if !ok {
		t.Fatal("job review missing")
	}
	return wf, job.Steps
}

func aur555Index(t *testing.T, steps []aur555Step, name string) int {
	t.Helper()
	for i, s := range steps {
		if s.Name == name {
			return i
		}
	}
	t.Fatalf("step %q missing", name)
	return -1
}

// AC-001: the SBOM step precedes the review step, and both name the same
// file: the SBOM is written by `aurumcode sbom --repo .` inside the
// reviewed checkout (the path sbom_generator.output_file resolves to) and
// the review container mounts that checkout as its cwd, where the gate
// reads the same relative output_file.
func TestAUR555SBOMStepPrecedesReview(t *testing.T) {
	_, steps := loadAUR555Workflow(t)
	sbom := aur555Index(t, steps, aur555SBOMStep)
	review := aur555Index(t, steps, aur555ReviewStep)
	if sbom >= review {
		t.Fatalf("SBOM step (index %d) must precede the review step (index %d)", sbom, review)
	}
	if strings.Contains(steps[sbom].If, "steps.review") {
		t.Fatalf("SBOM step must not depend on the review outcome: %q", steps[sbom].If)
	}
	if !strings.Contains(steps[sbom].Run, "sbom --repo .") && !strings.Contains(steps[sbom].Run, "sbom_args=(sbom --repo .") {
		t.Fatalf("SBOM step must generate into the checkout root (--repo .)")
	}
	if !strings.Contains(steps[review].Run, `.aurumcode-target:/github/workspace`) ||
		!strings.Contains(steps[review].Run, `-w /github/workspace`) {
		t.Fatalf("review must mount the same checkout the SBOM was written into as its cwd")
	}
	// The SBOM step is the one that owns the checkout directory.
	if steps[sbom].Env["POLICY_REPOSITORY"] == "" {
		t.Fatalf("SBOM step must resolve policy exactly like the review")
	}
}

// AC-002: optional secrets, exposed only to the review step.
func TestAUR555SecretsOptionalAndScopedToReview(t *testing.T) {
	wf, steps := loadAUR555Workflow(t)
	for _, name := range []string{"DTRACK_API_KEY", "DTRACK_PROJECT_ID"} {
		s, ok := wf.On.WorkflowCall.Secrets[name]
		if !ok {
			t.Fatalf("secret %s not declared in workflow_call.secrets", name)
		}
		if s.Required == nil || *s.Required {
			t.Fatalf("secret %s must be declared with required: false", name)
		}
	}
	for _, name := range []string{"LLM_API_KEY", "LLM_BASE_URL"} {
		if s := wf.On.WorkflowCall.Secrets[name]; s.Required == nil || !*s.Required {
			t.Fatalf("secret %s must stay required: true", name)
		}
	}
	review := aur555Index(t, steps, aur555ReviewStep)
	for i, s := range steps {
		for _, key := range []string{"DTRACK_API_KEY", "DTRACK_PROJECT_ID"} {
			_, has := s.Env[key]
			if i == review {
				want := "${{ secrets." + key + " }}"
				if s.Env[key] != want {
					t.Fatalf("review step env %s = %q, want %q", key, s.Env[key], want)
				}
			} else if has {
				t.Fatalf("step %q must not receive %s", s.Name, key)
			}
		}
		if i != review && strings.Contains(s.Run, "secrets.DTRACK") {
			t.Fatalf("step %q must not reference the Dependency-Track secrets", s.Name)
		}
	}
	if !strings.Contains(steps[review].Run, "-e DTRACK_API_KEY") || !strings.Contains(steps[review].Run, "-e DTRACK_PROJECT_ID") {
		t.Fatalf("review docker run must forward both variables into the container")
	}
}

// AC-003: ssor_dtrack on, secret absent. The gate is inconclusive by the
// policy mode and never approved: block fails the run, warn marks it
// inconclusive without an approval and without an upload. (The warn-mode
// case is also covered by TestAUR550SecretMissingWarnsNeverBlocks.)
func TestAUR555MissingSecretIsInconclusiveNeverApproved(t *testing.T) {
	for _, mode := range []string{"block", "warn"} {
		t.Run(mode, func(t *testing.T) {
			fs := &dtrackFakeServer{}
			srv := httptest.NewServer(fs.handler())
			defer srv.Close()
			useFakeDTrackClock(t)
			cleanFixture(t, dtrackConfigYAML(srv.URL, writeSBOMFixture(t), mode))
			t.Setenv("DTRACK_API_KEY", "")
			t.Setenv("DTRACK_PROJECT_ID", "")
			t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))

			var out, errOut strings.Builder
			code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
			all := out.String() + errOut.String()
			if mode == "block" && code == 0 {
				t.Fatalf("block mode must fail with a missing secret; output:\n%s", all)
			}
			if mode == "warn" && code != 0 {
				t.Fatalf("warn mode must not fail; exit=%d output:\n%s", code, all)
			}
			if !strings.Contains(all, "dtrack_secret_missing") {
				t.Fatalf("expected dtrack_secret_missing in output:\n%s", all)
			}
			if strings.Contains(out.String(), "**Verdict:** Approve") {
				t.Fatalf("inconclusive must never read as approved:\n%s", out.String())
			}
			if fs.uploadCalls.Load() != 0 {
				t.Fatalf("no upload may happen without the secrets")
			}
		})
	}
}

// AC-004 and the signing order: without ssor_dtrack nothing new is
// required (both secrets optional, no step needs them), and signing runs
// only after a successful review.
func TestAUR555WorkflowValidWithoutDTrackAndSignsOnlyAfterReview(t *testing.T) {
	wf, steps := loadAUR555Workflow(t)
	for _, name := range []string{"DTRACK_API_KEY", "DTRACK_PROJECT_ID"} {
		if s := wf.On.WorkflowCall.Secrets[name]; s.Required == nil || *s.Required {
			t.Fatalf("%s must be optional so callers without ssor_dtrack pass nothing", name)
		}
	}
	review := aur555Index(t, steps, aur555ReviewStep)
	sign := aur555Index(t, steps, aur555SignStep)
	if sign <= review {
		t.Fatalf("signing (index %d) must come after review (index %d)", sign, review)
	}
	for _, name := range []string{aur555SignStep, "Install cosign (AUR-551)"} {
		cond := steps[aur555Index(t, steps, name)].If
		if !strings.Contains(cond, "steps.review.outcome == 'success'") {
			t.Fatalf("step %q must run only when the review succeeded, got if: %q", name, cond)
		}
	}
}
