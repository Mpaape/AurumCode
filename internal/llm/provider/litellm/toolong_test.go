package litellm

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// The sizes of a refusal for length sit past the bound a log line gets, so
// the provider reads them from the whole message before bounding it.
func TestRefusalForLengthKeepsItsSizesPastTheLogBound(t *testing.T) {
	filler := strings.Repeat("padding ", 60)
	body := `{"error":{"type":"invalid_request_error","message":"ContextWindowExceededError ` + filler + `This model's maximum context length is 128000 tokens. However, your messages resulted in 1258937 tokens."}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	_, err := NewProvider("test-key", server.URL, "test-model").Complete("hello", llm.Options{})
	if err == nil {
		t.Fatal("a refused request returned no error")
	}
	var api *llm.APIError
	if !errors.As(err, &api) {
		t.Fatalf("error %q is not an *llm.APIError", err)
	}
	if api.Status != http.StatusBadRequest || !strings.HasSuffix(api.Message, "... (truncated)") {
		t.Fatalf("status %d, message %q: the text must stay bounded as before", api.Status, api.Message)
	}
	got, ok := llm.AsPromptTooLong(err)
	if !ok || got.Limit != 128000 || got.Requested != 1258937 {
		t.Fatalf("AsPromptTooLong = %+v, %v; want the sizes the provider sent", got, ok)
	}
}
