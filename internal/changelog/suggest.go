package changelog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The suggested entry: when the required check refuses a pull request, the
// tool still offers the lines a human can paste under the required section.
// The check keeps failing closed; the suggestion is advice, never a pass.
// Every source (commit subjects, model text) is untrusted: lines are
// escaped, bounded and filtered through the same rules the check applies,
// so a suggestion that would itself be refused is never offered.

// Suggestion sources, stable identifiers printed with the block.
const (
	SuggestionFromModel   = "modelo"
	SuggestionFromCommits = "commits"
)

// Suggestion is a ready-to-paste entry for the required section.
type Suggestion struct {
	Section string
	Lines   []string
	Source  string
}

// Empty reports whether there is nothing to offer.
func (s Suggestion) Empty() bool { return len(s.Lines) == 0 }

// noiseSubjectPrefixes mark commits that carry no change information of
// their own (merges, history rewrites).
var noiseSubjectPrefixes = []string{"merge ", "fixup!", "squash!", "amend!", "revert \"revert"}

// SuggestFromCommits derives the entry deterministically from commit
// subjects only. Bodies are never read: trailers and agent logs live there.
func (r Requirement) SuggestFromCommits(commits []Commit) Suggestion {
	n := len(commits)
	if n > MaxCommits {
		n = MaxCommits
	}
	candidates := make([]string, 0, n)
	for _, c := range commits[:n] {
		subject := strings.TrimSpace(firstLine(c.Subject))
		if isNoiseSubject(subject) {
			continue
		}
		if cl := Classify(Commit{Subject: subject}); cl.Conventional && strings.TrimSpace(cl.Description) != "" {
			subject = strings.TrimSpace(cl.Description)
		}
		candidates = append(candidates, subject)
	}
	return Suggestion{Section: r.Section, Lines: r.suggestionLines(candidates), Source: SuggestionFromCommits}
}

// modelAnswer is the structured answer the model is asked for.
type modelAnswer struct {
	Entry []string `json:"entry"`
}

// ErrUnusableModelAnswer: the model text is not the expected structure or
// yields no line the check would accept.
var ErrUnusableModelAnswer = errors.New("resposta do modelo sem entrada utilizável")

// SuggestFromModel validates the model text as {"entry": [...]} and keeps
// only lines the check would accept.
func (r Requirement) SuggestFromModel(text string) (Suggestion, error) {
	var a modelAnswer
	trimmed := strings.TrimSpace(text)
	if err := json.Unmarshal([]byte(trimmed), &a); err != nil {
		return Suggestion{}, fmt.Errorf("%w: %v", ErrUnusableModelAnswer, err)
	}
	if len(a.Entry) > MaxCommits {
		a.Entry = a.Entry[:MaxCommits]
	}
	lines := r.suggestionLines(a.Entry)
	if len(lines) == 0 {
		return Suggestion{}, ErrUnusableModelAnswer
	}
	return Suggestion{Section: r.Section, Lines: lines, Source: SuggestionFromModel}, nil
}

// suggestionLines turns raw candidates into bullets the check accepts:
// one line each, escaped, without agent-log markers, informative, within
// the line length, deduplicated and capped at MaxEntryLines.
func (r Requirement) suggestionLines(candidates []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range candidates {
		if len(out) >= r.MaxEntryLines {
			break
		}
		text := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(firstLine(raw)), "-*+> "))
		if text == "" || r.agentLogMarker(text) != "" {
			continue
		}
		line := "- " + escapeText(text)
		if limit := r.MaxLineLength; limit > len("...") && len([]rune(line)) > limit {
			line = string([]rune(line)[:limit-len("...")]) + "..."
		}
		if !r.informative(line) || r.agentLogMarker(line) != "" {
			continue
		}
		key := strings.ToLower(line)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, line)
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func isNoiseSubject(subject string) bool {
	lower := strings.ToLower(subject)
	for _, p := range noiseSubjectPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}
