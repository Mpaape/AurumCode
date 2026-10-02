package benchmark

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateMultilang = flag.Bool("update-aur523", false, "regenerate the AUR-523 corpus manifest and report (used by a PR that adds a case)")

const (
	mlRoot      = "testdata/multilang"
	mlReportDir = "out"
	mlReportJS  = "multilang-report.json"
	mlReportMD  = "multilang-report.md"
)

type mlEnv struct {
	corpus *MLCorpus
	bin    string
	work   string
}

func newMLEnv(t *testing.T) *mlEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the real binary")
	}
	root, err := FindRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := EnumerateMLCorpus(mlRoot)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	bin, err := BuildAurumBinary(root, work)
	if err != nil {
		t.Fatal(err)
	}
	return &mlEnv{corpus: corpus, bin: bin, work: work}
}

func (e *mlEnv) runAll(t *testing.T, tag string, read OutcomeReader) []CaseResult {
	t.Helper()
	var out []CaseResult
	for _, mc := range e.corpus.Cases {
		r, err := RunCase(e.bin, mc, e.corpus.PolicyDir, filepath.Join(e.work, tag, mc.ID), false)
		if err != nil {
			t.Fatalf("run %s: %v", mc.ID, err)
		}
		o, err := read(r)
		if err != nil {
			t.Fatalf("read %s: %v\nstdout:\n%s\nstderr:\n%s", mc.ID, err, r.Stdout, r.Stderr)
		}
		out = append(out, CaseResult{Case: mc, Outcome: o})
	}
	return out
}

func (e *mlEnv) report(t *testing.T, tag string) MLReport {
	return BuildMLReport(e.corpus, e.runAll(t, tag, ReadBinaryOutput))
}

func TestAUR523RealBinaryOutputIsTheSource(t *testing.T) {
	e := newMLEnv(t)
	if err := VerifyMLCorpus(e.corpus); err != nil {
		t.Fatal(err)
	}
	if err := CheckOutputProvenance(e.bin, e.corpus, ReadBinaryOutput, e.work); err != nil {
		t.Fatalf("the real reader must follow the binary: %v", err)
	}
	// Sanity on real runs: a hit defect is blocked (gate fail) by the binary.
	for _, res := range e.runAll(t, "ac001", ReadBinaryOutput) {
		c, o := res.Case, res.Outcome
		if c.Label == LabelDefect && c.SimulatedModel == ModelHit {
			if o.GateDecision != "fail" || o.ExitCode == 0 || len(o.Findings) == 0 {
				t.Fatalf("%s: a detected defect must fail the gate and exit non-zero: %+v", c.ID, o)
			}
		}
		if c.Label == LabelClean && c.SimulatedModel == ModelSilent {
			if o.GateDecision != "pass" || len(o.Findings) != 0 {
				t.Fatalf("%s: a silent clean case must pass with no findings: %+v", c.ID, o)
			}
		}
	}
}

func TestAUR523MutationFixtureReadIsRed(t *testing.T) {
	e := newMLEnv(t)
	err := CheckOutputProvenance(e.bin, e.corpus, ReadFixture, e.work)
	if err == nil {
		t.Fatal("MUT-001: reading findings from the fixture instead of the binary output must fail AC-001's provenance check")
	}
	if !strings.Contains(err.Error(), "not reading the binary output") {
		t.Fatalf("unexpected failure: %v", err)
	}
}

