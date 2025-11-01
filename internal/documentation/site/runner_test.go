package site

import (
	"context"
	"fmt"
	"testing"
)

func TestMockRunner(t *testing.T) {
	mock := NewMockRunner()

	// Setup outputs
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	// Test success
	output, err := mock.Run(context.Background(), "hugo", []string{"version"}, ".", nil)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if output != "hugo v0.134.2+extended" {
		t.Errorf("Output = %v, want hugo v0.134.2+extended", output)
	}

	// Test call recording
	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.Cmd != "hugo" {
		t.Errorf("Cmd = %v, want hugo", call.Cmd)
	}

	if len(call.Args) != 1 || call.Args[0] != "version" {
		t.Errorf("Args = %v, want [version]", call.Args)
	}
}

func TestMockRunnerError(t *testing.T) {
	mock := NewMockRunner()

	// Setup error
	expectedErr := fmt.Errorf("command not found")
	mock.WithError("missing", expectedErr)

	// Test error
	_, err := mock.Run(context.Background(), "missing", nil, ".", nil)
	if err == nil {
		t.Error("Expected error")
	}

	if err != expectedErr {
		t.Errorf("Error = %v, want %v", err, expectedErr)
	}
}

func TestMockRunnerReset(t *testing.T) {
	mock := NewMockRunner()

	// Make some calls
	mock.Run(context.Background(), "cmd1", nil, ".", nil)
	mock.Run(context.Background(), "cmd2", nil, ".", nil)

	if len(mock.GetCalls()) != 2 {
		t.Error("Expected 2 calls before reset")
	}

	// Reset
	mock.Reset()

	if len(mock.GetCalls()) != 0 {
		t.Error("Expected 0 calls after reset")
	}
}

func TestMockRunnerWithEnv(t *testing.T) {
	mock := NewMockRunner()

	env := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}

	mock.Run(context.Background(), "test", nil, "/tmp", env)

	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatal("Expected 1 call")
	}

	call := calls[0]
	if call.Workdir != "/tmp" {
		t.Errorf("Workdir = %v, want /tmp", call.Workdir)
	}

	if call.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %v, want bar", call.Env["FOO"])
	}
}

func TestMockRunnerDefaultSuccess(t *testing.T) {
	mock := NewMockRunner()

	// Call without setup - should default to success
	output, err := mock.Run(context.Background(), "unknown", nil, ".", nil)
	if err != nil {
		t.Errorf("Expected default success, got error: %v", err)
	}

	if output != "" {
		t.Errorf("Expected empty output, got: %v", output)
	}
}
