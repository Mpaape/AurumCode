package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func aur432Root(t *testing.T) string {
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

// aur432ScrubEnv returns the process environment with every variable that
// influences provider selection, prompt capture, or secret registration
// removed, so each run states its configuration explicitly.
func aur432ScrubEnv() []string {
	drop := map[string]bool{
		"AURUMCODE_LLM_FIXTURE":   true,
		"AURUMCODE_PROMPT_CAPTURE": true,
		"AURUM_SECRET_CANARY":     true,
		"LLM_API_KEY":             true,
		"LLM_BASE_URL":            true,
		"LLM_MODEL":               true,
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

// aur432BodyRecorder records the raw completion request bodies a local
// OpenAI-compatible endpoint receives, so the test can prove at the wire
// level that the prompt leaving the process carries no secret.
type aur432BodyRecorder struct {
	mu     sync.Mutex
	bodies []string
}

func (r *aur432BodyRecorder) record(body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bodies = append(r.bodies, body)
}

func (r *aur432BodyRecorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		return ""
	}
	return r.bodies[len(r.bodies)-1]
}

// IntegrationAUR432 proves AUR-432's outcome through the real binary: the
// planted synthetic secrets of tests/fixtures/repos/git-demo never reach
// the provider (observed via the offline provider's prompt capture and via
// the raw bytes a local loopback endpoint receives), never reach stdout or
// stderr even when the model echoes them back, and the endpoint note never
// reveals a password carried in LLM_BASE_URL userinfo.
func IntegrationAUR432(t *testing.T) {
	root := aur432Root(t)
	repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
	if _, err := os.Stat(repoDir); err != nil {
		t.Fatalf("required input missing: %s: %v", repoDir, err)
	}
	echoFixture := filepath.Join(root, "tests/fixtures/review/secret/response-echoes-secret.json")
	if _, err := os.Stat(echoFixture); err != nil {
		t.Fatalf("required input missing: %s: %v", echoFixture, err)
	}

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur432")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}

	planted := []string{
		"AURUM-FAKE-TOKEN-0000-0001",
		"AURUM-FAKE-PASSWORD-0000-0002",
		"AURUM-FAKE-WEBHOOK-0000-0003",
	}
	canary := planted[2]

	run := func(extraEnv []string, extraArgs ...string) (int, string, string) {
		args := append([]string{"review", "--base", "HEAD~1"}, extraArgs...)
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = append(aur432ScrubEnv(), extraEnv...)
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

	// Offline provider with prompt capture: the prompt that reached the
	// provider must carry no planted value, and the echoed values must not
	// reach stdout or stderr.
	capturePath := filepath.Join(t.TempDir(), "prompt.txt")
	offlineEnv := []string{
		"AURUMCODE_LLM_FIXTURE=" + echoFixture,
		"AURUMCODE_PROMPT_CAPTURE=" + capturePath,
		"AURUM_SECRET_CANARY=" + canary,
	}
	code, stdout, stderr := run(offlineEnv)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr=%s", code, stderr)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("the prompt capture was never written: %v", err)
	}
	prompt := string(captured)
	for _, secret := range planted {
		if strings.Contains(prompt, secret) {
			t.Error("a planted secret reached the provider prompt")
		}
		if strings.Contains(stdout, secret) {
			t.Error("a planted secret reached stdout")
		}
		if strings.Contains(stderr, secret) {
			t.Error("a planted secret reached stderr")
		}
	}
	if !strings.Contains(prompt, "[REDACTED]") {
		t.Error("the prompt carries no redaction marker where the secrets were")
	}
	for _, context := range []string{"config/demo-tokens.txt", "DEMO_API_TOKEN="} {
		if !strings.Contains(prompt, context) {
			t.Errorf("redaction destroyed review context %q in the prompt", context)
		}
	}
	if !strings.Contains(stdout, "config/demo-tokens.txt:6: [error]") {
		t.Fatalf("the finding citing the secret line was lost:\n%s", stdout)
	}
	if !strings.Contains(stdout, "DEMO_WEBHOOK_SECRET=[REDACTED]") {
		t.Fatalf("the echoed secret was not redacted on stdout:\n%s", stdout)
	}

	// Determinism: same input, same redacted prompt, same output.
	codeAgain, stdoutAgain, _ := run(offlineEnv)
	promptAgain, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("second prompt capture: %v", err)
	}
	if codeAgain != code || stdoutAgain != stdout || string(promptAgain) != prompt {
		t.Error("the redacted review is not deterministic")
	}

	// Wire level: a local OpenAI-compatible endpoint receives the request
	// bytes; no planted value may be in them. The endpoint's own response
	// echoes a planted value, which must still be redacted on stdout. The
	// endpoint URL carries a synthetic userinfo password that must never
	// reach stderr (the selection note prints the endpoint).
	reviewJSON, err := json.Marshal(map[string]interface{}{
		"issues": []map[string]interface{}{{
			"file":     "config/demo-tokens.txt",
			"line":     4,
			"severity": "error",
			"rule_id":  "security/hardcoded-secret",
			"message":  "Committed plaintext DEMO_API_TOKEN=" + planted[0] + " must be rotated.",
		}},
		"summary": "Deterministic local-endpoint response for AUR-432.",
	})
	if err != nil {
		t.Fatalf("marshaling review payload: %v", err)
	}
	recorder := &aur432BodyRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		recorder.record(string(raw))
		resp := map[string]interface{}{
			"id":      "chatcmpl-aur432",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "wired-local",
			"choices": []map[string]interface{}{{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": string(reviewJSON)},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encoding completion response: %v", err)
		}
	}))
	defer server.Close()

	fakePassword := "fake-" + "wire-pass-0421"
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parsing httptest URL: %v", err)
	}
	endpoint.User = url.UserPassword("aurum", fakePassword)

	wireEnv := []string{
		"LLM_API_KEY=test-key",
		"LLM_BASE_URL=" + endpoint.String(),
	}
	code, stdout, stderr = run(wireEnv, "--modelo", "wired-local")
	if code != 0 {
		t.Fatalf("expected exit 0 via the local endpoint, got %d\nstderr=%s", code, stderr)
	}
	body := recorder.last()
	if body == "" {
		t.Fatal("the local endpoint never received a completion request")
	}
	for _, secret := range planted {
		if strings.Contains(body, secret) {
			t.Error("a planted secret left the process on the wire")
		}
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Error("the wire request carries no redaction marker where the secrets were")
	}
	if !strings.Contains(stdout, "DEMO_API_TOKEN=[REDACTED]") {
		t.Fatalf("the endpoint-echoed secret was not redacted on stdout:\n%s", stdout)
	}
	if strings.Contains(stderr, fakePassword) {
		t.Error("the endpoint password reached stderr")
	}
	if !strings.Contains(stderr, `reviewing with model "wired-local"`) {
		t.Fatalf("the selection note is missing from stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, endpoint.Host) {
		t.Errorf("redaction destroyed the endpoint host in the selection note:\n%s", stderr)
	}
}
