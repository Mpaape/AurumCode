// Phase 4 of the --base path: write the compliance artifacts, print the
// report and decide the exit code.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// publish writes AUR-521's audit record and SARIF once the gate decision is
// final, prints the report and returns the exit code: the quality failure
// (AUR-458) outranks the policy gate (AUR-519), which outranks --fail-on.
func (b *baseReview) publish() (int, bool) {
	b.writeArtifacts()
	b.printReport()
	if b.qualityFailed {
		// "Did not review" outranks "reviewed and found things": exit 1,
		// the taxonomy's existing behavioral failure (docs/specs/AUR-458.md).
		return exitQualityNotReviewed, true
	}
	if code, closed := gateExitCode(b.gateRes); closed {
		return code, true
	}
	return b.failOnExit(), true
}

func (b *baseReview) writeArtifacts() {
	res := b.gateRes
	writeComplianceArtifacts(complianceArtifactInputs{
		auditoriaPath:          b.f.auditoria,
		sarifPath:              b.f.sarif,
		policyDir:              b.policyDir,
		centralCfg:             b.centralCfg,
		repo:                   os.Getenv("GITHUB_REPOSITORY"),
		reviewedSHA:            os.Getenv("GITHUB_SHA"),
		model:                  firstNonEmpty(b.f.modelo, os.Getenv("LLM_MODEL")),
		verdict:                canonicalVerdict(b.result),
		gate:                   *res,
		gateInconclusiveReason: res.Reason,
		analysisData:           res.AnalysisData,
		diff:                   b.diff,
		issues:                 b.run.IssuesForGate(),
		dynamicRules:           b.dynamicRules,
		coverageComplete:       !b.coverage.partial(),
		omittedFiles:           append(append([]string{}, b.coverage.IgnoredPaths...), b.coverage.FilteredPaths...),
	}, b.filter, b.stderr)
}

// printReport prints the --base report (AUR-490): diff notices, the report
// with its summary, the coverage notice (AUR-476), suggestions, the
// changelog and the findings. A skipped or failed quality review is never
// reported as "No issues found." (AUR-449/458): the report says it covers
// deterministic analysis only and the verdict is "comment".
func (b *baseReview) printReport() {
	result := b.result
	printNotices(b.stdout, b.filter, b.notices)
	if b.qualitySkipped || b.qualityFailed {
		result.Verdict = "comment"
		// canonicalVerdict reads this same flag, so a quality-degraded
		// result with no deterministic findings cannot canonicalize to
		// "approve" (AUR-449/458).
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["quality_degraded"] = "true"
		fmt.Fprintln(b.stdout, "LLM quality review did not run. The following report covers deterministic analysis only.")
	}
	fmt.Fprint(b.stdout, renderLocalReport(result, b.diff, b.reviewLanguage))
	if b.coverageText != "" {
		fmt.Fprint(b.stdout, "\n"+b.coverageText+"\n")
	}
	if len(b.skillNotices) > 0 {
		var notes strings.Builder
		fmt.Fprintf(&notes, "\n### %s\n\n", reviewCopyFor(b.reviewLanguage).limits)
		writeReviewBullets(&notes, b.skillNotices)
		fmt.Fprint(b.stdout, notes.String())
	}
	if suggestions := renderSuggestions(result, b.diff, b.reviewLanguage); suggestions != "" {
		fmt.Fprint(b.stdout, "\n"+suggestions)
	}
	if b.changelogText != "" {
		fmt.Fprint(b.stdout, "\n"+b.changelogText)
	}
	if (!b.qualitySkipped && !b.qualityFailed) || len(result.Issues) > 0 {
		printFindings(b.stdout, result, b.gateRes.Reason)
	}
	if !b.qualityFailed {
		persistReviewMemory(b.memoryStore, b.repoCfg.Review.Memory, b.memoryNotes, result.Issues, b.stderr, b.filter)
	}
	if b.f.seguranca {
		printSecurityFindings(b.stdout, b.filter, b.securityFindings)
	}
}

// failOnExit is the --fail-on CI gate (AUR-431): exit 3 when any finding,
// the security pass's included, sits at the chosen severity or above. The
// note goes to stderr so stdout stays byte-identical with and without it.
func (b *baseReview) failOnExit() int {
	if b.threshold <= 0 {
		return 0
	}
	gated := b.result.Issues
	if len(b.securityFindings) > 0 {
		gated = make([]types.ReviewIssue, 0, len(b.result.Issues)+len(b.securityFindings))
		gated = append(gated, b.result.Issues...)
		gated = append(gated, b.securityFindings...)
	}
	if n := countAtOrAbove(gated, b.threshold); n > 0 {
		fmt.Fprintf(b.stderr, "aurumcode review: %d finding(s) at severity %s or above (--fail-on %s)\n", n, b.thresholdName, b.thresholdName)
		return exitFindings
	}
	return 0
}
