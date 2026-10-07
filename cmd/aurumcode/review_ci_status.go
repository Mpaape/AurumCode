// The CI status a review publishes rests on facts of this execution: the
// model sees only concluded checks of other producers, and an analysis item
// about a running check, a status this product published or a scanner that
// did not run is discarded before publication.
package main

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/review/cistatus"
)

// ciStatusDiscardedKey is the result metadata key that counts the discarded
// CI analysis items.
const ciStatusDiscardedKey = "ci_status_discarded"

// ownStatusPrefix is the prefix of every commit status this product
// publishes ("aurumcode/").
var ownStatusPrefix = path.Dir(policyGateContext) + "/"

// readCIFacts parses the workflow-produced CI context.
func readCIFacts() cistatus.Context { return cistatus.Parse(readCIContext(), ownStatusPrefix) }

// noCIContext is a session without a CI context (a local review): only
// the own-status and scanner rules can apply.
func noCIContext() cistatus.Context { return cistatus.Parse("", ownStatusPrefix) }

// settleCIStatus keeps only the CI analysis items that rest on a fact of
// this execution, after every scanner of the session has run.
func (s *reviewState) settleCIStatus(ci cistatus.Context) {
	if s.result == nil || len(s.result.CIAnalysis) == 0 {
		return
	}
	executed := make([]string, 0, 2*len(s.scans))
	for _, scan := range s.scans {
		executed = append(executed, scan.Config.Name(), scan.Engine.Name())
	}
	kept, discarded := cistatus.Facts{Context: ci, Executed: executed}.Keep(s.result.CIAnalysis)
	s.result.CIAnalysis = kept
	if len(discarded) == 0 {
		return
	}
	if s.result.Metadata == nil {
		s.result.Metadata = map[string]string{}
	}
	s.result.Metadata[ciStatusDiscardedKey] = strconv.Itoa(len(discarded))
	names := make([]string, 0, len(discarded))
	for _, d := range discarded {
		names = append(names, d.Check+" ("+d.Reason+")")
	}
	fmt.Fprintf(s.stderr, "aurumcode review: %d ci_analysis item(s) discarded, not a fact of this execution: %s\n", len(discarded), strings.Join(names, ", "))
}
