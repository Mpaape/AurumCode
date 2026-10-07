// Findings read back from the review cache are untrusted input: the cache
// directory (AURUMCODE_CACHE_DIR) may be shared with other workflows of the
// same CI scope, so a stored entry can carry text nobody's model ever wrote.
// Every finding read from it passes the same redaction the model's answer
// passes at the model-output boundary before it can reach any sink (stdout,
// SARIF, audit, a PR comment or review).
package main

import (
	"context"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// redactCachedIssues returns a redacted copy of issues read from the cache.
// The fields are the ones the model-output boundary redacts on a fresh
// finding; Severity and Origin are left alone there too (a closed set and an
// engine stamp). Line is an integer.
func redactCachedIssues(f *redaction.Filter, issues []types.ReviewIssue) []types.ReviewIssue {
	if len(issues) == 0 {
		return issues
	}
	out := make([]types.ReviewIssue, len(issues))
	for i, issue := range issues {
		issue.ID = f.Redact(issue.ID)
		issue.File = f.Redact(issue.File)
		issue.Side = f.Redact(issue.Side)
		issue.RuleID = f.Redact(issue.RuleID)
		issue.Message = redactCachedMessage(f, issue.Message, issue.RuleID)
		issue.Impact = redactProse(f, issue.Impact)
		issue.Evidence = redactProse(f, issue.Evidence)
		issue.Suggestion = redactProse(f, issue.Suggestion)
		issue.Verification = redactProse(f, issue.Verification)
		if issue.Assessment != nil {
			assessment := *issue.Assessment
			assessment.EvidenceID = f.Redact(assessment.EvidenceID)
			assessment.Status = f.Redact(assessment.Status)
			assessment.Priority = f.Redact(assessment.Priority)
			assessment.Justification = redactProse(f, assessment.Justification)
			assessment.Suggestion = redactProse(f, assessment.Suggestion)
			assessment.Correlates = append([]string(nil), assessment.Correlates...)
			for j := range assessment.Correlates {
				assessment.Correlates[j] = f.Redact(assessment.Correlates[j])
			}
			issue.Assessment = &assessment
		}
		out[i] = issue
	}
	return out
}

// redactCachedMessage redacts a cached finding's message the way a fresh
// one is redacted: the model's text before the rule citation the engine
// appended, never the two together. Redacting "... (rule
// security/hardcoded-secret: Hardcoded Secrets)" as one string makes the
// filter read "secret: Hardcoded" as a key/value pair and mask the title, so
// a cache hit would print a different line than the run that stored it
// (AUR-441). The citation's title still passes the filter on its own: a
// forged entry cannot smuggle a secret through it.
func redactCachedMessage(f *redaction.Filter, message, ruleID string) string {
	opener := " (rule " + ruleID + ": "
	at := strings.LastIndex(message, opener)
	if ruleID == "" || at < 0 || !strings.HasSuffix(message, ")") {
		return redactProse(f, message)
	}
	body := message[:at]
	title := message[at+len(opener) : len(message)-1]
	return redactProse(f, body) + opener + f.Redact(title) + ")"
}

// redactProse redacts a finding's prose the way the model-output boundary
// does: a quoted diff line keeps its +/-/space marker, which would otherwise
// defeat the filter's line-anchored rules (an Authorization header quoted as
// "+Authorization: ..."). Bodies are redacted together so multi-line rules
// still apply; when a multi-line secret collapsed lines, markers no longer
// map and the redacted bodies are returned without them (fail closed).
func redactProse(f *redaction.Filter, text string) string {
	lines := strings.Split(text, "\n")
	markers := make([]string, len(lines))
	bodies := make([]string, len(lines))
	for i, line := range lines {
		if line != "" && strings.ContainsRune(diffLineMarkers, rune(line[0])) {
			markers[i], bodies[i] = line[:1], line[1:]
			continue
		}
		bodies[i] = line
	}
	out := strings.Split(f.Redact(strings.Join(bodies, "\n")), "\n")
	if len(out) != len(markers) {
		return strings.Join(out, "\n")
	}
	for i := range out {
		out[i] = markers[i] + out[i]
	}
	return strings.Join(out, "\n")
}

// diffLineMarkers are the one-character prefixes of a unified diff line.
const diffLineMarkers = "+- "

// gateContributor is the shape of one step of the gate pipeline, under the
// command's own names (only review_gate.go imports internal/gate).
type gateContributor interface {
	Name() string
	Origin() string
	Apply(ctx context.Context, run *gateRun, prior gateDecision) (gateDecision, error)
}

// redactedVerdictReuse wraps the verdict-reuse step: whatever findings it
// adds from a stored verdict are redacted with the run's filter before any
// later step (or sink) sees them. The run's own findings are a prefix the
// reuse never rewrites (monotonic union), so only the suffix it appended is
// read from the cache. A reused finding that redacts to one the run already
// has is dropped, keeping the union's exact-equality dedup.
type redactedVerdictReuse struct {
	gateContributor
}

func (c redactedVerdictReuse) Apply(ctx context.Context, run *gateRun, prior gateDecision) (gateDecision, error) {
	current := append([]types.ReviewIssue(nil), run.Review.Issues...)
	res, err := c.gateContributor.Apply(ctx, run, prior)
	merged := run.Review.Issues
	if !hasPrefix(merged, current) {
		// The union no longer keeps the run's findings as a prefix: treat
		// every finding as read from the cache rather than guess.
		run.Review.Issues = redactCachedIssues(run.Filter, merged)
		return res, err
	}
	seen := make(map[types.ReviewIssue]bool, len(merged))
	for _, issue := range current {
		seen[issue] = true
	}
	out := current
	for _, issue := range redactCachedIssues(run.Filter, merged[len(current):]) {
		if !seen[issue] {
			seen[issue] = true
			out = append(out, issue)
		}
	}
	run.Review.Issues = out
	return res, err
}

// hasPrefix reports whether issues starts with prefix, element by element.
func hasPrefix(issues, prefix []types.ReviewIssue) bool {
	if len(issues) < len(prefix) {
		return false
	}
	for i := range prefix {
		if issues[i] != prefix[i] {
			return false
		}
	}
	return true
}
