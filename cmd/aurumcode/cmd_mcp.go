// The mcp command: mounts the agent server (internal/mcpserver) on stdin
// and stdout, with the review session as its gateway. Nothing is decided
// here; the flags are the operator's limits, never a rule switch.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/mcpserver"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// mcpFlags are the operator's limits for every tool call of the session.
type mcpFlags struct {
	limite string
	tempo  time.Duration
}

// declareMCPFlags builds the FlagSet of `mcp` and the values it binds.
func declareMCPFlags() (*flag.FlagSet, *mcpFlags) {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	f := &mcpFlags{}
	fs.StringVar(&f.limite, "limite", "", "maximum USD each review may spend calling the model, the same cap as review --limite (default: no limit enforced)")
	fs.DurationVar(&f.tempo, "tempo", mcpserver.DefaultTimeout, "maximum time one tool call may take; a review that does not finish in time is answered as inconclusive, never as a pass")
	return fs, f
}

// runMCP serves the Model Context Protocol on in and out until in ends.
// The central policy comes from AURUMCODE_POLICY, exactly as for review;
// the repository is the working directory.
func runMCP(args []string, in io.Reader, stdout, stderr io.Writer, filter *redaction.Filter) int {
	fs, f := declareMCPFlags()
	if exit, ok := parseSubcommandFlags("mcp", fs, args, stdout, stderr); !ok {
		return exit
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "aurumcode mcp: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if f.limite != "" {
		if _, err := parseLimiteUSD(f.limite); err != nil {
			fmt.Fprintf(stderr, "aurumcode mcp: --limite: %v\n", err)
			return 2
		}
	}
	server := mcpserver.New(mcpserver.Options{
		Gateway:  &sessionGateway{filter: filter, limite: f.limite},
		Redactor: filter,
		Timeout:  f.tempo,
		Version:  version,
	})
	if err := server.Serve(context.Background(), in, stdout); err != nil {
		fmt.Fprintf(stderr, "aurumcode mcp: %v\n", err)
		return 1
	}
	return 0
}

// mcpSubcommand is the registry entry of `mcp`.
func mcpSubcommand() subcommand {
	return subcommand{
		name:       "mcp",
		docSection: "CLI `aurumcode mcp`",
		summary:    "Serve the review gate to a coding agent over the Model Context Protocol (stdio, read only).",
		example:    "claude mcp add aurum -- aurumcode mcp",
		flags:      func() *flag.FlagSet { fs, _ := declareMCPFlags(); return fs },
		run: func(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
			return runMCP(args, os.Stdin, stdout, stderr, filter)
		},
	}
}
