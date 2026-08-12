//go:build aur009_redaction

package unit

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	redaction "github.com/Mpaape/AurumCode/internal/security/redaction"
)

// TestAUR009 exercises the single redaction filter directly: structural
// rules, registered secret values, typed refusals and the line-buffered sink
// writer. Failure messages never print raw filter output; they print lengths
// and rule names so a leaking build cannot leak again through the test log.
func TestAUR009(t *testing.T) {
	const canary = "aur009-unit-canary-3f9d2c"
	t.Setenv(redaction.CanaryEnv, canary)
	f := redaction.FromEnv()

	assertRedacted := func(rule, input, expected string) {
		t.Helper()
		got := f.Redact(input)
		if strings.Contains(got, canary) {
			t.Fatalf("%s: canary survived redaction (output length %d)", rule, len(got))
		}
		if got != expected {
			t.Fatalf("%s: redacted output diverged (got length %d, want length %d)", rule, len(got), len(expected))
		}
	}

	assertRedacted("query-string",
		"push https://svc.example.test/a?sig="+canary+"&x=1 done",
		"push https://svc.example.test/a?"+redaction.Marker+" done")
	assertRedacted("auth-header",
		"X-Api-Key: "+canary,
		"X-Api-Key: "+redaction.Marker)
	assertRedacted("key-value",
		"+db_password="+canary,
		"+db_password="+redaction.Marker)
	assertRedacted("registered-secret",
		"note "+canary+" end",
		"note "+redaction.Marker+" end")

	// Credential shapes are constructed at runtime so no committed byte is
	// itself credential-shaped.
	skToken := "sk-" + strings.Repeat("a", 24)
	if got := f.Redact("body " + skToken + " tail"); strings.Contains(got, skToken) {
		t.Fatal("credential-shape: sk token survived redaction")
	}
	akToken := "AKIA" + strings.Repeat("Q", 16)
	if got := f.Redact("body " + akToken + " tail"); strings.Contains(got, akToken) {
		t.Fatal("credential-shape: AKIA token survived redaction")
	}

	// Typed redacted error: the wrapped rendering is canary-free and typed.
	wrapped := f.WrapError(errors.New("credential rejected for " + canary + " on host db-primary"))
	var redactedErr *redaction.RedactedError
	if !errors.As(wrapped, &redactedErr) {
		t.Fatal("WrapError did not produce a typed *RedactedError")
	}
	if strings.Contains(wrapped.Error(), canary) {
		t.Fatal("WrapError leaked the canary into the error rendering")
	}
	if wrapped.Error() != "credential rejected for "+redaction.Marker+" on host db-primary" {
		t.Fatalf("WrapError diverged from the stable marker rendering (length %d)", len(wrapped.Error()))
	}
	if f.WrapError(nil) != nil {
		t.Fatal("WrapError(nil) must stay nil")
	}

	// The writer redacts complete lines even when the secret is split across
	// two writes.
	var buf bytes.Buffer
	w, err := f.NewWriter(redaction.SinkLog, &buf)
	if err != nil {
		t.Fatalf("NewWriter(log) refused a canonical sink: %v", err)
	}
	header := "Authorization: Bearer " + canary
	half := len(header) / 2
	if _, err := w.Write([]byte(header[:half])); err != nil {
		t.Fatalf("chunked write failed: %v", err)
	}
	if _, err := w.Write([]byte(header[half:] + "\n")); err != nil {
		t.Fatalf("chunked write failed: %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}
	if buf.String() != "Authorization: "+redaction.Marker+"\n" {
		t.Fatalf("chunked header write diverged (length %d, effects %d)", buf.Len(), w.Effects())
	}
	if w.Effects() != 1 {
		t.Fatalf("chunked header write recorded %d effects, want 1", w.Effects())
	}

	// Every canonical sink is constructible; anything else is a typed refusal
	// whose rendering is itself redacted.
	for _, sink := range redaction.Sinks() {
		if _, err := f.NewWriter(sink, io.Discard); err != nil {
			t.Fatalf("canonical sink %s was refused: %v", sink, err)
		}
	}
	if len(redaction.Sinks()) != 7 {
		t.Fatalf("expected 7 canonical sinks, found %d", len(redaction.Sinks()))
	}
	_, err = f.NewWriter(redaction.Sink("telemetry-"+canary), io.Discard)
	var sinkErr *redaction.SinkError
	if !errors.As(err, &sinkErr) {
		t.Fatal("unauthorized sink was not refused with a typed *SinkError")
	}
	if strings.Contains(sinkErr.Error(), canary) {
		t.Fatal("SinkError rendering leaked the canary")
	}

	// The first value beyond the boundary is refused with a typed error
	// before any byte reaches the underlying sink.
	var limitBuf bytes.Buffer
	lw, err := f.NewWriter(redaction.SinkCache, &limitBuf)
	if err != nil {
		t.Fatalf("NewWriter(cache) refused a canonical sink: %v", err)
	}
	_, err = lw.Write(bytes.Repeat([]byte("B"), redaction.MaxInputBytes+1))
	var limitErr *redaction.LimitError
	if !errors.As(err, &limitErr) {
		t.Fatal("input beyond the boundary was not refused with a typed *LimitError")
	}
	if limitBuf.Len() != 0 || lw.Effects() != 0 {
		t.Fatalf("boundary refusal happened after a forbidden effect (bytes %d, effects %d)", limitBuf.Len(), lw.Effects())
	}

	// The exact boundary value passes untouched when it carries no secret.
	var okBuf bytes.Buffer
	ow, err := f.NewWriter(redaction.SinkEvidence, &okBuf)
	if err != nil {
		t.Fatalf("NewWriter(evidence) refused a canonical sink: %v", err)
	}
	atLimit := strings.Repeat("A", redaction.MaxInputBytes)
	if _, err := ow.Write([]byte(atLimit)); err != nil {
		t.Fatalf("boundary-sized write failed: %v", err)
	}
	if err := ow.Flush(); err != nil {
		t.Fatalf("boundary-sized flush failed: %v", err)
	}
	if okBuf.String() != atLimit {
		t.Fatalf("boundary-sized identity write diverged (length %d)", okBuf.Len())
	}

	// RedactBounded is the same boundary applied to direct string use.
	if _, err := f.RedactBounded(atLimit + "A"); !errors.As(err, &limitErr) {
		t.Fatal("RedactBounded did not refuse the first value beyond the boundary")
	}
	if out, err := f.RedactBounded("note " + canary); err != nil || strings.Contains(out, canary) {
		t.Fatal("RedactBounded failed to redact an in-bound input")
	}
}
