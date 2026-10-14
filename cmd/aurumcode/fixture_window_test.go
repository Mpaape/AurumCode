package main

import "testing"

func TestFixtureWindowReadsTheEnvironment(t *testing.T) {
	for raw, want := range map[string]int{"": 0, "  ": 0, "100000": 100000, " 42 ": 42} {
		t.Setenv("AURUMCODE_LLM_FIXTURE_WINDOW", raw)
		got, err := fixtureWindow()
		if err != nil || got != want {
			t.Errorf("AURUMCODE_LLM_FIXTURE_WINDOW=%q: got %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"0", "-5", "many", "1.5"} {
		t.Setenv("AURUMCODE_LLM_FIXTURE_WINDOW", raw)
		if got, err := fixtureWindow(); err == nil {
			t.Errorf("AURUMCODE_LLM_FIXTURE_WINDOW=%q accepted as %d: a window that is not a positive number must be refused", raw, got)
		}
	}
}
