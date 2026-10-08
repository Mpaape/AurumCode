package mcp

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A relative server command would run a file of the reviewed checkout: it
// is refused before anything starts.
func TestAUR469RelativeCommandIsRefused(t *testing.T) {
	for _, cmd := range []string{"./tools/mcp", "tools/mcp", "..\\mcp.exe", ".", ""} {
		if SafeCommand(cmd) {
			t.Errorf("SafeCommand(%q) = true", cmd)
		}
		if _, err := CommandDialer([]string{cmd}, nil)(context.Background()); cmd != "" && !errors.Is(err, ErrUnsafeCommand) {
			t.Errorf("%q: err = %v, want ErrUnsafeCommand", cmd, err)
		}
	}
	for _, cmd := range []string{"/usr/bin/adr-mcp", "adr-mcp"} {
		if !SafeCommand(cmd) {
			t.Errorf("SafeCommand(%q) = false", cmd)
		}
	}
}

// The server runs in an empty temporary directory, never the checkout, and
// the directory is removed when the connection closes.
func TestAUR469ServerRunsInAnEmptyTemporaryDirectory(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh in this image")
	}
	cwd, _ := os.Getwd()
	conn, err := CommandDialer([]string{sh, "-c", "pwd; ls -A | wc -l"}, nil)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(conn)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimSpace(sc.Text()))
	}
	_ = conn.Close()
	if len(lines) != 2 {
		t.Fatalf("output = %q", lines)
	}
	dir := lines[0]
	if dir == cwd || !strings.HasPrefix(filepath.Base(dir), "aurumcode-mcp-") || lines[1] != "0" {
		t.Fatalf("server ran in %q (cwd %q) with %s entries", dir, cwd, lines[1])
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the temporary directory survived Close: %v", err)
	}
}
