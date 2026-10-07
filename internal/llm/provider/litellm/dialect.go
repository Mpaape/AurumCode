package litellm

import (
	"net/http"
	"net/url"
)

// Dialect is how one OpenAI-compatible Chat Completions endpoint differs
// from another on the wire: how the key travels, which path and query the
// request uses, which field carries the reply cap and which structured
// output mode the endpoint honours. The values come from the provider
// catalog (internal/llm/provider/profiles); this package only applies them.
type Dialect struct {
	// Name labels the provider in errors and is what Name() reports.
	Name string
	// Auth is how the API key is sent.
	Auth AuthStyle
	// AuthHeader is the header that carries the key when Auth is AuthHeaderKey.
	AuthHeader string
	// Path is appended to the base URL.
	Path string
	// Query is added to every request URL (for example an api-version).
	Query map[string]string
	// TokenField is the JSON field that carries the reply cap.
	TokenField TokenField
	// StructuredOutput is the response_format mode the endpoint honours.
	StructuredOutput StructuredOutput
}

// AuthStyle is how a dialect authenticates.
type AuthStyle string

// TokenField is the request field that caps the reply.
type TokenField string

// StructuredOutput is the response_format support of an endpoint.
type StructuredOutput string

const (
	// AuthBearer sends "Authorization: Bearer <key>".
	AuthBearer AuthStyle = "bearer"
	// AuthHeaderKey sends the raw key in Dialect.AuthHeader and never an
	// Authorization header.
	AuthHeaderKey AuthStyle = "header"
	// AuthNone sends no credential at all.
	AuthNone AuthStyle = "none"

	// TokenFieldMaxTokens is the classic "max_tokens" field.
	TokenFieldMaxTokens TokenField = "max_tokens"
	// TokenFieldMaxCompletionTokens is "max_completion_tokens", the only
	// cap reasoning models accept.
	TokenFieldMaxCompletionTokens TokenField = "max_completion_tokens"

	// StructuredJSONSchema sends the caller's JSON Schema when it gave one
	// and the JSON object mode otherwise.
	StructuredJSONSchema StructuredOutput = "json_schema"
	// StructuredJSONObject only ever sends the JSON object mode.
	StructuredJSONObject StructuredOutput = "json_object"
	// StructuredNone never sends response_format: the endpoint ignores or
	// refuses it, and the answer is held to the schema by the parser.
	StructuredNone StructuredOutput = "none"

	// defaultName is the name the provider has always reported.
	defaultName = "litellm"
	// chatCompletionsPath is the OpenAI Chat Completions path.
	chatCompletionsPath = "/chat/completions"
)

// DefaultDialect is the dialect this provider always spoke before the
// catalog existed: bearer key, /chat/completions, max_tokens and the JSON
// Schema form of response_format. Without a selected provider profile the
// request is byte for byte the one it always was.
func DefaultDialect() Dialect {
	return Dialect{
		Name:             defaultName,
		Auth:             AuthBearer,
		Path:             chatCompletionsPath,
		TokenField:       TokenFieldMaxTokens,
		StructuredOutput: StructuredJSONSchema,
	}
}

// requestURL is the base URL plus the dialect's path and query.
func (d Dialect) requestURL(baseURL string) (string, error) {
	raw := baseURL + d.Path
	if len(d.Query) == 0 {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range d.Query {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// authorize puts the key where the dialect expects it. A named header
// replaces the bearer form entirely: the key is never sent twice.
func (d Dialect) authorize(req *http.Request, apiKey string) {
	switch d.Auth {
	case AuthHeaderKey:
		req.Header.Set(d.AuthHeader, apiKey)
	case AuthNone:
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

// replyCap returns the two cap fields, exactly one of them set (or none
// when the caller set no cap).
func (d Dialect) replyCap(maxTokens int) (classic, completion int) {
	if d.TokenField == TokenFieldMaxCompletionTokens {
		return 0, maxTokens
	}
	return maxTokens, 0
}

// jsonModeFormat is the response_format of a JSON-mode request that carries
// no schema on the wire: the object mode, unless the endpoint takes none.
func (d Dialect) jsonModeFormat(jsonMode bool) *responseFormat {
	if !jsonMode || d.StructuredOutput == StructuredNone {
		return nil
	}
	return &responseFormat{Type: string(StructuredJSONObject)}
}
