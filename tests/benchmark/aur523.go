package benchmark

// AUR-523: multi-language corpus that measures the recall of the REAL gate.
//
// The harness compiles the real aurumcode binary, runs it over every case of
// the corpus with the central policy and a deterministic fake provider, and
// reads the verdict and the findings from the binary's own output (SARIF and
// audit record), never from the fixture that stood in for the model.

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MultilangSchema      = "aurum.benchmark-multilang"
	MultilangMode        = "fake-provider-deterministic"
	multilangRulePrefix  = "seguranca#"
	multilangManifest    = "manifest.json"
	multilangCaseFile    = "case.json"
	LabelDefect          = "defect"
	LabelClean           = "clean"
	ModelHit             = "hit"
	ModelMiss            = "miss"
	ModelSilent          = "silent"
	ModelFalseAlarm      = "false_alarm"
	multilangFixtureLies = "not-in-the-diff.txt"
)

// MLCase is one corpus case: a directory with case.json and the source file.
type MLCase struct {
	ID             string `json:"id"`
	Language       string `json:"language"`
	File           string `json:"file"`
	Label          string `json:"label"`
	Rule           string `json:"rule,omitempty"`
	Line           int    `json:"line,omitempty"`
	SimulatedModel string `json:"simulated_model"`
	FalseAlarmLine int    `json:"false_alarm_line,omitempty"`

	Dir    string `json:"-"`
	SHA256 string `json:"-"`
}

// MLManifestEntry pins one case by digest.
type MLManifestEntry struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

// MLManifest is the versioned digest of the corpus.
type MLManifest struct {
	Schema       string            `json:"schema"`
	Cases        []MLManifestEntry `json:"cases"`
	CorpusSHA256 string            `json:"corpus_sha256"`
}

// MLCorpus is the enumerated corpus.
type MLCorpus struct {
	Root         string
	PolicyDir    string
	Cases        []MLCase
	CorpusSHA256 string
	PolicySHA256 string
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// digestTree hashes every regular file under dir (relative path + content).
func digestTree(dir string) (string, error) {
	var names []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			rel, _ := filepath.Rel(dir, p)
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(n)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s %s\n", n, sha256Hex(raw))
	}
	return sha256Hex([]byte(b.String())), nil
}

func manifestDigest(entries []MLManifestEntry) string {
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s %s\n", e.ID, e.SHA256)
	}
	return sha256Hex([]byte(b.String()))
}

// EnumerateMLCorpus lists the corpus directory (no list of languages or cases
// lives in code) and computes the digests. It does not compare them to the
// versioned manifest; see VerifyMLCorpus.
func EnumerateMLCorpus(root string) (*MLCorpus, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "cases"))
	if err != nil {
		return nil, fmt.Errorf("read corpus cases: %w", err)
	}
	c := &MLCorpus{Root: root, PolicyDir: filepath.Join(root, "policy")}
	var manifest []MLManifestEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, "cases", e.Name())
		raw, err := os.ReadFile(filepath.Join(dir, multilangCaseFile))
		if err != nil {
			return nil, fmt.Errorf("case %s: %w", e.Name(), err)
		}
		var mc MLCase
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&mc); err != nil {
			return nil, fmt.Errorf("case %s: %w", e.Name(), err)
		}
		if err := mc.validate(e.Name(), dir); err != nil {
			return nil, err
		}
		mc.Dir = dir
		mc.SHA256, err = digestTree(dir)
		if err != nil {
			return nil, err
		}
		c.Cases = append(c.Cases, mc)
		manifest = append(manifest, MLManifestEntry{ID: mc.ID, SHA256: mc.SHA256})
	}
	if len(c.Cases) == 0 {
		return nil, errors.New("corpus has no cases")
	}
	c.CorpusSHA256 = manifestDigest(manifest)
	c.PolicySHA256, err = digestTree(c.PolicyDir)
	return c, err
}

func (m MLCase) validate(dirName, dir string) error {
	if m.ID != dirName || m.Language == "" || m.File == "" || filepath.Base(m.File) != m.File {
		return fmt.Errorf("case %s: id, language and a top-level file are required", dirName)
	}
	if _, err := os.Stat(filepath.Join(dir, m.File)); err != nil {
		return fmt.Errorf("case %s: %w", dirName, err)
	}
	switch m.Label {
	case LabelDefect:
		if m.Rule == "" || m.Line <= 0 {
			return fmt.Errorf("case %s: a defect case needs rule and line", dirName)
		}
		if m.SimulatedModel != ModelHit && m.SimulatedModel != ModelMiss {
			return fmt.Errorf("case %s: simulated_model of a defect must be hit or miss", dirName)
		}
	case LabelClean:
		if m.SimulatedModel != ModelSilent && m.SimulatedModel != ModelFalseAlarm {
			return fmt.Errorf("case %s: simulated_model of a clean case must be silent or false_alarm", dirName)
		}
		if m.SimulatedModel == ModelFalseAlarm && m.FalseAlarmLine <= 0 {
			return fmt.Errorf("case %s: false_alarm needs false_alarm_line", dirName)
		}
	default:
		return fmt.Errorf("case %s: label must be defect or clean", dirName)
	}
	return nil
}

