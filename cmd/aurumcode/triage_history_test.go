package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// historyRepo is the incident of run 38650846531: a canary added on line
// 17 of plugin_test.go in an older commit of the range, and the reviewed
// HEAD where line 17 is `const (`. It returns the checkout and the old
// commit's id.
func historyRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	lines := func(at17 string) string {
		var b strings.Builder
		for n := 1; n <= 20; n++ {
			switch n {
			case 1:
				b.WriteString("package plugin\n")
			case 17:
				b.WriteString(at17 + "\n")
			default:
				b.WriteString(fmt.Sprintf("// linha %d\n", n))
			}
		}
		return b.String()
	}
	commit := func(source, parent string) string {
		blob := gitObject(t, dir, "blob", []byte(source))
		tree := gitObject(t, dir, "tree", treeEntry(t, "100644", "plugin_test.go", blob))
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author F <f@example.invalid> 1 +0000\ncommitter F <f@example.invalid> 1 +0000\n\nf\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	old := commit(lines(`	canarioDoTeste := "token=valor-sintetico-123456"`), "")
	head := commit(lines("const ("), old)
	for name, data := range map[string]string{".git/HEAD": "ref: refs/heads/main\n", ".git/refs/heads/main": head + "\n", ".git/config": "[core]\nrepositoryformatversion = 0\nbare = false\n", "plugin_test.go": lines("const (")} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, old
}

// scriptedTriage answers each triage prompt with answer(prompt).
type scriptedTriage struct {
	prompts []string
	answer  func(prompt string) string
}

func (c *scriptedTriage) Complete(_ context.Context, text string, _ llm.Options) (llm.Response, error) {
	c.prompts = append(c.prompts, text)
	return llm.Response{Text: c.answer(text)}, nil
}

func assessmentReply(status, why string) string {
	return fmt.Sprintf(`{"evidence_assessments":[{"evidence_id":"T1","status":%q,"justification":%q}]}`, status, why)
}

// historyState is a review whose gitleaks-like finding came from the old
// commit, with a declared gate and a model that answered the review.
func historyState(t *testing.T, caller *scriptedTriage) (*reviewState, string) {
	dir, old := historyRepo(t)
	finding := types.ReviewIssue{RuleID: "gitleaks:generic-api-key", File: "plugin_test.go", Line: 17, Severity: "error", Origin: gateOriginAnalysis, Commit: old}
	var errOut strings.Builder
	s := &reviewState{
		stderr: &errOut, result: &types.ReviewResult{}, model: modelReviewed, reviewLanguage: "pt-BR",
		cfg:            &config.Config{Gate: config.GateConfig{FailOn: []string{"high"}}},
		analysisIssues: []types.ReviewIssue{finding}, evidenceIDs: map[string]string{},
		diff:   &types.Diff{Files: []types.DiffFile{{Path: "plugin_test.go", Hunks: []types.DiffHunk{{NewStart: 17, Lines: []string{"+const ("}}}}}},
		filter: redaction.NewFilter(), verifyCaller: caller,
	}
	s.ctx, s.scanRoot = context.Background(), dir
	return s, old
}

// (1) The finding of an old commit is shown with that commit's content,
// saying the value is in the history; never the current line 17.
func TestTriageShowsTheCommitOfAHistoryFinding(t *testing.T) {
	caller := &scriptedTriage{answer: func(p string) string {
		if strings.Contains(p, "canarioDoTeste :=") {
			return assessmentReply("disputed", "canario sintetico de arquivo de teste")
		}
		return assessmentReply("needs_context", "o trecho mostrado e apenas const (")
	}}
	s, old := historyState(t, caller)
	s.triageUnassessed()
	sent := caller.prompts[0]
	if !strings.Contains(sent, "valor encontrado no commit "+old[:12]) || !strings.Contains(sent, ">   17| \tcanarioDoTeste :=") || strings.Contains(sent, ">   17| const (") {
		t.Fatalf("the triage must show the old commit's line 17, never the current one:\n%s", sent)
	}
	if a := s.analysisIssues[0].Assessment; a == nil || a.Status != types.AssessmentDisputed || s.triageFailure != "" {
		t.Fatalf("the model decided on the commit's content: %+v, failure %q", a, s.triageFailure)
	}
}

