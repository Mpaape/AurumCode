package site

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// allowedEnvNames is the explicit allowlist of parent-process environment
// variables forwarded to an external documentation tool. The runner never hands
// a child os.Environ(): the process that starts this pipeline holds the LLM
// provider key (LLM_API_KEY / OPENAI_API_KEY) plus whatever else CI exported,
// and every extractor shells out to third-party binaries. Inheritance would give
// each of them the whole secret set.
//
// Variables that let a caller inject code into the child (GOFLAGS -toolexec,
// NODE_OPTIONS/NODE_PATH, RUBYOPT/RUBYLIB, PERL5OPT, BASH_ENV, LD_PRELOAD,
// PSModulePath, GIT_SSH_COMMAND, GIT_ASKPASS) and variables that customarily
// embed credentials in a URL (GOPROXY, NPM_CONFIG_REGISTRY, NPM_CONFIG__AUTH)
// are deliberately absent. Adding a name here is a security decision.
var allowedEnvNames = []string{
	// Core process environment every tool needs.
	"PATH", "HOME", "TMPDIR", "TMP", "TEMP",
	"LANG", "LC_ALL", "LC_CTYPE", "TZ", "TERM",
	"USER", "LOGNAME",
	// Windows equivalents; absent on Linux runners, required there.
	"SYSTEMROOT", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "USERPROFILE",
	// Go toolchain: gomarkdoc / go doc.
	"GOPATH", "GOROOT", "GOBIN", "GOCACHE", "GOMODCACHE", "GOTMPDIR",
	"GOOS", "GOARCH", "CGO_ENABLED",
	// Node toolchain: npx, pagefind, typedoc.
	"NPM_CONFIG_CACHE", "NPM_CONFIG_PREFIX",
	// .NET toolchain: dotnet, xmldocmd.
	"DOTNET_ROOT", "DOTNET_CLI_HOME", "DOTNET_CLI_TELEMETRY_OPTOUT",
	"DOTNET_NOLOGO", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE",
	"NUGET_PACKAGES", "MSBUILDDISABLENODEREUSE",
	// Ruby toolchain: jekyll, bundler.
	"GEM_HOME", "GEM_PATH", "BUNDLE_GEMFILE", "BUNDLE_PATH", "BUNDLE_APP_CONFIG",
	"JEKYLL_ENV",
}

// deniedExplicitEnvNames are names a caller may not set through the explicit env
// map either: they turn "configure a tool" into "run arbitrary code as the
// pipeline". The runner refuses the whole call rather than dropping them
// silently, so a misconfiguration is visible instead of quietly ignored.
// pipeline. Keys are upper-case because the lookup upper-cases the candidate:
// Windows resolves environment names case-insensitively, so an exact-match
// denylist would let "nOdE_oPtIoNs" through to node.exe unchanged.
var deniedExplicitEnvNames = map[string]bool{
	"LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "LD_AUDIT": true,
	"DYLD_INSERT_LIBRARIES": true, "DYLD_LIBRARY_PATH": true,
	"BASH_ENV": true, "ENV": true, "IFS": true, "SHELLOPTS": true,
	"NODE_OPTIONS": true, "NODE_PATH": true,
	"RUBYOPT": true, "RUBYLIB": true, "PERL5OPT": true, "PERL5LIB": true,
	"PYTHONPATH": true, "PYTHONSTARTUP": true, "PYTHONHOME": true,
	"GOFLAGS": true, "GOENV": true,
	"PSMODULEPATH": true,
	"GIT_SSH_COMMAND": true, "GIT_SSH": true, "GIT_ASKPASS": true,
	"GIT_EXTERNAL_DIFF": true, "GIT_PAGER": true, "GIT_EDITOR": true,
	"GIT_PROXY_COMMAND": true, "GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"JAVA_TOOL_OPTIONS": true, "_JAVA_OPTIONS": true, "JDK_JAVA_OPTIONS": true,
	"PATH": true,
}

// deniedExplicitEnvPrefixes deny whole families rather than single names. Every
// "npm_config_<x>" sets npm config <x>, and npm's own node-options key rebuilds
// NODE_OPTIONS for the child node; GIT_CONFIG_COUNT/KEY_n/VALUE_n sets arbitrary
// git config, including core.pager and core.sshCommand, which are commands.
var deniedExplicitEnvPrefixes = []string{"NPM_CONFIG_", "GIT_CONFIG_"}

