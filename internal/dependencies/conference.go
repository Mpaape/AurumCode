package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
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
// against. Recognized is false when the scanner does not know the file:
// the model's extraction then stands alone. A missing scanner is
// ErrScannerMissing.
type Extractor interface {
	Extract(ctx context.Context, root, path string) (pkgs []Package, recognized bool, err error)
}

// OSVScanner runs osv-scanner over one file of the checked-out head and
// reads every package it extracted (--all-packages), whether or not it is
// vulnerable. Its own advisory lookup is not used: the advisories come from
// the Source.
type OSVScanner struct {
	Binary  string
	Command scanner.Command
}

// osvScannerArgs is the scanner's documented CLI contract: one lockfile, the
// JSON report, every package listed.
var osvScannerArgs = []string{"scan", "source", "--format", "json", "--all-packages", "--lockfile"}

type osvScannerReport struct {
	Results []struct {
		Packages []struct {
			Package struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Ecosystem string `json:"ecosystem"`
			} `json:"package"`
		} `json:"packages"`
	} `json:"results"`
}

// Extract runs the scanner. A nonzero exit with a readable report is the
// scanner's "vulnerabilities found"; an unreadable report means it does not
// recognize the file.
func (o OSVScanner) Extract(ctx context.Context, root, path string) ([]Package, bool, error) {
	command := o.Command
	if command == nil {
		command = scanner.ExecCommand
	}
	args := append(append([]string{}, osvScannerArgs...), filepath.Join(root, filepath.FromSlash(path)))
	stdout, _, err := command(ctx, root, o.Binary, args...)
	if errors.Is(err, exec.ErrNotFound) {
		return nil, false, fmt.Errorf("%w: %s", ErrScannerMissing, o.Binary)
	}
	var report osvScannerReport
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &report); jsonErr != nil || len(report.Results) == 0 {
		return nil, false, nil
	}
	var out []Package
	for _, r := range report.Results {
		for _, p := range r.Packages {
			out = append(out, Package{Ecosystem: p.Package.Ecosystem, Name: p.Package.Name, Version: p.Package.Version})
		}
	}
	return out, true, nil
}

// confer checks the model's head-side packages of each manifest against the
// scanner's extraction and declares every divergence. A missing scanner is
// inconclusive; a file the scanner does not know is the model's alone.
func confer(ctx context.Context, x Extractor, root string, changes []Change, report *Report) {
	byManifest := map[string][]Change{}
	var order []string
	for _, c := range changes {
		if c.Removed() {
			continue
		}
		if _, ok := byManifest[c.Manifest]; !ok {
			order = append(order, c.Manifest)
		}
		byManifest[c.Manifest] = append(byManifest[c.Manifest], c)
	}
	for _, manifest := range order {
		pkgs, recognized, err := x.Extract(ctx, root, manifest)
		if err != nil {
			report.fail(ReasonScannerMissing, err.Error())
			return
		}
		if !recognized {
			continue
		}
		for _, c := range byManifest[manifest] {
			if c.Head != "" && !scannerHas(pkgs, c) {
				report.Divergences = append(report.Divergences, fmt.Sprintf("%s: o modelo extraiu %s %s (%s) e o scanner nao", manifest, c.Name, c.Head, c.Ecosystem))
			}
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

func isStale(err error) bool { return errors.Is(err, ErrStale) }
