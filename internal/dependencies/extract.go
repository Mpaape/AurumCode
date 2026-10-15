package dependencies

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

//go:embed prompts/*.md
var prompts embed.FS

// Completer is the model the check asks, the same capability the reviewer
// uses (llm.Orchestrator implements it).
type Completer interface {
	CompleteMessages(ctx context.Context, messages []llm.Message, opts llm.Options) (llm.Response, error)
}

// ask sends one system prompt (an embedded template) and one user message
// and decodes the JSON object of the answer into out. Neither the message
// nor the answer has a size ceiling (issue #179): every manifest is sent
// whole and the model answers as long as it needs.
func ask(ctx context.Context, model Completer, template, user string, out any) error {
	system, err := prompts.ReadFile("prompts/" + template)
	if err != nil {
		return err
	}
	resp, err := model.CompleteMessages(ctx, []llm.Message{
		{Role: "system", Content: string(system)},
		{Role: "user", Content: user},
	}, llm.Options{System: string(system), Temperature: 0, JSONMode: true})
	if err != nil {
		return err
	}
	text := strings.TrimSpace(resp.Text)
	if start, end := strings.Index(text, "{"), strings.LastIndex(text, "}"); start >= 0 && end > start {
		text = text[start : end+1]
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("dependencies: model answer is not the expected JSON: %w", err)
	}
	return nil
}

// selectManifests asks the model which changed files declare or lock
// dependencies, and keeps only paths the diff actually holds.
func selectManifests(ctx context.Context, model Completer, diff *types.Diff) ([]string, error) {
	paths := diffPaths(diff)
	if len(paths) == 0 {
		return nil, nil
	}
	user, _ := json.Marshal(map[string][]string{"files": paths})
	var answer struct {
		Manifests []string `json:"manifests"`
	}
	if err := ask(ctx, model, "select.md", string(user), &answer); err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, p := range paths {
		known[p] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range answer.Manifests {
		if known[p] && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// extractChanges asks the model for the changed packages of manifests. The
// answer is grounded (ground.go) before it is trusted. Every manifest is
// sent whole, whatever its size.
func extractChanges(ctx context.Context, model Completer, diff *types.Diff, manifests []string) ([]Change, error) {
	var b strings.Builder
	for _, path := range manifests {
		fmt.Fprintf(&b, "=== %s\n%s\n", path, fileDiffText(diff, path))
	}
	if b.Len() == 0 {
		return nil, nil
	}
	var answer struct {
		Changes []Change `json:"changes"`
	}
	if err := ask(ctx, model, "extract.md", b.String(), &answer); err != nil {
		return nil, err
	}
	return answer.Changes, nil
}

// diffPaths lists the diff's file paths, sorted.
func diffPaths(diff *types.Diff) []string {
	if diff == nil {
		return nil
	}
	out := make([]string, 0, len(diff.Files))
	for _, f := range diff.Files {
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out
}

// fileDiffText is the hunks of path as unified diff lines.
func fileDiffText(diff *types.Diff, path string) string {
	var b strings.Builder
	for _, f := range diff.Files {
		if f.Path != path {
			continue
		}
		for _, h := range f.Hunks {
			fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
			for _, line := range h.Lines {
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}
