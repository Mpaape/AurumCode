package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// supportedVersions are the MCP protocol revisions this server speaks,
// newest first. A client asking for one of them gets it back; any other
// request is answered with the newest.
var supportedVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Reasons of an inconclusive answer the server itself produces.
const (
	reasonTimeout     = "time_limit"
	reasonBusy        = "previous_review_still_running"
	reasonSessionFail = "session_error"
)

// DefaultTimeout bounds one tool call when the command declares no limit.
const DefaultTimeout = 10 * time.Minute

// Options configure a Server.
type Options struct {
	// Gateway answers every review and rules question.
	Gateway Gateway
	// Redactor is the process's redaction filter; every answer passes it.
	Redactor Redactor
	// Timeout bounds one tool call; zero takes DefaultTimeout.
	Timeout time.Duration
	// Version is the binary's version, reported in serverInfo.
	Version string
}

// Server is one MCP session over a pair of streams.
type Server struct {
	opts Options
	// busy holds a token while a gateway call runs; a call abandoned at its
	// time limit keeps it until it really ends, so two sessions never
	// overlap.
	busy chan struct{}
	// last is every finding of the latest review or gate answer, by id,
	// with the redactor that answer used.
	last         map[string]Finding
	lastRedactor Redactor
}

// New returns a server for opts.
func New(opts Options) *Server {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	return &Server{opts: opts, busy: make(chan struct{}, 1), last: map[string]Finding{}}
}

// Serve answers messages from in on out until in ends or ctx is done.
// Each line is one JSON-RPC message; nothing else is ever written to out.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	w := &lineWriter{w: out}
	sc := newLineScanner(in)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		req, perr := decodeRequest(line)
		if perr != nil {
			if err := w.write(response{JSONRPC: jsonrpcVersion, ID: json.RawMessage("null"), Error: s.redactError(perr)}); err != nil {
				return err
			}
			continue
		}
		result, rerr := s.handle(ctx, req)
		if req.notification() {
			continue
		}
		msg := response{JSONRPC: jsonrpcVersion, ID: req.ID, Result: result}
		if rerr != nil {
			msg = response{JSONRPC: jsonrpcVersion, ID: req.ID, Error: s.redactError(rerr)}
		}
		if err := w.write(msg); err != nil {
			return err
		}
	}
	return sc.Err()
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}

// handle dispatches one request.
func (s *Server) handle(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params)
	case "notifications/initialized", "notifications/cancelled":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolSpecs()}, nil
	case "tools/call":
		return s.callTool(ctx, req.Params)
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
}

// initialize negotiates the protocol revision and declares the tools
// capability.
func (s *Server) initialize(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, invalidParams("invalid initialize params")
		}
	}
	version := supportedVersions[0]
	for _, v := range supportedVersions {
		if v == p.ProtocolVersion {
			version = v
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "aurumcode", "title": "AurumCode", "version": s.opts.Version},
		"instructions":    "Call aurum_gate with the branch you will merge into as base before every commit or push. Fix what it reports; never disable, weaken or except a rule to make it pass. inconclusive is never a pass.",
	}, nil
}

// callParams is the tools/call envelope.
type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// callTool validates the arguments of the named tool before anything runs,
// then answers through the redaction choke point.
func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p callParams
	if err := decodeStrict(params, &p); err != nil {
		return nil, err
	}
	var (
		answer   any
		redactor Redactor
		rerr     *rpcError
	)
	switch p.Name {
	case ToolReview, ToolGate:
		answer, redactor, rerr = s.review(ctx, p)
	case ToolRules:
		answer, rerr = s.rules(ctx, p.Arguments)
	case ToolExplain:
		answer, redactor, rerr = s.explain(p.Arguments)
	default:
		rerr = invalidParams("unknown tool: " + p.Name)
	}
	if rerr != nil {
		return nil, rerr
	}
	return toolResult(redactValue(answer, s.opts.Redactor, redactor)), nil
}

