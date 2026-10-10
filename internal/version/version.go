// Package version is the AurumCode binary's own version: the value stamped
// at build time with -ldflags "-X main.version=<value>" (the
// Dockerfile's AURUMCODE_VERSION build argument, which the review workflow
// sets to the tool's exact release tag or its short SHA), or dev for a build
// that stamped none.
//
// The parecer and the audit record name the version only when it was
// stamped: a dev build publishes exactly what it published before, so the
// recorded outputs of a local build never change.
package version

import "strings"

// Dev is the version of a build that stamped none.
const Dev = "dev"

// Info is one binary's version. The zero Info is a dev build.
type Info struct {
	value string
}

// New reads the value stamped at build time. A blank value (the build
// argument set to nothing) is Dev, never an empty version.
func New(stamped string) Info {
	return Info{value: strings.TrimSpace(stamped)}
}

// IsDev reports whether the build stamped no version.
func (i Info) IsDev() bool {
	return i.value == "" || i.value == Dev
}

// Label is the version as published: the stamped value, or Dev.
func (i Info) Label() string {
	if i.IsDev() {
		return Dev
	}
	return i.value
}
