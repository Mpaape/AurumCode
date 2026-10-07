package mcpserver

// The JSON-RPC 2.0 subset the MCP stdio transport needs: one message per
// line on stdin, one response per line on stdout, notifications (no id)
// never answered. This is the only file that knows the wire format; the
// decision to implement it here instead of importing an SDK keeps the
// binary free of a new dependency (see docs/specs for the trade-off).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

const jsonrpcVersion = "2.0"

// JSON-RPC error codes the server answers with.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// maxMessageBytes bounds one inbound line; a longer message ends the
// session with an error instead of being buffered without limit.
const maxMessageBytes = 1 << 20

// request is one inbound message. A missing id makes it a notification.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r request) notification() bool { return len(r.ID) == 0 }

// response is one outbound message: exactly one of Result and Error.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a protocol-level failure.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

func invalidParams(msg string) *rpcError { return &rpcError{Code: codeInvalidParams, Message: msg} }

// newLineScanner yields one raw message per line, bounded by
// maxMessageBytes.
func newLineScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	return sc
}

// lineWriter writes one JSON message per line, serialized.
type lineWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lineWriter) write(msg response) error {
	data, err := json.Marshal(msg)
	if err != nil {
		data, _ = json.Marshal(response{JSONRPC: jsonrpcVersion, ID: msg.ID, Error: &rpcError{Code: codeInternalError, Message: "response could not be encoded"}})
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.w.Write(append(data, '\n'))
	return err
}

// decodeRequest parses one line. A message that is not a single JSON-RPC
// 2.0 request object yields the error to answer with.
func decodeRequest(line []byte) (request, *rpcError) {
	var req request
	line = bytes.TrimSpace(line)
	if err := json.Unmarshal(line, &req); err != nil {
		if len(line) > 0 && line[0] == '[' {
			return req, &rpcError{Code: codeInvalidRequest, Message: "batch requests are not supported"}
		}
		return req, &rpcError{Code: codeParseError, Message: "invalid JSON"}
	}
	if req.JSONRPC != jsonrpcVersion || req.Method == "" {
		return req, &rpcError{Code: codeInvalidRequest, Message: "not a JSON-RPC 2.0 request"}
	}
	return req, nil
}
