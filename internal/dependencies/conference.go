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

// ErrScannerFailed marks an extraction that ran and cannot be trusted.
var ErrScannerFailed = errors.New("dependencies: extraction scanner failed")

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

// The scanner's documented exit codes (osv-scanner "Return codes"): 1 is
// "vulnerabilities found", 128 "no packages found"; 127 and every other
// code are errors.
const (
	osvScannerVulnerable = 1
	osvScannerNoPackages = 128
)

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

// Extract runs the scanner, bounded by scanner.Timeout. Only exit 0 (no
// vulnerabilities) and exit 1 (vulnerabilities found) carry a report; exit
// 128 is the scanner's documented "no packages found" (an empty extraction);
// any other exit, a timeout or a kill is a failed extraction even when the
// output happens to be JSON. Paths in the report are resolved against the
// absolute root, so a relative --repo still matches the changed paths.
func (o OSVScanner) Extract(ctx context.Context, root string, paths []string) (map[string][]Package, error) {
	command := o.Command
	if command == nil {
		command = scanner.ExecCommand
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScannerFailed, err)
	}
	ctx, cancel := context.WithTimeout(ctx, scanner.Timeout)
	defer cancel()
	stdout, _, err := command(ctx, absRoot, o.Binary, append(append([]string{}, osvScannerArgs...), absRoot)...)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrScannerMissing, o.Binary)
	}
	switch code := exitCode(err); code {
	case 0, osvScannerVulnerable:
	case osvScannerNoPackages:
		return map[string][]Package{}, nil
	default:
		return nil, fmt.Errorf("%w: exit %d (%v)", ErrScannerFailed, code, err)
	}
	var report osvScannerReport
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &report); jsonErr != nil {
		return nil, fmt.Errorf("%w: output unreadable: %v", ErrScannerFailed, jsonErr)
	}
	return report.packagesOf(absRoot, paths), nil
}

// exitCode is the scanner's exit status: 0 without error, the process's
// code for an exit error, -1 for anything else (a timeout, a kill, an I/O
// failure).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return -1
}

// packagesOf keeps the packages of the changed paths, keyed by the path
// relative to root.
func (r osvScannerReport) packagesOf(absRoot string, paths []string) map[string][]Package {
	wanted := map[string]bool{}
	for _, p := range paths {
		wanted[p] = true
	}
	out := map[string][]Package{}
	for _, res := range r.Results {
		source := res.Source.Path
		if !filepath.IsAbs(source) {
			source = filepath.Join(absRoot, source)
		}
		rel, relErr := filepath.Rel(absRoot, source)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !wanted[rel] {
			continue
		}
		for _, p := range res.Packages {
			out[rel] = append(out[rel], Package{Ecosystem: p.Package.Ecosystem, Name: p.Package.Name, Version: p.Package.Version})
		}
		if _, ok := out[rel]; !ok {
			out[rel] = nil
		}
	}
	return out
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
// ways, and returns the changes the source is asked about. When the scanner
// lists the package, its ecosystem and version win over the model's: the
// model's head version is kept only when the scanner lists it too; a single
// scanner version the change adds replaces a divergent model version (the
// divergence is declared); several candidates, or none the change adds, make
// the check inconclusive. A package the scanner does not list keeps the
// model's reading, declared. A package the scanner extracts from the added
// lines and the model left out is a gap: inconclusive.
func confer(diff *types.Diff, scanned map[string][]Package, changes []Change, report *Report) []Change {
	out := append([]Change(nil), changes...)
	for _, manifest := range sortedKeys(scanned) {
		pkgs := scanned[manifest]
		s := diffSides(diff, manifest)
		var mine []Change
		for i := range out {
			if out[i].Manifest != manifest {
				continue
			}
			if out[i].Head != "" {
				out[i] = alignWithScanner(out[i], pkgs, s, report)
			}
			mine = append(mine, out[i])
		}
		for _, p := range pkgs {
			if p.Version == "" || !strings.Contains(s.added, p.Version) || !strings.Contains(s.all, p.Name) || modelHas(mine, p) {
				continue
			}
			report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o scanner extraiu %s %s (%s), que a mudança adiciona, e o modelo não", manifest, p.Name, p.Version, p.Ecosystem))
			report.fail(ReasonExtractionGap, fmt.Sprintf("o modelo omitiu %s %s em %s", p.Name, p.Version, manifest))
		}
	}
	return out
}

// alignWithScanner applies the scanner's extraction to one model change.
func alignWithScanner(c Change, pkgs []Package, s sides, report *Report) Change {
	var named, added []Package
	for _, p := range pkgs {
		if !strings.EqualFold(p.Name, c.Name) {
			continue
		}
		named = append(named, p)
		if p.Version != "" && strings.Contains(s.added, p.Version) {
			added = append(added, p)
		}
	}
	for _, p := range named {
		if p.Version == c.Head {
			if p.Ecosystem != "" && p.Ecosystem != c.Ecosystem {
				report.Divergences = append(report.Divergences, fmt.Sprintf("%s: ecossistema de %s é %s pelo scanner, não %s", c.Manifest, c.Name, p.Ecosystem, c.Ecosystem))
				c.Ecosystem = p.Ecosystem
			}
			return c
		}
	}
	switch {
	case len(named) == 0:
		report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o modelo extraiu %s %s (%s) e o scanner não", c.Manifest, c.Name, c.Head, c.Ecosystem))
	case len(added) == 1:
		report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o modelo extraiu %s %s (%s); vale a extração do scanner, %s (%s)", c.Manifest, c.Name, c.Head, c.Ecosystem, added[0].Version, added[0].Ecosystem))
		c.Head = added[0].Version
		if added[0].Ecosystem != "" {
			c.Ecosystem = added[0].Ecosystem
		}
	default:
		report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o modelo extraiu %s %s (%s) e o scanner lista outra versão", c.Manifest, c.Name, c.Head, c.Ecosystem))
		report.fail(ReasonExtractionGap, fmt.Sprintf("versão de %s em %s diverge do scanner", c.Name, c.Manifest))
	}
	return c
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

func isMissing(err error) bool { return errors.Is(err, ErrScannerMissing) }
