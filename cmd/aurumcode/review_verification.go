// The adversarial verification of the model's findings, between the
// evidence phase and the gate: each model finding, the blocking ones first,
// is checked against the reviewed revision by a separate call of the same
// provider (internal/review/verify). A refuted finding leaves the parecer
// and the gate's input and stays visible, marked, among the limitations and
// in the audit; everything else keeps counting.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review/verify"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// verifyModelFindings runs the verification when review.verification is on
// and the model pass left a provider to call.
func (s *reviewState) verifyModelFindings() {
	if s.result == nil || s.cfg == nil || s.verifyCaller == nil || !s.cfg.Review.Verification.Active() {
		return
	}
	if !s.anyVerifiable() {
		return
	}
	v := &verify.Verifier{
		Caller:   s.verifyCaller,
		Declared: declaredSymbols(grammar.Default()),
		MaxCalls: s.cfg.Review.Verification.EffectiveMaxCalls(),
		Language: s.reviewLanguage,
		Redact:   s.redactText,
	}
	if rev, reason := s.verificationSource(); reason == "" {
		v.Source = rev
	} else if s.cfg.Review.Verification.Enabled != nil {
		// Without the reviewed revision every finding keeps counting (the
		// records say so); the notice is for the operator who asked for
		// the verification, not for every local run outside a checkout.
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", i18n.Format(s.reviewLanguage, "review.verification_unavailable", reason))
	}
	res := v.Verify(s.ctx, s.result.Issues, s.modelFindingBlocks)
	s.result.Issues = res.Kept
	s.verification = res.Records
	demoted := map[verify.Record]types.ReviewIssue{}
	for _, d := range res.Demoted {
		demoted[d.Record] = d.Issue
	}
	for _, rec := range res.Records {
		s.reportVerification(rec, demoted[rec])
	}
}

// anyVerifiable reports whether the model left a finding to verify.
func (s *reviewState) anyVerifiable() bool {
	for _, issue := range s.result.Issues {
		if verify.Candidate(issue) {
			return true
		}
	}
	return false
}

// verificationSource is the reviewed revision, or why it cannot be read.
func (s *reviewState) verificationSource() (verify.Source, string) {
	if s.scanRoot == "" {
		return nil, "sem checkout revisado"
	}
	if s.scanBlocked != "" {
		return nil, "checkout não verificado"
	}
	rev, err := s.reviewedRevision()
	if err != nil {
		return nil, s.redactText(err.Error())
	}
	return rev, ""
}

// reportVerification states one verified finding: a refuted one leaves the
// findings and is named, with the verifier's reason and quote, on stderr
// and among the limitations of the parecer (never dropped silently); a
// kept blocking one is said on stderr with why it still counts; a kept
// observation is only in the audit record.
func (s *reviewState) reportVerification(rec verify.Record, issue types.ReviewIssue) {
	if rec.Demoted {
		key := "review.verification_refuted"
		if !rec.Blocking {
			key = "review.verification_dropped"
		}
		line := i18n.Format(s.reviewLanguage, key, rec.Path, rec.Line, rec.RuleID, oneLine(rec.Reason), oneLine(rec.Quote))
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
		s.result.Limitations = append(s.result.Limitations, line)
		return
	}
	if rec.Outcome == verify.OutcomeSourceUnavailable {
		// Already said once, for the run, by verifyModelFindings.
		return
	}
	if !rec.Blocking {
		// A kept observation changes nothing: saying it "still blocks"
		// would be wrong, and the audit record already keeps the outcome.
		return
	}
	fmt.Fprintf(s.stderr, "aurumcode review: %s\n", i18n.Format(s.reviewLanguage, "review.verification_kept", rec.Path, rec.Line, rec.RuleID, keptReason(s.reviewLanguage, rec), rec.Outcome))
}

// keptReason says in plain words why a verified finding still blocks. The
// verifier's own reason is added only when it is the verifier's prose
// (confirmed, uncertain, a quote that is not in the code); a technical
// parse or provider detail stays in the audit record.
func keptReason(language string, rec verify.Record) string {
	text, ok := verificationOutcomeText(language, rec.Outcome)
	if !ok {
		return oneLine(rec.Reason)
	}
	switch rec.Outcome {
	case verify.OutcomeConfirmed, verify.OutcomeUncertain, verify.OutcomeQuoteNotFound:
		if reason := oneLine(rec.Reason); reason != "" {
			return text + " — " + reason
		}
	}
	return text
}

func verificationOutcomeText(language string, outcome verify.Outcome) (string, bool) {
	key := "review.verification_outcome." + string(outcome)
	text := i18n.Text(language, key)
	return text, text != key
}

// declaredSymbols is the grammar provider's definitions of a file.
func declaredSymbols(provider grammar.Provider) verify.Declarer {
	return func(path string, data []byte) []string {
		if grammar.LooksBinary(data) {
			return nil
		}
		return provider.Analyze(path, data).Symbols
	}
}

// appendAuditVerification adds the verification records to the audit
// record already written at path, as its last key, redacted like the rest.
func appendAuditVerification(path string, records []verify.Record, filter *redaction.Filter) error {
	if len(records) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := strings.TrimRight(string(data), "\n")
	if !strings.HasSuffix(body, "}") {
		return fmt.Errorf("registro de auditoria inesperado em %s", path)
	}
	extra, err := json.MarshalIndent(records, "  ", "  ")
	if err != nil {
		return err
	}
	head := strings.TrimRight(strings.TrimSuffix(body, "}"), "\n")
	out := head + ",\n  \"verification\": " + filter.Redact(string(extra)) + "\n}\n"
	return os.WriteFile(path, []byte(out), 0o600)
}
