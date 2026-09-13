// Package render turns review results and diffs into deterministic,
// PR-ready Markdown: a localized summary and a Mermaid flowchart.
//
// It is self-contained and configuration-free. The only knob is an optional
// language hint ("pt-BR"/"pt" for Portuguese, anything else falls back to
// English). It imports pkg/types read-only and uses only the standard
// library, so it can be dropped into any review pipeline without setup.
package render