func TestAUR523CorpusLabeledVersionedWithDigest(t *testing.T) {
	corpus, err := EnumerateMLCorpus(mlRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(corpus.Languages()); got < 6 {
		t.Fatalf("corpus covers %d languages, want at least 6", got)
	}
	defects, clean := 0, 0
	for _, mc := range corpus.Cases {
		switch mc.Label {
		case LabelDefect:
			defects++
			if mc.File == "" || mc.Line <= 0 || mc.Rule == "" {
				t.Fatalf("%s: defect without file, line and rule", mc.ID)
			}
		case LabelClean:
			clean++
		}
	}
	if defects == 0 || clean == 0 {
		t.Fatalf("corpus needs defect and clean cases, got %d/%d", defects, clean)
	}
	if *updateMultilang {
		if err := WriteMLManifest(corpus); err != nil {
			t.Fatal(err)
		}
	}
	if err := VerifyMLCorpus(corpus); err != nil {
		t.Fatal(err)
	}
	// Divergence must be refused: tamper with a copy of a case.
	tmp := t.TempDir()
	copyTree(t, mlRoot, tmp)
	victim := corpus.Cases[0]
	p := filepath.Join(tmp, "cases", victim.ID, victim.File)
	raw, _ := os.ReadFile(p)
	if err := os.WriteFile(p, append(raw, []byte("// tampered\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	tampered, err := EnumerateMLCorpus(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyMLCorpus(tampered); err == nil || !strings.Contains(err.Error(), "divergence") {
		t.Fatalf("a changed case must be refused, got %v", err)
	}
	if err := os.RemoveAll(filepath.Join(tmp, "cases", victim.ID)); err != nil {
		t.Fatal(err)
	}
	removed, err := EnumerateMLCorpus(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyMLCorpus(removed); err == nil {
		t.Fatal("a removed case must be refused")
	}
}

func TestAUR523ReportPerLanguage(t *testing.T) {
	e := newMLEnv(t)
	rep := e.report(t, "ac003")
	if len(rep.Languages) < 6 {
		t.Fatalf("report has %d languages", len(rep.Languages))
	}
	sawApproved := false
	for _, row := range rep.Languages {
		if row.Defects == 0 || row.RecallInterval.High < row.RecallInterval.Low {
			t.Fatalf("bad row %+v", row)
		}
		if row.RecallInterval.Low > row.Recall || row.Recall > row.RecallInterval.High {
			t.Fatalf("recall outside its interval: %+v", row)
		}
		if row.ApprovedWithDefect > 0 {
			sawApproved = true
		}
	}
	if rep.Total.Defects == 0 || rep.Total.Detected == 0 || !sawApproved || rep.Total.FalsePositives == 0 {
		t.Fatalf("the simulated misses and false alarms must show up in the report: %+v", rep.Total)
	}
	js, err := rep.JSON()
	if err != nil {
		t.Fatal(err)
	}
	md := rep.Markdown()
	if !bytes.Contains(md, []byte("| language |")) || !bytes.Contains(md, []byte("approved with defect")) {
		t.Fatal("markdown table is missing its columns")
	}
	if *updateMultilang {
		if err := os.MkdirAll(mlReportDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mlReportDir, mlReportJS), js, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mlReportDir, mlReportMD), md, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string][]byte{mlReportJS: js, mlReportMD: md} {
		have, err := os.ReadFile(filepath.Join(mlReportDir, name))
		if err != nil {
			t.Fatalf("versioned report missing (regenerate with -update-aur523): %v", err)
		}
		if !bytes.Equal(have, want) {
			t.Fatalf("versioned %s differs from the report this corpus and policy produce; regenerate it with -update-aur523, never by hand", name)
		}
	}
}

func TestAUR523ReportReproducible(t *testing.T) {
	e := newMLEnv(t)
	first, err := e.report(t, "run1").JSON()
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.report(t, "run2").JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("same corpus digest and policy must give a byte-identical report")
	}
	rep := e.report(t, "run3")
	if !bytes.Contains(first, []byte(rep.CorpusSHA256)) || !bytes.Contains(first, []byte(rep.PolicySHA256)) {
		t.Fatal("report header must carry the corpus and policy digests")
	}
	// A different policy digest must change the report header.
	other := *e.corpus
	other.PolicySHA256 = sha256Hex([]byte("other policy"))
	changed, _ := BuildMLReport(&other, e.runAll(t, "run4", ReadBinaryOutput)).JSON()
	if bytes.Equal(changed, first) {
		t.Fatal("the policy digest must be part of the report")
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
