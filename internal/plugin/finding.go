package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Finding is one result of a plugin, normalized by its format's adapter.
// Every text field is untrusted: it is redacted before it is written and
// never printed.
type Finding struct {
	// Class is the stable part of the rule id: a CWE ("cwe-79") or the
	// plugin's own vulnerability class.
	Class       string
	Title       string
	Message     string
	Path        string
	Line        int
	Severity    string // critical, high, medium, low or info
	Score       float64
	Fingerprint string
}

// Counts is what an ingest reports: numbers only, never a finding's text.
type Counts struct {
	Accepted      int // results written to the SARIF
	FalsePositive int // verdict FALSE_POSITIVE
	NoVerdict     int // no TRUE_POSITIVE verdict (null or unknown)
	OtherChannel  int // not the main channel (posture)
	Unverified    int // dropped as inconclusive by the plugin, sent as warnings
}

// Parsed is an adapter's answer for a conclusive run.
type Parsed struct {
	Findings []Finding
	Counts   Counts
}

var classUnsafe = regexp.MustCompile(`[^a-z0-9_.-]+`)

// normalizeClass makes a rule id segment: lowercase, no spaces or slashes.
func normalizeClass(cwe, class string) string {
	for _, v := range []string{cwe, class} {
		v = strings.Trim(classUnsafe.ReplaceAllString(strings.ToLower(strings.TrimSpace(v)), "-"), "-")
		if v != "" {
			return v
		}
	}
	return "unclassified"
}

// normalizeSeverity maps a severity onto critical..info; unknown is high,
// never dropped to a lower level.
func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return "critical"
	case "medium", "moderate", "warning":
		return "medium"
	case "low", "note":
		return "low"
	case "info", "informational", "none":
		return "info"
	default:
		return "high"
	}
}

// defaultScore is the security-severity of a severity without a CVSS score.
func defaultScore(severity string) float64 {
	switch severity {
	case "critical":
		return 9.5
	case "medium":
		return 5.5
	case "low":
		return 3.0
	case "info":
		return 0.0
	default:
		return 8.0
	}
}

// fallbackFingerprint is a stable identity when the plugin gives none: the
// rule, the path and the title, never the line (a line shift keeps the
// alert).
func fallbackFingerprint(rule, path, title string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", rule, path, title)))
	return hex.EncodeToString(sum[:])
}
