package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The golden files under testdata/promptgolden pin, byte for byte, the
// prompt text the engine produced before the review prompt became a
// template with named slots. Every section that already existed must keep
// rendering identically; only new, empty-by-default slots may be added.
// Regenerate deliberately with AURUM_UPDATE_PROMPT_GOLDEN=1.
const updatePromptGoldenEnv = "AURUM_UPDATE_PROMPT_GOLDEN"

func assertPromptGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "promptgolden", name)
	if os.Getenv(updatePromptGoldenEnv) == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v", path, err)
	}
	if string(want) != got {
		t.Fatalf("%s drifted from its golden bytes (len got=%d want=%d); first difference at byte %d", name, len(got), len(want), firstDifference(string(want), got))
	}
}

func firstDifference(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// goldenDiff mixes a source file with many hunks (so a string sort key
// "x:10" < "x:2" would reorder it), a test file, a config file and a
// documentation file.
func goldenDiff() *types.Diff {
	var hunks []types.DiffHunk
	for i := 0; i < 12; i++ {
		hunks = append(hunks, types.DiffHunk{
			OldStart: i*10 + 1, OldLines: 1, NewStart: i*10 + 1, NewLines: 2,
			Lines: []string{fmt.Sprintf(" func f%d() {", i), fmt.Sprintf("+\treturn %d", i)},
		})
	}
	return &types.Diff{Files: []types.DiffFile{
		{Path: "internal/app/service.go", Hunks: hunks},
		{Path: "internal/app/service_test.go", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, NewLines: 1, Lines: []string{"+func TestF(t *testing.T) {}"}}}},
		{Path: "config/app.yml", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, NewLines: 1, Lines: []string{"+timeout: 5"}}}},
		{Path: "docs/guide.md", Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, NewLines: 1, Lines: []string{"+A guide line."}}}},
	}}
}

func goldenContexts() map[string]ReviewContext {
	return map[string]ReviewContext{
		"empty": {},
		"full": {
			CI:              "- build: failure",
			Language:        "pt-BR",
			History:         "- reviewer said: check the loop",
			CodebaseContext: "- service.go is called by handler.go",
			MemoryNotes:     "- prior note: keep f0 pure",
		},
	}
}

func TestPromptGoldenBuildPrompt(t *testing.T) {
	diff := goldenDiff()
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	for name, rc := range goldenContexts() {
		parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
			SchemaKind: "review", Role: "reviewer",
			CIContext: rc.CI, ReviewHistory: rc.History, CodebaseContext: rc.CodebaseContext,
			MemoryNotes: rc.MemoryNotes, Language: rc.Language,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertPromptGolden(t, "buildprompt_"+name+"_system.txt", parts.System)
		assertPromptGolden(t, "buildprompt_"+name+"_user.txt", parts.User)
	}
}

type goldenContextProvider struct{ name, text string }

func (p goldenContextProvider) Name() string { return p.name }
func (p goldenContextProvider) Provide(context.Context, []string) (string, error) {
	return p.text, nil
}

func goldenContextProviders() []config.ContextProvider {
	return []config.ContextProvider{
		goldenContextProvider{name: ".aurumcode/prompt.md", text: "Prefer small functions."},
		goldenContextProvider{name: ".aurumcode/instructions/go.md", text: "Wrap errors with context."},
	}
}

func TestPromptGoldenContextBlock(t *testing.T) {
	block, warnings, err := config.BuildContextBlockWithWarnings(context.Background(), goldenContextProviders(), []string{"internal/app/service.go"}, redaction.NewFilter())
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	assertPromptGolden(t, "context_block.txt", block)
}

// TestPromptGoldenCapturedRequest pins the exact text a provider without a
// message capability receives for a full review, with and without the
// repository context block, under DefaultConfig.
func TestPromptGoldenCapturedRequest(t *testing.T) {
	for name, rc := range goldenContexts() {
		for _, withBlock := range []string{"plain", "repoctx"} {
			capture := filepath.Join(t.TempDir(), "prompt.txt")
			fake := &FakeProvider{Response: `{"verdict":"approve","issues":[],"summary":"ok"}`, CapturePath: capture}
			var provider llm.Provider = fake
			if withBlock == "repoctx" {
				wrapped, err := config.WrapProvider(context.Background(), fake, goldenContextProviders(), []string{"internal/app/service.go"}, redaction.NewFilter())
				if err != nil {
					t.Fatal(err)
				}
				provider = wrapped
			}
			reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil), DefaultConfig())
			if _, err := reviewer.GenerateReviewWithContext(context.Background(), goldenDiff(), rc); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(got)) == "" {
				t.Fatal("empty capture")
			}
			assertPromptGolden(t, "capture_"+name+"_"+withBlock+".txt", string(got))
		}
	}
}
