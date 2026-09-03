// Package core. contest.go's file stem is "contest": it CONTAINS the
// substring "test" but does not END in "_test" as a whole suffix. Must
// still be documented -- a third AC-003 trap, this one against the
// filename-stem check rather than the directory-component check.
package core

// Contest models an unrelated product concept whose name happens to embed
// the letters "test".
type Contest struct{}
