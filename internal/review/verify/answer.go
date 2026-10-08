package verify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// answer is the verifier's JSON reply.
type answer struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
	Quote   string `json:"quote"`
}

// parseAnswer reads the reply: one JSON object (a surrounding code fence or
// prose is tolerated) with a known verdict.
func parseAnswer(text string) (answer, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return answer{}, fmt.Errorf("a resposta não contém um objeto JSON")
	}
	var a answer
	if err := json.Unmarshal([]byte(text[start:end+1]), &a); err != nil {
		return answer{}, fmt.Errorf("a resposta não é o JSON pedido: %v", err)
	}
	a.Verdict = strings.ToLower(strings.TrimSpace(a.Verdict))
	switch a.Verdict {
	case VerdictConfirmed, VerdictRefuted, VerdictUncertain:
		return a, nil
	}
	return answer{}, fmt.Errorf("veredito desconhecido %q", a.Verdict)
}
