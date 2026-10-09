// The scanner pass of the evidence phase: every enabled scanner engine of
// quality_gates (quality_gates.sast is the semgrep alias) runs over the
// whole reviewed tree, through the injected executor, before the model. Its
// findings are deterministic evidence the model may weigh, never remove or
// downgrade: they come from the engine's report alone, and the model's
// answer is never consulted. Nothing here names an engine: what differs
// between engines is registry data (internal/scanner).
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// scanReasonUnverifiedCheckout is the suffix of --pr's own reason for a
// checkout not verified as the pull request's own head (AUR-515/536): the
// engine is never invoked, so a stale or unrelated tree is never scanned
// under the reviewed pull request's name. Prefixed by the engine's source,
// Semgrep keeps sast_unverified_checkout.
const scanReasonUnverifiedCheckout = "_unverified_checkout"

// runScanners runs every enabled scanner over root, except those deferred
// to the model's decision (onDemand). A non-empty blocked suffix makes each
// one inconclusive without invoking it. A scan's section
// is "policy" only when the effective entry is the central policy's own;
// the engine then runs hardened against the author's in-tree suppressions.
// Scanner issues never pass through config.ApplyRuleConfig. r is the
// reviewed commit range a history engine scans (empty when unknown: such an
// engine then fails closed).
func (s *reviewState) runScanners(root string, r scanner.Range, blocked string) {
	s.scans, s.deferredScans, s.scanVersions = nil, nil, nil
	s.scanRoot, s.scanRange, s.scanBlocked = root, r, blocked
	for _, entry := range s.cfg.QualityGates.EnabledScanners() {
		if s.onDemand(entry) {
			s.deferredScans = append(s.deferredScans, entry)
			continue
		}
		s.scans = append(s.scans, s.scanEntry(entry))
	}
}

// onDemand reports an entry the model decides about: with deliberation
// enabled, a scanner the configuration does not require is offered as a
// tool instead of running before the model. A required one always runs.
func (s *reviewState) onDemand(entry config.ScannerConfig) bool {
	return s.cfg.Deliberation.Active() && !entry.Required
}

// scanEntry runs one entry over the session's scan root under s.ctx, or
// states why it could not.
func (s *reviewState) scanEntry(entry config.ScannerConfig) gateScan {
	root, blocked := s.scanRoot, s.scanBlocked
	engine, ok := entry.Lookup()
	scan := gateScan{Config: entry, Engine: engine, Section: gateOriginRepo}
	trust := scanner.TrustRepository
	if s.centralCfg != nil && s.cfg.QualityGates.FromPolicy(entry.Name()) {
		scan.Section, trust = gateOriginPolicy, scanner.TrustPolicy
	}
	switch {
	case !ok:
		// Parse refuses an unregistered engine; defensive only.
		scan.Engine = scanner.Unregistered(entry.Name())
		scan.Reason = scanner.FailureReason(scan.Engine, scanner.Report{}, scanner.ErrNotRegistered)
	case blocked != "":
		scan.Reason = scan.Source() + blocked
	default:
		out := s.deps.scanners.Scan(s.ctx, engine, scanner.Request{Root: root, Trust: trust, Options: entry.Options, Range: s.scanRange, Ignored: s.cfg.IgnoresPath})
		if out.Version != "" {
			s.scanVersions = append(s.scanVersions, engine.Name()+"="+out.Version)
		}
		if scan.Reason = out.Reason; scan.Reason == "" {
			scan.Issues = s.scannerIssues(out.Findings, scan.Origin())
		} else {
			scan.Detail = scanner.Summarize(out.Err, s.redactor())
		}
	}
	return scan
}

// scannerIssues converts an engine's findings with their typed origin and
// redacts each message before it can be published.
func (s *reviewState) scannerIssues(findings []scanner.Finding, origin string) []types.ReviewIssue {
	issues := make([]types.ReviewIssue, 0, len(findings))
	for _, f := range findings {
		issue := f.ToIssue(origin)
		if s.filter != nil {
			issue.Message = s.filter.Redact(issue.Message)
		}
		issues = append(issues, issue)
	}
	return issues
}

// redactor is the review's redaction filter, or a fresh one: a failed
// scan's detail quotes engine output, which may quote the scanned content,
// and is never published unredacted.
func (s *reviewState) redactor() scanner.Redactor {
	if s.filter != nil {
		return s.filter.Redact
	}
	return redaction.NewFilter().Redact
}

// scanIssues is every scanner's issues, in declaration order.
func (s *reviewState) scanIssues() []types.ReviewIssue {
	var out []types.ReviewIssue
	for _, scan := range s.scans {
		out = append(out, scan.Issues...)
	}
	return out
}

// scannerReason is the first scanner's inconclusive reason, "" when every
// scan is trustworthy.
func (s *reviewState) scannerReason() string {
	for _, scan := range s.scans {
		if scan.Reason != "" {
			return scan.Reason
		}
	}
	return ""
}

// joinScanners states every inconclusive scan or joins its issues to the
// review's, once the model's answer exists.
func (s *reviewState) joinScanners() {
	for _, scan := range s.scans {
		if scan.Reason != "" {
			notice := scannerInconclusiveNotice(s.reviewLanguage, scan)
			fmt.Fprintf(s.stderr, "aurumcode review: %s\n", notice)
			s.result.Limitations = append(s.result.Limitations, notice)
		} else if len(scan.Issues) > 0 {
			s.result.Issues = append(s.result.Issues, scan.Issues...)
		}
	}
}

// scannerInconclusiveNotice is the declared limitation of a scan that could
// not produce trustworthy findings: a Limitations entry, never a finding.
func scannerInconclusiveNotice(language string, scan gateScan) string {
	label, engine := strings.ToUpper(scan.Source()), displayName(scan.Engine.Name())
	notice := i18n.Format(language, "notice.scanner_inconclusive", label, engine, scan.Reason, engine)
	if scan.Detail != "" {
		notice += " " + i18n.Format(language, "notice.scanner_detail", scan.Detail)
	}
	return notice
}

// displayName capitalizes an engine name for prose ("semgrep" -> "Semgrep").
func displayName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
