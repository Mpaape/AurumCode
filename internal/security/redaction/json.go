package redaction

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// reJSONSecretKey is a JSON object key whose value is a secret whatever it
// looks like: the same names the key=value rules recognize.
var reJSONSecretKey = regexp.MustCompile(`(?i)^(?:` + secretKeyNames + `|` + authHeaderNames + `)$`)

// RedactJSON redacts a serialized JSON document and always returns valid
// JSON when data is valid. It first redacts the document whole, as Redact
// does (a registered secret is found however JSON escaped it); when that
// result is still valid JSON it is returned. A rule can end its match inside
// an escape sequence (a bare value that swallows the `\` of `\"`), which
// would break the document; then every string literal is redacted on its
// own, decoded, and encoded back, and the value of a secret-named key is
// replaced whole, so the key/value rules spanning two strings still hold.
func (f *Filter) RedactJSON(data []byte) []byte {
	whole := f.Redact(string(data))
	if !json.Valid(data) || json.Valid([]byte(whole)) {
		return []byte(whole)
	}
	return f.redactJSONStrings(data)
}

// redactJSONStrings rewrites each string literal of a valid JSON document.
func (f *Filter) redactJSONStrings(data []byte) []byte {
	var out bytes.Buffer
	lastKey := ""
	for i := 0; i < len(data); {
		if data[i] != '"' {
			out.WriteByte(data[i])
			i++
			continue
		}
		end := stringEnd(data, i)
		var value string
		if err := json.Unmarshal(data[i:end], &value); err != nil {
			out.Write(data[i:end])
			i = end
			continue
		}
		isKey := nextIsColon(data, end)
		switch {
		case isKey:
			lastKey = value
			value = f.Redact(value)
		case reJSONSecretKey.MatchString(lastKey):
			value = Marker
			lastKey = ""
		default:
			value = f.Redact(value)
			lastKey = ""
		}
		out.Write(encodeJSONString(value))
		i = end
	}
	return out.Bytes()
}

// stringEnd is the index just past the closing quote of the string literal
// that opens at start.
func stringEnd(data []byte, start int) int {
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(data)
}

// nextIsColon reports whether the next non-space byte after i is a colon:
// the string just read is an object key.
func nextIsColon(data []byte, i int) bool {
	for ; i < len(data); i++ {
		switch data[i] {
		case ' ', '\t', '\r', '\n':
			continue
		case ':':
			return true
		default:
			return false
		}
	}
	return false
}

// encodeJSONString encodes s as a JSON string literal without HTML escaping.
func encodeJSONString(s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimRight(buf.Bytes(), "\n")
}
