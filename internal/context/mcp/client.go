package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ProtocolVersion is the MCP revision the client asks for.
const ProtocolVersion = "2025-06-18"

// maxLineBytes bounds one JSON-RPC message read from the server.
const maxLineBytes = 1 << 20

// Conn is one live connection to a server: newline-delimited JSON-RPC over
// a writer and a reader. Close ends the server.
type Conn interface {
	io.Reader
	io.Writer
	Close() error
}

// ErrMalformed marks an answer that is not the MCP shape the client asked for.
var ErrMalformed = errors.New("resposta MCP malformada")

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// toolResult is the tools/call result: text content items and isError.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// session drives one conversation on conn.
type session struct {
	conn   Conn
	lines  *bufio.Scanner
	nextID int
}

func newSession(conn Conn) *session {
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), maxLineBytes)
	return &session{conn: conn, lines: sc}
}

// callTool initializes the session and calls tool with args, returning the
// tool's joined text content.
func callTool(ctx context.Context, conn Conn, tool string, args map[string]any) (string, error) {
	s := newSession(conn)
	init := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "aurumcode", "version": "context-source"},
	}
	if _, err := s.request(ctx, "initialize", init); err != nil {
		return "", err
	}
	if err := s.send(rpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"}); err != nil {
		return "", err
	}
	raw, err := s.request(ctx, "tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return "", err
	}
	var res toolResult
	if err := json.Unmarshal(raw, &res); err != nil || res.Content == nil {
		return "", fmt.Errorf("%w: tools/call sem content", ErrMalformed)
	}
	var parts []string
	for _, c := range res.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	text := strings.Join(parts, "\n")
	if res.IsError {
		return "", fmt.Errorf("a ferramenta MCP %q respondeu com erro: %s", tool, text)
	}
	return text, nil
}

// request sends one call and reads lines until its response arrives;
// notifications and other ids from the server are skipped.
func (s *session) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.nextID++
	id := s.nextID
	if err := s.send(rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	for s.lines.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var resp rpcResponse
		if err := json.Unmarshal(s.lines.Bytes(), &resp); err != nil {
			return nil, fmt.Errorf("%w: %s não é JSON-RPC", ErrMalformed, method)
		}
		if resp.ID == nil || *resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: erro %d do servidor MCP: %s", method, resp.Error.Code, resp.Error.Message)
		}
		if len(resp.Result) == 0 {
			return nil, fmt.Errorf("%w: %s sem result", ErrMalformed, method)
		}
		return resp.Result, nil
	}
	if err := s.lines.Err(); err != nil {
		return nil, fmt.Errorf("lendo o servidor MCP: %w", err)
	}
	return nil, fmt.Errorf("o servidor MCP encerrou antes de responder %s", method)
}

func (s *session) send(req rpcRequest) error {
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = s.conn.Write(append(line, '\n'))
	return err
}
