package prompt

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// ResponseParser parses LLM responses into structured data
type ResponseParser struct{}

// NewResponseParser creates a new response parser
func NewResponseParser() *ResponseParser {
	return &ResponseParser{}
}

// canonicalReviewFields is the ONE findings schema the review prompt
// template (templates/review.md) is allowed to show the model: exactly the
// top-level JSON fields this parser turns into a types.ReviewResult a user
// can see. AUR-459 exists because the template used to show two findings
// schemas -- "line_comments" first, "issues" second -- while this parser
// read only "issues": the model picked the first one it was shown and a
// diff with three planted secrets printed "No issues found." with exit 0.
//
// TestReviewTemplateMatchesParser extracts the REAL top-level keys of the
// template's JSON example and requires this exact set, so a future edit
// that teaches the model a field nobody reads breaks the build instead of
// turning back into silence.
var canonicalReviewFields = []string{
	"ci_analysis",
	"issues",
	"iso_scores",
	"limitations",
	"strengths",
	"suggestions",
	"summary",
	"test_plan",
	"verdict",
}

// acceptedReviewFields is what this parser consumes: the canonical set
// plus "line_comments", kept as a tolerated alias rather than a taught
// schema. The template no longer shows it, but a model that saw an older
// prompt, a user-supplied .aurumcode/prompts/review.md, or a model that
// simply drifts to the PR-comment vocabulary still gets its findings read
// instead of dropped (see adoptLineComments). Every name here is proven to
// change the parse result by TestAcceptedReviewFieldsAreConsumed -- the
// list is a claim, that test is the fact.
var acceptedReviewFields = []string{
	"ci_analysis",
	"issues",
	"iso_scores",
	"limitations",
	"line_comments",
	"strengths",
	"suggestions",
	"summary",
	"test_plan",
	"verdict",
}

// ParseReviewResponse parses a review response from LLM
func (p *ResponseParser) ParseReviewResponse(response string) (*types.ReviewResult, error) {
	// Extract JSON from response (handle markdown code blocks)
	jsonContent := p.extractJSON(response)

	if jsonContent == "" {
		return p.degradedOrError(response, ParseErrorNoJSON, nil)
	}

	// Keep findings strict, but do not erase a review because an auxiliary
	// narrative section has the wrong JSON shape. The discarded section is
	// recorded in metadata so callers can diagnose the provider drift.
	var result types.ReviewResult
	var fields map[string]json.RawMessage
	var discarded []string
	for {
		result = types.ReviewResult{}
		err := json.Unmarshal([]byte(jsonContent), &result)
		if err == nil {
			break
		}
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			return p.degradedOrError(response, ParseErrorInvalidJSON, err)
		}
		section, _, _ := strings.Cut(typeErr.Field, ".")
		switch section {
		case "strengths", "suggestions", "ci_analysis", "test_plan", "limitations", "iso_scores", "summary", "evidence_assessments":
		default:
			return p.degradedOrError(response, ParseErrorInvalidJSON, err)
		}
		if fields == nil {
			if json.Unmarshal([]byte(jsonContent), &fields) != nil {
				return p.degradedOrError(response, ParseErrorInvalidJSON, err)
			}
		}
		if _, exists := fields[section]; !exists {
			return p.degradedOrError(response, ParseErrorInvalidJSON, err)
		}
		delete(fields, section)
		discarded = append(discarded, section)
		pruned, marshalErr := json.Marshal(fields)
		if marshalErr != nil {
			return p.degradedOrError(response, ParseErrorInvalidJSON, marshalErr)
		}
		jsonContent = string(pruned)
	}

	// result.Metadata decodes straight from the model's own top-level
	// "metadata" object when the response carries one: it is model-authored,
	// untrusted input, exactly like every other field json.Unmarshal just
	// populated above. AUR-519's degraded-parse detection (ParseModeKey,
	// IsDegradedParse below) must never be forgeable by a model replying
	// with a syntactically valid, otherwise-ordinary review that simply
	// includes "metadata":{"parse_mode":"degraded"} -- that would let a
	// compromised or confused provider manufacture an "inconclusive" status
	// a policy's gate.inconclusive is supposed to react to only on this
	// parser's own, engine-decided failure path (degradedOrError below).
	// Scrub the reserved key here, on every successful strict-JSON decode,
	// before any engine-owned metadata is ever written to this same map.
	delete(result.Metadata, ParseModeKey)
	// Same forgery risk, same fix, for AUR-519's gate-withheld marker
	// (cmd/aurumcode's reviewVerdictForLanguage/formalReviewEvent/
	// canonicalVerdict check PolicyGateWithheldKey, never the model's own
	// Verdict text, precisely so a model cannot talk its way into -- or
	// out of -- a gate-driven withholding by supplying
	// "metadata":{"policy_gate_withheld":"true"} itself).
	delete(result.Metadata, PolicyGateWithheldKey)
	// AUR-519 (B-C): the per-file coverage counts the prompt builder
	// computed (code_files_total/complete/partial/omitted,
	// PromptParts.Meta) are a different map the model's JSON body never
	// touches, but a model could still smuggle same-named keys into its
	// own "metadata" object and have them survive here unless scrubbed --
	// internal/review/reviewer.go overwrites these from the trusted
	// PromptParts.Meta right after this call, but scrubbing here too
	// means a caller that forgets that overwrite still fails closed
	// (absent) rather than trusting a model-supplied count.
	for _, key := range []string{"code_files_total", "code_files_complete", "code_files_partial", "code_files_omitted"} {
		delete(result.Metadata, key)
	}

	// Origin is engine-owned: a model cannot claim a finding came from a
	// deterministic analyzer. An assessment with a status outside the
	// closed set, or naming no evidence, is dropped rather than guessed.
	sanitizeModelIssueProvenance(result.Issues)

	// A finding the model reported under "line_comments" is a finding: it
	// becomes an issue here, before validation, so it travels the same
	// path as one the model reported under "issues".
	p.adoptLineComments(jsonContent, &result)
	// The example formerly showed iso_scores:{} even though the validator
	// treats every missing score as zero and rejects it. An empty object has
	// no scores to validate; normalize only that exact shape to absent.
	if result.ISOScores != nil {
		if fields == nil {
			_ = json.Unmarshal([]byte(jsonContent), &fields)
		}
		var isoFields map[string]json.RawMessage
		if raw, ok := fields["iso_scores"]; ok && json.Unmarshal(raw, &isoFields) == nil && len(isoFields) == 0 {
			result.ISOScores = nil
		}
	}

	// Validate
	if err := p.validateReviewResult(&result); err != nil {
		return p.degradedOrError(response, ParseErrorValidation, err)
	}
	if len(discarded) > 0 {
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["optional_sections_discarded"] = strings.Join(discarded, ",")
	}

	return &result, nil
}
