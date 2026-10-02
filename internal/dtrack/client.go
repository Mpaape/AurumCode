// Package dtrack is AUR-550's client for OWASP Dependency-Track v5's REST
// API. It uploads an already-generated SBOM (AUR-549's own job, never this
// package's), polls the server's processing token until the server itself
// says it is done, and reads one project's current metrics to compare
// against a gate's thresholds. It never administers a Dependency-Track
// project (create/rename/delete) -- that stays a human operation, per the
// card's own Non-goals.
//
// # Dependency-Track v5's ProjectMetrics field names
//
// GET /api/v1/metrics/project/{uuid}/current returns Dependency-Track's
// ProjectMetrics resource. This package reads exactly three of its fields,
// by the exact names Dependency-Track v5's own OpenAPI schema uses:
//
//   - "critical"              -- open, unsuppressed CRITICAL-severity
//     vulnerabilities, across every component of the project.
//   - "high"                  -- open, unsuppressed HIGH-severity
//     vulnerabilities, same scope.
//   - "policyViolationsTotal" -- every open, unsuppressed policy
//     violation regardless of its own state (FAIL/WARN/INFO) or type
//     (LICENSE/SECURITY/OPERATIONAL).
//
// Every one of the three decodes into a *int, never a plain int: a
// Dependency-Track response that omits a field (a metrics snapshot not yet
// computed right after BOM processing finishes, or a differently
// versioned server) must decode as "unknown", not as the silent zero a
// plain int would produce. A silent zero reads as "scanned clean" to a
// gate -- the single worst failure mode this card's own card text names
// ("resposta confiantemente errada") -- so EvaluateMetrics refuses to
// grade an incomplete response at all; the caller's gate turns that
// refusal into "inconclusive" per policy, never into an approval.
package dtrack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Sentinel errors a gate maps onto its own "inconclusive" semantics. They
// are wrapped (fmt.Errorf("...: %w", ErrX)) by the functions below, never
// returned bare, so errors.Is still matches while the message can add a
// status code or operation name -- never a response body or a secret.
var (
	// ErrUnreachable means the HTTP call itself never produced a response:
	// DNS failure, connection refused, TLS failure, context deadline.
	ErrUnreachable = errors.New("dtrack: server unreachable")
	// ErrHTTPStatus means a response arrived but its status code was not
	// the expected 2xx. See StatusError for the status code itself.
	ErrHTTPStatus = errors.New("dtrack: unexpected HTTP status")
	// ErrTimeout means PollUntilProcessed's own deadline (config
	// timeout_seconds) elapsed before the server ever reported
	// processing:false.
	ErrTimeout = errors.New("dtrack: bom processing did not finish before the configured timeout")
	// ErrMetricsIncomplete means the metrics response decoded but did not
	// carry all three required fields -- see EvaluateMetrics.
	ErrMetricsIncomplete = errors.New("dtrack: metrics response is missing a required field")
	// ErrInsecureHost means server_api_host failed AUR-550's own
	// HTTPS-only rule -- see ValidateHost.
	ErrInsecureHost = errors.New("dtrack: server_api_host must be https, or plain http on a loopback address only")
)

// StatusError carries the HTTP status code one call received. It never
// carries the response body: a Dependency-Track error page can echo
// request content (including a header value) back verbatim, so the body
// never becomes part of an error string that could reach stdout, stderr,
// the review body, the audit record or SARIF.
type StatusError struct {
	Op     string
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("dtrack: %s: unexpected http status %d", e.Op, e.Status)
}

func (e *StatusError) Unwrap() error { return ErrHTTPStatus }

