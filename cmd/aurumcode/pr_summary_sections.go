// Sections of the parecer that depend on the facts of this run: the CI
// status and the tests the change touches.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// metaAffectedTests is the metadata key with the tests the change touches,
// one "<name> (package <pkg>)" per line, found by the static test proposal.
const metaAffectedTests = "affected_tests"

// maxAffectedTestsShown bounds the names listed under the count.
const maxAffectedTestsShown = 10

// writeCIStatusSection renders the CI status. When every analysis item was
// discarded as not a fact of this run, the section says so in one line
// instead of disappearing.
func writeCIStatusSection(b *strings.Builder, result *types.ReviewResult, copy reviewCopy) {
	if len(result.CIAnalysis) == 0 {
		if discarded := atoiOrZero(result.Metadata[ciStatusDiscardedKey]); discarded > 0 {
			fmt.Fprintf(b, "#### %s\n\n%s\n\n", copy.ciStatus, countText(discarded, copy.ciNothingFailedOne, copy.ciNothingFailed))
		}
		return
	}
	fmt.Fprintf(b, "#### %s\n\n", copy.ciStatus)
	for _, analysis := range result.CIAnalysis {
		writeCIAnalysisItem(b, analysis, copy)
	}
	b.WriteString("\n")
}

// writeAffectedTests renders the tests the change touches as a count with
// the first names, never the whole list: a large change touches hundreds.
func writeAffectedTests(b *strings.Builder, result *types.ReviewResult, copy reviewCopy) {
	names := splitNonEmptyLines(result.Metadata[metaAffectedTests])
	if len(names) == 0 {
		return
	}
	packages := map[string]bool{}
	for _, name := range names {
		if _, pkg, ok := strings.Cut(name, " (package "); ok {
			packages[strings.TrimSuffix(pkg, ")")] = true
		}
	}
	fmt.Fprintf(b, "- %s", fmt.Sprintf(copy.affectedTests, len(names), len(packages)))
	shown := names
	if len(shown) > maxAffectedTestsShown {
		shown = shown[:maxAffectedTestsShown]
	}
	sort.Strings(shown)
	fmt.Fprintf(b, ": %s", strings.Join(shown, ", "))
	if len(names) > len(shown) {
		b.WriteString(", …")
	}
	b.WriteString("\n")
}
