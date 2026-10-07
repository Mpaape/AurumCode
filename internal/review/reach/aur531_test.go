package reach

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/llm"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
)

// scripted asks for one tool, then answers.
type scripted struct {
	call   llm.ToolCall
	answer string
	seen   []llm.Message
}

func (s *scripted) CompleteWithTools(_ context.Context, m []llm.Message, _ []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	s.seen = m
	for _, msg := range m {
		if msg.Role == llm.RoleTool {
			return llm.ToolResponse{Response: llm.Response{Text: s.answer, TokensIn: 1, TokensOut: 1}}, nil
		}
	}
	return llm.ToolResponse{Response: llm.Response{TokensIn: 1, TokensOut: 1}, ToolCalls: []llm.ToolCall{s.call}}, nil
}

func revision(t *testing.T, files map[string]string) *reviewtools.Revision {
	t.Helper()
	root := t.TempDir()
	tracked := map[string]analyzer.TrackedEntry{}
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		h := sha1.New()
		h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00" + content))
		tracked[rel] = analyzer.TrackedEntry{SHA: hex.EncodeToString(h.Sum(nil)), Mode: "100644"}
	}
	rev, err := reviewtools.NewRevision(reviewtools.RevisionOptions{Root: root, Tracked: tracked, Secret: func(string) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

var request = Request{Package: "pyyaml", Ecosystem: "PyPI", Manifest: "requirements.txt", AdvisoryID: "GHSA-test-0001", Summary: "yaml.load executes arbitrary code"}

func explain(t *testing.T, files map[string]string, answer string) (Explanation, *scripted) {
	t.Helper()
	rev := revision(t, files)
	model := &scripted{call: llm.ToolCall{ID: "c1", Name: reviewtools.SearchToolName, Arguments: json.RawMessage(`{"query":"yaml.load"}`)}, answer: answer}
	e := Explainer{
		Caller: model,
		Tools:  reviewtools.Tools(reviewtools.RepositoryOffers(rev, nil, grammar.Default(), nil)),
		Limits: deliberation.Limits{MaxRounds: 3, MaxCostTokens: 10000, PerToolTimeout: 5 * time.Second},
		Read:   rev.Read,
	}
	return e.Explain(context.Background(), request), model
}

const usesFixture = "import yaml\n\ndef load(text):\n    return yaml.load(text)\n"

// AC-001: the code imports and calls the vulnerable function: the
// explanation cites file and line, found through the repository tools.
func TestAUR531UseIsCitedWithFileAndLine(t *testing.T) {
	x, model := explain(t, map[string]string{"app/config.py": usesFixture}, `{"uses":"yes","locations":[{"file":"app/config.py","line":4}],"explanation":"load recebe texto do usuario"}`)
	if x.Uses != UsesYes || len(x.Locations) != 1 || x.Locations[0].Line != 4 {
		t.Fatalf("explanation = %+v", x)
	}
	if tool := model.seen[len(model.seen)-1]; !strings.Contains(tool.Content, "app/config.py:4:") {
		t.Fatalf("the search never reached the model: %+v", tool)
	}
	if line := Line(x, "pt-BR"); !strings.Contains(line, "usado em app/config.py:4") || !strings.Contains(line, "o achado, a severidade e o veredito não mudam") {
		t.Fatalf("line = %q", line)
	}
}

// AC-002: no use: the explanation says so, a location the revision does
// not hold is discarded, and the line still says the finding stands.
func TestAUR531NoUseSaysSoAndFindingStands(t *testing.T) {
	x, _ := explain(t, map[string]string{"app/main.py": "print('hi')\n"}, `{"uses":"no","locations":[],"explanation":"o pacote nao e importado"}`)
	if x.Uses != UsesNo || !strings.Contains(Line(x, "pt-BR"), "o modelo não achou uso") || !strings.Contains(Line(x, "pt-BR"), "não mudam") {
		t.Fatalf("explanation = %+v line=%q", x, Line(x, "pt-BR"))
	}
	invented, _ := explain(t, map[string]string{"app/main.py": "print('hi')\n"}, `{"uses":"yes","locations":[{"file":"app/ghost.py","line":9}],"explanation":"x"}`)
	if invented.Uses != UsesUnknown || len(invented.Discarded) != 1 || len(invented.Locations) != 0 {
		t.Fatalf("an invented location was kept: %+v", invented)
	}
}

// AC-003: an answer that tries to lower the severity or approve carries no
// field the explanation reads; a malformed answer is never "no use".
func TestAUR531DowngradeAttemptIsIgnored(t *testing.T) {
	x, _ := explain(t, map[string]string{"app/main.py": "print('hi')\n"}, `{"uses":"no","locations":[],"explanation":"sem uso","severity":"low","verdict":"approve","suppress":true}`)
	raw, _ := json.Marshal(x)
	for _, k := range []string{`"low"`, "approve", "suppress"} {
		if strings.Contains(string(raw), k) {
			t.Fatalf("the downgrade attempt reached the explanation: %s", raw)
		}
	}
	bad, _ := explain(t, map[string]string{"app/main.py": "print('hi')\n"}, `nao e json`)
	if bad.Uses != UsesUnknown || bad.Reason == "" || strings.Contains(Line(bad, "pt-BR"), "não achou uso") {
		t.Fatalf("a malformed answer read as no use: %+v", bad)
	}
}

// The explanation line follows the review language through the i18n
// catalog: Portuguese with accents, English otherwise.
func TestAUR531LineFollowsTheReviewLanguage(t *testing.T) {
	x := Explanation{Request: request, Uses: UsesNo, Text: "-"}
	if pt := Line(x, "pt-BR"); !strings.Contains(pt, "Alcance de GHSA-test-0001") || !strings.Contains(pt, "não mudam") {
		t.Fatalf("pt-BR line = %q", pt)
	}
	if en := Line(x, "en-US"); !strings.Contains(en, "Reach of GHSA-test-0001") || !strings.Contains(en, "the model found no use") || strings.Contains(en, "não") {
		t.Fatalf("en line = %q", en)
	}
}
