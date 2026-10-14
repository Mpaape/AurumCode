package redaction

import "testing"

// Masking is idempotent: a value already masked is not masked again, which
// left "[REDACTED]]" in documents redacted in several passes; a real value
// is still masked.
func TestRedactIsIdempotent(t *testing.T) {
	f := NewFilter()
	for _, in := range []string{"secret: abc123", "token=s3cr3tv4lue and more", "(rule x-secret: [REDACTED] Secrets)"} {
		once := f.Redact(in)
		if twice := f.Redact(once); twice != once {
			t.Errorf("Redact(Redact(%q)) = %q, want %q", in, twice, once)
		}
	}
	if got := f.Redact("secret: abc123"); got != "secret: "+Marker {
		t.Errorf("a real value was not masked: %q", got)
	}
}
