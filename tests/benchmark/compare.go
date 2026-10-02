package benchmark

import (
	"fmt"
	"sort"
)

type PipelineComparison struct {
	Status        string  `json:"status"`
	LocalName     string  `json:"local_name"`
	HostedName    string  `json:"hosted_name"`
	LocalMetrics  Metrics `json:"local_metrics"`
	HostedMetrics Metrics `json:"hosted_metrics"`
	Note          string  `json:"note"`
}

// ComparePipelines compares a self-hosted pipeline against a hosted product on
// the same corpus and model. If either side is unreachable the result is
// not-measured: absence of access is never a zero score.
func ComparePipelines(local, hosted Pipeline, corpus *Corpus, cfg Config) (PipelineComparison, error) {
	if local.Kind() != LocalPipeline {
		return PipelineComparison{}, fmt.Errorf("local pipeline %q has kind %q", local.Name(), local.Kind())
	}
	if hosted.Kind() != HostedProduct {
		return PipelineComparison{}, fmt.Errorf("hosted product %q has kind %q", hosted.Name(), hosted.Kind())
	}
	result := PipelineComparison{LocalName: local.Name(), HostedName: hosted.Name()}
	if !local.Available() || !hosted.Available() {
		result.Status = StatusNotMeasured
		result.Note = "a pipeline or product was not reachable; result is not measured"
		return result, nil
	}
	localRecords, err := (&Harness{Corpus: corpus, Config: cfg}).Run(local)
	if err != nil {
		return PipelineComparison{}, err
	}
	hostedRecords, err := (&Harness{Corpus: corpus, Config: cfg}).Run(hosted)
	if err != nil {
		return PipelineComparison{}, err
	}
	result.LocalMetrics = Aggregate(measuredMetrics(localRecords))
	result.HostedMetrics = Aggregate(measuredMetrics(hostedRecords))
	result.Status = StatusMeasured
	return result, nil
}

type ScenarioDelta struct {
	CaseID           string  `json:"case_id"`
	Variant          Variant `json:"variant"`
	BasePrecision    float64 `json:"base_precision"`
	VariantPrecision float64 `json:"variant_precision"`
	BaseRecall       float64 `json:"base_recall"`
	VariantRecall    float64 `json:"variant_recall"`
	PrecisionGain    float64 `json:"precision_gain"`
	RecallGain       float64 `json:"recall_gain"`
	NoiseDelta       int     `json:"noise_delta"`
	DuplicatesDelta  int     `json:"duplicates_delta"`
}

type ConfigComparison struct {
	Status        string          `json:"status"`
	Pipeline      string          `json:"pipeline"`
	Model         string          `json:"model"`
	CorpusDigest  string          `json:"corpus_digest"`
	BaseConfig    Config          `json:"base_config"`
	VariantConfig Config          `json:"variant_config"`
	Deltas        []ScenarioDelta `json:"deltas"`
	Aggregate     ScenarioDelta   `json:"aggregate"`
}

// CompareConfigs toggles context and/or varies versioned prompts and skills
// while holding the model and corpus constant. Each scenario records its own
// gain or loss so an aggregate cannot hide a regression.
func CompareConfigs(p Pipeline, corpus *Corpus, base, variant Config) (ConfigComparison, error) {
	if base.Model == "" || variant.Model == "" {
		return ConfigComparison{}, fmt.Errorf("both configs must name the model actually served")
	}
	if base.Model != variant.Model {
		return ConfigComparison{}, fmt.Errorf("model must be held constant: %q vs %q", base.Model, variant.Model)
	}
	if base.Context == variant.Context && base.Prompt == variant.Prompt && sameSkills(base.Skills, variant.Skills) {
		return ConfigComparison{}, fmt.Errorf("configs differ in nothing measurable")
	}
	result := ConfigComparison{
		Pipeline:      p.Name(),
		Model:         base.Model,
		CorpusDigest:  corpus.GroundTruthDigest(),
		BaseConfig:    base,
		VariantConfig: variant,
	}
	if !p.Available() {
		result.Status = StatusNotMeasured
		return result, nil
	}
	baseRecords, err := (&Harness{Corpus: corpus, Config: base}).Run(p)
	if err != nil {
		return ConfigComparison{}, err
	}
	variantRecords, err := (&Harness{Corpus: corpus, Config: variant}).Run(p)
	if err != nil {
		return ConfigComparison{}, err
	}
	baseIndex := indexRuns(baseRecords)
	for _, r := range variantRecords {
		key := runKey(r.CaseID, r.Variant)
		b, ok := baseIndex[key]
		if !ok {
			return ConfigComparison{}, fmt.Errorf("base run missing for %s", key)
		}
		result.Deltas = append(result.Deltas, delta(r.CaseID, r.Variant, b.Metrics, r.Metrics))
	}
	result.Aggregate = delta("*", "", Aggregate(measuredMetrics(baseRecords)), Aggregate(measuredMetrics(variantRecords)))
	result.Status = StatusMeasured
	return result, nil
}

func delta(caseID string, variant Variant, base, variantMetrics Metrics) ScenarioDelta {
	return ScenarioDelta{
		CaseID:           caseID,
		Variant:          variant,
		BasePrecision:    base.Precision,
		VariantPrecision: variantMetrics.Precision,
		BaseRecall:       base.Recall,
		VariantRecall:    variantMetrics.Recall,
		PrecisionGain:    variantMetrics.Precision - base.Precision,
		RecallGain:       variantMetrics.Recall - base.Recall,
		NoiseDelta:       variantMetrics.Noise - base.Noise,
		DuplicatesDelta:  variantMetrics.Duplicates - base.Duplicates,
	}
}

func indexRuns(records []RunRecord) map[string]RunRecord {
	index := make(map[string]RunRecord, len(records))
	for _, r := range records {
		index[runKey(r.CaseID, r.Variant)] = r
	}
	return index
}

func runKey(caseID string, variant Variant) string {
	return caseID + "|" + string(variant)
}

func sameSkills(left, right []string) bool {
	a := append([]string(nil), left...)
	b := append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
