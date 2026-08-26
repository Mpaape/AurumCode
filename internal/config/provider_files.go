package config

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// RepoPromptProvider reads one repository-wide instructions file
// (.aurumcode/prompt.md by default) and contributes its content, verbatim
// and untrusted, to every review -- the "prompt do repositorio sobrepondo
// o embutido" provider this card's Outcome names: it is layered ON TOP OF
// the engine's built-in review instructions as an extra, clearly-labeled
// section (see BuildContextBlock), never a silent replacement of them.
// Absent file: Provide returns "", nil -- the zero-config path.
type RepoPromptProvider struct{ Path string }

// NewRepoPromptProvider roots the provider at root/.aurumcode/prompt.md.
func NewRepoPromptProvider(root string) *RepoPromptProvider {
	return &RepoPromptProvider{Path: filepath.Join(root, ".aurumcode", "prompt.md")}
}

func (p *RepoPromptProvider) Name() string { return "repository prompt (.aurumcode/prompt.md)" }

func (p *RepoPromptProvider) Provide(_ context.Context, _ []string) (string, error) {
	data, err := os.ReadFile(p.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading %s: %w", p.Path, err)
	}
	return string(data), nil
}

// FileContextProvider reads one explicitly configured prompt, skill or
// documentation file from the local repository. Unlike the historical
// repository prompt, a configured file is intentional: a missing path is an
// error that the caller surfaces as a provider warning with the exact path.
type FileContextProvider struct {
	Root string
	File ContextFile
}

func NewFileContextProvider(root string, file ContextFile) *FileContextProvider {
	return &FileContextProvider{Root: root, File: file}
}

func (p *FileContextProvider) Name() string {
	return fmt.Sprintf("review %s (%s)", p.File.Kind, p.File.Path)
}

func (p *FileContextProvider) Provide(_ context.Context, _ []string) (string, error) {
	if err := validateContextPath(p.File.Path); err != nil {
		return "", err
	}
	clean := path.Clean(strings.ReplaceAll(p.File.Path, "\\", "/"))
	data, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(clean)))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", p.File.Path, err)
	}
	return string(data), nil
}

// TextContextProvider is the remote equivalent of FileContextProvider. The
// pull-request path reads configured files through GitHub's contents API and
// then uses this provider so local and PR reviews share the same rendering,
// redaction and token accounting.
type TextContextProvider struct {
	Kind string
	Path string
	Text string
}

func NewTextContextProvider(file ContextFile, text string) *TextContextProvider {
	return &TextContextProvider{Kind: file.Kind, Path: file.Path, Text: text}
}

func (p *TextContextProvider) Name() string {
	return fmt.Sprintf("review %s (%s)", p.Kind, p.Path)
}

func (p *TextContextProvider) Provide(_ context.Context, _ []string) (string, error) {
	return p.Text, nil
}

// PathInstructionsProvider reads every *.md file directly under
// .aurumcode/instructions/, each carrying a Copilot-style `applyTo`
// front-matter glob, and contributes the body of every file whose glob
// matches at least one changed path. A file with no applyTo is inert
// (never applied) rather than silently global -- an author who forgot the
// glob gets no contribution instead of an unexpectedly repo-wide one. No
// directory, or a directory with nothing matching: Provide returns "",
// nil -- the zero-config path.
type PathInstructionsProvider struct{ Dir string }

// NewPathInstructionsProvider roots the provider at
// root/.aurumcode/instructions.
func NewPathInstructionsProvider(root string) *PathInstructionsProvider {
	return &PathInstructionsProvider{Dir: filepath.Join(root, ".aurumcode", "instructions")}
}

func (p *PathInstructionsProvider) Name() string {
	return "path-scoped instructions (.aurumcode/instructions/*.md)"
}

func (p *PathInstructionsProvider) Provide(_ context.Context, changedPaths []string) (string, error) {
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading %s: %w", p.Dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names) // deterministic rendering order

	var parts []string
	for _, name := range names {
		full := filepath.Join(p.Dir, name)
		data, err := os.ReadFile(full)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", full, err)
		}
		applyTo, body, err := parseApplyToFrontMatter(string(data))
		if err != nil {
			return "", fmt.Errorf("%s: %w", full, err)
		}
		if applyTo == "" {
			continue
		}
		matched := false
		for _, cp := range changedPaths {
			if globMatch(applyTo, cp) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (applyTo: %s)\n%s", name, applyTo, strings.TrimSpace(body)))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n"), nil
}

// parseApplyToFrontMatter splits a Copilot-style instructions file into
// its `applyTo` glob and body. A file with no "---" front matter at all
// has no scope (applyTo == "") and its whole content is the body -- never
// applied, per the type doc above. An opened-but-unterminated front
// matter block is a loud error: a file the author started to scope but
// left broken must not silently fall back to "apply everywhere".
func parseApplyToFrontMatter(content string) (applyTo, body string, err error) {
	const delim = "---"
	if !strings.HasPrefix(content, delim) {
		return "", content, nil
	}
	rest := content[len(delim):]
	idx := strings.Index(rest, "\n"+delim)
	if idx == -1 {
		return "", "", fmt.Errorf("unterminated front matter (missing closing %q)", delim)
	}
	fm := rest[:idx]
	body = rest[idx+len(delim)+1:]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(k) == "applyTo" {
			applyTo = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return applyTo, body, nil
}

// DefaultProviders returns Camada 1's two file providers rooted at root,
// in the fixed order their contributions are rendered: the repository-
// wide prompt first, then path-scoped instructions. AUR-468 (skills),
// AUR-469 (MCP), AUR-470 (RAG) and AUR-471 (ISO policy) append their own
// providers to a slice built the same way; this function's return type
// (ContextProvider) and this ordering convention are the contract they
// build against.
func DefaultProviders(root string) []ContextProvider {
	return ConfiguredProviders(root, &Config{})
}

// ConfiguredProviders preserves the zero-config providers and adds the
// explicit context lists from review.context. The built-in review prompt is
// always assembled separately; these files are additive background only.
func ConfiguredProviders(root string, cfg *Config) []ContextProvider {
	if cfg == nil {
		cfg = &Config{}
	}
	files := cfg.Review.ContextFiles()
	providers := make([]ContextProvider, 0, len(files)+1)
	for _, file := range files {
		if file.Path == DefaultReviewPromptPath && file.Optional {
			providers = append(providers, NewRepoPromptProvider(root))
			continue
		}
		providers = append(providers, NewFileContextProvider(root, file))
	}
	// Keep the established Copilot-style path instructions convention for
	// existing local users. Explicit skills/docs come after it in stable order.
	providers = append(providers, NewPathInstructionsProvider(root))
	return providers
}
