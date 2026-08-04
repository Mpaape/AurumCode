package site

import "context"

// CommandRunner executes an external tool. Extractors depend on this interface
// rather than on os/exec directly, so a test can drive them without the
// toolchain installed and every invocation passes through one audited place.
type CommandRunner interface {
	Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error)
}
