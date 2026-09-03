// Package main is PRODUCT code living under cmd/latest. "latest" contains
// the substring "test" but is not the component "tests" -- the second
// AC-003 trap. It must always be documented.
package main

// Version reports the latest published version string.
func Version() string {
	return "latest"
}

func main() {}
