package scanner

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

func redactor() Redactor { return redaction.NewFilter().Redact }

// A credential that straddles the MaxDetailBytes cut is redacted whole: no
// prefix of it survives in the summary.
func TestAUR595SummaryRedactsATokenAtTheCut(t *testing.T) {
	token := "ghp_" + strings.Repeat("Q7w", 12)
	prefix := strings.Repeat("x", MaxDetailBytes-len(detailEllipsis)-8) + " "
	got := Summarize(errors.New(prefix+token+" trailing"), redactor())
	if strings.Contains(got, "ghp_") || strings.Contains(got, "Q7w") {
		t.Fatalf("summary leaks part of the token cut at the limit: %q", got)
	}
	if len(got) > MaxDetailBytes {
		t.Fatalf("summary has %d bytes, limit %d", len(got), MaxDetailBytes)
	}
}

// An auth header the tool printed on a line of its own is redacted before
// the lines are joined (its pattern is anchored at the start of a line).
func TestAUR595SummaryRedactsAHeaderOnItsOwnLine(t *testing.T) {
	secret := "tok" + strings.Repeat("9", 12)
	got := Summarize(errors.New("gitleaks: execution failed: exit status 2:\nAuthorization: Bearer "+secret), redactor())
	if strings.Contains(got, secret) || !strings.Contains(got, redaction.Marker) {
		t.Fatalf("summary keeps the header value: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("summary is not one line: %q", got)
	}
}

// The cut never splits a rune, and nothing is published without a redactor.
func TestAUR595SummaryCutsOnARuneBoundaryAndNeedsARedactor(t *testing.T) {
	got := Summarize(errors.New(strings.Repeat("ç", MaxDetailBytes)), redactor())
	if !utf8.ValidString(got) || len(got) > MaxDetailBytes || !strings.HasSuffix(got, detailEllipsis) {
		t.Fatalf("summary = %q (%d bytes)", got, len(got))
	}
	if got := Summarize(errors.New("boom"), nil); got != "" {
		t.Fatalf("summary without a redactor = %q, want empty", got)
	}
}