// Languages returns the sorted distinct languages of the corpus.
func (c *MLCorpus) Languages() []string {
	seen := map[string]bool{}
	var out []string
	for _, mc := range c.Cases {
		if !seen[mc.Language] {
			seen[mc.Language] = true
			out = append(out, mc.Language)
		}
	}
	sort.Strings(out)
	return out
}

// Manifest builds the manifest of the enumerated corpus.
func (c *MLCorpus) Manifest() MLManifest {
	m := MLManifest{Schema: MultilangSchema}
	for _, mc := range c.Cases {
		m.Cases = append(m.Cases, MLManifestEntry{ID: mc.ID, SHA256: mc.SHA256})
	}
	m.CorpusSHA256 = manifestDigest(m.Cases)
	return m
}

// WriteMLManifest regenerates manifest.json. Only used by the explicit update
// flag; a PR that adds a case regenerates it with the harness, never by hand.
func WriteMLManifest(c *MLCorpus) error {
	raw, err := json.MarshalIndent(c.Manifest(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.Root, multilangManifest), append(raw, '\n'), 0o644)
}

// VerifyMLCorpus recomputes the digests and refuses any divergence from the
// versioned manifest: a changed, added or removed case, or a wrong set digest.
func VerifyMLCorpus(c *MLCorpus) error {
	raw, err := os.ReadFile(filepath.Join(c.Root, multilangManifest))
	if err != nil {
		return fmt.Errorf("read corpus manifest: %w", err)
	}
	var want MLManifest
	if err := json.Unmarshal(raw, &want); err != nil {
		return fmt.Errorf("parse corpus manifest: %w", err)
	}
	if want.Schema != MultilangSchema {
		return fmt.Errorf("corpus manifest schema %q, want %q", want.Schema, MultilangSchema)
	}
	have := map[string]string{}
	for _, mc := range c.Cases {
		have[mc.ID] = mc.SHA256
	}
	pinned := map[string]bool{}
	for _, e := range want.Cases {
		pinned[e.ID] = true
		got, ok := have[e.ID]
		switch {
		case !ok:
			return fmt.Errorf("corpus digest divergence: case %s is in the manifest but missing on disk", e.ID)
		case got != e.SHA256:
			return fmt.Errorf("corpus digest divergence: case %s changed (manifest %s, disk %s)", e.ID, e.SHA256, got)
		}
	}
	for id := range have {
		if !pinned[id] {
			return fmt.Errorf("corpus digest divergence: case %s is on disk but not in the manifest", id)
		}
	}
	if want.CorpusSHA256 != c.CorpusSHA256 || manifestDigest(want.Cases) != want.CorpusSHA256 {
		return fmt.Errorf("corpus digest divergence: set digest %s, recomputed %s", want.CorpusSHA256, c.CorpusSHA256)
	}
	return nil
}

// ---- real binary ----

// BuildAurumBinary compiles the real CLI into outDir and returns its path.
func BuildAurumBinary(repoRoot, outDir string) (string, error) {
	bin := filepath.Join(outDir, "aurumcode")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/aurumcode")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build ./cmd/aurumcode: %v\n%s", err, out)
	}
	return bin, nil
}

// FindRepoRoot walks up from dir to the directory holding go.mod.
func FindRepoRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

// gitObject writes a loose git object and returns its id.
func gitObject(gitDir, kind string, body []byte) (string, error) {
	payload := append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...)
	sum := sha1.Sum(payload)
	id := hex.EncodeToString(sum[:])
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	if _, err := w.Write(payload); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	p := filepath.Join(gitDir, "objects", id[:2], id[2:])
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return id, os.WriteFile(p, z.Bytes(), 0o644)
}

func gitCommit(gitDir string, files map[string][]byte, parent string) (string, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var tree []byte
	for _, n := range names {
		blob, err := gitObject(gitDir, "blob", files[n])
		if err != nil {
			return "", err
		}
		raw, _ := hex.DecodeString(blob)
		tree = append(tree, []byte("100644 "+n+"\x00")...)
		tree = append(tree, raw...)
	}
	treeID, err := gitObject(gitDir, "tree", tree)
	if err != nil {
		return "", err
	}
	body := "tree " + treeID + "\n"
	if parent != "" {
		body += "parent " + parent + "\n"
	}
	body += "author Benchmark <benchmark@example.invalid> 1 +0000\ncommitter Benchmark <benchmark@example.invalid> 1 +0000\n\ncase\n"
	return gitObject(gitDir, "commit", []byte(body))
}

