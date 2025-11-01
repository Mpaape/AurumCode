package site

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

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

	// Set environment
	if len(env) > 0 {
		envVars := command.Environ()
		for k, v := range env {
			envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
		}
		command.Env = envVars
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	// Run command
	err := command.Run()
	if err != nil {
		// Include stderr in error
		if stderr.Len() > 0 {
			return "", fmt.Errorf("command failed: %w\nstderr: %s", err, stderr.String())
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	// Return combined output
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	return strings.TrimSpace(output), nil
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

	// Check for error
	if err, ok := m.errors[key]; ok {
		return "", err
	}

	// Return output
	if output, ok := m.outputs[key]; ok {
		return output, nil
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
