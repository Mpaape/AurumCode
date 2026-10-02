package benchmark

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type PipelineKind string

const (
	LocalPipeline PipelineKind = "local-pipeline"
	HostedProduct PipelineKind = "hosted-product"
)

type Variant string

const (
	VariantOriginal  Variant = "original"
	VariantUnchanged Variant = "unchanged"
	VariantPartial   Variant = "partial"
	VariantFixed     Variant = "fixed"
)

var AllVariants = []Variant{VariantOriginal, VariantUnchanged, VariantPartial, VariantFixed}

const (
	StatusMeasured    = "measured"
	StatusNotMeasured = "not-measured"
)

// Config is the provenance a run must record. A comparison is only valid when
// the model and corpus digest are held constant and the changed field is
// named.
type Config struct {
	Model    string   `json:"model"`
	Provider string   `json:"provider"`
	Prompt   string   `json:"prompt"`
	Skills   []string `json:"skills,omitempty"`
	Context  bool     `json:"context"`
}

func (c Config) Key() string {
	skills := append([]string(nil), c.Skills...)
	sort.Strings(skills)
	return strings.Join([]string{
		c.Provider, c.Model, c.Prompt, strings.Join(skills, "+"), fmt.Sprintf("context=%t", c.Context),
	}, "|")
}

type RunStats struct {
	LatencyMS int64
	CostUSD   float64
}

type Pipeline interface {
	Name() string
	Kind() PipelineKind
	Available() bool
	Synthetic() bool
	Review(c Case, v Variant, cfg Config) ([]Finding, RunStats, error)
}

type RunRecord struct {
	CaseID       string       `json:"case_id"`
	Variant      Variant      `json:"variant"`
	Pipeline     string       `json:"pipeline"`
	Kind         PipelineKind `json:"kind"`
	Config       Config       `json:"config"`
	Base         string       `json:"base"`
	Head         string       `json:"head"`
	CorpusDigest string       `json:"corpus_digest"`
	Findings     []Finding    `json:"findings"`
	Metrics      Metrics      `json:"metrics"`
	LatencyMS    int64        `json:"latency_ms"`
	CostUSD      float64      `json:"cost_usd"`
	Status       string       `json:"status"`
	Synthetic    bool         `json:"synthetic"`
}

type Harness struct {
	Corpus *Corpus
	Config Config
}

func (h *Harness) Run(p Pipeline) ([]RunRecord, error) {
	if h.Corpus == nil {
		return nil, errors.New("harness has no corpus")
	}
	digest := h.Corpus.GroundTruthDigest()
	records := make([]RunRecord, 0, len(h.Corpus.Cases)*len(AllVariants))
	for i := range h.Corpus.Cases {
		c := h.Corpus.Cases[i]
		for _, v := range AllVariants {
			rec := RunRecord{
				CaseID:       c.ID,
				Variant:      v,
				Pipeline:     p.Name(),
				Kind:         p.Kind(),
				Config:       h.Config,
				Base:         c.Base,
				Head:         c.Head,
				CorpusDigest: digest,
				Synthetic:    p.Synthetic(),
			}
			if !p.Available() {
				rec.Status = StatusNotMeasured
				rec.Findings = []Finding{}
				records = append(records, rec)
				continue
			}
			findings, stats, err := p.Review(c, v, h.Config)
			if err != nil {
				return nil, fmt.Errorf("pipeline %s case %s variant %s: %w", p.Name(), c.ID, v, err)
			}
			if findings == nil {
				findings = []Finding{}
			}
			rec.Findings = findings
			rec.Metrics = Evaluate(findings, c.Defects, DefaultLineTolerance)
			rec.LatencyMS = stats.LatencyMS
			rec.CostUSD = stats.CostUSD
			rec.Status = StatusMeasured
			records = append(records, rec)
		}
	}
	return records, nil
}

// RealScores drops fixture runs. A fixture validates the harness; it must
// never be presented as a score of a real model.
func RealScores(records []RunRecord) []RunRecord {
	real := make([]RunRecord, 0, len(records))
	for _, r := range records {
		if !r.Synthetic && r.Status == StatusMeasured {
			real = append(real, r)
		}
	}
	return real
}

func measuredMetrics(records []RunRecord) []Metrics {
	metrics := make([]Metrics, 0, len(records))
	for _, r := range records {
		if r.Status == StatusMeasured {
			metrics = append(metrics, r.Metrics)
		}
	}
	return metrics
}
