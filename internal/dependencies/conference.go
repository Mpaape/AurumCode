package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// ErrScannerMissing marks an extraction scanner that is not installed.
var ErrScannerMissing = errors.New("dependencies: extraction scanner unavailable")

// Package is one package a scanner extracted from a file.
type Package struct {
	Ecosystem string
	Name      string
	Version   string
}

// Extractor is the deterministic extraction the model's answer is checked
// against: for the changed paths it recognizes, the packages it extracted
// from the checked-out head. A path absent from the answer is a file the
// scanner does not know. A missing scanner is ErrScannerMissing; any other
// error is a failed extraction. Both are inconclusive, never "no packages".
type Extractor interface {
	Extract(ctx context.Context, root string, paths []string) (map[string][]Package, error)
}

// OSVScanner runs osv-scanner once over the checked-out head (recursive) and
// keeps every package it extracted (--all-packages) from the changed paths.
// Its own advisory lookup is not used: the advisories come from the Source.
type OSVScanner struct {
	Binary  string
	Command scanner.Command
}

// osvScannerArgs is the scanner's documented CLI contract: the JSON report
// of every package of every lockfile under the root.
var osvScannerArgs = []string{"scan", "source", "--format", "json", "--all-packages", "--recursive"}

// osvScannerNoPackages is the scanner's exit code for "no package sources
// found": a tree without any file it knows.
const osvScannerNoPackages = 128

type osvScannerReport struct {
	Results []struct {
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
		Packages []struct {
			Package struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Ecosystem string `json:"ecosystem"`
			} `json:"package"`
		} `json:"packages"`
	} `json:"results"`
}

// Extract runs the scanner, bounded by scanner.Timeout. A nonzero exit with
// a readable report is the scanner's "vulnerabilities found".
func (o OSVScanner) Extract(ctx context.Context, root string, paths []string) (map[string][]Package, error) {
	command := o.Command
	if command == nil {
		command = scanner.ExecCommand
	}
	ctx, cancel := context.WithTimeout(ctx, scanner.Timeout)
	defer cancel()
	stdout, _, err := command(ctx, root, o.Binary, append(append([]string{}, osvScannerArgs...), root)...)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrScannerMissing, o.Binary)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == osvScannerNoPackages {
		return map[string][]Package{}, nil
	}
	var report osvScannerReport
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &report); jsonErr != nil {
		return nil, fmt.Errorf("dependencies: extraction scanner output unreadable (%v): %w", err, jsonErr)
	}
	wanted := map[string]bool{}
	for _, p := range paths {
		wanted[p] = true
	}
	out := map[string][]Package{}
	for _, r := range report.Results {
		rel, relErr := filepath.Rel(root, r.Source.Path)
		if relErr != nil {
			rel = r.Source.Path
		}
		rel = filepath.ToSlash(rel)
		if !wanted[rel] {
			continue
		}
		for _, p := range r.Packages {
			out[rel] = append(out[rel], Package{Ecosystem: p.Package.Ecosystem, Name: p.Package.Name, Version: p.Package.Version})
		}
		if _, ok := out[rel]; !ok {
			out[rel] = nil
		}
	}
	return out, nil
}

// adoptScanned adds to manifests every changed file the scanner recognizes
// and the model did not name, and declares the omission: the model never
// removes deterministic evidence.
func adoptScanned(manifests []string, scanned map[string][]Package, report *Report) []string {
	chosen := map[string]bool{}
	for _, m := range manifests {
		chosen[m] = true
	}
	for _, path := range sortedKeys(scanned) {
		if !chosen[path] {
			manifests = append(manifests, path)
			report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o scanner reconhece o arquivo e o modelo nao o apontou como manifesto", path))
		}
	}
	return manifests
}

// confer checks the model's packages against the scanner's extraction, both
// ways. A head version the scanner does not list is a declared divergence.
// A package the scanner lists whose name and version the change adds, and
// which the model left out, is a gap: the check is inconclusive, because the
// model's answer is not complete.
func confer(diff *types.Diff, scanned map[string][]Package, changes []Change, report *Report) {
	for _, manifest := range sortedKeys(scanned) {
		pkgs := scanned[manifest]
		var mine []Change
		for _, c := range changes {
			if c.Manifest == manifest {
				mine = append(mine, c)
			}
		}
		for _, c := range mine {
			if c.Head != "" && !scannerHas(pkgs, c) {
				report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o modelo extraiu %s %s (%s) e o scanner nao", manifest, c.Name, c.Head, c.Ecosystem))
			}
		}
		s := diffSides(diff, manifest)
		for _, p := range pkgs {
			if p.Version == "" || !strings.Contains(s.added, p.Version) || !strings.Contains(s.all, p.Name) || modelHas(mine, p) {
				continue
			}
			report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o scanner extraiu %s %s (%s), que a mudanca adiciona, e o modelo nao", manifest, p.Name, p.Version, p.Ecosystem))
			report.fail(ReasonExtractionGap, fmt.Sprintf("o modelo omitiu %s %s em %s", p.Name, p.Version, manifest))
		}
	}
}

func scannerHas(pkgs []Package, c Change) bool {
	for _, p := range pkgs {
		if strings.EqualFold(p.Name, c.Name) && p.Version == c.Head {
			return true
		}
	}
	return false
}

func modelHas(changes []Change, p Package) bool {
	for _, c := range changes {
		if strings.EqualFold(c.Name, p.Name) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string][]Package) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isStale(err error) bool { return errors.Is(err, ErrStale) }
