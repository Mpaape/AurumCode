package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func aur532Report(t *testing.T, dir, name string, approvedWithDefect int, recall string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := `{"schema":"aurum.benchmark-multilang","total":{"cases":20,"defects":10,"recall":` + recall + `,"precision":0.9,"approved_with_defect":` + strconv.Itoa(approvedWithDefect) + `}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// AC-005: the measurement mode prints before/after and fails on a regression
// of "aprovado com defeito" or a missing report; the command needs its
// repositories otherwise.
func TestAUR532AC005MeasureCommand(t *testing.T) {
	dir := t.TempDir()
	before := aur532Report(t, dir, "antes.json", 1, "0.8")
	worse := aur532Report(t, dir, "pior.json", 3, "0.8")
	better := aur532Report(t, dir, "melhor.json", 0, "0.9")
	run := func(args ...string) (int, string) {
		var stdout, stderr bytes.Buffer
		code := runFeedback(args, &stdout, &stderr, nil, func(string) string { return "" })
		return code, stdout.String() + stderr.String()
	}
	if code, out := run("--medir", "--medicao-antes", before, "--medicao-depois", worse); code != 1 || !strings.Contains(out, "REGRESSÃO") {
		t.Fatalf("regression: exit %d\n%s", code, out)
	}
	if code, out := run("--medir", "--medicao-antes", before, "--medicao-depois", better); code != 0 || !strings.Contains(out, "| Recall | 0.8000 | 0.9000 |") {
		t.Fatalf("improvement: exit %d\n%s", code, out)
	}
	if code, out := run("--medir", "--medicao-antes", before); code != 1 || !strings.Contains(out, "Não medido") {
		t.Fatalf("missing report: exit %d\n%s", code, out)
	}
	if code, _ := run("--org", "o"); code != 2 {
		t.Fatalf("missing --repo-politica: exit %d", code)
	}
	if code, out := run("--repos", "o/a", "--repo-politica", "o/p", "--medicao-antes", filepath.Join(dir, "nao-existe.json")); code != 1 || !strings.Contains(out, "--medicao-antes") {
		t.Fatalf("unreadable report: exit %d\n%s", code, out)
	}
}
