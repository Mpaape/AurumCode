package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aur622Canary is the secret-shaped value the redaction must remove, built at
// run time so the literal never sits in the source.
var aur622Canary = "pass" + "word=" + "canario-aur622-7c1d"

// The fixture's texts: none of them may reach stdout or stderr.
const (
	aur622Title = "Consulta SQL montada por concatenacao"
	aur622File  = "app.py"
	aur622Desc  = "A entrada do usuario chega ao cursor sem parametro"
)

func aur622Finding(title, file string, line int, cwe, sev, verdict, channel, caseID string) map[string]any {
	return map[string]any{
		"finding": map[string]any{
			"title": title, "file": file, "line_start": line, "line_end": line,
			"vuln_class": "sql_injection", "severity": sev, "cwe": cwe,
			"impact": "leitura de dados", "description": aur622Desc + " " + aur622Canary,
			"recommendation": "usar parametros", "case_id": caseID,
			"verdict": verdict, "verdict_confidence": 9, "channel": channel,
		},
		"severity": sev, "exploitability_notes": "",
	}
}

// aur622Report is a findings.json of the harness: 2 TRUE_POSITIVE, 1
// FALSE_POSITIVE, unless mutate changes it.
func aur622Report(mutate func(map[string]any)) map[string]any {
	r := map[string]any{
		"schema_version": "1.0", "repo_root": "/alvo", "repo_name": "alvo", "git_sha": "0",
		"findings": []any{
			aur622Finding(aur622Title, aur622File, 11, "CWE-89", "critical", "TRUE_POSITIVE", "main", "vvaf1_"+strings.Repeat("a", 32)),
			aur622Finding("Erro detalhado exposto", aur622File, 9, "CWE-209", "medium", "TRUE_POSITIVE", "main", "vvaf1_"+strings.Repeat("b", 32)),
			aur622Finding("Rota sem autenticacao", aur622File, 7, "CWE-306", "medium", "FALSE_POSITIVE", "main", "vvaf1_"+strings.Repeat("c", 32)),
		},
		"chains": []any{}, "dropped": []any{}, "raw_findings_count": 4, "summary": "",
		"degraded": false, "degraded_reason": nil, "enrichment_unavailable": false, "unreachable_files": []any{},
	}
	if mutate != nil {
		mutate(r)
	}
	return r
}

func aur622HarnessSARIF(degraded, hollow, ok bool) map[string]any {
	return map[string]any{
		"version": "2.1.0",
		"runs": []any{map[string]any{
			"properties":  map[string]any{"hollow": hollow, "coverageDegraded": degraded},
			"invocations": []any{map[string]any{"executionSuccessful": ok}},
			"results":     []any{},
		}},
	}
}

// aur622HarnessReasons is a harness SARIF with coverageDegraded and reasons.
func aur622HarnessReasons(reasons ...any) map[string]any {
	doc := aur622HarnessSARIF(true, false, true)
	doc["runs"].([]any)[0].(map[string]any)["properties"].(map[string]any)["coverageDegradedReasons"] = reasons
	return doc
}

type aur622Run struct {
	repo, out, sarif string
	opts             Options
}