// ValidateHost enforces AUR-550's HTTPS-only rule: server_api_host must be
// an absolute https:// URL, with one narrow exception -- plain http is
// accepted only when the host is a loopback IP literal (127.0.0.0/8 or
// ::1), never the name "localhost" (a name is resolved by whatever
// resolver/hosts file the process trusts, which this rule refuses to
// trust) and never any other hostname. That one exception exists only so
// this package's own tests (and a card's own acceptance script) can point
// it at an httptest.Server, which always serves plain HTTP on a loopback
// address; it is never a production escape hatch. Userinfo and a query
// string are both refused outright: neither has any legitimate role in a
// server_api_host value, and both are places a credential could hide.
func ValidateHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("%w: server_api_host is empty", ErrInsecureHost)
	}
	u, err := url.Parse(host)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%w: %q is not an absolute URL", ErrInsecureHost, host)
	}
	if u.User != nil {
		return fmt.Errorf("%w: userinfo is not allowed in server_api_host", ErrInsecureHost)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: a query string or fragment is not allowed in server_api_host", ErrInsecureHost)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		ip := net.ParseIP(u.Hostname())
		if ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("%w: plain http is only allowed for a loopback IP literal (127.0.0.0/8 or ::1), got %q", ErrInsecureHost, u.Hostname())
	default:
		return fmt.Errorf("%w: unsupported scheme %q", ErrInsecureHost, u.Scheme)
	}
}

// Clock and Sleeper are the two injectable seams that keep this package's
// own tests fast and deterministic: production code passes time.Now and
// time.Sleep; a test passes a fake clock that advances exactly when the
// fake sleeper is called, so a 180-second configured timeout never costs
// a test 180 real seconds.
type Clock func() time.Time
type Sleeper func(time.Duration)

// Client is a configured connection to one Dependency-Track v5 server.
// Host is validated (ValidateHost) once, by NewClient, never re-validated
// per call.
type Client struct {
	host       string
	apiKey     string
	httpClient *http.Client
	now        Clock
	sleep      Sleeper
}

// NewClient builds a Client against host, refusing an insecure host
// before a single byte is ever sent (ValidateHost). The returned client's
// http.Client never follows a redirect: Go's own http.Client strips the
// Authorization and Cookie headers on a cross-host redirect but NOT a
// custom header such as X-Api-Key, so the only safe rule is to never
// follow one at all -- a 3xx response is surfaced as its own HTTP status
// instead.
func NewClient(host, apiKey string) (*Client, error) {
	if err := ValidateHost(host); err != nil {
		return nil, err
	}
	return &Client{
		host:   strings.TrimRight(strings.TrimSpace(host), "/"),
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now:   time.Now,
		sleep: time.Sleep,
	}, nil
}

// WithClock overrides the clock/sleeper a test uses. Production code never
// calls this: NewClient's defaults (time.Now/time.Sleep) are already
// correct for it.
func (c *Client) WithClock(now Clock, sleep Sleeper) *Client {
	c.now = now
	c.sleep = sleep
	return c
}

// maxResponseBytes bounds every response body this package ever reads,
// independent of the redaction filter further up the call stack: a
// malicious or misconfigured server cannot make this client hold an
// unbounded body in memory just to extract a token or three integers.
const maxResponseBytes = 1 << 20

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	return req, nil
}

// uploadResponse is Dependency-Track v5's BomSubmitResponse: the one field
// this package needs is "token", the processing token PollUntilProcessed
// polls next.
type uploadResponse struct {
	Token string `json:"token"`
}

