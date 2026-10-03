// AUR-521: the audit record a policy-governed review writes alongside its
// SARIF (sarif.go). It carries exactly the fields AC-001 names: the policy
// digest, the workflow and reviewed SHAs, the repo, the model, the verdict,
// the gate's own decision, the findings that actually blocked it, the
// exceptions applied to it (AUR-520's own field -- empty until that card
// lands), and the coverage this run achieved.
package render

import (
	"encoding/json"
	"os"
	"sort"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// AuditRecord is the complete, redacted-before-write compliance record for
// one review run.
type AuditRecord struct {
	// AC-001 requires every field above to be present, including on a
	// plain local run where several of these are naturally empty (no
	// GITHUB_* env, no --modelo) -- never omitted just because they
	// happen to be "". Only ExceptionsApplied/Coverage.Omitted use
	// omitempty, and only because they are slices whose empty encoding
	// ("[]"/absent) is equally explicit either way.
	PolicyDigest string `json:"policy_digest"`
	WorkflowSHA  string `json:"workflow_sha"`
	Repo         string `json:"repo"`
	ReviewedSHA  string `json:"reviewed_sha"`
	Model        string `json:"model"`
	Verdict      string `json:"verdict"`

	Gate AuditGate `json:"gate"`

	BlockingFindings []AuditFinding `json:"blocking_findings"`

	// ExceptionsApplied is AUR-520's own field: the exceptions this run
	// actually applied to a finding. It is always present as a list -- an
	// empty one until AUR-520 lands and starts populating it -- never
	// omitted, so a consumer parsing this record never has to special-case
	// its absence (see docs/specs/AUR-521.md).
	ExceptionsApplied []AuditException `json:"exceptions_applied"`

	Coverage AuditCoverage `json:"coverage"`

	// AnalysisData (AUR-533) names the analysis-data artifact this run used:
	// present only when the artifact was declared and resolved usable.
	AnalysisData *AnalysisDataAudit `json:"analysis_data,omitempty"`

	// EvidenceAssessments is every deterministic finding the model
	// assessed: the engine's origin beside the model's assessment. Absent
	// when the model assessed nothing.
	EvidenceAssessments []AuditEvidence `json:"evidence_assessments,omitempty"`
	// ProposedExceptions is the exceptions the model's disputes suggest,
	// as text a human may copy into `exceptions`. Never applied.
	ProposedExceptions string `json:"proposed_exceptions,omitempty"`
	// Deliberation is the model's tool conversation: what was offered,
	// asked for and not, each call with redacted arguments, and the limit
	// that stopped it. Absent when the model was offered no tool.
	Deliberation *deliberation.Transcript `json:"deliberation,omitempty"`
}

// AnalysisDataAudit, AuditFinding and AuditException are the gate's own
// structured facts (internal/gate/facts); the audit record presents them unchanged.
type (
	AnalysisDataAudit = facts.AnalysisDataAudit
	AuditFinding      = facts.AuditFinding
	AuditException    = facts.AuditException
)

// AuditGate is the gate's own decision for this run: "pass", "fail" or
// "inconclusive", plus the reason lines the gate itself produced (empty on
// a plain pass).
type AuditGate struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// AuditCoverage is AC-004's own coverage fact: whether the review was
// complete, and -- when it was not -- which files were left out and why
// this record cannot claim they were reviewed.
type AuditCoverage struct {
	Complete bool     `json:"complete"`
	Omitted  []string `json:"omitted_files,omitempty"`
}

// BuildAuditRecord assembles AC-001's full audit record from one finished
// review's already-resolved facts. blocking and exceptions are copied (never
// nil in the result, see ExceptionsApplied's own doc); omitted is sorted so
// the same run always serializes identically.
func BuildAuditRecord(policyDigest, workflowSHA, repo, reviewedSHA, model, verdict string, gate AuditGate, blocking []AuditFinding, exceptions []AuditException, complete bool, omitted []string) AuditRecord {
	blockingCopy := make([]AuditFinding, len(blocking))
	copy(blockingCopy, blocking)
	exceptionsCopy := make([]AuditException, len(exceptions))
	copy(exceptionsCopy, exceptions)
	omittedCopy := append([]string{}, omitted...)
	sort.Strings(omittedCopy)
	return AuditRecord{
		PolicyDigest:      policyDigest,
		WorkflowSHA:       workflowSHA,
		Repo:              repo,
		ReviewedSHA:       reviewedSHA,
		Model:             model,
		Verdict:           verdict,
		Gate:              gate,
		BlockingFindings:  blockingCopy,
		ExceptionsApplied: exceptionsCopy,
		Coverage: AuditCoverage{
			Complete: complete,
			Omitted:  omittedCopy,
		},
	}
}

// PolicySource is the declared central policy whose content the audit
// record identifies. *config.Config implements it; a nil policy, typed or
// not, means no policy was active this run.
type PolicySource interface {
	PolicyDigest(policyDir string) string
}

// PolicyDigest is the policy-change-detection digest the audit record
// carries (see config.Config.PolicyDigest). A nil policy returns "".
func PolicyDigest(policyDir string, policy PolicySource) string {
	if policy == nil {
		return ""
	}
	return policy.PolicyDigest(policyDir)
}

// redactAuditRecord runs every individual string field of rec through
// filter BEFORE it is ever marshaled to JSON. This is the fix for a real
// defect: redacting only the final, marshaled JSON text (as this function's
// caller also still does, defense in depth) misses a secret whose quote,
// backslash or embedded newline was escaped by json.Marshal into a
// different byte sequence than the one the redaction regexes match -- a
// secret containing `"`, `\` or a line break survives marshaling-after-
// redact but not redact-after-marshal. Redacting each field first, in its
// own unescaped form, closes that gap; the post-marshal pass then still
// catches anything that is only secret-shaped once assembled across field
// boundaries (e.g. a key name split from its value by JSON's own syntax).
func redactAuditRecord(filter *redaction.Filter, rec AuditRecord) AuditRecord {
	rec.PolicyDigest = filter.Redact(rec.PolicyDigest)
	rec.WorkflowSHA = filter.Redact(rec.WorkflowSHA)
	rec.Repo = filter.Redact(rec.Repo)
	rec.ReviewedSHA = filter.Redact(rec.ReviewedSHA)
	rec.Model = filter.Redact(rec.Model)
	rec.Verdict = filter.Redact(rec.Verdict)
	rec.Gate.Decision = filter.Redact(rec.Gate.Decision)
	rec.Gate.Reason = filter.Redact(rec.Gate.Reason)
	if rec.AnalysisData != nil {
		ad := AnalysisDataAudit{
			Digest:      filter.Redact(rec.AnalysisData.Digest),
			GeneratedAt: filter.Redact(rec.AnalysisData.GeneratedAt),
			Tag:         filter.Redact(rec.AnalysisData.Tag),
			Source:      filter.Redact(rec.AnalysisData.Source),
		}
		rec.AnalysisData = &ad
	}
	// New slices throughout: redaction must never mutate the caller's own
	// rec in place (a shared backing array would otherwise silently
	// rewrite data the caller might still hold a reference to).
	blocking := make([]AuditFinding, len(rec.BlockingFindings))
	for i, f := range rec.BlockingFindings {
		blocking[i] = AuditFinding{
			RuleID:   filter.Redact(f.RuleID),
			Path:     filter.Redact(f.Path),
			Line:     f.Line,
			Severity: filter.Redact(f.Severity),
			Origin:   filter.Redact(f.Origin),
		}
	}
	rec.BlockingFindings = blocking
	exceptions := make([]AuditException, len(rec.ExceptionsApplied))
	for i, e := range rec.ExceptionsApplied {
		exceptions[i] = AuditException{
			RuleID:        filter.Redact(e.RuleID),
			Path:          filter.Redact(e.Path),
			Justification: filter.Redact(e.Justification),
		}
	}
	rec.ExceptionsApplied = exceptions
	omitted := make([]string, len(rec.Coverage.Omitted))
	for i, p := range rec.Coverage.Omitted {
		omitted[i] = filter.Redact(p)
	}
	rec.Coverage.Omitted = omitted
	rec.EvidenceAssessments = redactAuditEvidence(filter, rec.EvidenceAssessments)
	rec.ProposedExceptions = filter.Redact(rec.ProposedExceptions)
	return rec
}

// WriteAuditRecord redacts every string field of rec (redactAuditRecord),
// marshals the result as indented JSON, then runs the complete text through
// filter a second time -- the single AUR-009 redaction filter every sink in
// this system writes through -- before writing it to path (AC-005). A
// registered secret (including the AURUM_SECRET_CANARY value), however it
// was escaped by JSON encoding, is replaced wherever it appears.
func WriteAuditRecord(path string, rec AuditRecord, filter *redaction.Filter) error {
	data, err := json.MarshalIndent(redactAuditRecord(filter, rec), "", "  ")
	if err != nil {
		return err
	}
	redacted := filter.Redact(string(data))
	return os.WriteFile(path, []byte(redacted+"\n"), 0o600)
}
