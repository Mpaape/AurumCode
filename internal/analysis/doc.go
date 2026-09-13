// Package analysis provides the deterministic static-analysis pass for the
// AurumCode review engine.
//
// It is model-free and zero-config: NewRunner builds a Runner over a fixed
// catalog of hand-audited regular expressions compiled into the binary, so
// applying it never needs a configuration file, credentials, network access,
// or an external binary. Analyze scans the added (RIGHT) and removed (LEFT)
// lines of a types.Diff and reports each catalog match as a Finding whose
// Message is a trusted, fixed catalog string (the matched source text is
// never echoed).
//
// Vet is the one optional, sandboxable escape hatch: it drives "go vet
// ./..." through an injected commandRunner — never exec'ing anything itself
// — and turns the standard file:line:col: message diagnostic format into
// Findings. Both entry points are deterministic and side-effect free.
package analysis