// materialize writes the case as a two-commit repository (base without the
// file, head with it) so the review diff is exactly the case file.
func materialize(c MLCase, repoDir string) error {
	source, err := os.ReadFile(filepath.Join(c.Dir, c.File))
	if err != nil {
		return err
	}
	gitDir := filepath.Join(repoDir, ".git")
	readme := []byte("# benchmark case\n")
	base, err := gitCommit(gitDir, map[string][]byte{"README.md": readme}, "")
	if err != nil {
		return err
	}
	head, err := gitCommit(gitDir, map[string][]byte{"README.md": readme, c.File: source}, base)
	if err != nil {
		return err
	}
	write := func(rel string, data []byte) error {
		p := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, data, 0o644)
	}
	if err := write(".git/HEAD", []byte("ref: refs/heads/main\n")); err != nil {
		return err
	}
	if err := write(".git/refs/heads/main", []byte(head+"\n")); err != nil {
		return err
	}
	if err := write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n")); err != nil {
		return err
	}
	if err := write("README.md", readme); err != nil {
		return err
	}
	return write(c.File, source)
}

// fakeModelResponse is the deterministic stand-in for the model, derived from
// the case label (and the optional simulated misbehavior). A real-model run is
// the owner's decision and cost; this card proves the harness with the fake.
// lie moves the claimed finding to a file that is not in the diff, which the
// binary must discard (the AC-001 control).
func fakeModelResponse(c MLCase, lie bool) ([]byte, error) {
	type issue struct {
		File         string `json:"file"`
		Line         int    `json:"line"`
		Severity     string `json:"severity"`
		RuleID       string `json:"rule_id"`
		Message      string `json:"message"`
		Evidence     string `json:"evidence"`
		Impact       string `json:"impact"`
		Verification string `json:"verification"`
	}
	resp := struct {
		Summary string  `json:"summary"`
		Verdict string  `json:"verdict"`
		Issues  []issue `json:"issues"`
	}{Summary: "benchmark fixture review", Verdict: "approve", Issues: []issue{}}
	line, rule := 0, ""
	switch {
	case c.Label == LabelDefect && c.SimulatedModel == ModelHit:
		line, rule = c.Line, c.Rule
	case c.Label == LabelClean && c.SimulatedModel == ModelFalseAlarm:
		line, rule = c.FalseAlarmLine, "sql-injection"
	}
	if line > 0 {
		src, err := os.ReadFile(filepath.Join(c.Dir, c.File))
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(src), "\n")
		if line > len(lines) {
			return nil, fmt.Errorf("case %s: line %d beyond file", c.ID, line)
		}
		file := c.File
		if lie {
			file = multilangFixtureLies
		}
		resp.Verdict = "changes_requested"
		resp.Issues = append(resp.Issues, issue{
			File: file, Line: line, Severity: "error", RuleID: multilangRulePrefix + rule,
			Message:      "Security policy violation: " + rule,
			Evidence:     strings.TrimSpace(lines[line-1]),
			Impact:       "Untrusted input reaches a sensitive sink.",
			Verification: "Apply the safe API named in the policy section and rerun the review.",
		})
	}
	return json.MarshalIndent(resp, "", "  ")
}

// RunArtifacts is everything one binary execution produced.
type RunArtifacts struct {
	Case        MLCase
	ExitCode    int
	Stdout      string
	Stderr      string
	FixturePath string
	SARIFPath   string
	AuditPath   string
}

// Outcome is what a reader extracts for scoring.
type Outcome struct {
	Findings     []Finding
	Verdict      string
	GateDecision string
	ExitCode     int
}

// Approved reports whether the run let the change through: exit 0 and a
// passing gate. An inconclusive or failed gate is never approval.
func (o Outcome) Approved() bool {
	return o.ExitCode == 0 && o.GateDecision == "pass"
}

// OutcomeReader extracts the outcome of one run.
type OutcomeReader func(RunArtifacts) (Outcome, error)

