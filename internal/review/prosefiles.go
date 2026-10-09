package review

import (
	"path"
	"strings"
)

// RuleAppliesToCode is the Rule.AppliesTo value that keeps a matcher away
// from prose files.
const RuleAppliesToCode = "code"

// proseExtensions are the files the security pass treats as prose: text a
// person reads, never code a machine runs. The 2026-10-08 self review of
// the AurumCode repository flagged "command injection" in a recorded
// demo log because the log quoted a rule about eval(); a regexp that
// describes a code shape has no defect to find in a log, a note or a
// document, only mentions.
var proseExtensions = map[string]bool{
	".txt":      true,
	".log":      true,
	".md":       true,
	".markdown": true,
	".rst":      true,
	".adoc":     true,
}

// IsProsePath reports whether p is a prose file by its extension.
func IsProsePath(p string) bool {
	return proseExtensions[strings.ToLower(path.Ext(p))]
}
