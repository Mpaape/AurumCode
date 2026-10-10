// The AurumCode version this binary reports: main.version as the build
// stamped it, read through internal/version, and the line of the parecer's
// details that names it.
package main

import (
	"fmt"

	toolversion "github.com/Mpaape/AurumCode/internal/version"
)

// toolVersion is this binary's version, from the value stamped into
// main.version at build time.
func toolVersion() toolversion.Info {
	return toolversion.New(version)
}

// toolVersionLine names the AurumCode that reviewed, or is empty for a dev
// build: the parecer of a build with no stamped version stays byte-identical
// to the one before the version was published (AUR-611), and the tutorials'
// recorded outputs with it.
func toolVersionLine(info toolversion.Info, copy reviewCopy) string {
	if info.IsDev() {
		return ""
	}
	return fmt.Sprintf(copy.toolVersion, info.Label())
}

// joinDetails appends line, when there is one, after the details' sections,
// separated by a blank line.
func joinDetails(sections, line string) string {
	switch {
	case line == "":
		return sections
	case sections == "":
		return line
	}
	return sections + "\n\n" + line
}
