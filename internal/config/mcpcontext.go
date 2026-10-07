package config

import (
	"fmt"
	"strings"
)

// MCPContextSource is one entry of review.context.mcp (AUR-469): a server
// the trusted configuration starts, the one tool called, and exactly what
// is sent. The answer is background for the prompt with origin
// mcp:<name>/<tool>; it never decides a rule, a gate or a permission.
type MCPContextSource struct {
	// Name identifies the source in the prompt and in warnings.
	Name string `yaml:"name"`
	// Command is the server's argv (stdio transport).
	Command []string `yaml:"command"`
	// Tool is the one tool called.
	Tool string `yaml:"tool"`
	// Arguments are static string arguments sent to the tool, redacted.
	Arguments map[string]string `yaml:"arguments"`
	// Send declares the dynamic payload: only "changed_paths" exists.
	Send []string `yaml:"send"`
	// Env names the environment variables passed to the server besides
	// PATH and HOME; nothing else of the review's environment reaches it.
	Env []string `yaml:"env"`
	// TimeoutSeconds bounds one call; 0 is ProviderTimeout, never above it.
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// mcpSendChangedPaths is the one dynamic payload a source may declare.
const mcpSendChangedPaths = "changed_paths"

// ValidateMCP refuses an MCP source without name, command or tool, with a
// duplicated name, an unknown send item or a timeout out of range.
func (c ReviewContextConfig) ValidateMCP() error {
	seen := map[string]bool{}
	for i, src := range c.MCP {
		where := fmt.Sprintf("review.context.mcp[%d]", i)
		switch {
		case strings.TrimSpace(src.Name) == "":
			return fmt.Errorf("%s: name is required", where)
		case seen[src.Name]:
			return fmt.Errorf("%s: name %q is duplicated", where, src.Name)
		case len(src.Command) == 0 || strings.TrimSpace(src.Command[0]) == "":
			return fmt.Errorf("%s (%s): command is required", where, src.Name)
		case strings.TrimSpace(src.Tool) == "":
			return fmt.Errorf("%s (%s): tool is required", where, src.Name)
		case src.TimeoutSeconds < 0 || src.TimeoutSeconds > int(ProviderTimeout.Seconds()):
			return fmt.Errorf("%s (%s): timeout_seconds must be between 0 and %d", where, src.Name, int(ProviderTimeout.Seconds()))
		}
		for _, item := range src.Send {
			if item != mcpSendChangedPaths {
				return fmt.Errorf("%s (%s): send %q is not supported (only %q)", where, src.Name, item, mcpSendChangedPaths)
			}
		}
		seen[src.Name] = true
	}
	return nil
}
