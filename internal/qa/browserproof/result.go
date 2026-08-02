package browserproof

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SchemaBrowserProofResultV1 identifies the only verdict contract this package
// publishes. A verdict without it is not a browser proof.
const SchemaBrowserProofResultV1 = "BrowserProofResultV1"

// Limits declared by AUR-423/AC-001. A request past any of them is refused as
// harness diagnosis instead of being trimmed silently.
const (
	MaxRoutes             = 8
	MaxPageBytes          = 4 << 20
	MaxNavigationDeadline = 30 * time.Second
)

// Refusal codes. CodeTargetAbsent, CodeUnreachableRoute, CodeEmptyRender and
// CodeTextMismatch describe the artifact under test. Every other code describes
// the harness or its environment and is reported as an inconclusive diagnosis,
// never as a functional failure and never as proof.
const (
	CodeTargetAbsent     = "BROWSERPROOF_TARGET_ABSENT"
	CodeUnreachableRoute = "BROWSERPROOF_UNREACHABLE_ROUTE"
	CodeEmptyRender      = "BROWSERPROOF_EMPTY_RENDER"
	CodeTextMismatch     = "BROWSERPROOF_TEXT_MISMATCH"

	CodeDriverMismatch         = "BROWSERPROOF_DRIVER_MISMATCH"
	CodeDriverAbsent           = "BROWSERPROOF_DRIVER_ABSENT"
	CodeDriverFault            = "BROWSERPROOF_DRIVER_FAULT"
	CodeSelectorAbsent         = "BROWSERPROOF_SELECTOR_ABSENT"
	CodeServerUnavailable      = "BROWSERPROOF_SERVER_UNAVAILABLE"
	CodeNavigationTimeout      = "BROWSERPROOF_NAVIGATION_TIMEOUT"
	CodeObservationUnsupported = "BROWSERPROOF_OBSERVATION_UNSUPPORTED"
	CodeJavaScriptUnsupported  = "BROWSERPROOF_JAVASCRIPT_UNSUPPORTED"
	CodeRequestInvalid         = "BROWSERPROOF_REQUEST_INVALID"
	CodeVerdictRejected        = "BROWSERPROOF_VERDICT_REJECTED"
)

// Outcome separates a verdict about the artifact from a verdict about the
// harness. Only OutcomeProved may be read as evidence that a feature works.
type Outcome string

const (
	OutcomeProved       Outcome = "proved"
	OutcomeRefused      Outcome = "refused"
	OutcomeInconclusive Outcome = "inconclusive"
)

// Per-route assertion results.
const (
	AssertionPass   = "pass"
	AssertionFail   = "fail"
	AssertionNotRun = "not-run"
)

var (
	// ErrUnsubstantiatedProof rejects a verdict that claims proof without an
	// observed selector and the text extracted from it.
	ErrUnsubstantiatedProof = errors.New("browserproof: proved verdict without an observed selector and extracted text")

	// ErrEvidenceNotSanitized rejects raw markup smuggled into the verdict.
	ErrEvidenceNotSanitized = errors.New("browserproof: verdict carries raw markup")

	// ErrDriverAbsent reports a browser driver that is not installed. It is an
	// environment diagnosis, never a functional failure.
	ErrDriverAbsent = errors.New("browserproof: browser driver is absent")
)

var markupPattern = regexp.MustCompile(`<[a-zA-Z/!?]`)

// RouteObservationV1 is what the browser reported for one declared route. It
// carries the selector, the extracted text and the per-assertion result, and it
// never carries raw HTML or a screenshot.
type RouteObservationV1 struct {
	Route         string `json:"route"`
	Status        int    `json:"status"`
	Selector      string `json:"selector"`
	ExpectedText  string `json:"expected_text"`
	ObservedText  string `json:"observed_text"`
	SelectorFound bool   `json:"selector_found"`
	Reachable     bool   `json:"reachable"`
	Assertion     string `json:"assertion"`
	Code          string `json:"code,omitempty"`
	ServedBytes   int64  `json:"served_bytes"`
	ServedDigest  string `json:"served_digest,omitempty"`
	StableAfterMs int64  `json:"stable_after_ms"`
}

// BrowserProofResultV1 is the published verdict contract of this package.
type BrowserProofResultV1 struct {
	Schema         string               `json:"schema"`
	Card           string               `json:"card,omitempty"`
	Outcome        Outcome              `json:"outcome"`
	Proved         bool                 `json:"proved"`
	Code           string               `json:"code,omitempty"`
	Detail         string               `json:"detail,omitempty"`
	ArtifactDigest string               `json:"artifact_digest"`
	DriverDigest   string               `json:"driver_digest"`
	DriverKind     string               `json:"driver_kind"`
	DriverRunsJS   bool                 `json:"driver_runs_javascript"`
	EntryRoute     string               `json:"entry_route"`
	DeadlineMs     int64                `json:"navigation_deadline_ms"`
	Routes         []RouteObservationV1 `json:"routes"`
}

