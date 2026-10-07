package mcpserver

// Tool names the server exposes. All four are read only.
const (
	ToolReview  = "aurum_review"
	ToolGate    = "aurum_gate"
	ToolRules   = "aurum_rules"
	ToolExplain = "aurum_explain"
)

// toolSpec is one tool as tools/list declares it.
type toolSpec struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations"`
}

// Argument patterns. A ref never starts with "-" (it can never be read as
// a flag) and a path is repository relative.
const (
	refPattern       = `^[A-Za-z0-9_][A-Za-z0-9._/~^@{}+-]{0,199}$`
	pathPattern      = `^[A-Za-z0-9_.][A-Za-z0-9._/@+ -]{0,511}$`
	findingIDPattern = `^[0-9a-f]{12}$`
	maxPaths         = 200
)

func baseSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"pattern":     refPattern,
		"description": "Ref the review compares HEAD against: the branch the change will merge into (for example main or origin/main) or a commit (HEAD~1).",
	}
}

func objectSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

// readOnly is the MCP annotation of every tool: nothing is written.
func readOnly() map[string]any {
	return map[string]any{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false}
}

// toolSpecs is the one declaration of the tools; tools/list and argument
// validation read it.
func toolSpecs() []toolSpec {
	return []toolSpec{
		{
			Name:        ToolReview,
			Title:       "Aurum review",
			Description: "Review the commits between base and HEAD with the repository's effective policy and skills. Returns structured findings (file, line, severity, cited rule, origin, evidence, suggestion) and the gate decision. Read only.",
			InputSchema: objectSchema(map[string]any{"base": baseSchema()}, "base"),
			Annotations: readOnly(),
		},
		{
			Name:        ToolGate,
			Title:       "Aurum gate",
			Description: "Ask whether the change between base and HEAD passes the same gate CI applies: pass, fail (with the blocking findings) or inconclusive (never a pass). Call it before every commit or push and fix what it reports.",
			InputSchema: objectSchema(map[string]any{"base": baseSchema()}, "base"),
			Annotations: readOnly(),
		},
		{
			Name:        ToolRules,
			Title:       "Aurum rules",
			Description: "List the skills (central policy and repository) and the citable rules that apply to the given repository-relative paths.",
			InputSchema: objectSchema(map[string]any{"paths": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": maxPaths,
				"items":    map[string]any{"type": "string", "pattern": pathPattern},
			}}, "paths"),
			Annotations: readOnly(),
		},
		{
			Name:        ToolExplain,
			Title:       "Aurum explain",
			Description: "Explain one finding of the last aurum_review or aurum_gate answer of this session: its rule, evidence, impact and suggested fix.",
			InputSchema: objectSchema(map[string]any{"finding_id": map[string]any{
				"type":        "string",
				"pattern":     findingIDPattern,
				"description": "The id field of a finding returned by aurum_review or aurum_gate.",
			}}, "finding_id"),
			Annotations: readOnly(),
		},
	}
}
