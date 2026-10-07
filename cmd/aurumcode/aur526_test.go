package main

// AUR-526 through the real command: the model reads a file of the reviewed
// revision with read_file; max_read_bytes makes the review partial
// (inconclusive, unpublished); the profile decorator keeps tool calling
// visible only when its provider has it.

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

const aur526Config = "deliberation:\n  enabled: true\n  max_rounds: 3\nquality_gates:\n  scanners:\n    - engine: fakescan\n"

// AC-001/AC-002: read_file is offered and returns the reviewed revision's
// own line, numbered, to the model.
func TestAUR526ReadFileReachesTheModelThroughTheCommand(t *testing.T) {
	aur580Setup(t, aur526Config, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"tool":"read_file","arguments":{"path":"app.go","start_line":3,"end_line":3}}]`))
	_, _, errOut, rec := aur580Run(t)
	if rec.Deliberation == nil || !contains(rec.Deliberation.Offered, reviewtools.ReadFileToolName) || !contains(rec.Deliberation.Offered, reviewtools.SymbolToolName) {
		t.Fatalf("repository tools not offered: %+v; stderr=%s", rec.Deliberation, errOut)
	}
	if len(rec.Deliberation.Calls) != 1 || rec.Deliberation.Calls[0].Status != "executed" {
		t.Fatalf("read_file call = %+v; stderr=%s", rec.Deliberation.Calls, errOut)
	}
	if !strings.Contains(errOut, "read_file") || !strings.Contains(errOut, "app.go:3-3") {
		t.Fatalf("stderr lacks the read_file record:\n%s", errOut)
	}
}

// AC-004: crossing max_read_bytes is a deliberation limit: non-zero exit,
// the gate inconclusive with deliberation_limit:max_read_bytes, nothing of
// the model published.
func TestAUR526ReadCeilingIsInconclusive(t *testing.T) {
	aur580Setup(t, strings.Replace(aur526Config, "max_rounds: 3", "max_rounds: 3\n  max_read_bytes: 16", 1), "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur580Fixture(t, `[{"tool":"read_file","arguments":{"path":"app.go"}}]`))
	code, out, errOut, rec := aur580Run(t)
	if code == 0 || !strings.Contains(errOut, "deliberation_limit:max_read_bytes") {
		t.Fatalf("exit=%d, want the read ceiling to stop the review; stderr=%s", code, errOut)
	}
	if strings.Contains(out, "sem varredura opcional") {
		t.Fatalf("a partial review published the model's text:\n%s", out)
	}
	if rec.Gate.Decision != "inconclusive" || rec.Deliberation == nil || rec.Deliberation.Limit != reviewtools.LimitMaxReadBytes {
		t.Fatalf("audit gate=%+v deliberation=%+v", rec.Gate, rec.Deliberation)
	}
}

type aur526ToolProvider struct{ seen []llm.Message }

func (p *aur526ToolProvider) Complete(string, llm.Options) (llm.Response, error) { return llm.Response{}, nil }
func (p *aur526ToolProvider) Tokens(string) (int, error)                         { return 1, nil }
func (p *aur526ToolProvider) Name() string                                       { return "tools" }
func (p *aur526ToolProvider) CompleteWithTools(m []llm.Message, _ []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	p.seen = m
	return llm.ToolResponse{}, nil
}

type aur526TextProvider struct{}

func (aur526TextProvider) Complete(string, llm.Options) (llm.Response, error) { return llm.Response{}, nil }
func (aur526TextProvider) Tokens(string) (int, error)                         { return 1, nil }
func (aur526TextProvider) Name() string                                       { return "text" }

// AC-007: the profile decorator forwards llm.ToolCaller with its prefix,
// and never invents it for a provider without tools.
func TestAUR526ProfileDecoratorForwardsToolCalling(t *testing.T) {
	profile := reviewprofile.Profile{Name: "seguranca", Version: "1"}
	base := &aur526ToolProvider{}
	caller, ok := llm.AsToolCaller(newProfileProvider(base, profile))
	if !ok {
		t.Fatal("the profile decorator hid the provider's tool calling")
	}
	if _, err := caller.CompleteWithTools(llm.SystemUserMessages("sys", "user"), nil, llm.Options{}); err != nil {
		t.Fatal(err)
	}
	if len(base.seen) != 2 || !strings.HasPrefix(base.seen[0].Content, "Perfil de revisao: seguranca") || !strings.HasSuffix(base.seen[0].Content, "sys") {
		t.Fatalf("the tool conversation lost the profile prefix: %+v", base.seen)
	}
	if _, ok := llm.AsToolCaller(newProfileProvider(aur526TextProvider{}, profile)); ok {
		t.Fatal("the profile decorator claimed tool calling for a text-only provider")
	}
}