// Upload sends bom as a multipart POST to /api/v1/bom with fields
// "project" (the Dependency-Track project UUID) and "bom" (the SBOM file
// content), and returns the processing token Dependency-Track assigns the
// upload. The RFC's own guidance -- one Dependency-Track project per
// microservice, never the same BOM to two projects without unifying them
// first -- is the caller's responsibility (which project UUID it passes
// in); this function only performs the one upload it is told to.
func (c *Client) Upload(ctx context.Context, project string, bom []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("project", project); err != nil {
		return "", fmt.Errorf("dtrack: upload: building multipart body: %w", err)
	}
	fw, err := mw.CreateFormFile("bom", "bom.json")
	if err != nil {
		return "", fmt.Errorf("dtrack: upload: building multipart body: %w", err)
	}
	if _, err := fw.Write(bom); err != nil {
		return "", fmt.Errorf("dtrack: upload: building multipart body: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("dtrack: upload: building multipart body: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/bom", &buf)
	if err != nil {
		return "", fmt.Errorf("dtrack: upload: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: upload", ErrUnreachable)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return "", &StatusError{Op: "upload", Status: resp.StatusCode}
	}
	var out uploadResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("dtrack: upload: decoding response: %w", err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", fmt.Errorf("dtrack: upload: response carried no token")
	}
	return out.Token, nil
}

// bomStatusResponse is Dependency-Track v5's BomProcessingResponse:
// "processing" is true while the server is still analyzing the uploaded
// BOM. It decodes as *bool, never bool: a response that omits the field
// is read as "still processing" (the loop keeps polling, and a timeout
// still fires if it never resolves), never as the silent false that would
// let an unfinished scan read as finished.
type bomStatusResponse struct {
	Processing *bool `json:"processing"`
}

func (c *Client) bomProcessing(ctx context.Context, token string) (*bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/bom/token/"+url.PathEscape(token), nil)
	if err != nil {
		return nil, fmt.Errorf("dtrack: poll: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: poll", ErrUnreachable)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return nil, &StatusError{Op: "poll", Status: resp.StatusCode}
	}
	var out bomStatusResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return nil, fmt.Errorf("dtrack: poll: decoding response: %w", err)
	}
	return out.Processing, nil
}

// PollUntilProcessed polls GET /api/v1/bom/token/{token} on interval until
// the server reports processing:false, or until timeout elapses (measured
// from the injected clock, never wall-clock time directly), whichever
// comes first. ErrTimeout is returned, never a zero/approved result, when
// the deadline passes first -- the caller's gate turns that into
// "inconclusive", per the card's own AC-003.
func (c *Client) PollUntilProcessed(ctx context.Context, token string, interval, timeout time.Duration) error {
	deadline := c.now().Add(timeout)
	for {
		processing, err := c.bomProcessing(ctx, token)
		if err != nil {
			return err
		}
		if processing != nil && !*processing {
			return nil
		}
		if !c.now().Before(deadline) {
			return fmt.Errorf("%w", ErrTimeout)
		}
		c.sleep(interval)
		if !c.now().Before(deadline) {
			return fmt.Errorf("%w", ErrTimeout)
		}
	}
}

// ProjectMetrics is Dependency-Track v5's ProjectMetrics resource,
// trimmed to the three fields AUR-550's gate compares against its own
// thresholds. See the package doc for the exact field-name documentation.
// Every field is a pointer: nil means the server's response did not carry
// that field at all, which EvaluateMetrics refuses to treat as zero.
type ProjectMetrics struct {
	Critical              *int `json:"critical"`
	High                   *int `json:"high"`
	PolicyViolationsTotal   *int `json:"policyViolationsTotal"`
}

// ProjectMetrics reads GET /api/v1/metrics/project/{project}/current for
// the given Dependency-Track project UUID.
func (c *Client) ProjectMetrics(ctx context.Context, project string) (ProjectMetrics, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/metrics/project/"+url.PathEscape(project)+"/current", nil)
	if err != nil {
		return ProjectMetrics{}, fmt.Errorf("dtrack: metrics: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProjectMetrics{}, fmt.Errorf("%w: metrics", ErrUnreachable)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		return ProjectMetrics{}, &StatusError{Op: "metrics", Status: resp.StatusCode}
	}
	var out ProjectMetrics
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return ProjectMetrics{}, fmt.Errorf("dtrack: metrics: decoding response: %w", err)
	}
	return out, nil
}

// Thresholds is AUR-550's quality_gates.ssor_dtrack.thresholds section,
// already parsed (internal/config.DTrackThresholds mirrors this shape
// exactly; cmd/aurumcode converts one into the other so this package
// never imports internal/config).
type Thresholds struct {
	MaxCritical      int
	MaxHigh          int
	PolicyViolations int
}

// Evaluation is the result of comparing one ProjectMetrics reading
// against Thresholds.
type Evaluation struct {
	Breach                bool
	Reasons               []string
	Critical              int
	High                  int
	PolicyViolationsTotal int
}

// EvaluateMetrics compares m against t. It returns ErrMetricsIncomplete
// -- never a breach, never an approval -- when any of the three required
// fields is nil: an incomplete response is unknown, not clean, and a
// silent zero reading as "no vulnerabilities" is exactly the
// confidently-wrong failure this gate exists to prevent.
func EvaluateMetrics(m ProjectMetrics, t Thresholds) (Evaluation, error) {
	if m.Critical == nil || m.High == nil || m.PolicyViolationsTotal == nil {
		return Evaluation{}, ErrMetricsIncomplete
	}
	eval := Evaluation{
		Critical:              *m.Critical,
		High:                  *m.High,
		PolicyViolationsTotal: *m.PolicyViolationsTotal,
	}
	if eval.Critical > t.MaxCritical {
		eval.Breach = true
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("critical %d > max_critical %d", eval.Critical, t.MaxCritical))
	}
	if eval.High > t.MaxHigh {
		eval.Breach = true
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("high %d > max_high %d", eval.High, t.MaxHigh))
	}
	if eval.PolicyViolationsTotal > t.PolicyViolations {
		eval.Breach = true
		eval.Reasons = append(eval.Reasons, fmt.Sprintf("policy_violations %d > policy_violations %d", eval.PolicyViolationsTotal, t.PolicyViolations))
	}
	return eval, nil
}

