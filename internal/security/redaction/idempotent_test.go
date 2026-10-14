package redaction

import (
	"strings"
	"testing"
)

// Masking is idempotent: a value already masked is not masked again, which
// left "[REDACTED]]" in documents redacted in several passes; a real value
// is still masked.
func TestRedactIsIdempotent(t *testing.T) {
	f := NewFilter()
	for _, in := range []string{"secret: abc123", "token=s3cr3tv4lue and more", "(rule x-secret: [REDACTED] Secrets)",
		`{"text": "Possível segredo (rule security/hardcoded-secret: Hardcoded Secrets)"}`} {
		once := f.Redact(in)
		if twice := f.Redact(once); twice != once {
			t.Errorf("Redact(Redact(%q)) = %q, want %q", in, twice, once)
		}
	}
	if got := f.Redact("secret: abc123"); got != "secret: "+Marker {
		t.Errorf("a real value was not masked: %q", got)
	}
	if got := f.Redact("token=[REDACTED]hunter2longsecret"); strings.Contains(got, "hunter2") {
		t.Errorf("a value that only starts with the marker leaked: %q", got)
	}
}
