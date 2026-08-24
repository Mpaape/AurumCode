// AUR-460 integration selector. DISTINCT ASSERTION: the COMPOSITION of
// internal/review.Reviewer, internal/llm.Orchestrator and the real
// internal/llm/provider/litellm.Provider against a gateway fixture that
// reproduces the measured defect -- it 400s on ANY request that carries an
// explicit "temperature" key, exactly like the gpt-5.6-luna/sol/terra
// family the 2026-08-14 live-gateway test hit. tests/unit/AUR-460.go
// already proves the raw JSON never carries the key in isolation; this
// program proves that guarantee survives the whole review pipeline -- diff
// analysis, prompt building, the orchestrator's budget/fallback wrapping,
// and result parsing -- and that GenerateReview still returns real
// findings end to end. tests/e2e/AUR-460.sh proves the same thing one
// layer further out, through the compiled CLI binary as a subprocess.
package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/review"
)

func aur460Root(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// restrictiveGatewayServer answers exactly like the measured gpt-5.6-luna
// gateway: a request whose raw body contains "temperature" gets the exact
// 400 the achado recorded; a request without it gets a real completion
// carrying the same finding tests/fixtures/review/known-problem-response.json
// does, so a passing test proves both "no temperature sent" AND "the
// review still produced its finding", not just the former in isolation.
func restrictiveGatewayServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var lastBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		lastBody = string(raw)

		if strings.Contains(lastBody, `"temperature"`) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported value: 'temperature' does not support 0.3 with this model. Only the default (1) value is supported."}}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "cmpl-aur460",
			"object": "chat.completion",
			"created": 0,
			"model": "gpt-5.6-luna",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "{\"issues\":[{\"file\":\"config/demo-tokens.txt\",\"line\":3,\"severity\":\"error\",\"rule_id\":\"security/hardcoded-secret\",\"message\":\"A credential-shaped value was committed in plain text.\",\"suggestion\":\"Remove the secret and rotate it.\"}],\"summary\":\"One planted credential was found.\"}"
				},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
		}`))
	}))
	return server, &lastBody
}

// IntegrationAUR460 proves the review pipeline reaches a gateway-served
// model family the pre-fix fixed Temperature default made unreachable, and
// still returns the real finding the model reported.
func IntegrationAUR460(t *testing.T) {
	root := aur460Root(t)
	demo := filepath.Join(root, "tests", "fixtures", "repos", "git-demo", "repo.git")
	if _, err := os.Stat(demo); err != nil {
		t.Skipf("infrastructure: demo fixture missing: %v", err)
	}

	repo, err := analyzer.OpenRepo(demo)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	diff, _, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	server, lastBody := restrictiveGatewayServer(t)
	defer server.Close()

	provider := litellm.NewProvider("test-key", server.URL, "gpt-5.6-luna")
	orch := llm.NewOrchestrator(provider, nil, nil)
	reviewer := review.NewReviewer(orch, review.DefaultConfig())

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview failed against the restrictive gateway fixture: %v -- last request body: %s", err, *lastBody)
	}

	if strings.Contains(*lastBody, `"temperature"`) {
		t.Fatalf("the request that reached the gateway carried a temperature key: %s", *lastBody)
	}

	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue from the gateway-served finding, got %d: %+v", len(result.Issues), result.Issues)
	}
	issue := result.Issues[0]
	if issue.File != "config/demo-tokens.txt" || issue.Line != 3 || issue.Severity != "error" {
		t.Fatalf("unexpected issue composed through the pipeline: %+v", issue)
	}
}
