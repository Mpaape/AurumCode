package review

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// A review in batches: when the reviewable content does not fit one prompt,
// the files are grouped by directory and packed into batches that each fit.
// Every batch is a complete review of its files (the same template, rule
// catalog, skills and context, with the evidence of its own files) and the
// results are consolidated into one. Files beyond the ceilings are never
// reviewed in silence: they are counted and named as omitted, which keeps the
// review partial and the approval withheld.

// BatchLimits bounds a review in batches: how many prompts and their summed
// estimated size. Zero means the embedded default.
type BatchLimits struct {
	MaxBatches      int
	MaxPromptTokens int
}

// DefaultBatchLimits returns the ceilings declared in
// internal/prompt/templates/limits.yml.
func DefaultBatchLimits() BatchLimits {
	limits := prompt.DefaultLimits()
	return BatchLimits{MaxBatches: limits.BatchMaxCount, MaxPromptTokens: limits.BatchMaxPromptTokens}
}

// withDefaults fills each zero ceiling from the embedded defaults.
func (l BatchLimits) withDefaults() BatchLimits {
	defaults := DefaultBatchLimits()
	if l.MaxBatches <= 0 {
		l.MaxBatches = defaults.MaxBatches
	}
	if l.MaxPromptTokens <= 0 {
		l.MaxPromptTokens = defaults.MaxPromptTokens
	}
	return l
}

// SetBatchLimits installs the configured ceilings; a zero value keeps the
// default.
func (r *Reviewer) SetBatchLimits(limits BatchLimits) {
	r.cfg.Batches = limits.withDefaults()
}

// Batch records one batch of a review: its files and its prompt's estimated
// size.
type Batch struct {
	Index           int      `json:"index"`
	Files           []string `json:"files"`
	EstimatedTokens int      `json:"estimated_tokens"`
}

// Batches returns the batches of the last review, nil when its diff fit one
// prompt.
func (r *Reviewer) Batches() []Batch { return r.batches }

// Metadata keys a review in batches adds to the result.
const (
	// MetaBatches is the number of batches reviewed.
	MetaBatches = "review_batches"
	// MetaOmittedPaths names the files left out by the ceilings, one per line.
	MetaOmittedPaths = "code_files_omitted_paths"
)

// needsBatches reports whether the single prompt left a file out, in whole
// or in part, and there is more than one file to split. Otherwise the review
// stays the single prompt it always was. A ceiling of one batch still plans:
// the files that do not fit are then named, not only counted.
func (r *Reviewer) needsBatches(diff *types.Diff, prepared preparedPrompt) bool {
	if diff == nil || len(diff.Files) < 2 {
		return false
	}
	return metaCount(prepared.parts.Meta, "code_files_omitted") > 0 || metaCount(prepared.parts.Meta, "code_files_partial") > 0
}

// batchPlan is the packing of a diff: the prompts of the batches that will
// be reviewed and the files left out by a ceiling.
type batchPlan struct {
	batches []plannedBatch
	outside []string
}

// plannedBatch is one batch's files and its assembled prompt.
type plannedBatch struct {
	files    []types.DiffFile
	prepared preparedPrompt
}

// reviewInBatches plans, reviews each batch and consolidates. A batch that
// fails fails the review, exactly as the single prompt would.
func (r *Reviewer) reviewInBatches(ctx context.Context, diff *types.Diff, reviewContext ReviewContext) (*types.ReviewResult, error) {
	plan, err := r.planBatches(diff, reviewContext)
	if err != nil {
		return nil, err
	}
	results := make([]*types.ReviewResult, 0, len(plan.batches))
	var transcripts []*deliberation.Transcript
	for i, batch := range plan.batches {
		r.transcript = nil
		result, err := r.reviewPrepared(ctx, batch.prepared)
		transcripts = append(transcripts, r.transcript)
		r.transcript = mergeTranscripts(transcripts)
		if err != nil {
			return nil, fmt.Errorf("review batch %d of %d: %w", i+1, len(plan.batches), err)
		}
		results = append(results, result)
		r.batches = append(r.batches, Batch{Index: i + 1, Files: filePaths(batch.files), EstimatedTokens: metaCount(batch.prepared.parts.Meta, "estimated_tokens")})
	}
	merged := mergeResults(results)
	declareOutside(merged, plan.outside)
	merged.Metadata[MetaBatches] = strconv.Itoa(len(plan.batches))
	return merged, nil
}

