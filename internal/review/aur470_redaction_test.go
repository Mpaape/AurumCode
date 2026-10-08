package review

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// AUR-470: the snippets are raw code of other files; the codebase context
// that carries them passes the AUR-009 filter before the prompt leaves.
func TestAUR470SnippetsAreRedactedBeforeTheModel(t *testing.T) {
	canary := "snip" + "pet-canary-" + "470-zz"
	t.Setenv(redaction.CanaryEnv, canary)
	root := t.TempDir()
	files := map[string]string{
		"src/shop/Contract.java": "package shop;\n\npublic class Contract {\n    public static long priceOf(String sku, String currency) { return 1L; }\n}\n",
		"src/shop/Caller.java":   "package shop;\n\nclass Caller {\n    String key = \"" + canary + "\";\n    long t() { return Contract.priceOf(key); }\n}\n",
	}
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pack, err := codebasectx.NewResolver().Resolve(root, []string{"src/shop/Contract.java"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(pack)
	if !strings.Contains(string(raw), canary) || len(pack.Snippets) == 0 {
		t.Fatalf("fixture: the snippet must carry the canary before redaction: %s", raw)
	}
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	provider := &FakeProvider{Response: `{"issues":[],"summary":"ok"}`, CapturePath: capture}
	reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil), DefaultConfig())
	if _, err := reviewer.GenerateReviewWithContext(context.Background(), aur526Diff(), ReviewContext{CodebaseContext: string(raw)}); err != nil {
		t.Fatal(err)
	}
	sent, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sent), canary) || !strings.Contains(string(sent), "src/shop/Caller.java") {
		t.Fatalf("the snippet reached the model unredacted (or not at all):\n%s", sent)
	}
}
