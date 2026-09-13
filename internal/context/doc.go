// Package context provides a bounded, deterministic reader for codebase
// dependency context. Given a repository root and a set of changed file
// paths, Resolve reports what else those changes can affect: the symbols the
// changed files define, the files that import or reference them (dependents),
// and the imports/dependencies of the changed files themselves.
//
// The output is a first slice, not a compiler-grade index. Extraction uses
// readable pure-Go heuristics — regexes over import statements and a simple
// reference scan — and every phase is bounded by Limits. Missing files, huge
// files, symlinks, and oversized repos are recorded in Pack.Dropped rather
// than returned as errors, and output is always sorted for determinism.
//
// A Pack is untrusted, bounded context: it is an observation only and never
// grants approval, publication authority, or any other capability. Consume
// it read-only, exactly as documented in INTEGRATION.md.
package context
