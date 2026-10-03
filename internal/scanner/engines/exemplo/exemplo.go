// Package exemplo is the example engine of the extension guide
// (docs/extensao.md): the smallest scanner.Scanner that honors the whole
// contract without an external binary. It reports, deterministically, every
// line of the reviewed tree that carries the marker Marker.
//
// The package itself registers nothing. Only a binary built with the build
// tag aurum_exemplo holds it (internal/scanner/engines/exemplo_registro.go
// imports it under that tag), so the default binary refuses `engine:
// exemplo` when the configuration is parsed, like any engine it does not
// hold.
package exemplo

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const (
	// Name is the engine's registered name, its category and its typed
	// origin.
	Name = "exemplo"
	// Marker is the text whose presence on a line is the example finding.
	Marker = "EXEMPLO-" + "ACHADO"
	// RuleID is the rule every example finding cites.
	RuleID = "exemplo:marca"
	// Version is the engine identity recorded in Report.Version.
	Version = "exemplo 1"
	// MaxFileBytes bounds the files the engine reads; a larger file is
	// skipped, never truncated.
	MaxFileBytes = 1 << 20
	// severity of every example finding; the entry's fail_on decides.
	severity = "error"
	// sideRight: the marker is content of the reviewed tree.
	sideRight = "RIGHT"
	// message is the finding's text. It does not repeat the marker, so a
	// report that quotes it never becomes a new finding.
	message = "linha marcada para o exemplo de engine do guia de extensao"
	// gitDir is the repository metadata the walk skips.
	gitDir = ".git"
)

// Engine is the registry entry of the example engine.
func Engine() scanner.Engine {
	return scanner.Engine{
		Scanner:  Scanner{},
		Category: Name,
		Origin:   Name,
		Validate: validateOptions,
	}
}

// Scanner implements scanner.Scanner.
type Scanner struct{}

// Name is the engine's registered name.
func (Scanner) Name() string { return Name }

// Run walks req.Root (without .git) and reports each line holding Marker.
// A tree it cannot walk is an error, which the executor reads as
// inconclusive; a clean tree is a complete report with zero findings.
func (Scanner) Run(ctx context.Context, req scanner.Request) (scanner.Report, error) {
	if strings.TrimSpace(req.Root) == "" {
		return scanner.Report{}, errors.New("exemplo: empty root")
	}
	var findings []scanner.Finding
	err := filepath.WalkDir(req.Root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == gitDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		found, err := scanFile(req.Root, path)
		findings = append(findings, found...)
		return err
	})
	if err != nil {
		return scanner.Report{}, fmt.Errorf("exemplo: %w", err)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return scanner.Report{Findings: findings, Complete: true, Version: Version}, nil
}

// scanFile reports the marked lines of one file, path relative to root.
func scanFile(root, path string) ([]scanner.Finding, error) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > MaxFileBytes {
		return nil, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []scanner.Finding
	lines := bufio.NewScanner(f)
	lines.Buffer(make([]byte, 0, 64*1024), MaxFileBytes)
	for n := 1; lines.Scan(); n++ {
		if strings.Contains(lines.Text(), Marker) {
			out = append(out, scanner.Finding{
				Path: filepath.ToSlash(rel), Line: n, Side: sideRight,
				RuleID: RuleID, Severity: severity, Message: message,
			})
		}
	}
	return out, lines.Err()
}

// validateOptions: the example engine takes no option.
func validateOptions(opts scanner.Options) error {
	for key := range opts {
		return fmt.Errorf("unknown option %q (exemplo takes no option)", key)
	}
	return nil
}