// (1) A commit that cannot be read shows no snippet at all: never the
// current file at an unrelated line.
func TestTriageNeverShowsTheCurrentFileForAnUnreadableCommit(t *testing.T) {
	caller := &scriptedTriage{answer: func(string) string { return assessmentReply("confirmed", "segredo real") }}
	s, _ := historyState(t, caller)
	s.analysisIssues[0].Commit = strings.Repeat("ab", 20)
	s.triageUnassessed()
	if sent := caller.prompts[0]; strings.Contains(sent, "const (") || !strings.Contains(sent, "não pôde ser lido") {
		t.Fatalf("an unreadable commit shows nothing of the current file:\n%s", sent)
	}
}

// (2) needs_context is asked once more with more context; decided then, it
// counts as decided; still undecided, it waits for the model.
func TestTriageNeedsContextIsAskedOnceMore(t *testing.T) {
	for _, tc := range []struct {
		name, second, status, failure string
	}{
		{"decided with context", assessmentReply("confirmed", "segredo real"), types.AssessmentConfirmed, ""},
		{"still undecided", assessmentReply("needs_context", "ainda nao sei"), types.AssessmentNeedsContext, triageReasonNeedsContext},
	} {
		caller := &scriptedTriage{answer: func(p string) string {
			if strings.Contains(p, "contexto ampliado") {
				return tc.second
			}
			return assessmentReply("needs_context", "preciso de mais")
		}}
		s, _ := historyState(t, caller)
		s.triageUnassessed()
		if len(caller.prompts) != 2 || !strings.Contains(caller.prompts[1], "    1| package plugin") || !strings.Contains(caller.prompts[1], "   20| // linha 20") {
			t.Fatalf("%s: the second ask must carry the whole (short) file; %d prompts", tc.name, len(caller.prompts))
		}
		if a := s.analysisIssues[0].Assessment; a == nil || a.Status != tc.status || s.triageFailure != tc.failure {
			t.Fatalf("%s: assessment %+v failure %q", tc.name, a, s.triageFailure)
		}
		tr := s.triage()
		if waits := tr.Awaiting && !tr.Assessed["analysis|gitleaks:generic-api-key|plugin_test.go|17"]; tc.failure != "" && !waits {
			t.Errorf("%s: an undecided finding must wait for the model, never count: %+v", tc.name, tr)
		}
	}
}

// (4) A deterministic finding the model refuted leaves the parecer (no
// observation line); one line in the details counts the dismissed alarms.
// The model's own findings are never filtered.
func TestRefutedAlarmLeavesTheParecer(t *testing.T) {
	refuted := types.ReviewIssue{RuleID: "gitleaks:generic-api-key", File: "demo/fixtures/findings.json", Line: 17, Severity: "error", Origin: "gitleaks", Message: "Possível segredo no código"}
	kept := types.ReviewIssue{RuleID: "analysis/sql", File: "app.go", Line: 3, Severity: "warning", Origin: gateOriginAnalysis, Message: "consulta montada"}
	model := types.ReviewIssue{RuleID: "gitleaks:generic-api-key", File: "demo/fixtures/findings.json", Line: 17, Severity: "warning", Message: "o modelo achou"}
	var errOut strings.Builder
	s := &reviewState{stderr: &errOut, result: &types.ReviewResult{}, reviewLanguage: "pt-BR", cfg: &config.Config{}}
	s.reportTriage([]gateDemotion{{Source: "secrets", Issue: refuted}})
	got := s.withoutDismissed([]types.ReviewIssue{refuted, kept, model})
	if len(got) != 2 || got[0].RuleID != "analysis/sql" || got[1].Origin != "" {
		t.Fatalf("only the refuted alarm leaves the parecer: %+v", got)
	}
	s.result.Issues = got
	body := formatPublishedReviewBody(s.result, &types.Diff{}, "pt-BR", false, "")
	if strings.Contains(body, "findings.json:17 gitleaks") || !strings.Contains(body, "alarmes estáticos descartados pelo modelo: 1") {
		t.Fatalf("the parecer must count the dismissed alarm in one line and not list it:\n%s", body)
	}
}
