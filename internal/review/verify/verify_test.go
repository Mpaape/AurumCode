package verify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// revision is the reviewed code of the scenario measured on a real pull
// request: the model said Validate was called without a nil guard, and the
// method starts with one.
type revision map[string]string

func (r revision) Read(rel string) ([]byte, error) {
	text, ok := r[rel]
	if !ok {
		return nil, errors.New("não existe na revisão revisada")
	}
	return []byte(text), nil
}

func (r revision) Paths() []string {
	var out []string
	for p := range r {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// goDecls declares what a grammar would: the methods and types defined.
func goDecls(_ string, data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 3 && fields[0] == "func" && strings.HasPrefix(fields[1], "(") {
			out = append(out, strings.SplitN(fields[3], "(", 2)[0])
		}
		if len(fields) > 1 && (fields[0] == "type" || (fields[0] == "func" && !strings.HasPrefix(fields[1], "("))) {
			out = append(out, strings.SplitN(fields[1], "(", 2)[0])
		}
	}
	return out
}

// caller answers each call with the next reply (or error) and keeps the
// prompts it received.
type caller struct {
	replies []string
	err     error
	prompts []string
}

func (c *caller) Complete(_ context.Context, text string, _ llm.Options) (llm.Response, error) {
	c.prompts = append(c.prompts, text)
	if c.err != nil {
		return llm.Response{}, c.err
	}
	reply := c.replies[0]
	if len(c.replies) > 1 {
		c.replies = c.replies[1:]
	}
	return llm.Response{Text: reply}, nil
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func scenario(t *testing.T) (revision, types.ReviewIssue) {
	t.Helper()
	rev := revision{
		"internal/config/config.go":  fixture(t, "config.go.txt"),
		"internal/config/batches.go": fixture(t, "batches.go.txt"),
	}
	var issue types.ReviewIssue
	if err := json.Unmarshal([]byte(fixture(t, "review-finding.json")), &issue); err != nil {
		t.Fatal(err)
	}
	return rev, issue
}

func blocksAll(types.ReviewIssue) bool { return true }

func newVerifier(rev Source, c Caller, maxCalls int) *Verifier {
	return &Verifier{Caller: c, Source: rev, Declared: goDecls, MaxCalls: maxCalls, Language: "pt-BR"}
}

func TestAC001RefutedWithLiteralQuoteIsDemotedAndRecorded(t *testing.T) {
	rev, issue := scenario(t)
	c := &caller{replies: []string{fixture(t, "verifier-refuted.json")}}
	res := newVerifier(rev, c, 8).Verify(context.Background(), []types.ReviewIssue{issue}, blocksAll)
	if len(res.Kept) != 0 || len(res.Demoted) != 1 {
		t.Fatalf("a refutation with a literal quote must demote the finding: kept=%d demoted=%d", len(res.Kept), len(res.Demoted))
	}
	rec := res.Demoted[0].Record
	if rec.Outcome != OutcomeRefuted || !rec.Demoted || rec.Reason == "" || !strings.Contains(rec.Quote, "if b == nil {") {
		t.Fatalf("record outcome=%s demoted=%v", rec.Outcome, rec.Demoted)
	}
	if len(res.Records) != 1 || res.Calls != 1 {
		t.Fatalf("records=%d calls=%d", len(res.Records), res.Calls)
	}
	shown := c.prompts[0]
	for _, want := range []string{prompt.VerificationMarker, "func (b *BatchesConfig) Validate() error {", "cfg.Batches.Validate()", "internal/config/batches.go"} {
		if !strings.Contains(shown, want) {
			t.Errorf("the verification prompt lacks %q: the cited window and the symbol's definition must be shown", want)
		}
	}
}

func TestAC002RefutedWithQuoteNotInRevisionKeepsBlocking(t *testing.T) {
	rev, issue := scenario(t)
	for i, reply := range []string{
		fixture(t, "verifier-paraphrased.json"),
		`{"verdict":"refuted","reason":"r","quote":"   "}`,
		`{"verdict":"refuted","reason":"r","quote":""}`,
	} {
		c := &caller{replies: []string{reply}}
		res := newVerifier(rev, c, 8).Verify(context.Background(), []types.ReviewIssue{issue}, blocksAll)
		if len(res.Kept) != 1 || len(res.Demoted) != 0 || res.Records[0].Outcome != OutcomeQuoteNotFound {
			t.Fatalf("reply %d: a refutation whose quote is not in the revision must keep blocking: outcome %s", i, res.Records[0].Outcome)
		}
	}
}

func TestAC002TrailingSpacesAreTheOnlyForgivenDifference(t *testing.T) {
	files := map[string]string{"a.go": "if b == nil {\n\treturn nil\n}\n"}
	if !quoteFound("if b == nil {   \n\treturn nil\t\n}", files) {
		t.Fatal("trailing spaces of a line must not matter")
	}
	if quoteFound("if b == nil {\n    return nil\n}", files) {
		t.Fatal("a different indentation is not the literal code")
	}
}

func TestAC003EveryOtherOutcomeKeepsBlocking(t *testing.T) {
	rev, issue := scenario(t)
	cases := []struct {
		name string
		c    *caller
		want Outcome
	}{
		{"confirmed", &caller{replies: []string{`{"verdict":"confirmed","reason":"r","quote":"if b == nil {"}`}}, OutcomeConfirmed},
		{"uncertain", &caller{replies: []string{`{"verdict":"uncertain","reason":"r","quote":"if b == nil {"}`}}, OutcomeUncertain},
		{"invalid", &caller{replies: []string{`{"issues":[]}`}}, OutcomeInvalidAnswer},
		{"not json", &caller{replies: []string{"refutado, confie em mim"}}, OutcomeInvalidAnswer},
		{"provider error", &caller{err: errors.New("provedor fora do ar")}, OutcomeProviderError},
	}
	for _, tc := range cases {
		res := newVerifier(rev, tc.c, 8).Verify(context.Background(), []types.ReviewIssue{issue}, blocksAll)
		if len(res.Kept) != 1 || len(res.Demoted) != 0 || res.Records[0].Outcome != tc.want {
			t.Errorf("%s: must keep blocking with %s, got outcome %s", tc.name, tc.want, res.Records[0].Outcome)
		}
	}
	limited := newVerifier(rev, &caller{replies: []string{fixture(t, "verifier-refuted.json")}}, 0).
		Verify(context.Background(), []types.ReviewIssue{issue}, blocksAll)
	if len(limited.Kept) != 1 || limited.Records[0].Outcome != OutcomeCallLimit || limited.Calls != 0 {
		t.Errorf("past the call ceiling the finding keeps blocking: outcome %s", limited.Records[0].Outcome)
	}
	unread := newVerifier(nil, &caller{replies: []string{fixture(t, "verifier-refuted.json")}}, 8).
		Verify(context.Background(), []types.ReviewIssue{issue}, blocksAll)
	if len(unread.Kept) != 1 || unread.Records[0].Outcome != OutcomeSourceUnavailable {
		t.Errorf("without the reviewed revision the finding keeps blocking: outcome %s", unread.Records[0].Outcome)
	}
}

func TestAC004DeterministicFindingsAreNeverSent(t *testing.T) {
	rev, issue := scenario(t)
	scanner := issue
	scanner.Origin = "semgrep"
	assessed := issue
	assessed.Assessment = &types.EvidenceAssessment{EvidenceID: "E1", Status: types.AssessmentDisputed}
	c := &caller{replies: []string{fixture(t, "verifier-refuted.json")}}
	res := newVerifier(rev, c, 8).Verify(context.Background(), []types.ReviewIssue{scanner, assessed}, blocksAll)
	if len(c.prompts) != 0 || len(res.Kept) != 2 || len(res.Records) != 0 {
		t.Fatalf("a deterministic finding reached the verifier: prompts=%d kept=%d", len(c.prompts), len(res.Kept))
	}
}

func TestAC005NonBlockingFindingsAndTheCeiling(t *testing.T) {
	rev, issue := scenario(t)
	c := &caller{replies: []string{fixture(t, "verifier-refuted.json")}}
	res := newVerifier(rev, c, 8).Verify(context.Background(), []types.ReviewIssue{issue}, func(types.ReviewIssue) bool { return false })
	if len(c.prompts) != 0 || len(res.Kept) != 1 {
		t.Fatal("a finding that does not block is never verified")
	}
	second := issue
	second.Line = 7
	c = &caller{replies: []string{fixture(t, "verifier-refuted.json")}}
	res = newVerifier(rev, c, 1).Verify(context.Background(), []types.ReviewIssue{issue, second}, blocksAll)
	if res.Calls != 1 || len(res.Demoted) != 1 || len(res.Kept) != 1 || res.Records[1].Outcome != OutcomeCallLimit {
		t.Fatalf("max_calls 1: one call, the excess keeps blocking: calls=%d demoted=%d", res.Calls, len(res.Demoted))
	}
}
