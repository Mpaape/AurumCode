// Review coverage: how many files of the diff the model actually reviewed,
// and why the others were not, for the partial-review notice.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// reviewCoverageBreakdown is the deterministic per-reason coverage figure the
// coverage pass renders. Each count is a distinct cause a file did not reach
// the model; keeping them separate is the point -- a reader must be able to
// tell "the repository config hides this path" from "the token budget dropped
// it" (AUR-476 AC-004: a configured omission is never presented as proof that
// the file's content -- e.g. tests -- does not exist).
type reviewCoverageBreakdown struct {
	// Total is the code-file figure the prompt builder emitted
	// (PromptParts.Meta["code_files_total"]). It is empty/zero when no prompt
	// was assembled (no provider, --seguranca-only), in which case the counts
	// below stand on their own.
	Total    int
	Complete int
	Partial  int
	Budget   int
	Ignored  int
	Filtered int
	// IgnoredPaths and FilteredPaths are the concrete paths cmd/aurumcode
	// owns the knowledge of. IgnoredPaths is what the review declares out of
	// its scope by construction: paths the repository's `ignore` hides and
	// binary files (nothing in them is reviewable by reading). They are
	// listed, never counted as partial coverage. FilteredPaths is what the
	// review wanted to read and could not (generated, too large, no patch):
	// those keep the review partial. Budget omissions are named by the
	// prompt builder inside the model input; only their count reaches cmd
	// through result.Metadata.
	IgnoredPaths  []string
	FilteredPaths []string
	// NoStructure names the changed files for which the grammar runtime has
	// no grammar, so no symbol or import context was produced for them. The
	// model still read their text; this is a declaration, not a failure, and
	// it never makes the review partial or inconclusive by itself -- only the
	// gate's own policy decides that.
	NoStructure []string
}

// covered counts every file that reached the model in full or in part. It is
// derived, never stored, so it cannot disagree with the parts.
func (c reviewCoverageBreakdown) covered() int { return c.Complete + c.Partial }

// uncovered counts every file the review meant to read and did not fully
// cover. Declared-ignored files are out of scope, not uncovered.
func (c reviewCoverageBreakdown) uncovered() int {
	return c.Budget + c.Filtered
}

// partial reports whether any file in the review's scope was left out in
// whole or in part. AC-002: this is the single predicate every sink uses, so
// a complete review can never grow a coverage notice on one path and not the
// other. A declared-ignored file (config `ignore`, binary) never makes the
// review partial: it is listed in the notice instead.
func (c reviewCoverageBreakdown) partial() bool {
	return c.Partial > 0 || c.uncovered() > 0
}

// declaredIgnored reports whether the notice has ignored files to list.
func (c reviewCoverageBreakdown) declaredIgnored() bool { return c.Ignored > 0 }

// mergeReviewCoverage combines a model-produced promptMeta with the
// deterministic facts cmd/aurumcode owns: which files the repository config
// ignored (removed before the model ever saw the diff) and which were filtered
// before the review (analyzer.DiffNotice). A file is counted at most once: a
// config-ignored path is ignored whatever its content; a file of a known
// binary format with binary content is declared ignored (declaredBinary);
// every other notice is filtered. When no prompt was
// assembled (promptMeta nil, e.g. the deterministic-only path) the prompt
// counts are simply absent and the configured/filtered counts remain.
func mergeReviewCoverage(promptMeta map[string]string, notices []analyzer.DiffNotice, rawFileCount int, ignoredPaths []string) reviewCoverageBreakdown {
	var c reviewCoverageBreakdown
	c.Total = atoiOrZero(promptMeta["code_files_total"])
	c.Complete = atoiOrZero(promptMeta["code_files_complete"])
	c.Partial = atoiOrZero(promptMeta["code_files_partial"])
	c.Budget = atoiOrZero(promptMeta["code_files_omitted"])
	ignored := dedupePaths(ignoredPaths)
	seen := make(map[string]struct{}, len(ignored)+len(notices))
	for _, p := range ignored {
		seen[p] = struct{}{}
	}
	var filtered []string
	for _, n := range notices {
		if n.Path == "" {
			continue
		}
		if _, ok := seen[n.Path]; ok {
			continue
		}
		seen[n.Path] = struct{}{}
		label := n.Path
		if n.Reason != "" {
			label = n.Path + " (" + n.Reason + ")"
		}
		if declaredBinary(n) {
			ignored = append(ignored, label)
			continue
		}
		filtered = append(filtered, label)
	}
	c.IgnoredPaths, c.Ignored = ignored, len(ignored)
	c.FilteredPaths, c.Filtered = filtered, len(filtered)
	// The code-file total must account for files the prompt builder never saw:
	// an ignored or filtered file is absent from the diff the builder measured,
	// so its own total undercounts the review's true denominator. Reconcile to
	// the raw diff file count when that is larger, so "covered + uncovered"
	// never silently drops a file that neither the prompt nor the config
	// counted.
	if raw := rawFileCount; raw > c.Total {
		c.Total = raw
	}
	return c
}

// dedupePaths returns paths in first-seen order with duplicates removed.
func dedupePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// maxCoveragePaths caps how many concrete paths each reason lists before
// switching to an explicit count, mirroring internal/prompt's
// maxOmittedBullets: the notice stays a bounded size no matter how large the
// diff is.
const maxCoveragePaths = 20

// writeCoveragePaths appends up to maxCoveragePaths of paths as sub-bullets,
// then a "and N more" line. Paths are repository paths from the diff/config,
// never model output, so they carry no untrusted bytes.
func writeCoveragePaths(b *strings.Builder, paths []string) {
	listed := 0
	for _, p := range paths {
		if listed >= maxCoveragePaths {
			fmt.Fprintf(b, "  - ... and %d more\n", len(paths)-listed)
			break
		}
		fmt.Fprintf(b, "  - %s\n", p)
		listed++
	}
}

// atoiOrZero parses a decimal metadata string, treating an absent or malformed
// value as zero. Metadata is engine-produced trusted text, never model output;
// a malformed value is still never allowed to crash a review.
func atoiOrZero(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// ignoredDiffPaths returns the paths of diff that the repository's explicit
// `ignore` config will drop, in diff order and without duplicates. It is the
// pre-filter snapshot FilterIgnoredPaths (internal/config) needs and AUR-476's
// coverage notice consumes: after the filter runs those paths are gone, so the
// reason they are absent from the model input can only be recovered before it.
// It is deliberately built on the public config.FilterIgnoredPaths behavior
// -- filtering a single-file probe outstanding -- rather than re-implementing
// the glob semantics, so "ignored" means exactly what the filter already
// means. A nil diff or a zero-config cfg returns nil, matching
// FilterIgnoredPaths' own no-op contract.
func ignoredDiffPaths(diff *types.Diff, cfg *config.Config) []string {
	if diff == nil || cfg == nil || len(cfg.Ignore) == 0 {
		return nil
	}
	var out []string
	for _, f := range diff.Files {
		probe := config.FilterIgnoredPaths(&types.Diff{Files: []types.DiffFile{f}}, cfg)
		if len(probe.Files) == 0 {
			out = append(out, f.Path)
		}
	}
	return out
}
