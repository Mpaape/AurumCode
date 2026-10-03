package litellm

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// CompleteMessages implements llm.MessageCompleter: the system messages
// travel as the request's system message and the user messages as its user
// message, instead of one concatenated user prompt. Any other role is a
// caller error here (tool conversations use CompleteWithTools).
func (p *Provider) CompleteMessages(messages []llm.Message, opts llm.Options) (llm.Response, error) {
	var system, user []string
	if opts.System != "" {
		system = append(system, opts.System)
	}
	for _, m := range messages {
		switch m.Role {
		case llm.RoleSystem:
			system = append(system, m.Content)
		case llm.RoleUser:
			user = append(user, m.Content)
		default:
			return llm.Response{}, fmt.Errorf("litellm: message role %q is not supported outside a tool conversation", m.Role)
		}
	}
	opts.System = strings.Join(system, "\n\n")
	return p.Complete(strings.Join(user, "\n\n"), opts)
}
