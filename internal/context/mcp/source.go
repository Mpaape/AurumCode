package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SendChangedPaths is the one dynamic payload a source may declare: the
// changed paths of the review, under "changed_paths".
const SendChangedPaths = "changed_paths"

// DialFunc starts (or connects to) the source's server.
type DialFunc func(ctx context.Context) (Conn, error)

// Source is one configured MCP context source. It satisfies the
// config.ContextProvider seam (Name, Provide) structurally.
type Source struct {
	// SourceName is the configured name; the origin shown in the prompt is
	// mcp:<name>/<tool>.
	SourceName string
	// Tool is the one tool called.
	Tool string
	// Arguments are the declared static arguments.
	Arguments map[string]string
	// Send lists the declared dynamic payload (SendChangedPaths).
	Send []string
	// Timeout bounds one whole call (start, initialize, tools/call).
	Timeout time.Duration
	// Dial starts the server.
	Dial DialFunc
	// Redact is the AUR-009 filter applied to every value sent; nil sends
	// nothing (fail closed: an unredacted payload never leaves).
	Redact func(string) string
	// MaxBytes bounds the answer: a larger one is refused as an omission
	// (never truncated, never a failure of the whole review); 0 = no bound.
	MaxBytes int

	mu     sync.Mutex
	cached map[string]cachedAnswer
}

type cachedAnswer struct {
	text string
	err  error
}

// Name is the origin of what this source contributes.
func (s *Source) Name() string { return "mcp:" + s.SourceName + "/" + s.Tool }

// Payload is exactly what is sent as the tool's arguments: the declared
// static arguments and, when declared, the changed paths, every value
// redacted.
func (s *Source) Payload(changedPaths []string) (map[string]any, error) {
	if s.Redact == nil {
		return nil, fmt.Errorf("fonte MCP %q sem filtro de redação: nada é enviado", s.SourceName)
	}
	out := make(map[string]any, len(s.Arguments)+1)
	for k, v := range s.Arguments {
		out[k] = s.Redact(v)
	}
	for _, item := range s.Send {
		if item != SendChangedPaths {
			return nil, fmt.Errorf("fonte MCP %q: send %q não é suportado", s.SourceName, item)
		}
		paths := make([]string, 0, len(changedPaths))
		for _, p := range changedPaths {
			paths = append(paths, s.Redact(p))
		}
		sort.Strings(paths)
		out[SendChangedPaths] = paths
	}
	return out, nil
}

// Provide calls the tool once per set of changed paths (the answer is
// reused for the same set within one review, so the cache digest and the
// prompt see the same bytes). Any failure is an error the caller turns
// into an omission warning.
func (s *Source) Provide(ctx context.Context, changedPaths []string) (string, error) {
	key := strings.Join(changedPaths, "\x00")
	s.mu.Lock()
	if a, ok := s.cached[key]; ok {
		s.mu.Unlock()
		return a.text, a.err
	}
	s.mu.Unlock()
	text, err := s.call(ctx, changedPaths)
	s.mu.Lock()
	if s.cached == nil {
		s.cached = map[string]cachedAnswer{}
	}
	s.cached[key] = cachedAnswer{text: text, err: err}
	s.mu.Unlock()
	return text, err
}

func (s *Source) call(ctx context.Context, changedPaths []string) (string, error) {
	payload, err := s.Payload(changedPaths)
	if err != nil {
		return "", err
	}
	if s.Dial == nil {
		return "", fmt.Errorf("fonte MCP %q sem transporte", s.SourceName)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := s.Dial(cctx)
	if err != nil {
		return "", fmt.Errorf("servidor MCP %q indisponível: %w", s.SourceName, err)
	}
	type answer struct {
		text string
		err  error
	}
	ch := make(chan answer, 1)
	go func() {
		text, err := callTool(cctx, conn, s.Tool, payload)
		ch <- answer{text: text, err: err}
	}()
	select {
	case a := <-ch:
		_ = conn.Close()
		if a.err == nil && s.MaxBytes > 0 && len(a.text) > s.MaxBytes {
			return "", fmt.Errorf("%w: a fonte MCP %q devolveu %d bytes, acima de %d", ErrMalformed, s.SourceName, len(a.text), s.MaxBytes)
		}
		return a.text, a.err
	case <-cctx.Done():
		_ = conn.Close()
		return "", fmt.Errorf("servidor MCP %q não respondeu em %s", s.SourceName, timeout)
	}
}