// deniedExplicitEnvName reports whether a caller-supplied name is refused.
func deniedExplicitEnvName(name string) bool {
	upper := strings.ToUpper(name)

	if deniedExplicitEnvNames[upper] {
		return true
	}

	for _, prefix := range deniedExplicitEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}

	return false
}

// RedactionMarker replaces any credential-shaped run found in tool output.
const RedactionMarker = "[REDACTED]"

// MaxStderrDiagnosticBytes bounds how much tool stderr may reach a diagnostic.
// Tool output is untrusted attacker-influenced data and ends up in a public
// Action log, so it is capped explicitly rather than "however much it printed".
const MaxStderrDiagnosticBytes = 4096

// redactScanLimit bounds the work the redaction patterns do on pathological
// output before anything is inspected.
const redactScanLimit = 64 * 1024

const truncationNotice = "\n[diagnostic truncated by AurumCode]"

// credentialPatterns match values shaped like credentials. They are matched
// against untrusted tool output only; a false positive costs a mangled log line,
// a false negative publishes a secret.
var credentialPatterns = []*regexp.Regexp{
	// PEM private key blocks, complete or dangling.
	regexp.MustCompile(`(?s)-----BEGIN[^-]{0,64}PRIVATE KEY-----.*?-----END[^-]{0,64}PRIVATE KEY-----`),
	regexp.MustCompile(`(?s)-----BEGIN[^-]{0,64}PRIVATE KEY-----.*`),
	// OpenAI-style keys, including sk-proj- and friends.
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`),
	// AWS access key ids.
	regexp.MustCompile(`(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA)[0-9A-Z]{12,}`),
	// GitHub tokens: personal, oauth, user-to-server, server-to-server, refresh.
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	// A few other unambiguous shapes.
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`glpat-[A-Za-z0-9_-]{16,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{30,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]{16,}=*`),
	// name=value where the name announces a secret.
	regexp.MustCompile(`(?i)\b[A-Za-z0-9_.-]*(?:api[_-]?key|apikey|secret|token|password|passwd|credential)[A-Za-z0-9_.-]*\s*[:=]\s*\S+`),
}

// credentialPrefixes are the literal openers of the patterns above. They are
// used to scrub a fragment that a size cut may have left dangling: half of a
// secret is still a secret.
var credentialPrefixes = []string{
	"-----BEGIN", "sk-", "AKIA", "ASIA", "AGPA", "AIDA", "AROA", "ANPA", "ANVA",
	"ghp_", "gho_", "ghu_", "ghs_", "ghr_", "xox", "glpat-", "AIza",
}

// Redact strips credential-shaped values from untrusted text and bounds its
// size. It is the only sanctioned way for external tool output to reach a log,
// an error string, or any other published surface.
func Redact(s string) string {
	truncated := false

	if len(s) > redactScanLimit {
		s = s[:redactScanLimit]
		truncated = true
	}

	for _, pattern := range credentialPatterns {
		s = pattern.ReplaceAllString(s, RedactionMarker)
	}

	if len(s) > MaxStderrDiagnosticBytes {
		s = s[:MaxStderrDiagnosticBytes]
		truncated = true
	}

	if truncated {
		s = scrubDanglingPrefix(s) + truncationNotice
	}

	return s
}

// scrubDanglingPrefix removes a trailing partial credential left by a size cut.
func scrubDanglingPrefix(s string) string {
	cut := -1

	for _, prefix := range credentialPrefixes {
		if idx := strings.LastIndex(s, prefix); idx > cut {
			cut = idx
		}
	}

	if cut < 0 {
		return s
	}

	return s[:cut] + RedactionMarker
}

// childEnvironment builds the child environment from the allowlist plus the
// caller's explicit variables. It never consults os.Environ() wholesale.
func childEnvironment(env map[string]string) ([]string, error) {
	for name := range env {
		if deniedExplicitEnvName(name) {
			return nil, fmt.Errorf("refusing to run: environment variable %s can inject code into the child process", name)
		}
	}

	vars := make([]string, 0, len(allowedEnvNames)+len(env))

	for _, name := range allowedEnvNames {
		if value, ok := os.LookupEnv(name); ok {
			vars = append(vars, name+"="+value)
		}
	}

	// Explicit values are appended last so they win: os/exec keeps the final
	// occurrence of a duplicated name.
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		vars = append(vars, name+"="+env[name])
	}

	return vars, nil
}

// DefaultRunner is the default command runner using exec.Command
type DefaultRunner struct {
	timeout time.Duration
}

// NewDefaultRunner creates a new default command runner
func NewDefaultRunner() *DefaultRunner {
	return &DefaultRunner{
		timeout: 5 * time.Minute,
	}
}

// WithTimeout sets the command timeout
func (r *DefaultRunner) WithTimeout(timeout time.Duration) *DefaultRunner {
	r.timeout = timeout
	return r
}

// Run executes a command and returns output
func (r *DefaultRunner) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Create command
	command := exec.CommandContext(ctx, cmd, args...)
	command.Dir = workdir

	// Set environment. This assignment is unconditional on purpose: leaving
	// command.Env nil makes the child inherit the parent's entire environment,
	// provider credentials included.
	envVars, err := childEnvironment(env)
	if err != nil {
		return "", err
	}

	command.Env = envVars

	// Capture output
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	// Run command
	if err := command.Run(); err != nil {
		// Tool stderr is untrusted data on its way to a public log: redact and
		// bound it, and let it out only as failure diagnostics.
		if stderr.Len() > 0 {
			return "", fmt.Errorf("command failed: %w (stderr: %s)", err, Redact(stderr.String()))
		}

		return "", fmt.Errorf("command failed: %w", err)
	}

	// Stderr never joins the success value. Callers log this string, and a tool
	// that chatters on stderr while succeeding would smuggle its content there.
	return strings.TrimSpace(stdout.String()), nil
}

// MockRunner is a mock command runner for testing
type MockRunner struct {
	outputs map[string]string
	errors  map[string]error
	calls   []MockCall
}

// MockCall records a command call
type MockCall struct {
	Cmd     string
	Args    []string
	Workdir string
	Env     map[string]string
}

// NewMockRunner creates a new mock runner
func NewMockRunner() *MockRunner {
	return &MockRunner{
		outputs: make(map[string]string),
		errors:  make(map[string]error),
		calls:   []MockCall{},
	}
}

// WithOutput sets the output for a specific command
func (m *MockRunner) WithOutput(cmd string, output string) *MockRunner {
	m.outputs[cmd] = output
	return m
}

// WithError sets an error for a specific command
func (m *MockRunner) WithError(cmd string, err error) *MockRunner {
	m.errors[cmd] = err
	return m
}

// Run executes a mock command
func (m *MockRunner) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	// Record call
	m.calls = append(m.calls, MockCall{
		Cmd:     cmd,
		Args:    args,
		Workdir: workdir,
		Env:     env,
	})

	// Build key for lookup
	key := cmd
	if len(args) > 0 {
		key = fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
	}

	// Check for exact error
	if err, ok := m.errors[key]; ok {
		return "", err
	}

	shortKey := key
	if len(args) > 0 {
		shortKey = fmt.Sprintf("%s %s", cmd, args[0])
	}

	// Fallback to less specific errors
	if err, ok := m.errors[shortKey]; ok {
		return "", err
	}
	if err, ok := m.errors[cmd]; ok {
		return "", err
	}

	// Return output
	if output, ok := m.outputs[key]; ok {
		return output, nil
	}
	if output, ok := m.outputs[shortKey]; ok {
		return output, nil
	}
	if output, ok := m.outputs[cmd]; ok {
		return output, nil
	}

	for storedKey, err := range m.errors {
		if strings.HasPrefix(key, storedKey) {
			return "", err
		}
	}

	for storedKey, output := range m.outputs {
		if strings.HasPrefix(key, storedKey) {
			return output, nil
		}
	}

	// Default success
	return "", nil
}

// GetCalls returns recorded calls
func (m *MockRunner) GetCalls() []MockCall {
	return m.calls
}

// Reset clears recorded calls
func (m *MockRunner) Reset() {
	m.calls = []MockCall{}
}
