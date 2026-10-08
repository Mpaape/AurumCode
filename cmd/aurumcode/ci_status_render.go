// One item of the CI status section: what the CI context observed is
// published as observed; what only the model says is labeled as its
// inference and never becomes a state of CI.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/review/cistatus"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// writeCIAnalysisItem renders one CI analysis item by what it rests on
// (cistatus.Context.Verify):
//   - a concluded check of the CI context: the observed state and link. A
//     passing check needs nothing more. A failing one whose evidence quotes
//     the log excerpt shows the observation and, apart, the model's cause
//     and fix; without that evidence the cause is unknown, the model's cause
//     is only a hypothesis, no fix is published and the reader gets how to
//     diagnose.
//   - anything else (no CI context, a name the context does not know, an
//     item that never went through Verify): the model's status is never
//     published; the cause is unknown and the model's cause a hypothesis.
func writeCIAnalysisItem(b *strings.Builder, analysis types.CIAnalysis, copy reviewCopy) {
	if analysis.Basis != cistatus.BasisCI {
		fmt.Fprintf(b, "- **%s — %s** (%s)\n", analysis.Check, copy.ciUnverified, copy.ciUnverifiedNote)
		writeSummaryField(b, copy.cause, copy.ciCauseUnknown)
		writeSummaryField(b, copy.ciHypothesis, analysis.Cause)
		return
	}
	fmt.Fprintf(b, "- **%s — %s** (%s%s)\n", analysis.Check, analysis.Observed, copy.ciVerified, linkSuffix(analysis.Link))
	if cistatus.Passed(analysis.Observed) {
		return
	}
	if analysis.Grounded {
		writeSummaryField(b, copy.ciObserved, analysis.Evidence)
		writeSummaryField(b, copy.ciInferredCause, analysis.Cause)
		writeSummaryField(b, copy.ciInferredFix, analysis.Fix)
		writeSummaryField(b, copy.nextVerification, analysis.NextVerification)
		return
	}
	writeSummaryField(b, copy.cause, copy.ciCauseUnknown)
	writeSummaryField(b, copy.ciHypothesis, analysis.Cause)
	writeSummaryField(b, copy.nextVerification, diagnoseText(analysis.Link, copy))
}

// linkSuffix appends the check's observed link to the verified label.
func linkSuffix(link string) string {
	if link == "" {
		return ""
	}
	return ": " + link
}

// diagnoseText is the orientation for a failure whose cause is unknown.
func diagnoseText(link string, copy reviewCopy) string {
	if link == "" {
		return copy.ciDiagnoseNoLink
	}
	return fmt.Sprintf(copy.ciDiagnose, link)
}
