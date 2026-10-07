package dependencies

import (
	"context"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Inputs is one dependency check. Model reads the diff; Source answers
// about advisories; Extractor cross-checks the model on the checked-out
// head at Root. Blocked, when set, is why the head checkout cannot be
// trusted (the extraction then never runs and the check is inconclusive).
type Inputs struct {
	Diff      *types.Diff
	Model     Completer
	Source    Source
	Extractor Extractor
	Root      string
	Blocked   string
}

// Check runs the dependency check. Every failure is a Reason on the report:
// no model, a model answer that is not the contract, an unreachable or stale
// source, a missing scanner, an unresolved version. None is ever "no
// findings".
func Check(ctx context.Context, in Inputs) Report {
	var report Report
	if in.Model == nil {
		report.fail(ReasonNoModel, "nenhum modelo para ler os manifestos")
		return report
	}
	manifests, err := selectManifests(ctx, in.Model, in.Diff)
	if err != nil {
		report.fail(ReasonModelFailed, err.Error())
		return report
	}
	report.Manifests = manifests
	if len(manifests) == 0 {
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
	if strings.TrimSpace(in.Blocked) != "" {
		report.fail(ReasonCheckoutBlocked, in.Blocked)
		return report
	}
	if in.Extractor != nil {
		confer(ctx, in.Extractor, in.Root, report.Changes, &report)
	}
	if report.Inconclusive() {
		return report
	}
	if in.Source == nil {
		report.fail(ReasonSourceFailed, "nenhuma fonte de advisories")
		return report
	}
	classify(ctx, in.Source, report.Changes, &report)
	return report
}
