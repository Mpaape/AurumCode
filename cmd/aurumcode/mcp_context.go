// MCP context sources (AUR-469): review.context.mcp entries become context
// providers whose answer goes to the prompt's repository-context slot with
// origin mcp:<name>/<tool>. Only trusted configuration starts a server: the
// central policy always; the repository's own config in --base (the
// operator's checkout) and in --pr only when it was read at the base ref,
// never from the pull request's head.
package main

import (
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/mcp"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// dialMCPCommand starts a configured server; tests replace it.
var dialMCPCommand = mcp.CommandDialer

// trustedMCPSources is the policy's sources, then the repository's when its
// config is trusted.
func trustedMCPSources(repo *config.Config, repoTrusted bool, central *config.Config) []config.MCPContextSource {
	var out []config.MCPContextSource
	if central != nil {
		out = append(out, central.Review.Context.MCP...)
	}
	if repo != nil && repoTrusted {
		out = append(out, repo.Review.Context.MCP...)
	}
	return out
}

// mcpContextProviders turns sources into context providers that send only
// their declared payload, redacted by filter.
func mcpContextProviders(sources []config.MCPContextSource, filter *redaction.Filter) []config.ContextProvider {
	redact := func(s string) string { return s }
	if filter != nil {
		redact = filter.Redact
	}
	out := make([]config.ContextProvider, 0, len(sources))
	for _, src := range sources {
		timeout := config.ProviderTimeout
		if src.TimeoutSeconds > 0 {
			timeout = time.Duration(src.TimeoutSeconds) * time.Second
		}
		out = append(out, &mcp.Source{
			SourceName: src.Name,
			Tool:       src.Tool,
			Arguments:  src.Arguments,
			Send:       src.Send,
			Timeout:    timeout,
			Dial:       dialMCPCommand(src.Command, src.Env),
			Redact:     redact,
			MaxBytes:   config.MaxProviderContributionBytes,
		})
	}
	return out
}
