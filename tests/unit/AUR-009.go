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

	// A structured payload is the dominant serialization here, so every value
	// rule is proved on the quoted spelling with a value the registered
	// secret rule cannot rescue.
	assertStructural := func(rule, input, expected string) {
		t.Helper()
		if got := f.Redact(input); got != expected {
			t.Fatalf("%s: redacted output diverged (got length %d, want length %d)", rule, len(got), len(expected))
		}
	}
	assertStructural("key-value-quoted-json",
		`{"password":"notarealpw0123","ok":true}`,
		`{"password":"`+redaction.Marker+`","ok":true}`)
	assertStructural("key-value-quoted-yaml",
		`api_key: "notarealtokenvalue"`,
		`api_key: "`+redaction.Marker+`"`)
	assertStructural("auth-header-in-json",
		`{"headers":{"Authorization":"Bearer notarealtoken9"}}`,
		`{"headers":{"Authorization":"`+redaction.Marker+`"}}`)
	assertStructural("url-userinfo",
		"remote https://admin:notarealpw0123@git.example.test/r.git fetched",
		"remote https://"+redaction.Marker+"@git.example.test/r.git fetched")
	assertRedacted("url-userinfo-canary",
		"clone https://admin:"+canary+"@git.example.test/r.git",
		"clone https://"+redaction.Marker+"@git.example.test/r.git")

	// Private-key banners are assembled at runtime so no committed byte of
	// this test is itself a credential banner.
	pemBanner := func(kind string) string { return "-----" + kind + " RSA PRIVATE" + " KEY" + "-----" }
	keyBlock := pemBanner("BEGIN") + "\nMIIBOgIBAAJBnotarealkey0123456789abcdef\n" +
		"wxyz0123456789notarealkeyMIIBOgIBAAJB\n" + pemBanner("END") + "\ntail ok"
	assertStructural("private-key-block",
		keyBlock, redaction.Marker+"\ntail ok")

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

	// A secret sliced by a line break must not reach the sink as two verbatim
	// fragments, whether the slice arrives in one write or across two.
	for _, mode := range []string{"single-write", "split-write"} {
		var slicedBuf bytes.Buffer
		sw, err := f.NewWriter(redaction.SinkDiff, &slicedBuf)
		if err != nil {
			t.Fatalf("NewWriter(diff) refused a canonical sink: %v", err)
		}
		cut := len(canary) / 2
		head, tail := "leak "+canary[:cut], canary[cut:]+" end\n"
		if mode == "single-write" {
			if _, err := sw.Write([]byte(head + "\n" + tail)); err != nil {
				t.Fatalf("%s: sliced write failed: %v", mode, err)
			}
		} else {
			if _, err := sw.Write([]byte(head + "\n")); err != nil {
				t.Fatalf("%s: sliced write failed: %v", mode, err)
			}
			if _, err := sw.Write([]byte(tail)); err != nil {
				t.Fatalf("%s: sliced write failed: %v", mode, err)
			}
		}
		if err := sw.Flush(); err != nil {
			t.Fatalf("%s: flush failed: %v", mode, err)
		}
		if strings.Contains(slicedBuf.String(), canary[:cut]) {
			t.Fatalf("%s: a newline-sliced canary fragment reached the sink (length %d)", mode, slicedBuf.Len())
		}
		if slicedBuf.String() != "leak "+redaction.Marker+" end\n" {
			t.Fatalf("%s: sliced write diverged (length %d, effects %d)", mode, slicedBuf.Len(), sw.Effects())
		}
	}

	// The same private-key block streamed line by line: the banner is
	// replaced once and the material never reaches the sink.
	var pemBuf bytes.Buffer
	pw, err := f.NewWriter(redaction.SinkFinding, &pemBuf)
	if err != nil {
		t.Fatalf("NewWriter(finding) refused a canonical sink: %v", err)
	}
	if _, err := pw.Write([]byte(keyBlock)); err != nil {
		t.Fatalf("private-key block write failed: %v", err)
	}
	if err := pw.Flush(); err != nil {
		t.Fatalf("private-key block flush failed: %v", err)
	}
	if pemBuf.String() != redaction.Marker+"\ntail ok" {
		t.Fatalf("streamed private-key block diverged (length %d, effects %d)", pemBuf.Len(), pw.Effects())
	}
	if pw.Effects() != 2 {
		t.Fatalf("streamed private-key block recorded %d effects, want 2", pw.Effects())
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
