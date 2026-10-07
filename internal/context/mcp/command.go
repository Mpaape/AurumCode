package mcp

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// commandConn is a server started as a child process speaking MCP on its
// standard input and output.
type commandConn struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
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
	return nil
}

// CommandDialer starts argv as the server. The child's environment is only
// PATH, HOME and the variables the configuration names in env: no other
// secret of the review's own environment reaches it.
func CommandDialer(argv []string, env []string) DialFunc {
	return func(ctx context.Context) (Conn, error) {
		if len(argv) == 0 {
			return nil, exec.ErrNotFound
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = childEnv(env)
		cmd.Stderr = io.Discard
		in, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return &commandConn{cmd: cmd, in: in, out: out}, nil
	}
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
