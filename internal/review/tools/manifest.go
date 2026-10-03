package tools

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/prompt"
)

// Offer is a tool with the cost the manifest declares for it.
type Offer struct {
	Tool deliberation.Tool
	// Cost says what running the tool takes and how large its result is,
	// in the words the prompt shows the model.
	Cost string
}

// ScannerCost is the declared cost of a scanner tool.
func ScannerCost(timeoutSeconds int) string {
	return fmt.Sprintf("varredura da árvore inteira, até %ds; resultado de até ~2000 tokens (uma linha por achado)", timeoutSeconds)
}

// ContextCost is the declared cost of the codebase-context tool.
const ContextCost = "leitura local limitada, menos de 1s; resultado de até ~2000 tokens"

// Manifest is the available-tools slot of the prompt: one entry per offer,
// named exactly as the model must call it.
func Manifest(offers []Offer) []prompt.ToolOffer {
	out := make([]prompt.ToolOffer, 0, len(offers))
	for _, o := range offers {
		spec := o.Tool.Spec()
		out = append(out, prompt.ToolOffer{Name: spec.Name, Description: spec.Description, Cost: o.Cost})
	}
	return out
}

// Tools is the offers' tools, in order.
func Tools(offers []Offer) []deliberation.Tool {
	out := make([]deliberation.Tool, 0, len(offers))
	for _, o := range offers {
		out = append(out, o.Tool)
	}
	return out
}
