package benchmark

import (
	"reflect"
	"strings"
	"testing"
)

type fixturePipeline struct {
	name      string
	kind      PipelineKind
	available bool
	synthetic bool
	latency   int64
	cost      float64
	reviewFn  func(Case, Variant, Config) []Finding
}

func (p *fixturePipeline) Name() string       { return p.name }
func (p *fixturePipeline) Kind() PipelineKind { return p.kind }
func (p *fixturePipeline) Available() bool    { return p.available }
func (p *fixturePipeline) Synthetic() bool    { return p.synthetic }
func (p *fixturePipeline) Review(c Case, v Variant, cfg Config) ([]Finding, RunStats, error) {
	if p.reviewFn == nil {
		return nil, RunStats{}, nil
	}
	return p.reviewFn(c, v, cfg), RunStats{LatencyMS: p.latency, CostUSD: p.cost}, nil
}

func loadTestCorpus(t *testing.T) *Corpus {
	t.Helper()
	corpus, err := LoadCorpus("testdata/corpus.json")
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	return corpus
}

func recordFor(t *testing.T, records []RunRecord, caseID string, variant Variant) RunRecord {
	t.Helper()
	for _, r := range records {
		if r.CaseID == caseID && r.Variant == variant {
			return r
		}
	}
	t.Fatalf("no record for %s/%s", caseID, variant)
	return RunRecord{}
}

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	const epsilon = 1e-9
	if got < want-epsilon || got > want+epsilon {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func TestAUR511CorpusCoverageFrozen(t *testing.T) {
	corpus := loadTestCorpus(t)
	splits := map[string]int{}
	languages := map[string]bool{}
	crossFile := false
	negatives := 0
	for _, c := range corpus.Cases {
		splits[c.Split]++
		languages[c.Language] = true
		if c.HasNegative {
			negatives++
			if len(c.Defects) != 0 {
				t.Fatalf("negative case %q carries defects", c.ID)
			}
		}
		for _, d := range c.Defects {
			if d.CrossFile {
				crossFile = true
			}
		}
	}
	if splits["dev"] == 0 || splits["holdout"] == 0 {
		t.Fatalf("expected dev and holdout cases, got %v", splits)
	}
	if len(languages) < 2 {
		t.Fatalf("expected multiple languages, got %v", languages)
	}
	if !crossFile {
		t.Fatal("expected a cross-file defect")
	}
	if negatives == 0 {
		t.Fatal("expected a defect-free negative")
	}
	if corpus.FrozenAt == "" {
		t.Fatal("ground truth must be frozen before execution")
	}
	first := corpus.GroundTruthDigest()
	reloaded := loadTestCorpus(t)
	if reloaded.GroundTruthDigest() != first {
		t.Fatal("ground-truth digest is not stable across loads")
	}
	reloaded.Cases[0].Defects[0].Line++
	if reloaded.GroundTruthDigest() == first {
		t.Fatal("mutating ground truth must change the digest")
	}
}

func TestAUR511HarnessMetricsAndRounds(t *testing.T) {
	corpus := loadTestCorpus(t)
	pipeline := &fixturePipeline{
		name:      "aurum-fixture",
		kind:      LocalPipeline,
		available: true,
		latency:   42,
		cost:      0.003,
		reviewFn: func(c Case, v Variant, _ Config) []Finding {
			if c.ID != "case-go-nil-handler" {
				out := []Finding{}
				for _, d := range c.Defects {
					out = append(out, Finding{CaseID: c.ID, File: d.File, Line: d.Line, Severity: d.Severity, RuleID: d.Category})
				}
				return out
			}
			switch v {
			case VariantOriginal, VariantUnchanged:
				return []Finding{
					{CaseID: c.ID, File: "service/handler.go", Line: 12, Severity: SeverityHigh, RuleID: "nil-deref"},
					{CaseID: c.ID, File: "service/repo.go", Line: 41, Severity: SeverityMedium, RuleID: "missing-error-check"},
					{CaseID: c.ID, File: "service/http.go", Line: 99, Severity: SeverityLow, RuleID: "style"},
				}
			case VariantPartial:
				return []Finding{
					{CaseID: c.ID, File: "service/handler.go", Line: 12, Severity: SeverityHigh, RuleID: "nil-deref"},
				}
			case VariantFixed:
				return []Finding{
					{CaseID: c.ID, File: "service/handler.go", Line: 12, Severity: SeverityHigh, RuleID: "nil-deref"},
					{CaseID: c.ID, File: "service/repo.go", Line: 40, Severity: SeverityMedium, RuleID: "missing-error-check"},
					{CaseID: c.ID, File: "service/repo.go", Line: 40, Severity: SeverityMedium, RuleID: "missing-error-check"},
				}
			}
			return nil
		},
	}
	cfg := Config{Model: "qwen2.5-coder-7b", Provider: "local-vllm", Prompt: "review/default-v1", Skills: []string{"go/errors-v1"}}
	records, err := (&Harness{Corpus: corpus, Config: cfg}).Run(pipeline)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(records) != len(corpus.Cases)*len(AllVariants) {
		t.Fatalf("expected every case in every round, got %d records", len(records))
	}
	original := recordFor(t, records, "case-go-nil-handler", VariantOriginal)
	if original.Base == "" || original.Head == "" {
		t.Fatal("record must pin base and head")
	}
	if original.CorpusDigest == "" || original.CorpusDigest != corpus.GroundTruthDigest() {
		t.Fatal("record must bind the frozen corpus digest")
	}
	if original.Config.Model != cfg.Model || original.Config.Prompt != cfg.Prompt || !reflect.DeepEqual(original.Config.Skills, cfg.Skills) {
		t.Fatalf("record must register model, provider, config, prompt and skills: %+v", original.Config)
	}
	if original.Status != StatusMeasured || original.LatencyMS != 42 || original.CostUSD != 0.003 {
		t.Fatal("record must carry latency and cost")
	}
	if original.Metrics.Matched != 2 || original.Metrics.Missed != 0 || original.Metrics.Noise != 1 || original.Metrics.Duplicates != 0 {
		t.Fatalf("unexpected original metrics: %+v", original.Metrics)
	}
	closeTo(t, "original precision", original.Metrics.Precision, 2.0/3.0)
	closeTo(t, "original recall", original.Metrics.Recall, 1.0)
	closeTo(t, "original location", original.Metrics.LocationAccuracy, 1.0/2.0)

	unchanged := recordFor(t, records, "case-go-nil-handler", VariantUnchanged)
	if unchanged.Metrics != original.Metrics {
		t.Fatal("an unchanged rerun must reproduce the same measurement")
	}

	partial := recordFor(t, records, "case-go-nil-handler", VariantPartial)
	if partial.Metrics.Matched != 1 || partial.Metrics.Missed != 1 || partial.Metrics.Noise != 0 {
		t.Fatalf("unexpected partial metrics: %+v", partial.Metrics)
	}
	closeTo(t, "partial recall", partial.Metrics.Recall, 0.5)

	fixed := recordFor(t, records, "case-go-nil-handler", VariantFixed)
	if fixed.Metrics.Matched != 2 || fixed.Metrics.Missed != 0 || fixed.Metrics.Duplicates != 1 {
		t.Fatalf("unexpected fixed metrics: %+v", fixed.Metrics)
	}
}

func TestAUR511LocalPilotFixtureIsNotRealScore(t *testing.T) {
	corpus := loadTestCorpus(t)
	fixture := &fixturePipeline{
		name:      "qwen-fixture",
		kind:      LocalPipeline,
		available: true,
		synthetic: true,
		reviewFn: func(c Case, _ Variant, _ Config) []Finding {
			out := []Finding{}
			for _, d := range c.Defects {
				out = append(out, Finding{CaseID: c.ID, File: d.File, Line: d.Line, Severity: d.Severity, RuleID: d.Category})
			}
			return out
		},
	}
	cfg := Config{Model: "qwen2.5-coder-7b", Provider: "local-vllm", Prompt: "review/default-v1"}
	records, err := (&Harness{Corpus: corpus, Config: cfg}).Run(fixture)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, r := range records {
		if !r.Synthetic {
			t.Fatal("a deterministic fixture run must be marked synthetic")
		}
	}
	if got := RealScores(records); len(got) != 0 {
		t.Fatalf("a fixture must never be presented as a real model score, got %d", len(got))
	}
	real := &fixturePipeline{name: "aurum", kind: LocalPipeline, available: true}
	realRecords, err := (&Harness{Corpus: corpus, Config: cfg}).Run(real)
	if err != nil {
		t.Fatalf("run real: %v", err)
	}
	if len(RealScores(realRecords)) == 0 {
		t.Fatal("a non-synthetic measured run must count as a real score")
	}
}

func TestAUR511ComparisonSeparatesAndReportsUncertainty(t *testing.T) {
	corpus := loadTestCorpus(t)
	cfg := Config{Model: "qwen2.5-coder-7b", Provider: "local-vllm", Prompt: "review/default-v1"}
	exact := func(c Case, _ Variant, _ Config) []Finding {
		out := []Finding{}
		for _, d := range c.Defects {
			out = append(out, Finding{CaseID: c.ID, File: d.File, Line: d.Line, Severity: d.Severity, RuleID: d.Category})
		}
		return out
	}
	local := &fixturePipeline{name: "aurum-local", kind: LocalPipeline, available: true, reviewFn: exact}
	hosted := &fixturePipeline{name: "hosted-reviewer", kind: HostedProduct, available: true, reviewFn: exact}
	result, err := ComparePipelines(local, hosted, corpus, cfg)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if result.Status != StatusMeasured {
		t.Fatalf("expected measured, got %q", result.Status)
	}
	if result.LocalName == result.HostedName {
		t.Fatal("local pipeline and hosted product must stay separate")
	}
	if result.LocalMetrics.RecallRange.High < result.LocalMetrics.RecallRange.Low {
		t.Fatal("interval must be ordered")
	}
	if result.LocalMetrics.PrecisionRange.High < result.LocalMetrics.PrecisionRange.Low {
		t.Fatal("interval must be ordered")
	}

	unreachable := &fixturePipeline{name: "hosted-reviewer", kind: HostedProduct, available: false}
	result, err = ComparePipelines(local, unreachable, corpus, cfg)
	if err != nil {
		t.Fatalf("compare unreachable: %v", err)
	}
	if result.Status != StatusNotMeasured {
		t.Fatalf("lack of access must be not-measured, got %q", result.Status)
	}
	if result.HostedMetrics.TotalFindings != 0 || result.HostedMetrics.Precision != 0 {
		t.Fatal("not-measured must not be scored as zero")
	}
}

func TestAUR511ContextAndPromptVariants(t *testing.T) {
	corpus := loadTestCorpus(t)
	pipeline := &fixturePipeline{
		name:      "aurum",
		kind:      LocalPipeline,
		available: true,
		reviewFn: func(c Case, v Variant, cfg Config) []Finding {
			out := []Finding{}
			if v != VariantOriginal {
				return out
			}
			for _, d := range c.Defects {
				if !cfg.Context && c.ID == "case-go-nil-handler" && d.ID == "d2" {
					continue
				}
				if cfg.Context && c.ID == "case-ts-crossfile" {
					continue
				}
				out = append(out, Finding{CaseID: c.ID, File: d.File, Line: d.Line, Severity: d.Severity, RuleID: d.Category})
			}
			return out
		},
	}
	base := Config{Model: "qwen2.5-coder-7b", Provider: "local-vllm", Prompt: "review/default-v1", Context: false}
	variant := base
	variant.Context = true
	variant.Skills = []string{"ts/strict-null-v1"}
	comparison, err := CompareConfigs(pipeline, corpus, base, variant)
	if err != nil {
		t.Fatalf("compare configs: %v", err)
	}
	if comparison.Status != StatusMeasured {
		t.Fatalf("expected measured, got %q", comparison.Status)
	}
	if comparison.CorpusDigest != corpus.GroundTruthDigest() || comparison.Model != base.Model {
		t.Fatal("comparison must hold model and corpus constant")
	}
	if _, err := CompareConfigs(pipeline, corpus, base, Config{Model: "other", Provider: "local-vllm", Prompt: "review/default-v1"}); err == nil {
		t.Fatal("changing the model must be refused")
	}
	sawGain := false
	sawLoss := false
	for _, d := range comparison.Deltas {
		if d.CaseID == "case-go-nil-handler" && d.Variant == VariantOriginal && d.RecallGain > 0 {
			sawGain = true
		}
		if d.CaseID == "case-ts-crossfile" && d.Variant == VariantOriginal && d.RecallGain < 0 {
			sawLoss = true
		}
	}
	if !sawGain {
		t.Fatal("expected a recorded per-scenario gain")
	}
	if !sawLoss {
		t.Fatal("expected a recorded per-scenario loss")
	}
}

func TestAUR511SuggestionApplicabilityAndHumanDecision(t *testing.T) {
	defect := Defect{ID: "d1", File: "a.go", Line: 3, Severity: SeverityHigh, Category: "hardcoded-secret", Marker: `secret := "hunter2"`}
	head := map[string]string{"a.go": "package a\n\nsecret := \"hunter2\"\n"}
	goodPatch := "--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,3 @@\n package a\n \n-secret := \"hunter2\"\n+secret := os.Getenv(\"SECRET\")\n"
	good := EvaluateSuggestion(defect, head, Suggestion{DefectID: "d1", File: "a.go", Patch: goodPatch}, func(map[string]string) bool { return true })
	if !good.Applicable || !good.Applies || !good.DefectFixed || !good.TestsPreserved {
		t.Fatalf("a patch that applies, keeps tests and fixes the defect must be applicable: %+v", good)
	}
	cosmeticPatch := "--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,4 @@\n package a\n \n secret := \"hunter2\"\n+// reviewed\n"
	cosmetic := EvaluateSuggestion(defect, head, Suggestion{DefectID: "d1", File: "a.go", Patch: cosmeticPatch}, func(map[string]string) bool { return true })
	if cosmetic.Applicable || cosmetic.DefectFixed {
		t.Fatalf("a patch that leaves the defect must not count: %+v", cosmetic)
	}
	breaking := EvaluateSuggestion(defect, head, Suggestion{DefectID: "d1", File: "a.go", Patch: goodPatch}, func(map[string]string) bool { return false })
	if breaking.Applicable || breaking.TestsPreserved {
		t.Fatalf("a patch that breaks relevant tests must not count: %+v", breaking)
	}
	empty := EvaluateSuggestion(defect, head, Suggestion{DefectID: "d1", File: "a.go", Patch: ""}, func(map[string]string) bool { return true })
	if empty.Applicable || empty.Reason != "empty-patch" {
		t.Fatalf("an empty patch must be refused: %+v", empty)
	}
	nonApplying := EvaluateSuggestion(defect, head, Suggestion{DefectID: "d1", File: "a.go", Patch: "--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,3 @@\n package b\n \n-secret := \"hunter2\"\n+secret := os.Getenv(\"SECRET\")\n"}, func(map[string]string) bool { return true })
	if nonApplying.Applicable || nonApplying.Applies {
		t.Fatalf("a patch that does not apply must be refused: %+v", nonApplying)
	}

	entries := []TriageEntry{
		{FindingID: "f1", Decision: DecisionAccepted, ActiveTriageMS: 120000, DecisionMS: 30000, Comments: 0},
		{FindingID: "f2", Decision: DecisionRejected, ActiveTriageMS: 60000, DecisionMS: 15000, Comments: 9},
		{FindingID: "f3", Decision: DecisionInconclusive, ActiveTriageMS: 5000, DecisionMS: 1000, Comments: 4},
	}
	if AcceptedCount(entries) != 1 {
		t.Fatalf("only an explicit decision may count as accepted, got %d", AcceptedCount(entries))
	}
	if (TriageEntry{FindingID: "f4", Decision: DecisionInconclusive, Comments: 99}).Accepted() {
		t.Fatal("comment count must never be read as acceptance")
	}
	if entries[0].ActiveTriageMS == entries[0].DecisionMS {
		t.Fatal("active triage time and decision time must be recorded separately")
	}
}

func TestAUR511MutationDuplicateAndFalseFinding(t *testing.T) {
	defects := []Defect{{ID: "d1", File: "x.go", Line: 10, Severity: SeverityHigh, Category: "nil-deref"}}
	baseFindings := []Finding{{CaseID: "case-x", File: "x.go", Line: 10, Severity: SeverityHigh, RuleID: "nil-deref"}}
	base := Evaluate(baseFindings, defects, DefaultLineTolerance)
	duplicated := append(append([]Finding{}, baseFindings...), Finding{CaseID: "case-x", File: "x.go", Line: 11, Severity: SeverityHigh, RuleID: "nil-deref"})
	withDuplicate := Evaluate(duplicated, defects, DefaultLineTolerance)
	if withDuplicate.Duplicates != base.Duplicates+1 {
		t.Fatalf("duplicate must be counted: %+v", withDuplicate)
	}
	if withDuplicate.Precision >= base.Precision {
		t.Fatal("a duplicate must not improve precision")
	}
	if withDuplicate.Noise != base.Noise {
		t.Fatal("a duplicate is not raw noise")
	}
	withFalse := Evaluate(append(append([]Finding{}, baseFindings...), Finding{CaseID: "case-x", File: "y.go", Line: 3, Severity: SeverityLow, RuleID: "style"}), defects, DefaultLineTolerance)
	if withFalse.Noise != base.Noise+1 {
		t.Fatalf("a false finding must be counted as noise: %+v", withFalse)
	}
	if withFalse.Precision >= base.Precision {
		t.Fatal("a false finding must not improve precision")
	}
}

func TestAUR511MutationNegativeCoverage(t *testing.T) {
	corpus := loadTestCorpus(t)
	if err := corpus.Validate(); err != nil {
		t.Fatalf("base corpus must validate: %v", err)
	}
	mutated := *corpus
	mutated.Cases = nil
	for _, c := range corpus.Cases {
		if c.HasNegative {
			continue
		}
		mutated.Cases = append(mutated.Cases, c)
	}
	err := mutated.Validate()
	if err == nil {
		t.Fatal("removing the negative must invalidate corpus coverage")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Fatalf("expected a negative-coverage failure, got %v", err)
	}
}

func TestAUR511AdjudicationBlind(t *testing.T) {
	items := []DivergenceItem{
		{ID: "aurum-1", CaseID: "case-x", Origin: "aurum", Finding: Finding{CaseID: "case-x", File: "x.go", Line: 4}},
		{ID: "ocr-1", CaseID: "case-y", Origin: "ocr", Finding: Finding{CaseID: "case-y", File: "y.go", Line: 9}},
	}
	anon, mapping := Anonymize(items)
	if len(anon) != len(items) || len(mapping) != len(items) {
		t.Fatalf("anonymization must cover every item: %d %d", len(anon), len(mapping))
	}
	if LeaksOrigin(anon, []string{"aurum", "ocr"}) {
		t.Fatal("anonymized items must not leak the origin")
	}
	for _, item := range anon {
		if item.Origin != "" {
			t.Fatal("anonymized item still carries an origin")
		}
		if !strings.HasPrefix(item.ID, "DIV-") {
			t.Fatalf("expected an opaque label, got %q", item.ID)
		}
	}
	again, _ := Anonymize(items)
	if !reflect.DeepEqual(anon, again) {
		t.Fatal("anonymization must be deterministic")
	}
	verdicts := Verdicts([]Adjudication{{AnonID: anon[0].ID, Verdict: "confirmed", Judge: "reviewer"}})
	if len(verdicts) != 1 || verdicts[anon[0].ID] != "confirmed" {
		t.Fatalf("blind verdicts must be recorded: %v", verdicts)
	}
}
