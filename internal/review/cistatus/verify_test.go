package cistatus

import (
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestVerifyNeverTakesTheModelsState: an item no concluded check supports
// gets no observed state, whatever status the model wrote.
func TestVerifyNeverTakesTheModelsState(t *testing.T) {
	for _, raw := range []string{"", `[{"name":"Lint","state":"SUCCESS"}]`, "not json"} {
		got := Parse(raw, "aurumcode/").Verify([]types.CIAnalysis{{Check: "CI", Status: "success", Evidence: "CI verde"}})
		if got[0].Basis != BasisModel || got[0].Observed != "" || got[0].Grounded {
			t.Fatalf("context %q: model state taken as observed: %+v", raw, got[0])
		}
	}
}

// TestVerifyObservedStateAndLink: a concluded check gives the state and the
// link from the context; a repeated name reports the failure.
func TestVerifyObservedStateAndLink(t *testing.T) {
	ctx := Parse(`[{"name":"Lint","state":"SUCCESS","link":"l1"},{"name":"lint","state":"FAILURE","link":"l2"}]`, "aurumcode/")
	got := ctx.Verify([]types.CIAnalysis{{Check: "Lint", Status: "success"}})[0]
	if got.Basis != BasisCI || got.Observed != "failure" || got.Link != "l2" {
		t.Fatalf("observed = %+v, want the failing entry", got)
	}
	if Passed(got.Observed) || !Passed("success") || !Passed("SKIPPED") {
		t.Fatal("Passed misclassifies states")
	}
}

// TestVerifyGroundedOnlyWhenEvidenceQuotesTheExcerpt: evidence is grounded
// only when it quotes the supplied excerpt; no excerpt, no grounding.
func TestVerifyGroundedOnlyWhenEvidenceQuotesTheExcerpt(t *testing.T) {
	ctx := Parse(`[{"name":"Lint","state":"FAILURE","excerpt":"main.go:3:1:   x declared\n and not used"}]`, "aurumcode/")
	cases := map[string]bool{"main.go:3:1: x declared and not used": true, "x foi declarado": false, "": false}
	for evidence, want := range cases {
		if got := ctx.Verify([]types.CIAnalysis{{Check: "Lint", Evidence: evidence}})[0].Grounded; got != want {
			t.Fatalf("evidence %q grounded=%v want %v", evidence, got, want)
		}
	}
	bare := Parse(`[{"name":"Lint","state":"FAILURE"}]`, "aurumcode/")
	if bare.Verify([]types.CIAnalysis{{Check: "Lint", Evidence: "anything"}})[0].Grounded {
		t.Fatal("grounded without any excerpt")
	}
}
