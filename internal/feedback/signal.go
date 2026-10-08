package feedback

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// Kind is what a signal says about the policy.
type Kind string

const (
	// KindFalsePositive: a finding a human dismissed as a false positive.
	KindFalsePositive Kind = "falso_positivo"
	// KindTruePositive: a finding that disappeared because its lines were fixed.
	KindTruePositive Kind = "verdadeiro_positivo"
	// KindEscaped: a defect that passed the gate, reported with /aurum perdeu.
	KindEscaped Kind = "defeito_escapado"
)

// Signal is one observation about the policy, with the evidence that
// identifies it. Skill and Section come from the rule id "skill#secao".
type Signal struct {
	ID          string `json:"id"`
	Kind        Kind   `json:"tipo"`
	Repo        string `json:"repo"`
	SHA         string `json:"sha"`
	Rule        string `json:"regra,omitempty"`
	Skill       string `json:"skill,omitempty"`
	Section     string `json:"secao,omitempty"`
	Path        string `json:"arquivo,omitempty"`
	Line        int    `json:"linha,omitempty"`
	Description string `json:"descricao,omitempty"`
	Source      string `json:"origem"`
}

// SplitRule splits a citable rule id "skill#secao". A rule without "#"
// (a built-in rule or a scanner rule) keeps the whole id as Rule and has
// no skill: the signal still counts, but no skill change can cite it as
// the skill it targets.
func SplitRule(rule string) (skill, section string) {
	rule = strings.TrimSpace(rule)
	i := strings.Index(rule, "#")
	if i <= 0 || i == len(rule)-1 {
		return "", ""
	}
	return rule[:i], rule[i+1:]
}

// signalID derives the identity of a signal from its stable source only
// (kind, repository and the source's own id), never from a timestamp or from
// redacted text, so the same observation always has the same id.
func signalID(kind Kind, repo, sourceKey string) string {
	sum := sha256.Sum256([]byte(string(kind) + "\x00" + strings.ToLower(repo) + "\x00" + sourceKey))
	return hex.EncodeToString(sum[:])[:16]
}

// newSignal builds a signal and redacts every free-text field before it
// can reach the model or the pull request.
func newSignal(filter *redaction.Filter, kind Kind, repo, sourceKey string, s Signal) Signal {
	if filter == nil {
		filter = redaction.NewFilter()
	}
	s.ID = signalID(kind, repo, sourceKey)
	s.Kind = kind
	s.Repo = repo
	s.Skill, s.Section = SplitRule(s.Rule)
	for _, field := range []*string{&s.SHA, &s.Rule, &s.Skill, &s.Section, &s.Path, &s.Description, &s.Source} {
		*field = bound(filter.Redact(*field), maxField)
	}
	return s
}

// maxField bounds every text field of a signal.
const maxField = 2000

func bound(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

// Dedupe keeps the first signal of each id and sorts by id, so the same
// observations always produce the same list.
func Dedupe(in []Signal) []Signal {
	seen := map[string]bool{}
	out := make([]Signal, 0, len(in))
	for _, s := range in {
		if seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }
