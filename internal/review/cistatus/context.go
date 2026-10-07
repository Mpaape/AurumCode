// Package cistatus separates what one review execution observed about CI
// from what it was merely told. The CI context a workflow hands the review
// (the pull request's checks at the start of the job) mixes concluded
// checks with checks still running and with statuses this product published
// itself on an earlier round. Only concluded checks of other producers are
// facts the model may analyze; the rest is withheld from it, and an analysis
// item about them is discarded before the review is published.
package cistatus

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Check is one entry of the CI context, as `gh pr checks --json
// name,state,workflow,link` reports it.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Workflow string `json:"workflow,omitempty"`
	Link     string `json:"link,omitempty"`
}

// concludedStates are the check states that carry a result. Any other state,
// including an unknown or empty one, is a check without a result yet.
var concludedStates = map[string]bool{
	"SUCCESS": true, "FAILURE": true, "ERROR": true, "CANCELLED": true,
	"SKIPPED": true, "NEUTRAL": true, "TIMED_OUT": true, "ACTION_REQUIRED": true,
	"STARTUP_FAILURE": true, "STALE": true,
}

// Concluded reports whether state carries a result.
func Concluded(state string) bool { return concludedStates[normalizeState(state)] }

// normalizeState folds "in progress", "in-progress" and "IN_PROGRESS" alike.
func normalizeState(state string) string {
	s := strings.ToUpper(strings.TrimSpace(state))
	return strings.NewReplacer(" ", "_", "-", "_").Replace(s)
}

// Context is the CI context split into what the model may see and what it
// may not. The zero value is "no CI context supplied".
type Context struct {
	supplied   bool
	unreadable bool
	concluded  []Check
	ownCount   int
	// pendingOnly holds the normalized names that have no concluded entry:
	// an analysis of them can only be invented.
	pendingOnly  map[string]bool
	pendingCount int
	ownPrefix    string
}

// Parse splits raw (the workflow-produced JSON array) with ownPrefix, the
// status-context prefix this product publishes under. Text that is not a
// JSON array of checks is unreadable: nothing of it reaches the model,
// because unparsed text would bypass the split.
func Parse(raw, ownPrefix string) Context {
	c := Context{ownPrefix: normalizeName(ownPrefix), pendingOnly: map[string]bool{}}
	if strings.TrimSpace(raw) == "" {
		return c
	}
	c.supplied = true
	var checks []Check
	if err := json.Unmarshal([]byte(raw), &checks); err != nil {
		c.unreadable = true
		return c
	}
	concludedNames := map[string]bool{}
	for _, check := range checks {
		name := normalizeName(check.Name)
		switch {
		case c.Own(name):
			c.ownCount++
		case Concluded(check.State):
			c.concluded = append(c.concluded, check)
			concludedNames[name] = true
		default:
			c.pendingCount++
			c.pendingOnly[name] = true
		}
	}
	for name := range c.pendingOnly {
		if concludedNames[name] {
			delete(c.pendingOnly, name)
		}
	}
	return c
}

// Own reports a name this product published itself.
func (c Context) Own(name string) bool {
	return c.ownPrefix != "" && strings.HasPrefix(normalizeName(name), c.ownPrefix)
}

// withoutResult reports a name the context only knows as still running.
func (c Context) withoutResult(name string) bool { return c.pendingOnly[normalizeName(name)] }

// ModelText is the CI context the model receives: the concluded checks of
// other producers, plus a note on what was withheld. Empty when no context
// was supplied, so the prompt keeps its own "nothing supplied" text.
func (c Context) ModelText() string {
	if !c.supplied {
		return ""
	}
	if c.unreadable {
		return unreadableNote
	}
	var b strings.Builder
	if len(c.concluded) > 0 {
		data, err := json.Marshal(c.concluded)
		if err != nil {
			return unreadableNote
		}
		b.Write(data)
		b.WriteByte('\n')
	} else {
		b.WriteString(noConcludedNote)
		b.WriteByte('\n')
	}
	if c.pendingCount > 0 || c.ownCount > 0 {
		fmt.Fprintf(&b, withheldNote, c.pendingCount, c.ownCount)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// Texts the model reads. They describe the context; they never ask it to
// claim a check passed.
const (
	unreadableNote  = "O contexto de CI fornecido não pôde ser lido e foi omitido. Não afirme estado, falha ou sucesso de nenhum check."
	noConcludedNote = "Nenhum check concluído no contexto de CI fornecido. Não afirme falha nem sucesso de check."
	withheldNote    = "Omitidos deste contexto: %d check(s) ainda sem resultado (em andamento) e %d status publicado(s) pelo próprio AurumCode numa rodada anterior. Não são fatos desta execução: não crie item de ci_analysis para eles."
)

func normalizeName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
