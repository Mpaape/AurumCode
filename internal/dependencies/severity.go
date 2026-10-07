package dependencies

import "github.com/Mpaape/AurumCode/internal/config"

// SeverityUnknown is an advisory whose source gave no severity. The gate
// counts it as failing whenever fail_on is declared.
const SeverityUnknown = config.SeverityUnknown

// NormalizeSeverity reads the advisory source's severity word as
// critical/high/medium/low, or SeverityUnknown (config owns the vocabulary
// the policy's fail_on is written in).
func NormalizeSeverity(raw string) string { return config.NormalizeDependencySeverity(raw) }
