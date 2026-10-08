package dependencies

import (
	"context"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// ScanInputs is a whole-tree dependency scan of a checked-out branch, with
// no change to compare against: every declared package is checked against
// the advisory source as it is today. Paths are the tracked files; ReadFile
// reads one of them.
type ScanInputs struct {
	Root      string
	Paths     []string
	ReadFile  func(path string) ([]byte, error)
	Model     Completer
	Source    Source
	Extractor Extractor
}

// Scan reads the packages of the tree: the scanner's extraction of every
// file it recognizes is used as is (deterministic, any size), and the model
// reads only the manifests the scanner does not know, grounded in their
// content. Every advisory found is StatusIntroduced (present on the
// branch). Any failure is a Reason, never "no advisories".
func Scan(ctx context.Context, in ScanInputs) Report {
	var report Report
	if in.Model == nil {
		report.fail(ReasonNoModel, "nenhum modelo para ler os manifestos")
		return report
	}
	scanned := map[string][]Package{}
	if in.Extractor != nil {
		got, err := in.Extractor.Extract(ctx, in.Root, in.Paths)
		if err != nil {
			report.fail(scannerReason(err), err.Error())
			return report
		}
		scanned = got
	}
	if pathBytes(in.Paths) > maxManifestBytes {
		report.fail(ReasonManifestsOmitted, "lista de arquivos acima do limite do modelo")
		return report
	}
	selected, err := selectManifests(ctx, in.Model, pathsOnly(in.Paths))
	if err != nil {
		report.fail(ReasonModelFailed, err.Error())
		return report
	}
	changes := scannedChanges(scanned)
	modelOnly := missingFrom(selected, scanned)
	if len(modelOnly) > 0 {
		kept, ok := readModelOnly(ctx, in, modelOnly, &report)
		if !ok {
			return report
		}
		changes = append(changes, kept...)
	}
	report.Manifests = append(sortedKeys(scanned), modelOnly...)
	report.Changes = changes
	if in.Source == nil {
		report.fail(ReasonSourceFailed, "nenhuma fonte de advisories")
		return report
	}
	classify(ctx, in.Source, changes, &report)
	return report
}

// readModelOnly has the model read the manifests the scanner does not know,
// each whole file as added lines, grounded like a change.
func readModelOnly(ctx context.Context, in ScanInputs, paths []string, report *Report) ([]Change, bool) {
	diff := &types.Diff{}
	for _, p := range paths {
		data, err := in.ReadFile(p)
		if err != nil {
			report.fail(ReasonManifestsOmitted, "leitura de "+p+": "+err.Error())
			return nil, false
		}
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			lines = append(lines, "+"+line)
		}
		diff.Files = append(diff.Files, types.DiffFile{Path: p, Hunks: []types.DiffHunk{{NewStart: 1, NewLines: len(lines), Lines: lines}}})
	}
	raw, omitted, err := extractChanges(ctx, in.Model, diff, paths)
	switch {
	case err != nil:
		report.fail(ReasonModelFailed, err.Error())
		return nil, false
	case len(omitted) > 0:
		report.fail(ReasonManifestsOmitted, "manifestos acima do limite: "+strings.Join(omitted, ", "))
		return nil, false
	}
	kept, discarded := ground(diff, paths, raw)
	report.Discarded = append(report.Discarded, discarded...)
	return kept, true
}

// scannedChanges is every package the scanner extracted, as a package
// present on the branch. A package without a name or a version is never
// dropped: it is unresolved, and the scan is then inconclusive, so a scan
// never concludes over fewer packages than the tree declares.
func scannedChanges(scanned map[string][]Package) []Change {
	var out []Change
	for _, path := range sortedKeys(scanned) {
		for _, p := range scanned[path] {
			c := Change{Manifest: path, Ecosystem: p.Ecosystem, Name: p.Name, Head: p.Version}
			if p.Name == "" || p.Version == "" {
				c.Unresolved = true
			}
			out = append(out, c)
		}
	}
	return out
}

func missingFrom(selected []string, scanned map[string][]Package) []string {
	var out []string
	for _, p := range selected {
		if _, ok := scanned[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func pathsOnly(paths []string) *types.Diff {
	diff := &types.Diff{}
	for _, p := range paths {
		diff.Files = append(diff.Files, types.DiffFile{Path: p})
	}
	return diff
}

func pathBytes(paths []string) int {
	n := 0
	for _, p := range paths {
		n += len(p) + 4
	}
	return n
}

func scannerReason(err error) string {
	if isMissing(err) {
		return ReasonScannerMissing
	}
	return ReasonScannerFailed
}
