package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// SkillToolName is the on-demand skill-section tool.
const SkillToolName = "skill_section"

// SkillSection is one convention section of a configured skill file: the
// rule id the model cites, its title and its full text.
type SkillSection struct {
	Title string
	Text  string
}

// skillArgs names the section the model asks for.
type skillArgs struct {
	RuleID string `json:"rule_id" desc:"rule_id de uma seção de skill do catálogo de regras"`
}

// SkillTool returns, on demand, the full text of a skill section whose id
// the rule catalog already lists, so the prompt carries the ids and the
// model reads only the sections it needs.
type SkillTool struct {
	sections map[string]SkillSection
}

// NewSkillTool offers sections, keyed by rule id.
func NewSkillTool(sections map[string]SkillSection) *SkillTool {
	return &SkillTool{sections: sections}
}

// Spec implements deliberation.Tool.
func (t *SkillTool) Spec() llm.ToolSpec {
	ids := make([]string, 0, len(t.sections))
	for id := range t.sections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return llm.ToolSpec{
		Name:        SkillToolName,
		Description: "Devolve o texto completo de uma seção de skill de convenção: " + strings.Join(ids, ", ") + ".",
		Parameters:  llm.SchemaOf(skillArgs{}),
	}
}

// Run implements deliberation.Tool.
func (t *SkillTool) Run(_ context.Context, raw json.RawMessage) (deliberation.Result, error) {
	var args skillArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return deliberation.Result{}, err
	}
	section, ok := t.sections[args.RuleID]
	if !ok {
		return deliberation.Result{}, fmt.Errorf("%q não é uma seção de skill configurada", args.RuleID)
	}
	text := fmt.Sprintf("%s: %s\n%s", args.RuleID, section.Title, section.Text)
	sum := sha256.Sum256([]byte(text))
	return deliberation.Result{Content: text, Summary: section.Title, Digest: hex.EncodeToString(sum[:])}, nil
}

// SkillCost is the declared cost of the skill-section tool.
const SkillCost = "leitura em memória, imediata; resultado do tamanho da seção"
