package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer answers on one end of a pipe: initialize, then tools/call
// with reply (raw JSON result) or raw lines when garbage is set. It records
// the tools/call arguments it received.
type fakeServer struct {
	reply   string
	garbage string
	silent  bool
	mu      sync.Mutex
	args    map[string]any
	tool    string
}

func (f *fakeServer) dial(context.Context) (Conn, error) {
	client, server := net.Pipe()
	go f.serve(server)
	return client, nil
}

func (f *fakeServer) serve(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var req struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		var result string
		switch req.Method {
		case "initialize":
			result = `{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"fake"}}`
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			f.mu.Lock()
			f.tool, f.args = p.Name, p.Arguments
			f.mu.Unlock()
			if f.silent {
				time.Sleep(time.Second)
				return
			}
			if f.garbage != "" {
				_, _ = conn.Write([]byte(f.garbage + "\n"))
				return
			}
			result = f.reply
		}
		line, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": json.RawMessage(result)})
		if _, err := conn.Write(append(line, '\n')); err != nil {
			return
		}
	}
}

func textReply(text string) string {
	b, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": text}}})
	return string(b)
}

// canary is assembled at run time so no credential-shaped literal is ever
// committed.
func canary() string { return "mcp" + "-canary-" + "469" + "-xyz" }

func redactCanary(s string) string { return strings.ReplaceAll(s, canary(), "[REDACTED]") }

// AC-001: only the declared payload, redacted, reaches the server; the
// answer comes back with the source as its origin.
func TestAUR469DeclaredRedactedPayloadAndOrigin(t *testing.T) {
	srv := &fakeServer{reply: textReply("ADR-12: pagamentos usam centavos")}
	src := &Source{SourceName: "adr", Tool: "lookup", Arguments: map[string]string{"scope": "pagamentos " + canary()}, Send: []string{SendChangedPaths}, Timeout: time.Second, Dial: srv.dial, Redact: redactCanary}
	text, err := src.Provide(context.Background(), []string{"pay/" + canary() + ".go", "pay/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "ADR-12: pagamentos usam centavos" || src.Name() != "mcp:adr/lookup" {
		t.Fatalf("text=%q origin=%q", text, src.Name())
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.tool != "lookup" || len(srv.args) != 2 {
		t.Fatalf("server received tool=%q args=%v, want only scope and changed_paths", srv.tool, srv.args)
	}
	sent, _ := json.Marshal(srv.args)
	if strings.Contains(string(sent), canary()) || !strings.Contains(string(sent), "[REDACTED]") {
		t.Fatalf("the payload was not redacted: %s", sent)
	}
}

// AC-001: without a declared send, no path leaves; without a redaction
// filter, nothing is sent at all.
func TestAUR469NothingUndeclaredIsSent(t *testing.T) {
	srv := &fakeServer{reply: textReply("ok")}
	src := &Source{SourceName: "adr", Tool: "lookup", Timeout: time.Second, Dial: srv.dial, Redact: redactCanary}
	if _, err := src.Provide(context.Background(), []string{"secret/path.go"}); err != nil {
		t.Fatal(err)
	}
	if len(srv.args) != 0 {
		t.Fatalf("undeclared payload sent: %v", srv.args)
	}
	unredacted := &Source{SourceName: "adr", Tool: "lookup", Timeout: time.Second, Dial: srv.dial}
	if _, err := unredacted.Provide(context.Background(), nil); err == nil {
		t.Fatal("a source without a redaction filter sent its payload")
	}
}

// AC-002: an absent, slow or malformed server is an error (the caller's
// omission warning), never a hang or a partial answer.
func TestAUR469AbsentSlowAndMalformedServersAreErrors(t *testing.T) {
	cases := map[string]*Source{
		"absent":     {Dial: func(context.Context) (Conn, error) { return nil, errors.New("exec: not found") }},
		"slow":       {Dial: (&fakeServer{silent: true}).dial},
		"garbage":    {Dial: (&fakeServer{garbage: "not json"}).dial},
		"no content": {Dial: (&fakeServer{reply: `{"other":1}`}).dial},
		"tool error": {Dial: (&fakeServer{reply: `{"content":[{"type":"text","text":"boom"}],"isError":true}`}).dial},
		"too large":  {Dial: (&fakeServer{reply: textReply(strings.Repeat("x", 100))}).dial, MaxBytes: 10},
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			src.SourceName, src.Tool, src.Timeout, src.Redact = "adr", "lookup", 100*time.Millisecond, redactCanary
			started := time.Now()
			text, err := src.Provide(context.Background(), nil)
			if err == nil || text != "" {
				t.Fatalf("text=%q err=%v, want an error and no text", text, err)
			}
			if time.Since(started) > 2*time.Second {
				t.Fatalf("the source took %s, over its timeout", time.Since(started))
			}
		})
	}
}
