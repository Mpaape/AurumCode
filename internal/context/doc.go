// Package context provides a bounded, deterministic reader for codebase
// dependency context. Given a repository root and a set of changed file
// paths, Resolve reports what else those changes can affect: the symbols the
// changed files define, the files that import or reference them (dependents),
// and the imports/dependencies of the changed files themselves.
//
// The output is a first slice, not a compiler-grade index. Structure
// (symbols and imports) comes from the grammar provider (internal/grammar,
// any language it knows); references are a bounded whole-word scan for the
// changed files' symbols. Around each reference the pack carries a numbered
// excerpt (Snippet, AUR-470) labeled as a use or a test, never as a defect.
// Every phase is bounded by Limits. Missing files, huge files, symlinks,
// paths the policy excludes (WithExclude: ignore globs, secret files) and
// oversized repos are recorded in Pack.Dropped rather than returned as
// errors, and output is always sorted for determinism.
//
// A Pack is untrusted, bounded context: it is an observation only and never
// grants approval, publication authority, or any other capability. Consume
// it read-only, exactly as documented in INTEGRATION.md.
package context
