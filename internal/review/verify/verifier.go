package verify

import (
	"context"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Caller is the provider the review already uses (llm.Orchestrator), so
// each verification call is charged to the review's own cost ceiling.
type Caller interface {
	Complete(ctx context.Context, prompt string, opts llm.Options) (llm.Response, error)
}

// reasonNoRevision is the record's reason when no reviewed revision could
// be read.
const reasonNoRevision = "revisão revisada indisponível para a verificação"

// reasonCallLimit is the record's reason past the call ceiling.
const reasonCallLimit = "teto de chamadas da verificação atingido"

// Verifier checks the model's findings against the reviewed code.
type Verifier struct {
	Caller Caller
	// Source is the reviewed revision; nil keeps every finding blocking.
	Source   Source
	Declared Declarer
	// MaxCalls bounds the calls; a finding beyond it keeps blocking.
	MaxCalls int
	// Language is the language of the verifier's reason.
	Language string
	// Redact filters the prompt and any error text; nil leaves them.
	Redact func(string) string
}

// Candidate reports whether issue is the model's own finding: the
// deterministic engines' findings (an Origin) and the model's assessments
// of them are never verified here.
func Candidate(issue types.ReviewIssue) bool {
	return issue.Origin == "" && issue.Assessment == nil
}

// Verify sends every candidate to the verifier, the ones that block first
// (the call ceiling protects the gate's input before the observations),
// and returns the findings that still count, the demoted ones and every
// record, all in input order.
func (v *Verifier) Verify(ctx context.Context, issues []types.ReviewIssue, blocks func(types.ReviewIssue) bool) Result {
	var res Result
	records := make([]*Record, len(issues))
	for _, blocking := range []bool{true, false} {
		for i, issue := range issues {
			if !Candidate(issue) || blocks(issue) != blocking {
				continue
			}
			rec := v.verifyOne(ctx, issue, &res.Calls)
			rec.Blocking = blocking
			records[i] = &rec
		}
	}
	for i, issue := range issues {
		rec := records[i]
		if rec == nil {
			res.Kept = append(res.Kept, issue)
			continue
		}
		res.Records = append(res.Records, *rec)
		if rec.Demoted {
			res.Demoted = append(res.Demoted, Demotion{Issue: issue, Record: *rec})
			continue
		}
		res.Kept = append(res.Kept, issue)
	}
	return res
}

// verifyOne is one finding's verification; every path but a refutation
// with a quote found in the shown code keeps it blocking.
func (v *Verifier) verifyOne(ctx context.Context, issue types.ReviewIssue, calls *int) Record {
	rec := Record{RuleID: issue.RuleID, Path: issue.File, Line: issue.Line}
	if v.Source == nil || v.Caller == nil {
		return v.kept(rec, OutcomeSourceUnavailable, reasonNoRevision)
	}
	if *calls >= v.MaxCalls {
		return v.kept(rec, OutcomeCallLimit, reasonCallLimit)
	}
	shown, files, err := excerpts(v.Source, v.Declared, issue)
	if err != nil {
		return v.kept(rec, OutcomeSourceUnavailable, err.Error())
	}
	text, err := prompt.BuildVerificationPrompt(prompt.VerificationInput{
		Language: v.Language, RuleID: issue.RuleID, File: issue.File, Line: issue.Line,
		Severity: issue.Severity, Message: issue.Message, Evidence: issue.Evidence, Excerpts: shown,
	})
	if err != nil {
		return v.kept(rec, OutcomeProviderError, err.Error())
	}
	*calls++
	resp, err := v.Caller.Complete(ctx, v.redact(text), llm.Options{JSONMode: true})
	if err != nil {
		return v.kept(rec, OutcomeProviderError, err.Error())
	}
	a, err := parseAnswer(resp.Text)
	if err != nil {
		return v.kept(rec, OutcomeInvalidAnswer, err.Error())
	}
	rec.Verdict, rec.Reason, rec.Quote = a.Verdict, v.redact(a.Reason), v.redact(a.Quote)
	switch {
	case a.Verdict == VerdictConfirmed:
		rec.Outcome = OutcomeConfirmed
	case a.Verdict == VerdictUncertain:
		rec.Outcome = OutcomeUncertain
	case !quoteFound(a.Quote, files):
		rec.Outcome = OutcomeQuoteNotFound
	default:
		rec.Outcome, rec.Demoted = OutcomeRefuted, true
	}
	return rec
}

// kept records a finding that keeps blocking for reason.
func (v *Verifier) kept(rec Record, outcome Outcome, reason string) Record {
	rec.Outcome, rec.Reason = outcome, v.redact(reason)
	return rec
}

func (v *Verifier) redact(text string) string {
	if v.Redact == nil {
		return text
	}
	return v.Redact(text)
}
