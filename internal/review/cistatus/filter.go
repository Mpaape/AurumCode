package cistatus

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/review/tools"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Facts is what this execution observed: the split CI context and the
// scanners that actually ran (each by entry and engine name).
type Facts struct {
	Context  Context
	Executed []string
}

// Discard is one analysis item removed from the review and why.
type Discard struct {
	Check  string
	Reason string
}

// Reasons a CI analysis item is discarded.
const (
	ReasonOwnStatus     = "own_status"
	ReasonWithoutResult = "without_result"
	ReasonScannerNotRun = "scanner_not_run"
	reasonNone          = ""
)

// Keep returns the items that rest on a fact of this execution and the ones
// it discarded. An item is discarded when it is about a status this product
// published, a check without a result, or a scanner that did not run in
// this execution. Every other item is kept: a concluded failure is never
// hidden because the model named it differently.
func (f Facts) Keep(items []types.CIAnalysis) ([]types.CIAnalysis, []Discard) {
	kept := make([]types.CIAnalysis, 0, len(items))
	var discarded []Discard
	executed := f.executedSet()
	for _, item := range items {
		if reason := f.reason(item, executed); reason != reasonNone {
			discarded = append(discarded, Discard{Check: item.Check, Reason: reason})
			continue
		}
		kept = append(kept, item)
	}
	return kept, discarded
}

func (f Facts) reason(item types.CIAnalysis, executed map[string]bool) string {
	switch {
	case f.Context.Own(item.Check):
		return ReasonOwnStatus
	case f.Context.withoutResult(item.Check), stillRunning(item.Status):
		return ReasonWithoutResult
	}
	if engine, ok := scannerEngine(item.Check); ok && !executed[engine] {
		return ReasonScannerNotRun
	}
	return reasonNone
}

// stillRunning reports a status the model itself labels as without result.
func stillRunning(status string) bool {
	return pendingStates[normalizeState(status)]
}

// pendingStates are the states of a check that has not finished.
var pendingStates = map[string]bool{
	"PENDING": true, "QUEUED": true, "IN_PROGRESS": true, "WAITING": true,
	"REQUESTED": true, "EXPECTED": true, "RUNNING": true,
}

// scannerEngine resolves an item that names a scanner: its tool name
// (scanner_<engine>) or a registered engine name.
func scannerEngine(check string) (string, bool) {
	name := normalizeName(check)
	if engine, ok := strings.CutPrefix(name, tools.ScannerToolPrefix); ok && engine != "" {
		return engine, true
	}
	if _, ok := scanner.Lookup(name); ok {
		return name, true
	}
	return "", false
}

func (f Facts) executedSet() map[string]bool {
	out := make(map[string]bool, len(f.Executed))
	for _, name := range f.Executed {
		if n := normalizeName(name); n != "" {
			out[n] = true
		}
	}
	return out
}