// Validate rejects any verdict that does not carry the evidence it claims. A
// caller must run it before trusting Proved.
func (r BrowserProofResultV1) Validate() error {
	if r.Schema != SchemaBrowserProofResultV1 {
		return fmt.Errorf("browserproof: schema %q is not %q", r.Schema, SchemaBrowserProofResultV1)
	}

	switch r.Outcome {
	case OutcomeProved, OutcomeRefused, OutcomeInconclusive:
	default:
		return fmt.Errorf("browserproof: unknown outcome %q", r.Outcome)
	}

	if r.Proved != (r.Outcome == OutcomeProved) {
		return fmt.Errorf("browserproof: proved=%v contradicts outcome %q", r.Proved, r.Outcome)
	}

	for i, route := range r.Routes {
		if route.Route == "" {
			return fmt.Errorf("browserproof: route %d has no path", i)
		}
		if markupPattern.MatchString(route.ObservedText) {
			return fmt.Errorf("%w: route %q", ErrEvidenceNotSanitized, route.Route)
		}
		switch route.Assertion {
		case AssertionPass, AssertionFail, AssertionNotRun:
		default:
			return fmt.Errorf("browserproof: route %q has unknown assertion %q", route.Route, route.Assertion)
		}
	}

	if !r.Proved {
		if r.Code == "" {
			return errors.New("browserproof: a verdict that is not proved must carry a refusal code")
		}
		return nil
	}

	if r.Code != "" {
		return fmt.Errorf("browserproof: proved verdict carries refusal code %q", r.Code)
	}
	if r.ArtifactDigest == "" || r.DriverDigest == "" || r.DriverKind == "" {
		return errors.New("browserproof: proved verdict must record the artifact and driver identity")
	}
	if len(r.Routes) == 0 {
		return fmt.Errorf("%w: no route was observed", ErrUnsubstantiatedProof)
	}

	for _, route := range r.Routes {
		if route.Selector == "" || strings.TrimSpace(route.ObservedText) == "" {
			return fmt.Errorf("%w: route %q", ErrUnsubstantiatedProof, route.Route)
		}
		if route.Assertion != AssertionPass {
			return fmt.Errorf("browserproof: proved verdict has route %q with assertion %q", route.Route, route.Assertion)
		}
		if !route.SelectorFound || !route.Reachable || route.Status != 200 {
			return fmt.Errorf("browserproof: proved verdict has unobserved route %q", route.Route)
		}
		if route.ExpectedText != "" && !strings.Contains(route.ObservedText, route.ExpectedText) {
			return fmt.Errorf("browserproof: route %q extracted %q, which does not contain %q",
				route.Route, route.ObservedText, route.ExpectedText)
		}
	}

	return nil
}

// ParseResultV1 decodes a verdict and refuses anything it cannot substantiate,
// including unknown fields such as an embedded screenshot.
func ParseResultV1(data []byte) (BrowserProofResultV1, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var result BrowserProofResultV1
	if err := decoder.Decode(&result); err != nil {
		return BrowserProofResultV1{}, fmt.Errorf("browserproof: decode verdict: %w", err)
	}
	if err := result.Validate(); err != nil {
		return BrowserProofResultV1{}, err
	}

	return result, nil
}

// ProofError is returned by Run for every outcome that is not proof, so a caller
// that ignores the verdict still cannot read a refusal as success.
type ProofError struct {
	Card    string
	Code    string
	Outcome Outcome
	Detail  string
}

func (e *ProofError) Error() string {
	card := e.Card
	if card == "" {
		card = "browserproof"
	}
	if e.Detail == "" {
		return fmt.Sprintf("%s: %s (%s)", card, e.Code, e.Outcome)
	}
	return fmt.Sprintf("%s: %s (%s): %s", card, e.Code, e.Outcome, e.Detail)
}

// outcomeOfCode maps a refusal code to the verdict class it belongs to.
func outcomeOfCode(code string) Outcome {
	switch code {
	case "":
		return OutcomeProved
	case CodeTargetAbsent, CodeUnreachableRoute, CodeEmptyRender, CodeTextMismatch:
		return OutcomeRefused
	default:
		return OutcomeInconclusive
	}
}
