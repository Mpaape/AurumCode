package main

// The verification sits between the evidence and the gate: a refuted model
// finding leaves the gate's input and stays visible, a scanner finding is
// never sent, review.verification.enabled false sends nothing, and the
// audit keeps every record.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/review/verify"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// verifyTestdata is the scenario measured on a real pull request, shared
// with internal/review/verify.
const verifyTestdata = "../../internal/review/verify/testdata/"

// scriptedVerifier answers every verification call with reply.
type scriptedVerifier struct {
	reply   string
	prompts []string
}

func (c *scriptedVerifier) Complete(_ context.Context, text string, _ llm.Options) (llm.Response, error) {
	c.prompts = append(c.prompts, text)
	return llm.Response{Text: c.reply}, nil
}

// verificationCheckout writes the scenario's two files as the head commit
// of a repository built without the git binary (aur522Repo) and returns
// its directory.
func verificationCheckout(t *testing.T) string {
	t.Helper()
	head := map[string][]byte{}
	for name, src := range map[string]string{"internal/config/config.go": "config.go.txt", "internal/config/batches.go": "batches.go.txt"} {
		data, err := os.ReadFile(verifyTestdata + src)
		if err != nil {
			t.Fatal(err)
		}
		head[name] = data
	}
	return aur522Repo(t, map[string][]byte{"README.md": []byte("base\n")}, head, "")
}

func verificationState(t *testing.T, cfg *config.Config, caller verify.Caller) (*reviewState, *bytes.Buffer, types.ReviewIssue, types.ReviewIssue) {
	t.Helper()
	data, err := os.ReadFile(verifyTestdata + "review-finding.json")
	if err != nil {
		t.Fatal(err)
	}
	var model types.ReviewIssue
	if err := json.Unmarshal(data, &model); err != nil {
		t.Fatal(err)
	}
	scannerIssue := types.ReviewIssue{File: model.File, Line: model.Line, Severity: "error", RuleID: "semgrep/nil-call", Message: "Validate chamado sem guarda", Origin: "semgrep"}
	var stderr bytes.Buffer
	st := newReviewState(session.LocalDiff, reviewIO{stdout: &bytes.Buffer{}, stderr: &stderr, filter: redaction.NewFilter()})
	s := &st
	s.cfg, s.scanRoot, s.verifyCaller, s.reviewLanguage = cfg, verificationCheckout(t), caller, "pt-BR"
	s.result = &types.ReviewResult{Issues: []types.ReviewIssue{model, scannerIssue}}
	return s, &stderr, model, scannerIssue
}

func readVerifyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(verifyTestdata + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestVerificationDemotesRefutedModelFindingBeforeTheGate(t *testing.T) {
	caller := &scriptedVerifier{reply: readVerifyFixture(t, "verifier-refuted.json")}
	s, stderr, model, scannerIssue := verificationState(t, &config.Config{}, caller)
	s.verifyModelFindings()
	if len(caller.prompts) != 1 || !strings.Contains(caller.prompts[0], prompt.VerificationMarker) || strings.Contains(caller.prompts[0], scannerIssue.RuleID) {
		t.Fatalf("exactly the model finding is sent, never the scanner's: %d prompt(s)", len(caller.prompts))
	}
	if len(s.result.Issues) != 1 || s.result.Issues[0].Origin != "semgrep" {
		t.Fatalf("the refuted model finding still reaches the gate: %+v", s.result.Issues)
	}
	if n := blocking.Ungated().Count(s.result.Issues); n != 1 {
		t.Fatalf("only the scanner finding blocks now, got %d", n)
	}
	marked := strings.Join(s.result.Limitations, "\n")
	if !strings.Contains(marked, "refutado pela verificação") || !strings.Contains(marked, model.RuleID) || !strings.Contains(stderr.String(), "refutado pela verificação") {
		t.Fatalf("the refuted finding must stay visible and marked: %q / %q", marked, stderr.String())
	}
	if len(s.result.Suggestions) != 0 {
		t.Fatalf("a refuted finding is named among the limitations, never republished as a suggestion: %+v", s.result.Suggestions)
	}
	if len(s.verification) != 1 || !s.verification[0].Blocking {
		t.Fatalf("the record must say the finding was blocking: %+v", s.verification)
	}
	audit := filepath.Join(t.TempDir(), "auditoria.json")
	if err := os.WriteFile(audit, []byte("{\n  \"verdict\": \"comment\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendAuditVerification(audit, s.verification, redaction.NewFilter()); err != nil {
		t.Fatal(err)
	}
	var rec struct {
		Verdict      string          `json:"verdict"`
		Verification []verify.Record `json:"verification"`
	}
	data, _ := os.ReadFile(audit)
	if err := json.Unmarshal(data, &rec); err != nil || rec.Verdict != "comment" || len(rec.Verification) != 1 || !rec.Verification[0].Demoted || rec.Verification[0].Outcome != verify.OutcomeRefuted {
		t.Fatalf("the audit must keep the refutation with its mark: %v (%d records)", err, len(rec.Verification))
	}
}

func TestVerificationKeepsBlockingWhenTheQuoteIsNotTheCode(t *testing.T) {
	caller := &scriptedVerifier{reply: readVerifyFixture(t, "verifier-paraphrased.json")}
	s, stderr, model, _ := verificationState(t, &config.Config{}, caller)
	s.verifyModelFindings()
	if len(s.result.Issues) != 2 || s.result.Issues[0].RuleID != model.RuleID {
		t.Fatalf("a paraphrased quote must keep the model finding blocking: %+v", s.result.Issues)
	}
	if !strings.Contains(stderr.String(), "continua bloqueando") || !strings.Contains(stderr.String(), string(verify.OutcomeQuoteNotFound)) {
		t.Fatalf("why it still blocks must be said: %q", stderr.String())
	}
}

func TestVerificationDisabledSendsNothing(t *testing.T) {
	off := false
	cfg := &config.Config{Review: config.ReviewConfig{Verification: config.ReviewVerificationConfig{Enabled: &off}}}
	caller := &scriptedVerifier{reply: readVerifyFixture(t, "verifier-refuted.json")}
	s, _, _, _ := verificationState(t, cfg, caller)
	s.verifyModelFindings()
	if len(caller.prompts) != 0 || len(s.result.Issues) != 2 || len(s.verification) != 0 {
		t.Fatalf("review.verification.enabled false must send nothing and keep every finding: %d prompt(s)", len(caller.prompts))
	}
}
