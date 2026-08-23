package javascript

import (
	"context"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// SelectExtractor is the single decision point AUR-463 adds: it probes for
// `typedoc` once (the same lookup JSExtractor.Validate performs) and returns
// the extractor a composition root should register for JavaScript.
//
//   - typedoc found and working: returns JSExtractor. Every later call
//     behaves exactly as before this card -- the same subprocess, the same
//     flags, the same output -- so a host with typedoc installed keeps
//     getting typedoc's exact output, byte for byte (AC-003).
//   - typedoc absent (a genuine exec.ErrNotFound, classified by
//     extractors.MissingTool): returns NativeExtractor. Its Validate always
//     succeeds and its Extract reads JSDoc comments directly from source
//     text, so a host without the npm toolchain now gets real documentation
//     instead of internal/pipeline's unconditional
//     "required tool not in PATH" skip.
//   - typedoc found but broken (installed, non-ErrNotFound failure): still
//     returns JSExtractor. Silently swapping to native here would turn a
//     broken toolchain into a report that looks identical to a clean run;
//     the broken-install error must keep surfacing through
//     JSExtractor.Validate, exactly as it does today.
//
// This exists because internal/pipeline.ExtractorPipeline (outside this
// card's paths) skips a language unconditionally whenever Extractor.Validate
// returns any error, with no fallback of its own -- so the choice between
// "the tool" and "the native reader" has to be made once, here, at
// registration time, not inside Validate itself. JSExtractor.Validate keeps
// reporting a missing typedoc as extractors.ToolUnavailableError: that
// contract (pinned by
// internal/documentation/extractors/tool_unavailable_test.go and
// tool_failure_test.go) is still the correct description of JSExtractor
// itself -- a thin wrapper around the typedoc subprocess. What changed is
// that a composition root now has a real alternative to registering
// JSExtractor unconditionally, and SelectExtractor is that choice.
func SelectExtractor(ctx context.Context, runner site.CommandRunner) extractors.Extractor {
	jsExt := NewJSExtractor(runner)

	if err := jsExt.Validate(ctx); err != nil {
		if _, missing := extractors.MissingTool(err); missing {
			return NewNativeExtractor()
		}
		// Installed but broken: fall through and register JSExtractor
		// anyway, so the broken-toolchain error still surfaces through its
		// own Validate the next time the pipeline calls it, instead of being
		// masked by a silent swap to native.
	}

	return jsExt
}
