// AUR-521: the audit record a policy-governed review writes alongside its
// SARIF (sarif.go). It carries exactly the fields AC-001 names: the policy
// digest, the workflow and reviewed SHAs, the repo, the model, the verdict,
// the gate's own decision, the findings that actually blocked it, the
// exceptions applied to it (AUR-520's own field -- empty until that card
// lands), and the coverage this run achieved.
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
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
}

// AnalysisDataAudit identifies the verified analysis-data artifact a review
// used: the manifest's set digest, its generation date (RFC 3339, UTC) and
// the release tag.
type AnalysisDataAudit struct {
	Digest      string `json:"digest"`
	GeneratedAt string `json:"generated_at"`
	Tag         string `json:"tag"`
	// Source is "remote" (release listing answered) or "cache" (listing
	// failed; a locally cached copy, re-verified, was used).
	Source string `json:"source,omitempty"`
}

// AuditGate is the gate's own decision for this run: "pass", "fail" or
// "inconclusive", plus the reason lines the gate itself produced (empty on
// a plain pass).
type AuditGate struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// AuditFinding is one finding that actually contributed to the gate's
// decision -- not every finding the review raised, only the ones the gate
// matched against its threshold.
type AuditFinding struct {
	RuleID   string `json:"rule_id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	// Origin is where the finding came from: skills, analysis, security,
	// or a scanner engine's typed origin (sast for semgrep).
	Origin string `json:"origin,omitempty"`
}

// AuditException documents one AUR-520 exception applied to a finding. Its
// shape is fixed now so AUR-520 only has to populate it, never redesign the
// audit record around it.
type AuditException struct {
	RuleID        string `json:"rule_id"`
	Path          string `json:"path"`
	Justification string `json:"justification"`
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

// PolicyDigest is AC-001's policy-change-detection digest: sha256 over the
// central policy's own config.yml bytes, then every skill file its
// review.context.skills names (in that declared order -- deterministic
// because it is the policy author's own order, never re-sorted into a
// different one), hex-encoded. A nil cfg (no policy declared this run)
// returns "" -- an empty digest means no policy was active, never "a
// policy whose content happens to hash to the empty string".
//
// A skill file this function cannot read contributes only its own
// (trimmed) path to the digest and no error: LoadCentralPolicy has already
// refused to start the run at all when a non-optional skill was missing, so
// this is defense in depth, never the enforcement point -- and it must
// never panic or abort a review just to compute an audit-trail digest.
func PolicyDigest(policyDir string, cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	h := sha256.New()
	configPath := filepath.Join(policyDir, config.DefaultConfigPath)
	if data, err := os.ReadFile(configPath); err == nil {
		h.Write(data)
	}
	for _, skill := range cfg.Review.Context.Skills {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			continue
		}
		h.Write([]byte("\x00" + skill + "\x00"))
		if data, err := os.ReadFile(filepath.Join(policyDir, filepath.FromSlash(skill))); err == nil {
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
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
