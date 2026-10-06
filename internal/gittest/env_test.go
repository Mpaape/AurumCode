package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// hostileConfig is a global git configuration of the kind a shared
// container can carry between sessions: every commit must be signed by a
// program that always fails, and a hooks path whose pre-commit refuses.
func hostileConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	hooks := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[commit]\n\tgpgsign = true\n[gpg]\n\tprogram = false\n[core]\n\thooksPath = " + hooks + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitconfig"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// commitAndPack builds the packed two-commit fixture the git-binary tests
// use and reports the first git error.
func commitAndPack(t *testing.T, env []string) error {
	t.Helper()
	dir := t.TempDir()
	steps := [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "a.txt"},
		{"commit", "-q", "-m", "one"},
		{"gc", "-q", "--aggressive"},
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range steps {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return &gitError{args: args, out: string(out), err: err}
		}
	}
	loose, err := filepath.Glob(filepath.Join(dir, ".git", "objects", "??"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loose) != 0 {
		t.Fatalf("loose object directories remain after gc: %v", loose)
	}
	return nil
}

type gitError struct {
	args []string
	out  string
	err  error
}

func (e *gitError) Error() string { return "git " + e.args[0] + ": " + e.err.Error() + ": " + e.out }

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
}

// TestAmbientHomeConfigBreaksAPlainFixture proves the hazard is real: the
// same fixture built with the caller's environment fails under a hostile
// global configuration in HOME.
func TestAmbientHomeConfigBreaksAPlainFixture(t *testing.T) {
	requireGit(t)
	t.Setenv("HOME", hostileConfig(t))
	t.Setenv("GIT_CONFIG_GLOBAL", "") // registers the restore; then truly unset
	if err := os.Unsetenv("GIT_CONFIG_GLOBAL"); err != nil {
		t.Fatal(err)
	}
	env := append(append([]string(nil), os.Environ()...), Identity...)
	if err := commitAndPack(t, env); err == nil {
		t.Fatal("a fixture built from the ambient environment ignored the hostile global configuration; the hazard this package removes was not reproduced")
	}
}

// TestHermeticEnvIgnoresHostileHome: the same hostile HOME no longer
// reaches the fixture.
func TestHermeticEnvIgnoresHostileHome(t *testing.T) {
	requireGit(t)
	t.Setenv("HOME", hostileConfig(t))
	if err := commitAndPack(t, HermeticEnv(t.TempDir())); err != nil {
		t.Fatalf("hermetic fixture read the ambient HOME configuration: %v", err)
	}
}

// TestHermeticEnvIgnoresHostileGlobalFile: a caller that exports
// GIT_CONFIG_GLOBAL pointing at a hostile file does not reach it either.
func TestHermeticEnvIgnoresHostileGlobalFile(t *testing.T) {
	requireGit(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(hostileConfig(t), ".gitconfig"))
	if err := commitAndPack(t, HermeticEnv(t.TempDir())); err != nil {
		t.Fatalf("hermetic fixture read the caller's GIT_CONFIG_GLOBAL: %v", err)
	}
}
