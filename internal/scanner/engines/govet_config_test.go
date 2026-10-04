package engines_test

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
)

// AC-004: govet runs only when quality_gates.scanners declares it; a
// configuration without it keeps the review exactly as it was.
func TestGovetRunsOnlyWhenDeclared(t *testing.T) {
	without, err := config.Parse([]byte("quality_gates:\n  sast:\n    engine: semgrep\n"), "teste")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range without.QualityGates.EnabledScanners() {
		if entry.Name() == "govet" {
			t.Fatal("govet runs without being declared")
		}
	}
	with, err := config.Parse([]byte("quality_gates:\n  scanners:\n    - engine: govet\n      fail_on: warning\ngate:\n  sources: [lint]\n"), "teste")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entries := with.QualityGates.EnabledScanners()
	if len(entries) != 1 || entries[0].Name() != "govet" {
		t.Fatalf("enabled = %+v, want govet", entries)
	}
	if engine, ok := entries[0].Lookup(); !ok || engine.Category != "lint" || engine.TypedOrigin() != "govet" {
		t.Fatalf("engine = %+v", engine)
	}
	if _, err := config.Parse([]byte("quality_gates:\n  scanners:\n    - engine: govet\n      options:\n        flags: [-toolexec]\n"), "teste"); err == nil {
		t.Fatal("an option of govet was accepted")
	}
}
