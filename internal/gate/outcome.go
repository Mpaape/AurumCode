package gate

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// ApplyOutcome publishes the decision's lines (stderr and the review's
// limitations) and, when the gate failed or was inconclusive, sets the
// engine-owned marker that withholds approval regardless of the model's
// own verdict. Shared by --base and --pr.
func ApplyOutcome(run *Run, gateResult *Result) {
	result := run.Review
	if gateResult.Active {
		for _, line := range gateResult.Lines {
			fmt.Fprintf(run.Stderr, "aurumcode review: policy gate: %s\n", line)
			result.Limitations = append(result.Limitations, "policy gate: "+line)
		}
		if gateResult.Fail || gateResult.Inconclusive {
			// B-V: PolicyGateWithheldKey is the ONLY mechanism that
			// withholds approval; result.Verdict is model-controlled.
			if result.Metadata == nil {
				result.Metadata = make(map[string]string)
			}
			result.Metadata[prompt.PolicyGateWithheldKey] = "true"
		}
	}
}

// Withheld reports whether the engine withheld approval for this review.
func Withheld(r *types.ReviewResult) bool {
	return r != nil && r.Metadata[prompt.PolicyGateWithheldKey] == "true"
}