// aur622Setup writes a tree with app.py and the plugin's output files.
func aur622Setup(t *testing.T, report any, harness any) *aur622Run {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, aur622File), []byte("print('x')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan := filepath.Join(repo, "security-scan")
	if err := os.MkdirAll(scan, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, v any) {
		var data []byte
		if s, ok := v.(string); ok {
			data = []byte(s)
		} else {
			data, _ = json.Marshal(v)
		}
		if err := os.WriteFile(filepath.Join(scan, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if report != nil {
		write("findings.json", report)
	}
	if harness != nil {
		write("alvo_20261014_report.sarif", harness)
	}
	out := t.TempDir()
	r := &aur622Run{repo: repo, out: out, sarif: filepath.Join(out, "plugin.sarif")}
	r.opts = Options{Name: "vvah", Format: "vvah-findings", Input: filepath.Join(scan, "findings.json"), Repo: repo, SARIF: r.sarif, ToolVersion: "test"}
	return r
}

func (r *aur622Run) run(t *testing.T) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(r.opts, "pt-BR", &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func assertNoFindingText(t *testing.T, where, s string) {
	t.Helper()
	for _, leak := range []string{aur622Title, aur622File, aur622Desc, "canario-aur622", "Erro detalhado", "Rota sem"} {
		if strings.Contains(s, leak) {
			t.Fatalf("%s carries finding text %q: %s", where, leak, s)
		}
	}
}

// AC-002: 2 TRUE_POSITIVE + 1 FALSE_POSITIVE -> valid SARIF with 2 results,
// stable rule/fingerprint/category, the secret redacted, counts only on
// stdout.
func TestAUR622IngestWritesOnlyTruePositives(t *testing.T) {
	r := aur622Setup(t, aur622Report(nil), aur622HarnessSARIF(false, false, true))
	code, stdout, stderr := r.run(t)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	assertNoFindingText(t, "stdout", stdout)
	assertNoFindingText(t, "stderr", stderr)
	if !strings.Contains(stdout, "2 alertas enviados") || !strings.Contains(stdout, "falso positivo 1") {
		t.Fatalf("stdout does not count: %q", stdout)
	}
	raw, err := os.ReadFile(r.sarif)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "canario-aur622") {
		t.Fatalf("secret not redacted in SARIF")
	}
	var doc struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name string `json:"name"`
				} `json:"driver"`
			} `json:"tool"`
			AutomationDetails struct {
				ID string `json:"id"`
			} `json:"automationDetails"`
			Results []struct {
				RuleID              string            `json:"ruleId"`
				PartialFingerprints map[string]string `json:"partialFingerprints"`
				Properties          map[string]string `json:"properties"`
				Locations           []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	run := doc.Runs[0]
	if doc.Version != "2.1.0" || run.Tool.Driver.Name != "AurumCode plugin vvah" || run.AutomationDetails.ID != "aurumcode/plugin/vvah/" || len(run.Results) != 2 {
		t.Fatalf("SARIF = %s", raw)
	}
	res := run.Results[0]
	if res.RuleID != "plugin/vvah/cwe-89" || res.PartialFingerprints[FingerprintKey] != "vvaf1_"+strings.Repeat("a", 32) ||
		res.Properties["security-severity"] != "9.5" || res.Locations[0].PhysicalLocation.ArtifactLocation.URI != aur622File {
		t.Fatalf("result 0 = %+v", res)
	}
	// Stable: a second ingest writes the same document.
	again, _ := os.ReadFile(r.sarif)
	if code, _, _ = r.run(t); code != 0 {
		t.Fatal("second ingest failed")
	}
	if second, _ := os.ReadFile(r.sarif); !bytes.Equal(again, second) {
		t.Fatal("SARIF is not stable across ingests")
	}
}

// AC-002: the posture channel and a null verdict are counted, never sent.
func TestAUR622IngestCountsOtherChannelAndNoVerdict(t *testing.T) {
	report := aur622Report(func(r map[string]any) {
		fs := r["findings"].([]any)
		fs = append(fs, aur622Finding("postura", aur622File, 1, "CWE-1", "low", "TRUE_POSITIVE", "posture", ""))
		f := aur622Finding("sem veredito", aur622File, 2, "", "low", "", "main", "")
		f["finding"].(map[string]any)["verdict"] = nil
		r["findings"] = append(fs, f)
	})
	r := aur622Setup(t, report, aur622HarnessSARIF(false, false, true))
	code, stdout, stderr := r.run(t)
	if code != 0 || !strings.Contains(stdout, "2 alertas enviados") || !strings.Contains(stdout, "outro canal 1") || !strings.Contains(stdout, "sem veredito 1") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// AC-003: every inconclusive run exits 1 and writes NO SARIF.
func TestAUR622InconclusiveWritesNoSARIF(t *testing.T) {
	okHarness := aur622HarnessSARIF(false, false, true)
	cases := map[string]struct {
		report  any
		harness any
		exit    int
		errors  string
		reason  string
	}{
		"degraded":                {aur622Report(func(r map[string]any) { r["degraded"] = true }), okHarness, 0, "", ReasonDegraded},
		"invalid json":            {"{not json", okHarness, 0, "", ReasonOutputInvalid},
		"schema 2.x":              {aur622Report(func(r map[string]any) { r["schema_version"] = "2.0" }), okHarness, 0, "", ReasonSchemaVersion},
		"plugin exit":             {aur622Report(nil), okHarness, 1, "", ReasonPluginExit},
		"missing file":            {nil, okHarness, 0, "", ReasonOutputMissing},
		"harness sarif missing":   {aur622Report(nil), nil, 0, "", ReasonOutputMissing},
		"coverage degraded empty": {aur622Report(func(r map[string]any) { r["findings"] = []any{} }), aur622HarnessSARIF(true, false, true), 0, "", ReasonHollow},
		"hollow":                  {aur622Report(nil), aur622HarnessSARIF(false, true, true), 0, "", ReasonHollow},
		"execution failed":        {aur622Report(nil), aur622HarnessSARIF(false, false, false), 0, "", ReasonExecutionFailed},
		"analysis stage errors":   {aur622Report(nil), okHarness, 0, `{"stage": "s4", "error": "StructuredOutputValidationError"}` + "\n", ReasonAnalysisErrors},
		"zero_analysis":           {aur622Report(func(r map[string]any) { r["findings"] = []any{} }), aur622HarnessReasons("zero_analysis"), 0, "", ReasonHollow},
		"known and unknown":       {aur622Report(nil), aur622HarnessReasons("s6_undetermined", "something_new"), 0, "", ReasonHollow},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := aur622Setup(t, c.report, c.harness)
			r.opts.PluginExit = c.exit
			if c.errors != "" {
				if err := os.WriteFile(filepath.Join(r.repo, "security-scan", "alvo_20261014_errors.jsonl"), []byte(c.errors), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			code, stdout, stderr := r.run(t)
			if code != 1 {
				t.Fatalf("exit=%d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
			}
			if _, err := os.Stat(r.sarif); !os.IsNotExist(err) {
				t.Fatalf("a SARIF was written for an inconclusive run")
			}
			if !strings.Contains(stderr, "["+c.reason+"]") || stdout != "" {
				t.Fatalf("stdout=%q stderr=%q, want reason %s", stdout, stderr, c.reason)
			}
			assertNoFindingText(t, "stderr", stderr)
		})
	}
}

// An errors.jsonl of a stage outside the analysis keeps the run conclusive.
func TestAUR622NonAnalysisErrorIsConclusive(t *testing.T) {
	r := aur622Setup(t, aur622Report(nil), aur622HarnessSARIF(false, false, true))
	if err := os.WriteFile(filepath.Join(r.repo, "security-scan", "x_errors.jsonl"), []byte(`{"stage": "s9"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := r.run(t); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
}

// AC-004: a finding outside the tree fails the ingest, never disappears.
func TestAUR622PathOutsideTreeFails(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.py"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"absolute": filepath.Join(outside, "secret.py"),
		"dotdot":   "../" + filepath.Base(outside) + "/secret.py",
		"missing":  "nao-existe.py",
		"symlink":  "link/secret.py",
		"output":   "security-scan/findings.json",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			report := aur622Report(func(r map[string]any) {
				r["findings"].([]any)[1].(map[string]any)["finding"].(map[string]any)["file"] = path
			})
			r := aur622Setup(t, report, aur622HarnessSARIF(false, false, true))
			if err := os.Symlink(outside, filepath.Join(r.repo, "link")); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := r.run(t)
			if code != 1 || stdout != "" || !strings.Contains(stderr, "nº 2") {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if strings.Contains(stderr, outside) || strings.Contains(stderr, "secret.py") {
				t.Fatalf("stderr names the refused path: %s", stderr)
			}
			if _, err := os.Stat(r.sarif); !os.IsNotExist(err) {
				t.Fatal("a SARIF was written although a finding was refused")
			}
		})
	}
}

// The generic sarif adapter: coverage loss is inconclusive, results go
// through with the plugin's fingerprint.
func TestAUR622GenericSARIFAdapter(t *testing.T) {
	doc := aur622HarnessSARIF(false, false, true)
	run := doc["runs"].([]any)[0].(map[string]any)
	run["results"] = []any{map[string]any{
		"ruleId": "py/sql", "level": "error", "message": map[string]any{"text": aur622Title},
		"locations":           []any{map[string]any{"physicalLocation": map[string]any{"artifactLocation": map[string]any{"uri": aur622File}, "region": map[string]any{"startLine": 3}}}},
		"partialFingerprints": map[string]any{"k": "fp-1"},
	}}
	r := aur622Setup(t, nil, nil)
	data, _ := json.Marshal(doc)
	in := filepath.Join(r.repo, "out.sarif")
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	r.opts.Format, r.opts.Input = "sarif", in
	code, stdout, stderr := r.run(t)
	if code != 0 || !strings.Contains(stdout, "1 alerta enviado") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	run["properties"] = map[string]any{"coverageDegraded": true}
	data, _ = json.Marshal(doc)
	os.WriteFile(in, data, 0o644)
	os.Remove(r.sarif)
	if code, _, _ := r.run(t); code != 1 {
		t.Fatalf("coverageDegraded generic SARIF exit=%d", code)
	}
	if _, err := os.Stat(r.sarif); !os.IsNotExist(err) {
		t.Fatal("SARIF written for a degraded generic run")
	}
}

// Destination: a public repository never uses issues.
func TestAUR622DestinationNeverIssuesOnPublic(t *testing.T) {
	if Destination("public", false) != DestinationSecurity || Destination("", false) != DestinationSecurity {
		t.Fatal("public or unknown visibility must use the Security tab")
	}
	if Destination("private", false) != DestinationIssues || Destination("private", true) != DestinationSecurity {
		t.Fatal("private destination")
	}
	if CheckDestination("public", DestinationIssues) == nil || CheckDestination("private", DestinationIssues) != nil {
		t.Fatal("CheckDestination")
	}
}

// The real run: 4 TRUE_POSITIVE, a candidate dropped as INCONCLUSIVE (plus
// DUPLICATE and FALSE_POSITIVE drops) and coverageDegraded only for
// s6_undetermined -> conclusive, 5 results, the doubt as a warning.
func TestAUR622UnverifiedDropBecomesWarning(t *testing.T) {
	report := aur622Report(func(r map[string]any) {
		fs := r["findings"].([]any)[:2]
		fs = append(fs,
			aur622Finding("Terceiro", aur622File, 3, "CWE-306", "medium", "TRUE_POSITIVE", "main", "vvaf1_"+strings.Repeat("d", 32)),
			aur622Finding("Quarto", aur622File, 4, "CWE-798", "high", "TRUE_POSITIVE", "main", "vvaf1_"+strings.Repeat("e", 32)))
		r["findings"] = fs
		drop := func(reason string, line int) map[string]any {
			return map[string]any{"file": aur622File, "line": line, "vuln_class": "sql_injection", "title": aur622Title,
				"chunk_id": "c1", "reason": reason, "detail": aur622Desc, "canonical_idx": 0}
		}
		r["dropped"] = []any{drop("INCONCLUSIVE", 12), drop("DUPLICATE", 11), drop("FALSE_POSITIVE", 5)}
	})
	r := aur622Setup(t, report, aur622HarnessReasons("s6_undetermined"))
	code, stdout, stderr := r.run(t)
	if code != 0 || !strings.Contains(stdout, "5 alertas enviados") || !strings.Contains(stdout, "não verificados enviados como aviso: 1") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertNoFindingText(t, "stdout", stdout)
	raw, _ := os.ReadFile(r.sarif)
	var doc struct {
		Runs []struct {
			Results []struct {
				RuleID              string            `json:"ruleId"`
				Level               string            `json:"level"`
				PartialFingerprints map[string]string `json:"partialFingerprints"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	res := doc.Runs[0].Results
	if len(res) != 5 {
		t.Fatalf("results = %d", len(res))
	}
	last := res[4]
	if last.RuleID != "plugin/vvah/nao-verificado/sql_injection" || last.Level != "warning" || last.PartialFingerprints[FingerprintKey] == "" {
		t.Fatalf("unverified result = %+v", last)
	}
	// Stable across a small line shift inside the same block of ten.
	if unverifiedFinding(vvahDropped{File: "a.py", Line: 12, VulnClass: "x"}).Fingerprint != unverifiedFinding(vvahDropped{File: "a.py", Line: 17, VulnClass: "x"}).Fingerprint {
		t.Fatal("unverified fingerprint moves with the line")
	}
}
