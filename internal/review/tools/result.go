package tools

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/Mpaape/AurumCode/internal/deliberation"
)

// charged is a repository tool's result after the byte budget accepted it.
// redact is applied first, so the budget counts what the model receives;
// the digest identifies the content for the cache keys.
func charged(budget *ByteBudget, redact func(string) string, content, summary string) (deliberation.Result, error) {
	if redact != nil {
		content = redact(content)
	}
	if err := budget.Charge(len(content)); err != nil {
		return deliberation.Result{}, err
	}
	sum := sha256.Sum256([]byte(content))
	return deliberation.Result{Content: content, Summary: summary, Digest: hex.EncodeToString(sum[:])}, nil
}
