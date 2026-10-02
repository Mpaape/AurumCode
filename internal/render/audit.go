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
	PolicyDigest string `json:"policy_digest"`
	WorkflowSHA  string `json:"workflow_sha,omitempty"`
	Repo         string `json:"repo,omitempty"`
	ReviewedSHA  string `json:"reviewed_sha,omitempty"`
	Model        string `json:"model,omitempty"`
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

// WriteAuditRecord marshals rec as indented JSON, runs the complete text
// through filter -- the single AUR-009 redaction filter every sink in this
// system writes through -- and writes the redacted result to path (AC-005).
// Every string in rec passes through the same filter before any byte
// reaches disk; a registered secret (including the AURUM_SECRET_CANARY
// value) is replaced wherever it appears in the serialized record, field
// boundaries or not.
func WriteAuditRecord(path string, rec AuditRecord, filter *redaction.Filter) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	redacted := filter.Redact(string(data))
	return os.WriteFile(path, []byte(redacted+"\n"), 0o600)
}
