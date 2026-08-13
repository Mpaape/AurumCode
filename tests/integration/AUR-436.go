package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func aur436IntegrationRoot(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

// aur436ScrubEnv returns the process environment with every provider
// selection variable removed, so each run states its configuration
// explicitly.
func aur436ScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE": true,
		"LLM_API_KEY":           true,
		"LLM_BASE_URL":          true,
		"LLM_MODEL":             true,
	}
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !drop[name] {
			env = append(env, kv)
		}
	}
	return env
}

// aur436ModelRecorder is a local, loopback-only OpenAI-compatible endpoint
// (the same wire shape internal/llm/provider/litellm speaks). It records
// the "model" field of every completion request so the test can prove the
// name typed after --modelo is the name that actually reaches a local
// endpoint -- the litellm-to-local-model path the card promises, with no
// external network involved.
type aur436ModelRecorder struct {
	mu     sync.Mutex
	models []string
}

func (r *aur436ModelRecorder) record(model string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models = append(r.models, model)
}

func (r *aur436ModelRecorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.models) == 0 {
		return ""
	}
	return r.models[len(r.models)-1]
}

// IntegrationAUR436 exercises --modelo end to end against a real local
// HTTP endpoint: build the real cmd/aurumcode binary, point LLM_BASE_URL
// at an httptest server on loopback, and assert that the model named by
// --modelo is the model sent on the wire; that --modelo overrides
// LLM_MODEL; that without --modelo the pre-existing selection is
// untouched; and that an unreachable local endpoint yields the clear
// unavailability error, never an empty review with exit 0.
func IntegrationAUR436(t *testing.T) {
	root := aur436IntegrationRoot(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur436")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	// The review the local endpoint answers with: one finding, in the
	// shape internal/prompt.ResponseParser validates. Marshaled with
	// encoding/json so the JSON-inside-JSON nesting cannot drift.
	reviewJSON, err := json.Marshal(map[string]interface{}{
		"issues": []map[string]interface{}{
			{
				"file":     "config/demo-tokens.txt",
				"line":     4,
				"severity": "warning",
				"message":  "A planted, synthetic finding served by the local endpoint.",
			},
		},
		"summary": "Deterministic local-endpoint response for AUR-436.",
	})
	if err != nil {
		t.Fatalf("marshaling review payload: %v", err)
	}

	recorder := &aur436ModelRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		recorder.record(body.Model)
		resp := map[string]interface{}{
			"id":      "chatcmpl-aur436",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   body.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": string(reviewJSON)},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encoding completion response: %v", err)
		}
	}))
	defer server.Close()

	run := func(extraEnv []string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur436ScrubEnv(), extraEnv...)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("running %v: %v", args, err)
			}
			code = ee.ExitCode()
		}
		return code, stdout.String(), stderr.String()
	}

	liveEnv := []string{"LLM_API_KEY=test-key", "LLM_BASE_URL=" + server.URL}

	// The name typed after --modelo is the model sent to the local
	// endpoint.
	code, stdout, stderr := run(liveEnv, "--modelo", "llama3-local")
	if code != 0 {
		t.Fatalf("expected exit 0 via the local endpoint, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if got := recorder.last(); got != "llama3-local" {
		t.Fatalf("expected the wire to carry model %q, got %q", "llama3-local", got)
	}
	if !strings.Contains(stdout, "config/demo-tokens.txt") {
		t.Fatalf("expected the local endpoint's finding on stdout, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, `reviewing with model "llama3-local"`) {
		t.Fatalf("expected the selection note on stderr, got:\n%s", stderr)
	}

	// --modelo overrides LLM_MODEL: the flag, not the environment,
	// commands the selection.
	code, _, _ = run(append(liveEnv, "LLM_MODEL=gpt-4"), "--modelo", "phi3-mini")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if got := recorder.last(); got != "phi3-mini" {
		t.Fatalf("--modelo must override LLM_MODEL: expected %q on the wire, got %q", "phi3-mini", got)
	}

	// Without --modelo the pre-existing selection is untouched: LLM_MODEL
	// decides, and no selection note appears.
	code, _, stderr = run(append(liveEnv, "LLM_MODEL=demo-model"))
	if code != 0 {
		t.Fatalf("expected exit 0 without --modelo, got %d", code)
	}
	if got := recorder.last(); got != "demo-model" {
		t.Fatalf("without --modelo LLM_MODEL must decide: expected %q, got %q", "demo-model", got)
	}
	if strings.Contains(stderr, "reviewing with model") {
		t.Fatalf("without --modelo no selection note may appear, got:\n%s", stderr)
	}

	// An unreachable local endpoint is an unavailable model: clear error,
	// exit 1, never an empty review. The server is closed first so its
	// port refuses connections.
	deadURL := server.URL
	server.Close()
	code, stdout, stderr = run([]string{"LLM_API_KEY=test-key", "LLM_BASE_URL=" + deadURL}, "--modelo", "llama3-local")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unreachable endpoint, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "No issues found.") {
		t.Fatalf("an unreachable endpoint must not report an empty review, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, `model "llama3-local" is unavailable`) {
		t.Fatalf("expected the error to name the model, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "LLM_BASE_URL") {
		t.Fatalf("expected the error to say how to fix the endpoint, got:\n%s", stderr)
	}
}