// Outcome is Run's complete result: exactly one of Breach, Inconclusive or
// (both false) approved. Reasons never contains a response body, a raw
// error string, or a secret -- only stable tokens and the integers read
// from the metrics response -- so a caller can publish it verbatim to the
// review body, the audit record and SARIF without redacting it a second
// time.
type Outcome struct {
	Active              bool
	Breach              bool
	Inconclusive        bool
	InconclusiveReason  string
	Reasons             []string
	Critical            int
	High                int
	PolicyViolationsTotal int
}

// Reason tokens Run's Outcome.InconclusiveReason ever carries -- stable,
// non-server-authored strings a gate/audit/SARIF can publish safely.
const (
	ReasonUnreachable        = "dtrack_unreachable"
	ReasonHTTPError          = "dtrack_http_error"
	ReasonTimeout            = "dtrack_timeout"
	ReasonMetricsIncomplete  = "dtrack_metrics_incomplete"
)

// Run performs the full AUR-550 sequence against one already-built
// Client: upload bom to project, poll until the server finishes
// processing it (interval/timeout from config), then read and evaluate
// the project's current metrics against thresholds. Any failure along the
// way -- unreachable server, non-2xx HTTP status, polling timeout, an
// incomplete metrics response -- produces Outcome.Inconclusive with a
// stable reason, never a breach and never a silent approval.
func Run(ctx context.Context, client *Client, project string, bom []byte, thresholds Thresholds, pollInterval, timeout time.Duration) Outcome {
	out := Outcome{Active: true}

	token, err := client.Upload(ctx, project, bom)
	if err != nil {
		out.Inconclusive = true
		out.InconclusiveReason = reasonFor(err)
		return out
	}

	if err := client.PollUntilProcessed(ctx, token, pollInterval, timeout); err != nil {
		out.Inconclusive = true
		out.InconclusiveReason = reasonFor(err)
		return out
	}

	metrics, err := client.ProjectMetrics(ctx, project)
	if err != nil {
		out.Inconclusive = true
		out.InconclusiveReason = reasonFor(err)
		return out
	}

	eval, err := EvaluateMetrics(metrics, thresholds)
	if err != nil {
		out.Inconclusive = true
		out.InconclusiveReason = ReasonMetricsIncomplete
		return out
	}

	out.Breach = eval.Breach
	out.Reasons = eval.Reasons
	out.Critical = eval.Critical
	out.High = eval.High
	out.PolicyViolationsTotal = eval.PolicyViolationsTotal
	return out
}

func reasonFor(err error) string {
	switch {
	case errors.Is(err, ErrTimeout):
		return ReasonTimeout
	case errors.Is(err, ErrUnreachable):
		return ReasonUnreachable
	case errors.Is(err, ErrHTTPStatus):
		return ReasonHTTPError
	default:
		return ReasonHTTPError
	}
}