// ReadBinaryOutput is the real reader: verdict and findings come from the
// SARIF document and the audit record the binary wrote.
func ReadBinaryOutput(r RunArtifacts) (Outcome, error) {
	o := Outcome{ExitCode: r.ExitCode}
	rawAudit, err := os.ReadFile(r.AuditPath)
	if err != nil {
		return o, fmt.Errorf("audit record: %w (exit %d, stderr: %s)", err, r.ExitCode, r.Stderr)
	}
	var audit struct {
		Verdict string `json:"verdict"`
		Gate    struct {
			Decision string `json:"decision"`
		} `json:"gate"`
	}
	if err := json.Unmarshal(rawAudit, &audit); err != nil {
		return o, fmt.Errorf("audit record: %w", err)
	}
	o.Verdict, o.GateDecision = audit.Verdict, audit.Gate.Decision
	rawSARIF, err := os.ReadFile(r.SARIFPath)
	if err != nil {
		return o, fmt.Errorf("sarif: %w", err)
	}
	var sarif struct {
		Runs []struct {
			Results []struct {
				RuleID    string `json:"ruleId"`
				Level     string `json:"level"`
				Locations []struct {
					Physical struct {
						Artifact struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(rawSARIF, &sarif); err != nil {
		return o, fmt.Errorf("sarif: %w", err)
	}
	for _, run := range sarif.Runs {
		for _, res := range run.Results {
			f := Finding{CaseID: r.Case.ID, RuleID: res.RuleID, Severity: Severity(res.Level)}
			if len(res.Locations) > 0 {
				f.File = res.Locations[0].Physical.Artifact.URI
				f.Line = res.Locations[0].Physical.Region.StartLine
			}
			o.Findings = append(o.Findings, f)
		}
	}
	return o, nil
}

// ReadFixture is the MUT-001 mutant: it reads the findings from the fixture
// that stood in for the model instead of from the binary's output. It exists
// only so the acceptance can prove that reading the fixture is detected.
func ReadFixture(r RunArtifacts) (Outcome, error) {
	o := Outcome{ExitCode: r.ExitCode}
	raw, err := os.ReadFile(r.FixturePath)
	if err != nil {
		return o, err
	}
	var resp struct {
		Verdict string `json:"verdict"`
		Issues  []struct {
			File   string `json:"file"`
			Line   int    `json:"line"`
			RuleID string `json:"rule_id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return o, err
	}
	o.Verdict = resp.Verdict
	for _, i := range resp.Issues {
		o.Findings = append(o.Findings, Finding{CaseID: r.Case.ID, File: i.File, Line: i.Line, RuleID: i.RuleID})
	}
	o.GateDecision = "pass"
	if len(o.Findings) > 0 {
		o.GateDecision = "fail"
	}
	return o, nil
}

// RunCase executes the real binary on one case in a fresh temp repository.
func RunCase(bin string, c MLCase, policyDir, workDir string, lie bool) (RunArtifacts, error) {
	r := RunArtifacts{Case: c}
	repo := filepath.Join(workDir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return r, err
	}
	if err := materialize(c, repo); err != nil {
		return r, err
	}
	resp, err := fakeModelResponse(c, lie)
	if err != nil {
		return r, err
	}
	r.FixturePath = filepath.Join(workDir, "response.json")
	r.SARIFPath = filepath.Join(workDir, "review.sarif")
	r.AuditPath = filepath.Join(workDir, "audit.json")
	if err := os.WriteFile(r.FixturePath, resp, 0o644); err != nil {
		return r, err
	}
	cmd := exec.Command(bin, "review", "--base", "HEAD~1", "--politica", policyDir,
		"--auditoria", r.AuditPath, "--sarif", r.SARIFPath)
	cmd.Dir = repo
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + workDir,
		"XDG_CACHE_HOME=" + filepath.Join(workDir, "cache"),
		"AURUMCODE_CACHE_DIR=" + filepath.Join(workDir, "cache"),
		"AURUMCODE_LLM_FIXTURE=" + r.FixturePath,
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	r.Stdout, r.Stderr = stdout.String(), stderr.String()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return r, err
		}
		r.ExitCode = ee.ExitCode()
	}
	return r, nil
}

// CheckOutputProvenance is the AC-001 control. For the first defect case it
// feeds the binary a fixture whose claimed finding claims a finding on a file that
// is not in the diff. The binary must discard it: a reader that follows the binary
// sees no finding, a reader that follows the fixture still sees one. The check
// fails (returns an error) when the reader disagrees with the binary.
func CheckOutputProvenance(bin string, c *MLCorpus, read OutcomeReader, workDir string) error {
	for _, mc := range c.Cases {
		if mc.Label != LabelDefect || mc.SimulatedModel != ModelHit {
			continue
		}
		dir := filepath.Join(workDir, "provenance-"+mc.ID)
		r, err := RunCase(bin, mc, c.PolicyDir, dir, true)
		if err != nil {
			return err
		}
		real, err := ReadBinaryOutput(r)
		if err != nil {
			return err
		}
		if len(real.Findings) != 0 {
			return fmt.Errorf("control case %s: the binary kept a finding on a file outside the diff", mc.ID)
		}
		got, err := read(r)
		if err != nil {
			return err
		}
		if len(got.Findings) != len(real.Findings) {
			return fmt.Errorf("control case %s: reader reports %d findings while the binary output has %d: the reader is not reading the binary output", mc.ID, len(got.Findings), len(real.Findings))
		}
		return nil
	}
	return errors.New("no defect case with a hit model to build the control")
}
