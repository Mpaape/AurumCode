// Package goproject is a tiny, synthetic Go project used as a deterministic
// fixture by AUR-424's acceptance test. It exists to hand the standard-library
// based Go documentation extractor (internal/documentation/extractors/go) real
// exported symbols and real doc comments to parse, so a green acceptance run
// proves that actual source was read and rendered instead of an empty pass.
package goproject
