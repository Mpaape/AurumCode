// Package engines is the closed list of scanner engines compiled into the
// binary: importing it registers every one of them. Adding an engine is
// one import line here and its own package under internal/scanner.
package engines

import (
	// Semgrep, the SAST engine.
	_ "github.com/Mpaape/AurumCode/internal/scanner/semgrep"
)