// planBatches packs the files by directory, greedily, into prompts that fit,
// then applies the ceilings in order: from the first batch past the count
// or past the summed size on, no batch is reviewed and its files are
// declared outside. Each candidate prompt is assembled once.
func (r *Reviewer) planBatches(diff *types.Diff, reviewContext ReviewContext) (batchPlan, error) {
	assembled := map[string]preparedPrompt{}
	prepare := func(files []types.DiffFile) (preparedPrompt, error) {
		key := strings.Join(filePaths(files), "\x00")
		if p, ok := assembled[key]; ok {
			return p, nil
		}
		p, err := r.preparePrompt(&types.Diff{Files: files}, batchContext(reviewContext, files))
		if err == nil {
			assembled[key] = p
		}
		return p, err
	}
	measure := func(files []types.DiffFile) (bool, error) {
		p, err := prepare(files)
		if err != nil {
			return false, err
		}
		meta := p.parts.Meta
		return metaCount(meta, "code_files_omitted") == 0 && metaCount(meta, "code_files_partial") == 0, nil
	}
	packed, err := packUnits(groupByDirectory(diff.Files), measure)
	if err != nil {
		return batchPlan{}, err
	}
	var plan batchPlan
	total, stopped := 0, false
	for _, files := range packed {
		p, err := prepare(files)
		if err != nil {
			return batchPlan{}, err
		}
		tokens := metaCount(p.parts.Meta, "estimated_tokens")
		stopped = stopped || len(plan.batches) >= r.cfg.Batches.MaxBatches || total+tokens > r.cfg.Batches.MaxPromptTokens
		if stopped {
			plan.outside = append(plan.outside, filePaths(files)...)
			continue
		}
		total += tokens
		plan.batches = append(plan.batches, plannedBatch{files: files, prepared: p})
	}
	return plan, nil
}

// measureFunc assembles the prompt of a candidate batch and reports whether
// every file fits whole.
type measureFunc func(files []types.DiffFile) (fits bool, err error)

// packUnits packs units (directories) greedily, in order, into batches that
// fit. A directory stays whole in a batch when it fits; one that does not is
// added file by file, so a batch is filled before the next one starts. A
// single file that does not fit even alone is a batch of its own, reviewed
// in part and declared so by its prompt's coverage.
func packUnits(units [][]types.DiffFile, measure measureFunc) ([][]types.DiffFile, error) {
	p := &packer{measure: measure}
	for _, unit := range units {
		if err := p.add(unit); err != nil {
			return nil, err
		}
	}
	p.flush()
	return p.batches, nil
}

// packer holds the batches closed so far and the one being filled.
type packer struct {
	measure measureFunc
	batches [][]types.DiffFile
	current []types.DiffFile
}

// add places one unit: in the current batch when it fits, else file by
// file, else in a new batch.
func (p *packer) add(unit []types.DiffFile) error {
	candidate := append(append([]types.DiffFile{}, p.current...), unit...)
	fits, err := p.measure(candidate)
	if err != nil || fits {
		if fits {
			p.current = candidate
		}
		return err
	}
	if len(unit) > 1 {
		for _, f := range unit {
			if err := p.add([]types.DiffFile{f}); err != nil {
				return err
			}
		}
		return nil
	}
	p.flush()
	alone, err := p.measure(unit)
	if err != nil {
		return err
	}
	if alone {
		p.current = unit
		return nil
	}
	p.batches = append(p.batches, unit)
	return nil
}

// flush closes the batch being filled.
func (p *packer) flush() {
	if len(p.current) > 0 {
		p.batches = append(p.batches, p.current)
		p.current = nil
	}
}

// groupByDirectory groups the files by their directory, directories in
// sorted order and files in diff order within each.
func groupByDirectory(files []types.DiffFile) [][]types.DiffFile {
	byDir := map[string][]types.DiffFile{}
	var dirs []string
	for _, f := range files {
		dir := path.Dir(f.Path)
		if _, ok := byDir[dir]; !ok {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], f)
	}
	sort.Strings(dirs)
	units := make([][]types.DiffFile, 0, len(dirs))
	for _, dir := range dirs {
		units = append(units, byDir[dir])
	}
	return units
}

// singleFileUnits makes one unit per file.
func singleFileUnits(files []types.DiffFile) [][]types.DiffFile {
	units := make([][]types.DiffFile, 0, len(files))
	for _, f := range files {
		units = append(units, []types.DiffFile{f})
	}
	return units
}

// batchContext is the review context of one batch: everything the single
// prompt carries, with the evidence narrowed to the batch's files (and the
// evidence tied to no file). Evidence ids are assigned once over the whole
// review, so an id names the same finding in every batch.
func batchContext(reviewContext ReviewContext, files []types.DiffFile) ReviewContext {
	in := make(map[string]bool, len(files))
	for _, f := range files {
		in[f.Path] = true
	}
	var evidence []prompt.EvidenceItem
	for _, item := range reviewContext.Evidence {
		if item.File == "" || in[item.File] {
			evidence = append(evidence, item)
		}
	}
	reviewContext.Evidence = evidence
	return reviewContext
}

// filePaths returns the paths of files, in order.
func filePaths(files []types.DiffFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// metaCount reads a non-negative count from engine metadata, zero when
// absent or malformed.
func metaCount(meta map[string]string, key string) int {
	n, err := strconv.Atoi(meta[key])
	if err != nil || n < 0 {
		return 0
	}
	return n
}
