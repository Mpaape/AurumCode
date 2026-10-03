// Finding severities: their rank order and the --fail-on level parser.
package main

import (
	"fmt"
	"strings"
)

// parseFailOnLevel maps a --fail-on level to the internal severity rank it
// gates on, returning the canonical engine severity name alongside. The
// accepted spellings are the engine's own severity vocabulary -- error,
// warning, info, exactly the three values internal/prompt.ResponseParser
// admits -- plus the CI-conventional aliases high, medium and low.
// Matching is case-insensitive; anything else is an error.
func parseFailOnLevel(level string) (int, string, error) {
	switch strings.ToLower(level) {
	case "high", "error":
		return rankError, "error", nil
	case "medium", "warning":
		return rankWarning, "warning", nil
	case "low", "info":
		return rankInfo, "info", nil
	default:
		return 0, "", fmt.Errorf("--fail-on: unknown level %q (accepted: high|error, medium|warning, low|info)", level)
	}
}

// Severity ranks, ordered so that "at the chosen severity or above" is a
// plain >= comparison. 0 is reserved for "no gate configured".
const (
	rankInfo    = 1
	rankWarning = 2
	rankError   = 3
)

// severityRank ranks a finding's severity. The parser guarantees every
// issue carries one of error/warning/info (in any letter case, see
// internal/prompt/parser.go's validation), so the default arm is
// unreachable in practice; it still ranks unknown values as error so the
// gate fails closed rather than silently waving a finding through.
func severityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "info":
		return rankInfo
	case "warning":
		return rankWarning
	case "error":
		return rankError
	default:
		return rankError
	}
}
