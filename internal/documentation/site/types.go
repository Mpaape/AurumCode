package site

import "context"

// CommandRunner executes shell commands (mockable interface)
type CommandRunner interface {
	Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error)
}

// Builder builds static site artifacts
type Builder interface {
	Build(ctx context.Context, workdir string) error
	Validate(ctx context.Context) error
	GetVersion(ctx context.Context) (string, error)
}

// BuildConfig configures the site build
type BuildConfig struct {
	WorkDir   string
	OutputDir string
	BaseURL   string
	Minify    bool
	Env       map[string]string
}

// BuildResult contains the result of a build
type BuildResult struct {
	Success    bool
	OutputPath string
	Duration   int64 // milliseconds
	Logs       string
	Error      error
}
