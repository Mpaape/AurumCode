// Package prosefiles names the files a person reads rather than a machine
// runs: logs, notes and documents. A matcher that describes a code shape (a
// shell call, a SQL string, an HTML sink) finds only mentions in them, never
// a defect. It is a leaf package so both the security pass (internal/review)
// and the deterministic catalog (internal/analysis) share one list.
package prosefiles

import (
	"path"
	"strings"
)

// extensions are the prose file extensions. The 2026-10-08 self review of
// the AurumCode repository flagged "command injection" in a recorded demo
// log because the log quoted a rule about eval(); that is the class of file
// this list exists for.
var extensions = map[string]bool{
	".txt":      true,
	".log":      true,
	".md":       true,
	".markdown": true,
	".rst":      true,
	".adoc":     true,
}

// IsProsePath reports whether p is a prose file by its extension, case
// insensitively.
func IsProsePath(p string) bool {
	return extensions[strings.ToLower(path.Ext(p))]
}
