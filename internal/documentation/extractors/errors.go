package extractors

import (
	"errors"
	"fmt"
	"os"
)

// ToolUnavailableError reports that an external documentation tool an extractor
// depends on is not installed on the host. Consumers of an extractor treat it as a
// reason to skip that language, never as an extraction failure.
//
// It is reserved for a genuine executable lookup failure (exec.ErrNotFound). A
// tool that is installed and exits non-zero is an extraction error: reporting it
// as unavailable would turn a broken toolchain into a silent skip.
type ToolUnavailableError struct {
	// Tool is the executable that was looked up, e.g. "gomarkdoc".
	Tool string

	// Hint is optional installation guidance.
	Hint string

	// Cause is the underlying lookup failure, if any.
	Cause error
}

// NewToolUnavailableError builds a ToolUnavailableError for a missing executable.
func NewToolUnavailableError(tool, hint string, cause error) *ToolUnavailableError {
	return &ToolUnavailableError{Tool: tool, Hint: hint, Cause: cause}
}

func (e *ToolUnavailableError) Error() string {
	if e.Hint == "" {
		return fmt.Sprintf("%s not found", e.Tool)
	}

	return fmt.Sprintf("%s not found: %s", e.Tool, e.Hint)
}

// Unwrap exposes the underlying lookup failure to errors.Is and errors.As.
func (e *ToolUnavailableError) Unwrap() error { return e.Cause }

// MissingTool returns the name of the unavailable tool and true when err is, or
// wraps, a ToolUnavailableError.
func MissingTool(err error) (string, bool) {
	var toolErr *ToolUnavailableError
	if errors.As(err, &toolErr) {
		return toolErr.Tool, true
	}

	return "", false
}

// ErrNoOutputProduced reports that an external tool exited successfully without
// writing the documentation it was asked to write. Counting such a run as
// generated documentation makes an extractor lie to its caller: the publishing
// step later finds nothing on disk.
var ErrNoOutputProduced = errors.New("no output produced")

// ConfirmOutputFile verifies on disk that tool actually wrote path with content.
// It is the only evidence an extractor may use to count a document as generated;
// a zero exit status is not evidence.
func ConfirmOutputFile(tool, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s exited successfully but %s was not written: %w: %v",
			tool, path, ErrNoOutputProduced, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s exited successfully but %s is a directory, not a document: %w",
			tool, path, ErrNoOutputProduced)
	}

	if info.Size() == 0 {
		return fmt.Errorf("%s exited successfully but %s is empty: %w",
			tool, path, ErrNoOutputProduced)
	}

	return nil
}

// ConfirmOutputFiles verifies that a tool that owns a whole output directory left
// at least one file behind. found is the on-disk listing the caller collected.
func ConfirmOutputFiles(tool, dir string, found []string) error {
	if len(found) == 0 {
		return fmt.Errorf("%s exited successfully but wrote no documentation under %s: %w",
			tool, dir, ErrNoOutputProduced)
	}

	return nil
}
