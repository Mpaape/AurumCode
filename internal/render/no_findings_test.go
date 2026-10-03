package render

import "testing"

func TestNoFindingsLine(t *testing.T) {
	cases := map[string]string{
		"":                     "No issues found.",
		" , ":                  "No issues found.",
		"sast_execution_error": "Sem achados nas fontes concluídas; inconclusivo: sast_execution_error",
		"sast_execution_error,partial_coverage,sast_execution_error": "Sem achados nas fontes concluídas; inconclusivo: sast_execution_error, partial_coverage",
	}
	for in, want := range cases {
		if got := NoFindingsLine(in); got != want {
			t.Errorf("NoFindingsLine(%q) = %q, want %q", in, got, want)
		}
	}
}
