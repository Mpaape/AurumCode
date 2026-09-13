// Package testgen proposes deterministic Go test cases from a code diff and
// runs them inside an injected sandbox runner.
//
// Propose derives test-case names from the Go functions and methods changed by
// a diff without executing anything. Run invokes a caller-supplied Runner once
// per distinct package, so the sandbox stays fully in the caller's control and
// this package never runs a command itself.
package testgen
