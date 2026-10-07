package dependencies

import (
	"context"
	"errors"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Inputs is one dependency check. Model reads the diff; Source answers
// about advisories; Extractor cross-checks the model on the checked-out
// head at Root. Blocked, when set, is why the head checkout cannot be
// trusted (the extraction then never runs and the check is inconclusive).
// Registry, when set, vets every new or updated package with its live
// registry metadata (suspicion.go).
type Inputs struct {
	Diff      *types.Diff
	Model     Completer
	Source    Source
	Extractor Extractor
	Registry  Registry
	Root      string
	Blocked   string
}

// Check runs the dependency check. Every failure is a Reason on the report:
// no model, a model answer that is not the contract, an unreachable or stale
// source, a missing or failing scanner, an extraction the scanner shows to
// be incomplete, an unresolved version. None is ever "no findings".
func Check(ctx context.Context, in Inputs) Report {
	var report Report
	switch {
	case in.Model == nil:
		report.fail(ReasonNoModel, "nenhum modelo para ler os manifestos")
		return report
	case strings.TrimSpace(in.Blocked) != "":
		report.fail(ReasonCheckoutBlocked, in.Blocked)
		return report
	}
	scanned, manifests, ok := readManifests(ctx, in, &report)
	if !ok || len(manifests) == 0 {
		return report
	}
	raw, omitted, err := extractChanges(ctx, in.Model, in.Diff, manifests)
	if err != nil {
		report.fail(ReasonModelFailed, err.Error())
		return report
	}
	if len(omitted) > 0 {
		report.fail(ReasonManifestsOmitted, "manifestos acima do limite: "+strings.Join(omitted, ", "))
		return report
	}
	report.Changes, report.Discarded = ground(in.Diff, manifests, raw)
	confer(in.Diff, scanned, report.Changes, &report)
	if report.Inconclusive() {
		return report
	}
	if in.Source == nil {
		report.fail(ReasonSourceFailed, "nenhuma fonte de advisories")
		return report
	}
	classify(ctx, in.Source, report.Changes, &report)
	if report.Inconclusive() || in.Registry == nil {
		return report
	}
	vetRegistry(ctx, in.Model, in.Registry, &report)
	return report
}

// readManifests asks the model which changed files are manifests and the
// scanner which changed files it recognizes; the union is read, and a file
// only the scanner named is declared. ok is false when either failed.
func readManifests(ctx context.Context, in Inputs, report *Report) (map[string][]Package, []string, bool) {
	manifests, err := selectManifests(ctx, in.Model, in.Diff)
	if err != nil {
		report.fail(ReasonModelFailed, err.Error())
		return nil, nil, false
	}
	var scanned map[string][]Package
	if in.Extractor != nil {
		scanned, err = in.Extractor.Extract(ctx, in.Root, diffPaths(in.Diff))
		if err != nil {
			reason := ReasonScannerFailed
			if errors.Is(err, ErrScannerMissing) {
				reason = ReasonScannerMissing
			}
			report.fail(reason, err.Error())
			return nil, nil, false
		}
		manifests = adoptScanned(manifests, scanned, report)
	}
	report.Manifests = manifests
	return scanned, manifests, true
}
