package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// commandConn is a server started as a child process speaking MCP on its
// standard input and output.
type commandConn struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
	dir string
}

func (c *commandConn) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *commandConn) Write(p []byte) (int, error) { return c.in.Write(p) }

// Close ends the child: stdin closed, process killed, waited for.
func (c *commandConn) Close() error {
	_ = c.in.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	_ = os.RemoveAll(c.dir)
	return nil
}

// ErrUnsafeCommand refuses a server command that is neither an absolute path
// nor a bare name resolved from PATH: a relative path would run a file of
// the reviewed checkout.
var ErrUnsafeCommand = errors.New("comando MCP deve ser caminho absoluto ou nome resolvido pelo PATH")

// CommandDialer starts argv as the server. The child runs in an empty
// temporary directory (never the checkout under review, removed on Close)
// and its environment is only PATH, HOME and the variables the
// configuration names in env: no other secret of the review's own
// environment reaches it.
func CommandDialer(argv []string, env []string) DialFunc {
	return func(ctx context.Context) (Conn, error) {
		if len(argv) == 0 {
			return nil, exec.ErrNotFound
		}
		if !SafeCommand(argv[0]) {
			return nil, fmt.Errorf("%w: %q", ErrUnsafeCommand, argv[0])
		}
		dir, err := os.MkdirTemp("", "aurumcode-mcp-")
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Env = childEnv(env)
		cmd.Stderr = io.Discard
		in, err := cmd.StdinPipe()
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		return &commandConn{cmd: cmd, in: in, out: out, dir: dir}, nil
	}
}

// SafeCommand accepts an absolute path or a bare program name.
func SafeCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || command == "." || command == ".." {
		return false
	}
	if filepath.IsAbs(command) || strings.HasPrefix(command, "/") {
		return true
	}
	return !strings.ContainsAny(command, "/\\")
}

func childEnv(names []string) []string {
	var out []string
	for _, name := range append([]string{"PATH", "HOME"}, names...) {
		if v, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+v)
		}
	}
	return out
}
