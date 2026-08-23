package prompt

import "github.com/Mpaape/AurumCode/internal/analyzer"

// AUR-467: the review sends every changed line through the same CODE rule
// catalog (rulecatalog.go), regardless of what kind of file it came from.
// Measured 2026-08-14 against the real gateway: a 26-file commit, 15 of
// them `.mjs`, came back with seven findings, all seven landed on
// AGENTS.md (a markdown file), and every one cited a code-quality rule --
// quality/dead-code on a "what NOT to do" section, quality/long-function
// on prose, quality/high-complexity on invariant rules written in
// Portuguese. None of the fifteen `.mjs` files got a single comment. Two
// causes, both fixed here:
//
//  1. Nothing told the model what kind of file it was looking at, so it
//     matched the closest-sounding code rule to what it saw -- and what it
//     saw was text.
//  2. budgeting.go's BuildContextSegments gave every non-test, non-config
//     file the same PriorityHigh tier and sorted ties alphabetically by
//     path. "AGENTS.md" (leading byte 0x41) sorts before almost every
//     lowercase source path in a typical tree, so its hunks filled the
//     token budget first; TrimToFit (see budgeting.go) stops at the first
//     segment that no longer fits, so once the budget was spent on
//     AGENTS.md the `.mjs` files' hunks never reached the model at all.
//     tests/unit/AUR-467.go reconstructs this ordering effect against a
//     synthetic diff shaped like the measured commit (uppercase doc file +
//     many lowercase code files) as this card's required measurement --
//     the original user diff was never captured, so this is a
//     reconstruction, not a replay.
//
// The fix: classify each file by internal/analyzer's own language
// category (already used elsewhere in this codebase, not a new hand-rolled
// extension list) and exclude prose-classified files from the pool that
// competes for the code token budget and receives the code rule catalog.
// This is deliberately NOT a decision to stop reviewing documentation --
// see the Non-goals in .board/cards/*/AUR-467.md -- it is a decision that,
// absent a prose rule catalog (explicitly out of scope for this card), the
// correct behavior is to not apply the code catalog to prose, and to
// DECLARE the exclusion (see coverage.go) rather than silently drop it.
//
// isProseLanguage decides "code vs. prose" from analyzer's own
// GetLanguageCategory -- reused, not reimplemented, so this file carries
// no second extension list. Deliberately fail CLOSED: DetectLanguage
// returns "unknown" for any extension it does not recognize (.txt, .vue,
// an extensionless NOTICE file, ...), and GetLanguageCategory("unknown")
// falls through to "other", not "documentation". So unrecognized files
// are treated as code and keep competing for the code budget and keep
// receiving code-rule findings. Classifying "unknown" as prose would
// silently stop reviewing every unmapped source file -- exactly the
// AC-002 rejection this card's acceptance forbids ("um candidato que zera
// o AC-001 revisando menos codigo").
func isProseLanguage(language string, detector *analyzer.LanguageDetector) bool {
	return detector.GetLanguageCategory(language) == "documentation"
}

// classifyFile returns the detected language and whether path is prose
// (documentation) rather than code, per isProseLanguage.
func classifyFile(path string, detector *analyzer.LanguageDetector) (language string, isProse bool) {
	language = detector.DetectLanguage(path)
	return language, isProseLanguage(language, detector)
}
