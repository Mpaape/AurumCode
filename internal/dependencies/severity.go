package dependencies

import "strings"

// SeverityUnknown is an advisory whose source gave no severity. The gate
// counts it at the top rank: a severity that cannot be read never lets a
// vulnerability pass.
const SeverityUnknown = "unknown"

// NormalizeSeverity reads the advisory source's severity word (GitHub's
// LOW/MODERATE/HIGH/CRITICAL, or the gate's own spellings) as
// critical/high/medium/low; anything else is SeverityUnknown.
func NormalizeSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical":
		return "critical"
	case "high", "error":
		return "high"
	case "moderate", "medium", "warning":
		return "medium"
	case "low", "info":
		return "low"
	default:
		return SeverityUnknown
	}
}
