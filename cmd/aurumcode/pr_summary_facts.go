// The facts line of the published summary: gate, files, parts, context.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// factsLine is the line under the headline: the gate, how many files were
// reviewed and in how many parts, and whether
// the codebase context was reduced to fit the model's window.
func factsLine(result *types.ReviewResult, diff *types.Diff, copy reviewCopy, rule blocking.Rule) string {
	gate := copy.factsNoGate
	if rule.Gated() {
		gate = copy.factsGatePassed
		if rule.Fails() {
			gate = copy.factsGateFailed
		}
	}
	parts := []string{gate}
	files := 0
	if diff != nil {
		files = len(diff.Files)
	} else if n, err := strconv.Atoi(result.Metadata["total_files"]); err == nil {
		files = n
	}
	if files > 0 {
		filesText := countText(files, copy.factsFilesOne, copy.factsFiles)
		if n, err := strconv.Atoi(result.Metadata[metaReviewParts]); err == nil && n > 1 {
			filesText += " " + fmt.Sprintf(copy.factsParts, n)
		}
		parts = append(parts, filesText)
	}
	if result.Metadata[metaContextReduced] != "" {
		parts = append(parts, copy.factsContextReduced)
	}
	return strings.Join(parts, " · ")
}
