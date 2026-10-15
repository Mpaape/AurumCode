package scanner

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func lookupFrom(env map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		v, ok := env[name]
		return v, ok
	}
}

func TestChildEnvironmentIsExplicit(t *testing.T) {
	parent := lookupFrom(map[string]string{"PATH": "/bin", "HOME": "/h", "LLM_API_KEY": "s", "GOCACHE": "/c", "GOPROXY": "https://proxy.example"})
	got := ChildEnvironment(parent, Environment{Pass: []string{"GOCACHE", "GOPROXY"}, Fixed: []string{"GOPROXY=off"}})
	want := []string{"PATH=/bin", "HOME=/h", "GOCACHE=/c", "GOPROXY=off"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("env = %v, want %v", got, want)
	}
	if empty := ChildEnvironment(lookupFrom(nil), Environment{}); empty == nil {
		t.Fatal("an empty environment is nil, which exec reads as inherit")
	}
}

func TestGitSafeDirectoriesDropsOtherKeys(t *testing.T) {
	parent := lookupFrom(map[string]string{
		"GIT_CONFIG_COUNT": "2",
		"GIT_CONFIG_KEY_0": "http.extraheader", "GIT_CONFIG_VALUE_0": "AUTHORIZATION: x",
		"GIT_CONFIG_KEY_1": "safe.directory", "GIT_CONFIG_VALUE_1": "/w",
	})
	want := []string{"GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=/w", "GIT_CONFIG_COUNT=1"}
	if got := GitSafeDirectories(parent); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := GitSafeDirectories(lookupFrom(map[string]string{"GIT_CONFIG_COUNT": "x"})); got != nil {
		t.Fatalf("got %v", got)
	}
}

// The production command runs the child with exactly the given variables.
func TestIsolatedCommandUsesOnlyGivenEnvironment(t *testing.T) {
	t.Setenv("LLM_API_KEY", "sentinela")
	out, _, err := IsolatedCommand([]string{"ONLY=1"})(context.Background(), t.TempDir(), "/usr/bin/env")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "ONLY=1" {
		t.Fatalf("child env = %q", out)
	}
	if out, _, _ := ExecCommand(context.Background(), t.TempDir(), "/usr/bin/env"); strings.Contains(out, "sentinela") {
		t.Fatalf("ExecCommand passed a secret: %q", out)
	}
}

// Issue #179: an output past the old 32 MiB ceiling is kept whole, so a
// large Semgrep or Gitleaks report is read instead of turning the scan
// inconclusive.
func TestIsolatedCommandKeepsOutputPastTheOldCeiling(t *testing.T) {
	const n = 40000000
	out, _, err := IsolatedCommand(nil)(context.Background(), t.TempDir(), "/bin/sh", "-c", "head -c 40000000 /dev/zero")
	if err != nil {
		t.Fatalf("an output of %d bytes failed the scan: %v", n, err)
	}
	if len(out) != n {
		t.Fatalf("the output was cut: got %d bytes, want %d", len(out), n)
	}
}
