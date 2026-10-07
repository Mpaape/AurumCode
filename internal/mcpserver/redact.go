package mcpserver

import "encoding/json"

// redactValue is the one choke point every tool answer passes before it is
// written: v is encoded, every string in it (keys excluded) is redacted by
// each redactor in order, and the redacted tree is returned. A value that
// cannot be encoded is replaced by an error string, never written raw.
func redactValue(v any, redactors ...Redactor) any {
	data, err := json.Marshal(v)
	if err != nil {
		return "response could not be encoded"
	}
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		return "response could not be encoded"
	}
	return walkRedact(tree, redactors)
}

func walkRedact(v any, redactors []Redactor) any {
	switch t := v.(type) {
	case string:
		for _, r := range redactors {
			if r != nil {
				t = r.Redact(t)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = walkRedact(t[i], redactors)
		}
		return t
	case map[string]any:
		for k, item := range t {
			t[k] = walkRedact(item, redactors)
		}
		return t
	}
	return v
}

// redactError passes a protocol error through the same redaction as every
// tool answer: an error may quote a configuration or skill file, or an
// argument, and nothing reaches stdout unredacted.
func (s *Server) redactError(e *rpcError) *rpcError {
	if e == nil {
		return nil
	}
	msg, _ := redactValue(e.Message, s.opts.Redactor, s.lastRedactor).(string)
	return &rpcError{Code: e.Code, Message: msg}
}
