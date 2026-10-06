package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// noStructureDiffPaths returns the diff's files for which the grammar provider
// has no grammar, so the review declares that their structural context was not
// produced. The answer is the provider's; no extension or language is named
// here.
func noStructureDiffPaths(p grammar.Provider, diff *types.Diff) []string {
	if diff == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, f := range diff.Files {
		if _, ok := seen[f.Path]; ok {
			continue
		}
		seen[f.Path] = struct{}{}
		if p.Detect(f.Path, nil) == "" {
			out = append(out, f.Path)
		}
	}
	return out
}

// applyStructuralCoverage is the one place both review paths (--base and --pr)
// record what the review could not cover: files without a grammar are
// declared (never a failure by themselves), and any file the review meant to
// read and could not (generated, too large, no patch) withholds approval with
// the same
// engine-owned marker the gate uses, which the model can neither set nor
// erase (AUR-522: an unreviewed file never counts as approved). A binary or
// config-ignored file is declared in the notice, out of the review's scope,
// and withholds nothing.
func applyStructuralCoverage(p grammar.Provider, diff *types.Diff, c *reviewCoverageBreakdown, result *types.ReviewResult) {
	c.NoStructure = noStructureDiffPaths(p, diff)
	if len(c.FilteredPaths) == 0 || result == nil {
		return
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata[prompt.PolicyGateWithheldKey] = "true"
}

// maxInspectBytes bounds how much of a checked-out file is read for the
// content check; one byte past the analyzer's own size limit is enough for it
// to say "too large".
const maxInspectBytes = (1 << 20) + 1

// uninspectedPRNotices is the --pr counterpart of --base's blob classification.
// It runs the SAME content check (analyzer.ClassifyBlob) over each changed
// file, reading the verified checkout in dir when there is one and otherwise
// the added lines of the API patch. A file with no patch and no readable
// verified copy was never inspected, so it is declared not reviewed: nothing
// without inspected content counts as reviewed.
func uninspectedPRNotices(diff *types.Diff, dir string) []analyzer.DiffNotice {
	if diff == nil {
		return nil
	}
	var notices []analyzer.DiffNotice
	for _, f := range diff.Files {
		content, ok := prFileContent(f, dir)
		if !ok {
			notices = append(notices, analyzer.DiffNotice{
				Path:    f.Path,
				Message: "not reviewed, no patch (binary or large): " + f.Path,
				Reason:  analyzer.NoticeReasonNoPatch,
			})
			continue
		}
		if n := analyzer.ClassifyBlob(f.Path, content); n != nil {
			notices = append(notices, *n)
		}
	}
	return notices
}

// declaredBinary reports a notice the review declares ignored instead of
// partial: binary content (the analyzer's own content check) in a file whose
// extension the binary-format catalog lists (grammar.KnownBinaryFormat). A
// NUL byte in a script, a source file, a file without extension or of an
// unknown format is not enough: such a file stays unreviewed and withholds
// approval, so a crafted byte can never hide code from the review.
func declaredBinary(n analyzer.DiffNotice) bool {
	return n.Reason == analyzer.NoticeReasonBinary && grammar.KnownBinaryFormat(n.Path)
}

// splitBinaryFiles separates the diff's declared binary files (by the same content
// check, analyzer.ClassifyBlob, over the verified checkout or the patch) from
// the rest. A file whose content nobody can read is not split out: it stays
// in the diff and uninspectedPRNotices keeps it unreviewed.
func splitBinaryFiles(diff *types.Diff, dir string) ([]analyzer.DiffNotice, *types.Diff) {
	if diff == nil {
		return nil, diff
	}
	var notices []analyzer.DiffNotice
	rest := *diff
	rest.Files = nil
	for _, f := range diff.Files {
		if content, ok := prFileContent(f, dir); ok {
			if n := analyzer.ClassifyBlob(f.Path, content); n != nil && declaredBinary(*n) {
				notices = append(notices, *n)
				continue
			}
		}
		rest.Files = append(rest.Files, f)
	}
	return notices, &rest
}

// prFileContent returns the bytes to inspect for one changed file: the
// verified checkout's copy when readable, else the added/context lines of its
// patch. ok is false when neither exists.
func prFileContent(f types.DiffFile, dir string) ([]byte, bool) {
	if dir != "" {
		if data, err := readRegularFile(filepath.Join(dir, filepath.FromSlash(f.Path))); err == nil {
			return data, true
		}
	}
	if len(f.Hunks) == 0 {
		return nil, false
	}
	var b strings.Builder
	for _, h := range f.Hunks {
		for _, line := range h.Lines {
			if line != "" && (line[0] == '+' || line[0] == ' ') {
				b.WriteString(line[1:])
				b.WriteByte('\n')
			}
		}
	}
	return []byte(b.String()), true
}

// readRegularFile reads at most maxInspectBytes of a regular, non-symlink file.
func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return io.ReadAll(io.LimitReader(fh, maxInspectBytes))
}
