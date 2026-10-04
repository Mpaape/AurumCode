package engines_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// secretVariables stand for the secrets a CI job holds while it reviews.
var secretVariables = map[string]string{
	"LLM_API_KEY":           "sentinela-llm-api-key",
	"GITHUB_TOKEN":          "sentinela-github-token",
	"AURUMCODE_LLM_API_KEY": "sentinela-aurumcode-key",
	"GOFLAGS":               "-toolexec=/sentinela/toolexec",
}

// allowedNames is every variable a child may receive: the base list, the
// govet caches and fixed values, and the renumbered safe.directory entries.
var allowedNames = map[string]bool{
	"GOCACHE": true, "GOPATH": true, "GOMODCACHE": true, "GOROOT": true,
	"GOTOOLCHAIN": true, "GOPROXY": true,
	"GIT_CONFIG_COUNT": true, "GIT_CONFIG_KEY_0": true, "GIT_CONFIG_VALUE_0": true,
}

// shellNames are the variables /bin/sh itself exports to the fake's own
// child (the env dump), not something the executor passed.
var shellNames = map[string]bool{"PWD": true, "SHLVL": true, "_": true, "OLDPWD": true}

// fakeBinaries answer just enough for each engine to reach its scan; every
// one first appends its whole environment to a dump file whose absolute
// path is written into the script (never passed through the environment).
var fakeBinaries = map[string]string{
	"semgrep":  `echo '{"results": [], "errors": []}'`,
	"gitleaks": `case "$1" in version) echo v8.30.1;; esac`,
	"git":      `case "$*" in *is-shallow-repository*) echo false;; esac`,
	"go":       `case "$1" in env) echo go1.27.1;; vet) echo '{}';; esac`,
}

func writeFakes(t *testing.T) (bin, dumps string) {
	t.Helper()
	bin, dumps = t.TempDir(), t.TempDir()
	for name, body := range fakeBinaries {
		dump := filepath.Join(dumps, name+".env")
		script := fmt.Sprintf("#!/bin/sh\n/usr/bin/env >> %s\necho ---- >> %s\n%s\n", dump, dump, body)
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bin, dumps
}

// AC-003: no registered engine's child process receives a secret of the
// reviewing process; it receives only the declared, explicit environment.
func TestNoEngineChildInheritsSecrets(t *testing.T) {
	bin, dumps := writeFakes(t)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	for name, value := range secretVariables {
		t.Setenv(name, value)
	}
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "http.extraheader")
	t.Setenv("GIT_CONFIG_VALUE_0", "AUTHORIZATION: basic sentinela-extraheader")
	t.Setenv("GIT_CONFIG_KEY_1", "safe.directory")
	t.Setenv("GIT_CONFIG_VALUE_1", "/github/workspace")
	commit := strings.Repeat("a", 40)
	req := scanner.Request{Root: t.TempDir(), Range: scanner.Range{Base: commit, Head: strings.Repeat("b", 40)}}
	for _, name := range []string{"semgrep", "gitleaks", "govet"} {
		engine, ok := scanner.Lookup(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		scanner.Executor{}.Scan(context.Background(), engine, req)
	}
	for binary := range fakeBinaries {
		raw, err := os.ReadFile(filepath.Join(dumps, binary+".env"))
		if err != nil {
			t.Fatalf("%s never ran: %v", binary, err)
		}
		checkDump(t, binary, string(raw))
	}
}

func checkDump(t *testing.T, binary, dump string) {
	t.Helper()
	if !strings.Contains(dump, "PATH=") {
		t.Errorf("%s: no PATH in its environment:\n%s", binary, dump)
	}
	if strings.Contains(dump, "sentinela") {
		t.Errorf("%s received a secret of the reviewing process:\n%s", binary, dump)
	}
	for _, line := range strings.Split(dump, "\n") {
		key, _, ok := strings.Cut(line, "=")
		if !ok || shellNames[key] {
			continue
		}
		if !allowedNames[key] && !isBase(key) {
			t.Errorf("%s received %s, outside the declared environment", binary, key)
		}
	}
	if binary == "git" && !strings.Contains(dump, "GIT_CONFIG_VALUE_0=/github/workspace") {
		t.Errorf("git lost the safe.directory entry:\n%s", dump)
	}
}

func isBase(key string) bool {
	for _, name := range scanner.BaseEnvironment {
		if name == key {
			return true
		}
	}
	return false
}
