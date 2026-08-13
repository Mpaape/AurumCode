package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func aur433IntegrationRoot(t *testing.T) string {
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

// aur433ScrubEnv returns the process environment with every provider
// selection or cost pricing variable removed, so each run states its
// configuration explicitly.
func aur433ScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":           true,
		"AURUMCODE_PROMPT_CAPTURE":        true,
		"LLM_API_KEY":                     true,
		"LLM_BASE_URL":                    true,
		"LLM_MODEL":                       true,
		"AURUMCODE_LLM_INPUT_USD_PER_1K":  true,
		"AURUMCODE_LLM_OUTPUT_USD_PER_1K": true,
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

// IntegrationAUR433 exercises --limite end to end against a real local HTTP
// endpoint (the same wire shape internal/llm/provider/litellm speaks, no
// external network involved): it counts the literal number of completion
// requests the endpoint receives, and proves that number is exactly zero
// for a run whose estimate exceeds --limite (MUT-001's core requirement,
// stated as a real, numeric call counter rather than the offline fixture's
// capture-file proxy tests/unit/AUR-433.go and the acceptance script use),
// and exactly one for a run comfortably under it.
func IntegrationAUR433(t *testing.T) {
	root := aur433IntegrationRoot(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur433")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	// The review the local endpoint answers with: one finding, in the shape
	// internal/prompt.ResponseParser validates, citing an embedded rule so
	// it survives the AUR-434 rule-citation gate.
	reviewJSON, err := json.Marshal(map[string]interface{}{
		"issues": []map[string]interface{}{
			{
				"file":     "config/demo-tokens.txt",
				"line":     4,
				"severity": "warning",
				"rule_id":  "security/hardcoded-secret",
				"message":  "A planted, synthetic finding served by the local endpoint.",
			},
		},
		"summary": "Deterministic local-endpoint response for AUR-433.",
	})
	if err != nil {
		t.Fatalf("marshaling review payload: %v", err)
	}

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		requests.Add(1)
		resp := map[string]interface{}{
			"id":      "chatcmpl-aur433",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "aur433-local",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": string(reviewJSON)},
					"finish_reason": "stop",
				},
			},
			// A large usage so a naive "trust the response" cost accounting
			// would look expensive; the point of this test is that the
			// REQUEST never lands at all when the pre-call estimate refuses,
			// so this figure cannot leak into a spent amount either way.
			"usage": map[string]int{"prompt_tokens": 500, "completion_tokens": 500, "total_tokens": 1000},
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
		cmd.Env = append(aur433ScrubEnv(), extraEnv...)
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

	// Over-limit: the estimate is checked before the model is ever called
	// (internal/llm.Orchestrator.Complete reserves before invoking
	// provider.Complete), so refusing must mean the HTTP request never
	// reaches the endpoint at all -- the literal, numeric call counter
	// MUT-001 requires.
	code, stdout, stderr := run(liveEnv, "--modelo", "aur433-local", "--limite", "0.0001")
	if code != 1 {
		t.Fatalf("expected exit 1 for an over-limit run, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("MUT-001: expected zero requests to reach the provider when over --limite, got %d", got)
	}
	if !strings.Contains(stderr, "refusing to call the model") {
		t.Fatalf("expected a clear refusal message, got:\n%s", stderr)
	}

	// Comfortably under the limit: exactly one request reaches the
	// endpoint, and the review completes normally.
	code, stdout, stderr = run(liveEnv, "--modelo", "aur433-local", "--limite", "50")
	if code != 0 {
		t.Fatalf("expected exit 0 via the local endpoint, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected exactly one request to reach the provider, got %d", got)
	}
	if !strings.Contains(stdout, "config/demo-tokens.txt") {
		t.Fatalf("expected the local endpoint's finding on stdout, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "actual cost") {
		t.Fatalf("expected the actual-cost line after a successful call, got:\n%s", stderr)
	}
}
