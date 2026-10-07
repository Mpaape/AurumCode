package review

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur526Answer anchors the contract finding on the changed line and
// reports the untouched caller it read through the tool.
const aur526Answer = `{
  "issues": [
    {"file": "src/shop/Contract.java", "line": 5, "severity": "error",
     "rule_id": "quality/dead-code",
     "message": "priceOf now requires a currency, but Caller.total still calls it with only the sku.",
     "impact": "The caller no longer compiles.",
     "evidence": "find_symbol: src/shop/Caller.java:6 calls Contract.priceOf(sku).",
     "verification": "Compile Caller.java."},
    {"file": "src/shop/Caller.java", "line": 6, "severity": "error",
     "rule_id": "quality/dead-code",
     "message": "This call passes one argument to a two-argument method.",
     "impact": "Build break.",
     "evidence": "Caller.java line 6 reads Contract.priceOf(sku).",
     "verification": "Compile Caller.java."}
  ],
  "summary": "Contract changed; the caller outside the diff was read."
}`

// toolModel asks for one tool, then answers.
type toolModel struct {
	FakeProvider
	call   llm.ToolCall
	answer string
	seen   [][]llm.Message
}

func (m *toolModel) CompleteWithTools(messages []llm.Message, _ []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	m.seen = append(m.seen, messages)
	for _, msg := range messages {
		if msg.Role == llm.RoleTool {
			return llm.ToolResponse{Response: llm.Response{Text: m.answer, TokensIn: 10, TokensOut: 10}}, nil
		}
	}
	return llm.ToolResponse{Response: llm.Response{TokensIn: 10, TokensOut: 1}, ToolCalls: []llm.ToolCall{m.call}}, nil
}

func aur526Revision(t *testing.T, budget int) *reviewtools.Revision {
	t.Helper()
	root := t.TempDir()
	tracked := map[string]analyzer.TrackedEntry{}
	for _, name := range []string{"Contract.java", "Caller.java"} {
		data, err := os.ReadFile(filepath.Join("tools", "testdata", "aur526", name))
		if err != nil {
			t.Fatal(err)
		}
		rel := "src/shop/" + name
		if err := os.MkdirAll(filepath.Join(root, "src", "shop"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), data, 0o644); err != nil {
			t.Fatal(err)
		}
		h := sha1.New()
		h.Write([]byte("blob " + strconv.Itoa(len(data)) + "\x00"))
		h.Write(data)
		tracked[rel] = analyzer.TrackedEntry{SHA: hex.EncodeToString(h.Sum(nil)), Mode: "100644"}
	}
	rev, err := reviewtools.NewRevision(reviewtools.RevisionOptions{Root: root, Tracked: tracked, Secret: func(string) bool { return false }, Budget: reviewtools.NewByteBudget(budget)})
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func aur526Diff() *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{Path: "src/shop/Contract.java", Lang: "java", Hunks: []types.DiffHunk{{
		OldStart: 5, OldLines: 1, NewStart: 5, NewLines: 1,
		Lines: []string{"-    public static long priceOf(String sku) {", "+    public static long priceOf(String sku, String currency) {"},
	}}}}}
}

func aur526Review(t *testing.T, budget int, call llm.ToolCall) (*types.ReviewResult, *toolModel, *Reviewer, error) {
	t.Helper()
	model := &toolModel{call: call, answer: aur526Answer}
	orch := llm.NewOrchestrator(model, nil, nil)
	offers := reviewtools.RepositoryOffers(aur526Revision(t, budget), aur526Diff(), grammar.Default(), nil)
	reviewer := NewReviewer(orch, DefaultConfig())
	reviewer.SetDeliberation(&Deliberation{
		Caller:  orch,
		Tools:   reviewtools.Tools(offers),
		Limits:  deliberation.Limits{MaxRounds: 4, MaxCostTokens: 100000, PerToolTimeout: 10 * time.Second},
		Partial: reviewtools.PartialLimit(offers),
	})
	res, err := reviewer.GenerateReview(context.Background(), aur526Diff())
	return res, model, reviewer, err
}

// AC-001 and AC-005: the model reads the Java caller outside the diff
// through the tool; the final finding stays anchored on the changed line
// and the caller finding goes through the same outside-diff gate as any
// other finding (never result.Issues).
func TestAUR526JavaCallerReadThroughToolsFindingAnchoredInDiff(t *testing.T) {
	res, model, reviewer, err := aur526Review(t, 0, llm.ToolCall{ID: "c1", Name: reviewtools.SymbolToolName, Arguments: json.RawMessage(`{"name":"priceOf"}`)})
	if err != nil {
		t.Fatal(err)
	}
	last := model.seen[len(model.seen)-1]
	if tool := last[len(last)-1]; tool.Role != llm.RoleTool || !strings.Contains(tool.Content, "src/shop/Caller.java:6") {
		t.Fatalf("the model never received the caller from the tool: %+v", tool)
	}
	if len(res.Issues) != 1 || res.Issues[0].File != "src/shop/Contract.java" || res.Issues[0].Line != 5 {
		t.Fatalf("issues = %+v, want the finding anchored on Contract.java:5", res.Issues)
	}
	for _, o := range OutsideDiffFindings(res) {
		if o.File == "src/shop/Contract.java" {
			t.Fatalf("the anchored finding leaked into the outside channel: %+v", o)
		}
	}
	if tr := reviewer.Transcript(); tr == nil || len(tr.Requested) != 1 || tr.Requested[0] != reviewtools.SymbolToolName {
		t.Fatalf("transcript = %+v", tr)
	}
}

// AC-004: a result that crosses max_read_bytes makes the review partial: a
// deliberation limit error, no answer, the transcript naming the limit.
func TestAUR526ByteCeilingIsADeliberationLimit(t *testing.T) {
	res, _, reviewer, err := aur526Review(t, 64, llm.ToolCall{ID: "c1", Name: reviewtools.ReadFileToolName, Arguments: json.RawMessage(`{"path":"src/shop/Caller.java"}`)})
	var limit *deliberation.LimitError
	if !errors.As(err, &limit) || limit.Limit != reviewtools.LimitMaxReadBytes || res != nil {
		t.Fatalf("err=%v res=%+v, want a max_read_bytes limit and no result", err, res)
	}
	if tr := reviewer.Transcript(); tr.Limit != reviewtools.LimitMaxReadBytes || tr.Outcome != "deliberation_limit:max_read_bytes" {
		t.Fatalf("transcript = %+v", tr)
	}
}
