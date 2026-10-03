// Inputs of the gate verdict key that only the command can know (the binary
// it is running); the key itself is built in internal/gate.
package main

import "runtime/debug"

// binaryIdentity folds the running binary's own identity into the key:
// the -ldflags -X main.version value (see main.go's own doc) plus, when
// the Go toolchain embedded them (a build with VCS info available --
// `go build` inside a git checkout, not stripped), the exact commit
// (vcs.revision) and whether the working tree had uncommitted changes
// (vcs.modified) at build time. A stale binary from before a logic change
// in THIS card (or in EvaluateGate, or in the rule engine) must never
// have its stored verdict mistaken for one produced by the current code.
func binaryIdentity() string {
	id := "version:" + version
	if info, ok := debug.ReadBuildInfo(); ok {
		var revision, modified string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if revision != "" {
			id += ";vcs.revision:" + revision
		}
		if modified != "" {
			id += ";vcs.modified:" + modified
		}
	}
	return id
}