// toolResult wraps a redacted answer as MCP tool content: the same JSON as
// text and as structured content.
func toolResult(answer any) map[string]any {
	text, err := json.MarshalIndent(answer, "", "  ")
	if err != nil {
		text = []byte(`"response could not be encoded"`)
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": answer,
		"isError":           false,
	}
}

// review runs aurum_review or aurum_gate through the gateway.
func (s *Server) review(ctx context.Context, p callParams) (any, Redactor, *rpcError) {
	args, rerr := parseBaseArgs(p.Arguments)
	if rerr != nil {
		return nil, nil, rerr
	}
	outcome, err := s.runGateway(ctx, func(c context.Context) (SessionOutcome, error) {
		return s.opts.Gateway.Review(c, ReviewRequest{Base: args.Base})
	})
	if err != nil {
		return failedAnswer(reasonOf(err)), nil, nil
	}
	gate := answerFor(outcome)
	s.remember(gate, outcome.Redactor)
	if p.Name == ToolGate {
		return gate, outcome.Redactor, nil
	}
	return reviewAnswer{gateAnswer: gate, Report: outcome.Report, Diagnostics: outcome.Diagnostics}, outcome.Redactor, nil
}

// remember keeps the findings of the latest answer for aurum_explain.
func (s *Server) remember(a gateAnswer, r Redactor) {
	s.last = map[string]Finding{}
	for _, f := range append(append([]Finding{}, a.Findings...), a.Blocking...) {
		s.last[f.ID] = f
	}
	s.lastRedactor = r
}

// rules answers aurum_rules.
func (s *Server) rules(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	args, rerr := parseRulesArgs(raw)
	if rerr != nil {
		return nil, rerr
	}
	c, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	set, err := s.opts.Gateway.Rules(c, args.Paths)
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: "rules unavailable: " + err.Error()}
	}
	return set, nil
}

// explain answers aurum_explain from the latest answer of this session.
func (s *Server) explain(raw json.RawMessage) (any, Redactor, *rpcError) {
	args, rerr := parseExplainArgs(raw)
	if rerr != nil {
		return nil, nil, rerr
	}
	f, ok := s.last[args.FindingID]
	if !ok {
		return nil, nil, invalidParams("unknown finding_id: call aurum_review or aurum_gate first and use an id it returned")
	}
	return map[string]any{
		"finding": f,
		"how_to_fix": fmt.Sprintf("Rule %s (%s) at %s:%d. Apply the suggestion or remove the cause in the code, then call aurum_gate again; an exception or a disabled rule is a policy decision for a human, not a fix.",
			f.RuleID, f.Origin, f.File, f.Line),
	}, s.lastRedactor, nil
}

// errTimeout and errBusy are the gateway calls that did not finish.
var (
	errTimeout = errors.New(reasonTimeout)
	errBusy    = errors.New(reasonBusy)
)

// runGateway runs call under the time limit, one at a time.
func (s *Server) runGateway(ctx context.Context, call func(context.Context) (SessionOutcome, error)) (SessionOutcome, error) {
	// cancel runs only when runGateway returns: cancelling inside the
	// goroutine would make c.Done() ready next to an already delivered
	// result, and select would then report a finished call as a timeout.
	c, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	select {
	case s.busy <- struct{}{}:
	case <-c.Done():
		return SessionOutcome{}, errBusy
	}
	type result struct {
		o   SessionOutcome
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-s.busy }()
		o, err := call(c)
		done <- result{o, err}
	}()
	select {
	case r := <-done:
		return r.o, r.err
	case <-c.Done():
		return SessionOutcome{}, errTimeout
	}
}

// reasonOf names a gateway failure; it is always inconclusive.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, errTimeout):
		return reasonTimeout
	case errors.Is(err, errBusy):
		return reasonBusy
	}
	return reasonSessionFail + ": " + err.Error()
}
