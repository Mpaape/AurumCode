package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ParseErrorKind classifies why ParseReviewResponse could not produce a
// structured result. It exists so a caller (or an operator reading logs) can
// tell "the model said something, but not JSON we could use" apart from
// every other kind of failure, instead of matching on an error string.
type ParseErrorKind string

// ParseError is the typed, clear failure ParseReviewResponse returns instead
// of silently degrading to an empty result. See the "Parser fallback" note
// in docs/specs/AUR-430.md for the reasoning: a caller (or a human reading
// the review) must be able to tell "no problems found" apart from "the
// engine could not understand the model," and a bare error string forces
// every caller back to substring matching to make that distinction.
type ParseError struct {
	Kind ParseErrorKind
	// Diagnostics contain no model-authored text. They distinguish truncation
	// and malformed upstream JSON from extraction/parser defects in CI logs.
	InputBytes     int
	RawJSONValid   bool
	SyntaxOffset   int64
	FinishReason   string
	TypeField      string
	ExpectedType   string
	ActualType     string
	ValidationCode string
	// Raw is the response that failed to parse, bounded to a safe preview
	// length so a large or adversarial response cannot balloon an error
	// message that ends up in logs.
	Raw string
	Err error
}

const parseErrorRawPreviewLimit = 512

func newParseError(kind ParseErrorKind, raw string, err error) *ParseError {
	inputBytes := len(raw)
	rawJSONValid := json.Valid([]byte(strings.TrimSpace(raw)))
	var syntaxErr *json.SyntaxError
	var syntaxOffset int64
	if errors.As(err, &syntaxErr) {
		syntaxOffset = syntaxErr.Offset
	}
	if len(raw) > parseErrorRawPreviewLimit {
		raw = raw[:parseErrorRawPreviewLimit] + "... (truncated)"
	}
	parseErr := &ParseError{Kind: kind, Raw: raw, Err: err, InputBytes: inputBytes, RawJSONValid: rawJSONValid, SyntaxOffset: syntaxOffset}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		parseErr.TypeField = typeErr.Field
		parseErr.ExpectedType = typeErr.Type.String()
		parseErr.ActualType = typeErr.Value
	}
	if kind == ParseErrorValidation && err != nil {
		switch message := err.Error(); {
		case strings.HasPrefix(message, "invalid verdict"):
			parseErr.ValidationCode = "invalid_verdict"
		case strings.Contains(message, "missing file path"):
			parseErr.ValidationCode = "issue_missing_file"
		case strings.Contains(message, "missing severity"):
			parseErr.ValidationCode = "issue_missing_severity"
		case strings.Contains(message, "missing message"):
			parseErr.ValidationCode = "issue_missing_message"
		case strings.Contains(message, "invalid severity"):
			parseErr.ValidationCode = "issue_invalid_severity"
		case strings.HasPrefix(message, "ISO score"):
			parseErr.ValidationCode = "iso_score_out_of_range"
		default:
			parseErr.ValidationCode = "other"
		}
	}
	return parseErr
}

func (e *ParseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("prompt: %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("prompt: %s", e.Kind)
}

func (e *ParseError) Unwrap() error { return e.Err }

const (
	// ParseErrorNoJSON means no JSON object could be located in the
	// response at all (no code fence, no bare `{...}`).
	ParseErrorNoJSON ParseErrorKind = "no_json_found"
	// ParseErrorInvalidJSON means a JSON-shaped span was found but it did
	// not decode even after repairJSON's best-effort cleanup.
	ParseErrorInvalidJSON ParseErrorKind = "invalid_json"
	// ParseErrorValidation means the JSON decoded but failed structural
	// validation (missing required issue fields, out-of-range scores).
	ParseErrorValidation ParseErrorKind = "validation_failed"
	// ParseErrorNoFindings means neither the JSON path nor the degraded
	// freeform-text fallback (see degradedExtract) could recover a single
	// finding from the response.
	ParseErrorNoFindings ParseErrorKind = "no_findings_recovered"
)
